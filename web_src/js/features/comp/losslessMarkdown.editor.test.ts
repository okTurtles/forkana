// Integration tests for the lossless markdown tracker against the REAL Toast UI Editor
// (issue #262). losslessMarkdown.test.ts and markdownThreeWayMerge.test.ts exercise the
// pieces against hand-written fakes; these tests pin the actual Toast UI 3.2.2 behaviour the
// fakes imitate, so they cannot silently drift apart after a dependency bump.
//
// Visual-mode edits are made by dispatching ProseMirror transactions straight at the WYSIWYG
// view, which is what a real keystroke ends up doing — it goes through the same change
// events and the same serializer as a user typing.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from './losslessMarkdown.ts';
import {installBase64WidgetPatch, createBase64WidgetRule} from './base64ImageWidget.ts';

// Deliberately contains the constructs Toast UI's WYSIWYG serializer damages: a
// reference-style link, a link reference definition, a bare (auto-linked) URL, `-` bullets
// and underscores/periods in plain text.
const LINK_LINE = 'See [the program][1] and https://example.com/bare_link and snake_case_word.';
const DEFINITION_LINE = '[1]: https://example.com/ref "Ref title"';
const SAMPLE = [
  '# Research program',
  '',
  'Intro paragraph about the topic.',
  '',
  LINK_LINE,
  '',
  'A middle paragraph that will be edited.',
  '',
  '- bullet with_underscore',
  '- second bullet',
  '',
  'An inline [link](./Research_program "Research program") too.',
  '',
  'Closing paragraph at the end.',
  '',
  DEFINITION_LINE,
  '',
].join('\n');

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg', widgets = false) {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const ref = {current: null as any};
  const options: any = {el, initialEditType, usageStatistics: false};
  if (widgets) options.widgetRules = [createBase64WidgetRule(() => ref.current)];
  const editor = new Editor(options);
  ref.current = editor;
  // Same install order as toast-editor.ts / ToastCommentEditor.ts.
  installBase64WidgetPatch(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, textarea};
}

function wysiwygView(editor: any) {
  return editor.wwEditor.view;
}

// Replaces `needle` with `replacement` inside the WYSIWYG document.
function editInVisual(editor: any, needle: string, replacement: string) {
  const view = wysiwygView(editor);
  let hit: {from: number, to: number} | null = null;
  view.state.doc.descendants((node: any, pos: number) => {
    if (hit) return false;
    if (node.isText && typeof node.text === 'string') {
      const at = node.text.indexOf(needle);
      if (at >= 0) hit = {from: pos + at, to: pos + at + needle.length};
    }
    return !hit;
  });
  if (!hit) throw new Error(`text not found in the WYSIWYG document: ${needle}`);
  view.dispatch(view.state.tr.replaceWith(hit.from, hit.to, view.state.schema.text(replacement)));
}

function deleteParagraphInVisual(editor: any, needle: string) {
  const view = wysiwygView(editor);
  let hit: {from: number, to: number} | null = null;
  view.state.doc.descendants((node: any, pos: number) => {
    if (hit) return false;
    if (node.type.name === 'paragraph' && node.textContent.includes(needle)) {
      hit = {from: pos, to: pos + node.nodeSize};
    }
    return !hit;
  });
  if (!hit) throw new Error(`paragraph not found in the WYSIWYG document: ${needle}`);
  view.dispatch(view.state.tr.delete(hit.from, hit.to));
}

function appendParagraphInVisual(editor: any, text: string) {
  const view = wysiwygView(editor);
  const paragraph = view.state.schema.nodes.paragraph.create(null, view.state.schema.text(text));
  view.dispatch(view.state.tr.insert(view.state.doc.content.size, paragraph));
}

// Asserts every line of SAMPLE is still present byte-for-byte, except the listed ones.
function expectSamplePreserved(output: string, ...excluded: string[]) {
  for (const line of SAMPLE.split('\n')) {
    if (!line || excluded.includes(line)) continue;
    expect(output).toContain(line);
  }
}

// This is the bug being fixed, asserted against the real serializer. If a Toast UI upgrade
// ever makes this round-trip lossless, the merge layer can be simplified/removed.
test('the raw Toast UI WYSIWYG serializer mangles links (the #262 root cause)', () => {
  const el = document.createElement('div');
  document.body.append(el);
  const editor = new Editor({el, initialEditType: 'wysiwyg', usageStatistics: false});
  editor.setMarkdown(SAMPLE);
  const serialized: string = editor.getMarkdown();

  expect(serialized).not.toBe(SAMPLE);
  // Reference links and their definitions are escaped into literal text -> no hyperlink.
  expect(serialized).toContain('\\[the program\\]\\[1\\]');
  expect(serialized).toContain('\\[1\\]:');
  // Bare URLs and plain text get backslash-escaped too.
  expect(serialized).toContain('https://example\\.com/bare\\_link');
  expect(serialized).toContain('snake\\_case\\_word');
});

