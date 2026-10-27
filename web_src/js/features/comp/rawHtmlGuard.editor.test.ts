// Raw HTML Toast UI cannot convert, against the REAL Toast UI Editor built like the article
// editor (issue #446): such an article must open in the Visual editor, keep its bytes, and
// text typed in the Visual editor must not be saved as HTML the editor cannot read back.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from './losslessMarkdown.ts';
import {createBase64WidgetRule, installBase64WidgetPatch} from './base64ImageWidget.ts';
import {articleEditorMarkdownOptions} from './toastEditorOptions.ts';
import {escapeHtmlStarts, installRawHtmlGuard, isConvertibleHtmlBlock, rawInlineHtmlWidgetRule} from './rawHtmlGuard.ts';

// Each of these made setMarkdown throw "Cannot read properties of null (reading '1')" in
// Toast UI's htmlBlock / htmlInline converter, so the Edit page never loaded.
const UNCONVERTIBLE_BLOCKS = [
  '<!DOCTYPE html>',
  '<?php echo "hi"; ?>',
  '<![CDATA[ x < y ]]>',
  '<!---->',
  '<!-- a -- b -->',
  '  <div class="footer">indented</div>',
  '<div',
];
const UNCONVERTIBLE_INLINE = [
  'Inline <!-- note --> comment.',
  'Inline <?php echo 1; ?> instruction.',
  'Inline <!DOCTYPE html> declaration.',
];

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function article(middle: string): string {
  return `# Mark down\n\nFirst paragraph.\n\n${middle}\n\nLast paragraph.\n`;
}

// The article editor's construction order (toast-editor.ts).
function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg', withRawInlineWidget = true) {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const ref = {current: null as any};
  const widgetRules: unknown[] = [createBase64WidgetRule(() => ref.current)];
  if (withRawInlineWidget) widgetRules.push(rawInlineHtmlWidgetRule);
  const editor = new Editor({el, initialEditType, usageStatistics: false, widgetRules, ...articleEditorMarkdownOptions});
  ref.current = editor;
  installRawHtmlGuard(editor, widgetRules);
  installBase64WidgetPatch(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, textarea, el};
}

function visualText(el: HTMLElement): string {
  return el.querySelector('.toastui-editor-ww-container .ProseMirror').textContent;
}

function typeInVisual(editor: any, text: string) {
  const view = editor.wwEditor.view;
  view.dispatch(view.state.tr.insertText(text, 1, 1));
}

afterEach(() => {
  document.body.innerHTML = '';
});

// Pins the Toast UI 3.2.2 bug the guard works around. If an upgrade fixes it, this fails and
// the guard's load half can go.
test('Toast UI alone throws on an HTML block that does not start with a tag', () => {
  const el = document.createElement('div');
  document.body.append(el);
  const editor = new Editor({el, initialEditType: 'wysiwyg', usageStatistics: false});
  expect(() => editor.setMarkdown(article('<!DOCTYPE html>'))).toThrow(/reading '1'/);
});

describe('an article with HTML Toast UI cannot convert', () => {
  test.each([...UNCONVERTIBLE_BLOCKS, ...UNCONVERTIBLE_INLINE])('opens in the Visual editor: %s', async (html) => {
    const source = article(html);
    const {editor, el} = createEditor(source);
    await flush();
    // The HTML is shown as written, between the paragraphs around it.
    const text = visualText(el);
    expect(text).toContain('First paragraph.');
    expect(text).toContain(html.trim().replace(/\s+/g, ' ').slice(0, 8));
    expect(text).toContain('Last paragraph.');
    // Nothing was edited: the source comes back byte for byte.
    expect(editor.getMarkdown()).toBe(source);
  });

  test('switching from the Source editor to the Visual editor does not throw', async () => {
    const source = article(UNCONVERTIBLE_BLOCKS.join('\n\n'));
    const {editor} = createEditor(source, 'markdown');
    await flush();
    expect(() => editor.changeMode('wysiwyg')).not.toThrow();
    // The Source editor drops the final newline of any article (not specific to HTML).
    expect(editor.getMarkdown()).toBe(source.trimEnd());
  });

  test('a Visual edit elsewhere keeps the raw HTML block byte for byte', async () => {
    const source = article('<!DOCTYPE html>\n\n  <div class="footer">indented</div>');
    const {editor} = createEditor(source);
    await flush();
    const view = editor.wwEditor.view;
    let pos = -1;
    view.state.doc.descendants((node: any, at: number) => {
      if (pos < 0 && node.isText && node.text === 'Last paragraph.') pos = at;
      return pos < 0;
    });
    view.dispatch(view.state.tr.insertText('Edited. ', pos, pos));
    await flush();
    const saved = editor.getMarkdown();
    expect(saved).toContain('Edited. Last paragraph.');
    expect(saved).toContain('\n<!DOCTYPE html>\n');
    expect(saved).toContain('\n  <div class="footer">indented</div>\n');
  });
});

describe('text typed in the Visual editor', () => {
  test.each([
    ['<!DOCTYPE html>', '\\<!DOCTYPE html>'],
    ['<?php echo 1; ?>', '\\<?php echo 1; ?>'],
    ['<!-- a -- b -->', '\\<!-- a -- b -->'],
    ['<![CDATA[x]]>', '\\<![CDATA[x]]>'],
    ['<div', '\\<div'],
  ])('%s is saved as literal text the editor can open again', async (typed, saved) => {
    const {editor} = createEditor('Hello world\n');
    await flush();
    typeInVisual(editor, `${typed} `);
    await flush();
    const markdown = editor.getMarkdown();
    expect(markdown.startsWith(`${saved} Hello world`)).toBe(true);

    // The fork from the issue: open the saved article again.
    const reopened = createEditor(markdown);
    await flush();
    expect(visualText(reopened.el)).toContain(`${typed} Hello world`);
  });
});

