// An article's YAML front matter (the Wikipedia imports start with one: title, source,
// license, attribution, fetched_at) against the REAL Toast UI Editor, as the article
// editor builds it. Toast UI has no idea of front matter: its `---` opener is a thematic
// break and the lines under it a paragraph (or a setext heading, with the closing `---`),
// so without the editor's frontMatter option a Visual edit rewrote it as `***` + a run-together heading, which then renders
// as an <hr> and a big heading (seen on /subject/Prodromus/pierre).

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from './losslessMarkdown.ts';
import {installBase64WidgetPatch} from './base64ImageWidget.ts';
import {articleEditorMarkdownOptions} from './toastEditorOptions.ts';

const FRONT_MATTER = [
  '---',
  'title: "Prodromus"',
  'source: https://en.wikipedia.org/wiki/Prodromus',
  'license: CC BY-SA 4.0',
  'attribution: Wikipedia contributors',
  'fetched_at: 2025-11-11T13:56:22Z',
  '---',
].join('\n');
const BODY = [
  '',
  '# About prodromi',
  '',
  'A prodromus is a preliminary publication intended as the basis for a later, more comprehensive work.',
  '',
  'It is a second paragraph with snake_case_words.',
  '',
].join('\n');
const ARTICLE = `${FRONT_MATTER}\n${BODY}`;

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg') {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  // the article editor's own options (toast-editor.ts)
  const editor = new Editor({el, initialEditType, usageStatistics: false, ...articleEditorMarkdownOptions});
  installBase64WidgetPatch(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, textarea};
}

function editInVisual(editor: any, needle: string, replacement: string) {
  const view = editor.wwEditor.view;
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
  const selectionCtor: any = view.state.selection.constructor;
  let tr = view.state.tr.replaceWith(hit.from, hit.to, view.state.schema.text(replacement));
  tr = tr.setSelection(selectionCtor.near(tr.doc.resolve(hit.from + replacement.length)));
  view.dispatch(tr);
}

describe('YAML front matter survives the editor', () => {
  test('untouched, from Visual mode', () => {
    const {editor} = createEditor(ARTICLE);
    expect(editor.getMarkdown()).toBe(ARTICLE);
  });

  test('a word changed in the body in Visual mode', async () => {
    const {editor, textarea} = createEditor(ARTICLE);
    editInVisual(editor, 'preliminary', 'first');
    await flush();
    const output: string = editor.getMarkdown();
    // (a Visual edit drops the file's final newline, as it does without front matter)
    expect(output.trimEnd()).toBe(ARTICLE.replace('preliminary', 'first').trimEnd());
    expect(textarea.value).toBe(output);
  });

  test('a word changed in the body in Source mode', async () => {
    const {editor} = createEditor(ARTICLE, 'markdown');
    editor.setMarkdown(ARTICLE.replace('preliminary', 'first'));
    await flush();
    expect(editor.getMarkdown()).toBe(ARTICLE.replace('preliminary', 'first'));
  });

  test('a Visual edit, then Source and back, then another Visual edit', async () => {
    const {editor} = createEditor(ARTICLE);
    editInVisual(editor, 'preliminary', 'first');
    await flush();
    editor.changeMode('markdown');
    await flush();
    editor.changeMode('wysiwyg');
    await flush();
    editInVisual(editor, 'second paragraph', 'last paragraph');
    await flush();
    expect(editor.getMarkdown().trimEnd()).toBe(ARTICLE.replace('preliminary', 'first').replace('second paragraph', 'last paragraph').trimEnd());
  });

  test('a Visual edit next to the front matter', async () => {
    const {editor} = createEditor(ARTICLE);
    editInVisual(editor, 'About prodromi', 'On prodromi');
    await flush();
    const output: string = editor.getMarkdown();
    expect(output.startsWith(`${FRONT_MATTER}\n`)).toBe(true);
    expect(output).toContain('# On prodromi');
  });
});