describe('documents nobody edited in Visual mode', () => {
  test('untouched content submits byte-identical from Visual mode', () => {
    const {editor, textarea} = createEditor(SAMPLE);
    expect(editor.getMarkdown()).toBe(SAMPLE);
    expect(textarea.value).toBe(SAMPLE);
  });

  test('Visual -> Source without edits leaves the source untouched', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    editor.changeMode('markdown');
    await flush();
    expect(editor.getMarkdown()).toBe(SAMPLE);
    expect(textarea.value).toBe(SAMPLE);
  });

  test('repeated mode switching never rewrites the source', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    for (let i = 0; i < 3; i++) {
      editor.changeMode('markdown');
      await flush();
      editor.changeMode('wysiwyg');
      await flush();
    }
    expect(editor.getMarkdown()).toBe(SAMPLE);
    expect(textarea.value).toBe(SAMPLE);
  });

  // The submit path a user takes when they fix something in Source mode and then flip back
  // to Visual before pressing "Submit Changes": the stale WYSIWYG baseline must not leak.
  test('Source edit survives a switch back to Visual mode', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    editor.changeMode('markdown');
    await flush();
    const edited = `${SAMPLE}\nAnother [ref][1] line.\n`;
    editor.setMarkdown(edited);
    await flush();
    editor.changeMode('wysiwyg');
    await flush();
    expect(editor.getMarkdown()).toBe(edited);
    expect(textarea.value).toBe(edited);
  });

  test('the editor starts in Source mode losslessly too', async () => {
    const {editor, textarea} = createEditor(SAMPLE, 'markdown');
    expect(editor.getMarkdown()).toBe(SAMPLE);
    editor.changeMode('wysiwyg');
    await flush();
    editor.changeMode('markdown');
    await flush();
    expect(editor.getMarkdown()).toBe(SAMPLE);
    expect(textarea.value).toBe(SAMPLE);
  });
});

describe('Visual edits are merged back onto the pristine source', () => {
  // THE ACCEPTANCE TEST for issue #262 part 1: one character typed in an unrelated paragraph
  // used to re-serialize the whole article and destroy every reference link in it.
  test('an edit in the middle leaves reference links and definitions byte-identical', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    editInVisual(editor, 'A middle paragraph that will be edited.', 'A middle paragraph that was edited!');
    await flush();
    const output: string = editor.getMarkdown();

    expect(output).toContain('A middle paragraph that was edited!');
    expect(output).toContain(LINK_LINE);
    expect(output).toContain(DEFINITION_LINE);
    expect(output).not.toContain('\\[the program\\]');
    expect(output).not.toContain('\\[1\\]:');
    expect(output).not.toContain('snake\\_case');
    expectSamplePreserved(output, 'A middle paragraph that will be edited.');
    expect(textarea.value).toBe(output);
  });

  test('an edit at the start leaves the rest byte-identical', async () => {
    const {editor} = createEditor(SAMPLE);
    editInVisual(editor, 'Intro paragraph about the topic.', 'Intro paragraph, rewritten.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('Intro paragraph, rewritten.');
    expectSamplePreserved(output, 'Intro paragraph about the topic.');
  });

  test('an edit at the end leaves the rest byte-identical', async () => {
    const {editor} = createEditor(SAMPLE);
    editInVisual(editor, 'Closing paragraph at the end.', 'Closing paragraph, extended.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('Closing paragraph, extended.');
    expectSamplePreserved(output, 'Closing paragraph at the end.');
  });

  test('deleting a paragraph removes only that paragraph', async () => {
    const {editor} = createEditor(SAMPLE);
    deleteParagraphInVisual(editor, 'A middle paragraph that will be edited.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).not.toContain('A middle paragraph');
    expectSamplePreserved(output, 'A middle paragraph that will be edited.');
  });

  test('adding a paragraph appends it without disturbing the article', async () => {
    const {editor} = createEditor(SAMPLE);
    appendParagraphInVisual(editor, 'A brand new closing note.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('A brand new closing note.');
    expectSamplePreserved(output);
  });

  // The one line the user actually touched legitimately gets the serializer's spelling —
  // its reference link is collateral damage of editing that very sentence. Everything else,
  // including the link *definition* that makes it resolve, must survive.
  test('editing the paragraph that holds the reference link only re-serializes that line', async () => {
    const {editor} = createEditor(SAMPLE);
    editInVisual(editor, 'and snake_case_word.', 'and snake_case_word, extended.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('\\[the program\\]\\[1\\]'); // that line, re-serialized
    expect(output).toContain(DEFINITION_LINE); // but the definition is intact
    expectSamplePreserved(output, LINK_LINE);
  });

  test('a base64 image survives an unrelated Visual edit', async () => {
    const image = `![alt text](data:image/png;base64,${'A'.repeat(60)}==)`;
    const document_ = `Intro_paragraph here.\n\n${image}\n\nOutro with [a ref][1].\n\n[1]: https://example.com/x\n`;
    const {editor} = createEditor(document_, 'wysiwyg', true);
    expect(editor.getMarkdown()).toBe(document_);
    editInVisual(editor, 'Intro_paragraph here.', 'Intro_paragraph here, edited.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain(image);
    expect(output).toContain('Outro with [a ref][1].');
    expect(output).toContain('[1]: https://example.com/x');
    expect(output).not.toContain('$$widget');
  });

  test('the Source editor shows the merged text, not the escaped serialization', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    editInVisual(editor, 'A middle paragraph that will be edited.', 'A middle paragraph that was edited!');
    await flush();
    editor.changeMode('markdown');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('A middle paragraph that was edited!');
    expect(output).toContain(LINK_LINE);
    expect(output).toContain(DEFINITION_LINE);
    expect(textarea.value).toBe(output);
    // And it round-trips: going back to Visual and out again changes nothing further.
    editor.changeMode('wysiwyg');
    await flush();
    editor.changeMode('markdown');
    await flush();
    expect(editor.getMarkdown()).toBe(output);
  });

  test('a whole-document rewrite falls back to the serialization', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    const view = wysiwygView(editor);
    const paragraph = view.state.schema.nodes.paragraph.create(null, view.state.schema.text('Everything_replaced.'));
    view.dispatch(view.state.tr.replaceWith(0, view.state.doc.content.size, paragraph));
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).not.toContain('Research program');
    expect(output).toContain('Everything_replaced'.replace('_', '\\_'));
    expect(textarea.value).toBe(output);
  });
});
