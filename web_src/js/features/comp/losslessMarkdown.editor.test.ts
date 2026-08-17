// Integration tests for the lossless markdown tracker against the REAL Toast UI Editor
// (issue #262). losslessMarkdown.test.ts and markdownThreeWayMerge.test.ts exercise the
// pieces against hand-written fakes; these tests pin the actual Toast UI 3.2.2 behavior the
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

function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg', withWidgetRules = false) {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const ref = {current: null as any};
  const options: any = {el, initialEditType, usageStatistics: false};
  if (withWidgetRules) options.widgetRules = [createBase64WidgetRule(() => ref.current)];
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

// Replaces `needle` with `replacement` inside the WYSIWYG document, leaving the caret right
// after the replacement — where a user who just typed it would have left it.
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
  // `Selection` is not importable here: Toast UI bundles its own copy of prosemirror-state.
  const selectionCtor: any = view.state.selection.constructor;
  let tr = view.state.tr.replaceWith(hit.from, hit.to, view.state.schema.text(replacement));
  tr = tr.setSelection(selectionCtor.near(tr.doc.resolve(hit.from + replacement.length)));
  view.dispatch(tr);
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
    const article = `Intro_paragraph here.\n\n${image}\n\nOutro with [a ref][1].\n\n[1]: https://example.com/x\n`;
    const {editor} = createEditor(article, 'wysiwyg', true);
    expect(editor.getMarkdown()).toBe(article);
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

  // The serializer rewrites the *length* of setext underlines and table delimiter rows
  // (`==============` -> `===`, `| - | - |` -> `| --- | --- |`). normalizeLine treats rule
  // lines as length-insensitive so these keep their original bytes too.
  test('tables and setext headings keep their original formatting', async () => {
    const article = [
      'Setext Heading',
      '==============',
      '',
      'An intro_paragraph here.',
      '',
      '| a | b |',
      '| - | - |',
      '| 1 | 2 |',
      '',
      'See [ref][1] at the end.',
      '',
      '[1]: https://example.com/x',
      '',
    ].join('\n');
    const {editor} = createEditor(article);
    expect(editor.getMarkdown()).toBe(article);
    editInVisual(editor, 'An intro_paragraph here.', 'An intro_paragraph, edited.');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).toContain('==============');
    expect(output).toContain('| - | - |');
    expect(output).toContain('See [ref][1] at the end.');
    expect(output).toContain('[1]: https://example.com/x');
  });

  // Switching to Source mode replaces the editor's document with the merged text, and that
  // replacement has to pass cursorToEnd=true (see the comment in losslessMarkdown.ts), which
  // on its own drops the caret at the bottom of the article. The caret is captured before the
  // replacement and re-applied after it.
  describe('the caret survives the switch to Source mode', () => {
    const caretLine = (editor: any): number => editor.getSelection()[0][0];
    const lastLine = (editor: any): number => editor.getMarkdown().split('\n').length;

    // Makes the same edit on an editor with no tracker installed and reports where Toast UI
    // puts the caret on its own. That is the position the writeback must not destroy: it is
    // both what the editor did before this feature existed and what a user expects.
    async function untrackedCaretLine(needle: string, replacement: string): Promise<number> {
      const el = document.createElement('div');
      document.body.append(el);
      const editor: any = new Editor({el, initialEditType: 'wysiwyg', usageStatistics: false});
      installBase64WidgetPatch(editor);
      editor.setMarkdown(SAMPLE);
      editInVisual(editor, needle, replacement);
      await flush();
      editor.changeMode('markdown');
      await flush();
      return caretLine(editor);
    }

    test('after a Visual edit the caret stays where Toast UI would have put it', async () => {
      const needle = 'A middle paragraph that will be edited.';
      const replacement = 'A middle paragraph that was edited!';
      const {editor} = createEditor(SAMPLE);
      editInVisual(editor, needle, replacement);
      await flush();
      editor.changeMode('markdown');
      await flush();
      expect(caretLine(editor)).toBe(await untrackedCaretLine(needle, replacement));
      // ...which is the edited line, in the middle of the article, not its end.
      expect(editor.getMarkdown().split('\n')[caretLine(editor) - 1]).toBe(replacement);
      expect(caretLine(editor)).toBeLessThan(lastLine(editor));
    });

    // The merged line is the shorter, unescaped spelling, so the column Toast UI mapped
    // against the serialized document can point past its end.
    test('the caret survives an edit on a line the serializer escapes', async () => {
      const needle = 'and snake_case_word.';
      const replacement = 'and snake_case_word, extended.';
      const {editor} = createEditor(SAMPLE);
      editInVisual(editor, needle, replacement);
      await flush();
      editor.changeMode('markdown');
      await flush();
      const line = caretLine(editor);
      expect(line).toBe(await untrackedCaretLine(needle, replacement));
      expect(line).toBeLessThan(lastLine(editor));
      // The column is clamped into the line, so the position is always valid.
      const [[, ch]] = editor.getSelection();
      expect(ch).toBeLessThanOrEqual(editor.getMarkdown().split('\n')[line - 1].length + 1);
    });

    test('with no Visual edit at all the caret still lands near where it was', async () => {
      const {editor} = createEditor(SAMPLE);
      const view = wysiwygView(editor);
      const selectionCtor: any = view.state.selection.constructor;
      // Inside "An inline [link](...) too." — line 12 of SAMPLE, well before the end.
      let hit = 0;
      view.state.doc.descendants((node: any, pos: number) => {
        if (hit) return false;
        if (node.isText && node.text?.includes('An inline')) hit = pos + 3;
        return !hit;
      });
      view.dispatch(view.state.tr.setSelection(selectionCtor.near(view.state.doc.resolve(hit))));
      editor.changeMode('markdown');
      await flush();
      expect(editor.getMarkdown()).toBe(SAMPLE); // still lossless
      // Toast UI's own mapping is only line-accurate, so allow its off-by-one; the point is
      // that the caret is at the paragraph the user was in and not at the end of the article.
      expect(caretLine(editor)).toBeGreaterThanOrEqual(12);
      expect(caretLine(editor)).toBeLessThanOrEqual(13);
    });

    test('the restored caret does not break a later mode switch', async () => {
      const {editor} = createEditor(SAMPLE);
      editInVisual(editor, 'Intro paragraph about the topic.', 'Intro paragraph, rewritten.');
      await flush();
      for (let i = 0; i < 2; i++) {
        editor.changeMode('markdown');
        await flush();
        editor.changeMode('wysiwyg');
        await flush();
      }
      editor.changeMode('markdown');
      await flush();
      const output: string = editor.getMarkdown();
      expect(output).toContain('Intro paragraph, rewritten.');
      expect(output).toContain(DEFINITION_LINE);
    });
  });

  test('a whole-document rewrite falls back to the serialization', async () => {
    const {editor, textarea} = createEditor(SAMPLE);
    const view = wysiwygView(editor);
    const paragraph = view.state.schema.nodes.paragraph.create(null, view.state.schema.text('Everything_replaced.'));
    view.dispatch(view.state.tr.replaceWith(0, view.state.doc.content.size, paragraph));
    await flush();
    const output: string = editor.getMarkdown();
    expect(output).not.toContain('Research program');
    expect(output).toContain('Everything\\_replaced');
    expect(textarea.value).toBe(output);
  });
});
