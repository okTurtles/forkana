// Integration tests for the lossless markdown tracker against the REAL Toast UI Editor
// (issue #262). losslessMarkdown.test.ts exercises the tracker against a hand-written fake;
// these tests pin the actual Toast UI 3.2.2 behaviour the tracker is built on, so the fake
// cannot silently drift away from it after a dependency bump.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from './losslessMarkdown.ts';
import {installBase64WidgetPatch} from './base64ImageWidget.ts';

// Deliberately contains the constructs Toast UI's WYSIWYG serializer damages:
// a reference-style link, a link reference definition, a bare (auto-linked) URL and
// underscores/periods in plain text.
const SAMPLE = [
  '# Research program',
  '',
  'See [the program][1] and https://example.com/bare_link and snake_case_word.',
  '',
  'An inline [link](./Research_program "Research program") too.',
  '',
  '[1]: https://example.com/ref "Ref title"',
  '',
].join('\n');

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg') {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const editor = new Editor({el, initialEditType, usageStatistics: false});
  // Same install order as toast-editor.ts / ToastCommentEditor.ts.
  installBase64WidgetPatch(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, textarea};
}

// This is the bug being fixed, asserted against the real serializer. If a Toast UI upgrade
// ever makes this round-trip lossless, the tracker can be simplified/removed.
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
  // Bare URLs lose GFM auto-linking because `.` and `_` get backslash-escaped.
  expect(serialized).toContain('https://example\\.com/bare\\_link');
  // Plain text is escaped too.
  expect(serialized).toContain('snake\\_case\\_word');
});

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

// KNOWN GAP (issue #262 part 1 is only partially fixed): a single character typed in the
// Visual editor makes Toast UI's serialization authoritative for the WHOLE document, so
// reference links / auto-links elsewhere in the article are still destroyed. Asserted here
// so the limitation is explicit and the day it is fixed this test fails loudly.
test('a genuine Visual edit still adopts the lossy serialization document-wide', async () => {
  const {editor, textarea} = createEditor(SAMPLE);
  editor.focus();
  editor.insertText('X');
  await flush();
  const result: string = editor.getMarkdown();
  expect(result).toContain('X');
  expect(result).toContain('\\[the program\\]\\[1\\]'); // <- the remaining bug
  expect(textarea.value).toBe(result);
});