// Review of #471, finding 1: inline HTML Toast UI cannot convert is a widget, not text, so
// it is not escaped (and a hidden comment not made visible) when its paragraph is edited.
describe('inline HTML Toast UI cannot convert, in a paragraph edited in the Visual editor', () => {
  test.each([
    '<!-- c -->',
    '<?php echo 1; ?>',
    '<!DOCTYPE x>',
    '<![CDATA[x]]>',
  ])('%s is saved unchanged', async (html) => {
    const {editor, el} = createEditor(`a ${html} b\n\nOther paragraph.\n`);
    await flush();
    expect(el.querySelector('.toastui-editor-ww-container .raw-inline-html').textContent).toBe(html);
    const view = editor.wwEditor.view;
    let pos = -1;
    view.state.doc.descendants((node: any, at: number) => {
      if (pos < 0 && node.isText && node.text === ' b') pos = at + node.text.length;
      return pos < 0;
    });
    view.dispatch(view.state.tr.insertText(' edited', pos, pos));
    await flush();
    expect(editor.getMarkdown().trimEnd()).toBe(`a ${html} b edited\n\nOther paragraph.`);
  });

  test('and in the Source editor after the Visual edit', async () => {
    const {editor} = createEditor('a <!-- c --> b\n');
    await flush();
    typeInVisual(editor, 'x ');
    await flush();
    editor.changeMode('markdown');
    await flush();
    expect(editor.getMarkdown().trimEnd()).toBe('x a <!-- c --> b');
  });

  // CI review of #471: the fallback to plain text. A widget's placeholder ends at the first
  // `$$`, so HTML containing `$$` is kept as text rather than be cut short there. As text it
  // is escaped once its paragraph is edited (visible in Read mode), but never truncated.
  test('with `$$` in it, it is kept as text: escaped when edited, never cut short', async () => {
    const html = '<?php echo "$$"; ?>';
    const {editor, el} = createEditor(`a ${html} b\n`);
    await flush();
    expect(el.querySelector('.raw-inline-html')).toBeNull();
    expect(visualText(el)).toBe(`a ${html} b`);
    expect(editor.getMarkdown()).toBe(`a ${html} b\n`);
    typeInVisual(editor, 'x ');
    await flush();
    const saved = editor.getMarkdown();
    expect(saved.trimEnd()).toBe(`x a \\${html} b`);
    const reopened = createEditor(saved);
    await flush();
    expect(visualText(reopened.el)).toBe(`x a ${html} b`);
  });

  test('without the widget rule, it opens as text, escaped when edited', async () => {
    const {editor, el} = createEditor('a <!-- c --> b\n', 'wysiwyg', false);
    await flush();
    expect(el.querySelector('.raw-inline-html')).toBeNull();
    expect(visualText(el)).toBe('a <!-- c --> b');
    typeInVisual(editor, 'x ');
    await flush();
    expect(editor.getMarkdown().trimEnd()).toBe('x a \\<!-- c --> b');
  });
});

// Review of #471, finding 2: only plain (and link) text goes through the escaping text
// writer; code is written verbatim by Toast UI's own paths (convertInline's noEscape for
// inline code, the code block writer). Pinned so a Toast UI upgrade that moves them shows.
describe('typed `<div>` outside plain text', () => {
  test.each([
    ['inline code', (schema: any) => schema.nodes.paragraph.create(null, schema.text('<div>', [schema.marks.code.create()])), '`<div>`'],
    ['a code block', (schema: any) => schema.nodes.codeBlock.create(null, schema.text('<div>')), '```\n<div>\n```'],
    ['a link text', (schema: any) => schema.nodes.paragraph.create(null, schema.text('<div>', [schema.marks.link.create({linkUrl: 'https://example.com'})])), '[\\<div>](https://example.com)'],
  ])('in %s', async (_where, build, saved) => {
    const {editor} = createEditor('Hello world\n');
    await flush();
    const view = editor.wwEditor.view;
    view.dispatch(view.state.tr.replaceWith(0, view.state.doc.content.size, build(view.state.schema)));
    await flush();
    expect(editor.getMarkdown().trimEnd()).toBe(saved);
  });
});

test('isConvertibleHtmlBlock', () => {
  expect(isConvertibleHtmlBlock('<div>x</div>')).toBe(true);
  expect(isConvertibleHtmlBlock('</details>')).toBe(true);
  expect(isConvertibleHtmlBlock('<!-- ok -->')).toBe(true);
  for (const html of UNCONVERTIBLE_BLOCKS) expect(isConvertibleHtmlBlock(html)).toBe(false);
});

test('escapeHtmlStarts', () => {
  expect(escapeHtmlStarts('a <b and <!x and <?y and </z')).toBe('a \\<b and \\<!x and \\<?y and \\</z');
  // Already escaped, or not able to start HTML: unchanged.
  expect(escapeHtmlStarts('\\<div> a < b <3')).toBe('\\<div> a < b <3');
  // An escaped backslash does not escape the `<` after it.
  expect(escapeHtmlStarts('\\\\<div>')).toBe('\\\\\\<div>');
  // The backslash can end the markdown written before the text.
  expect(escapeHtmlStarts('<div>', 'x\\')).toBe('<div>');
  expect(escapeHtmlStarts('<div>', 'x\\\\')).toBe('\\<div>');
});
