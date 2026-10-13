// The link and cite tools' dialogs (issue #399) against the real Toast UI Editor, with the
// server's answers mocked.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from '../comp/losslessMarkdown.ts';
import {installInternalLinkView} from './internal-link-view.ts';
import {currentView, findText} from './editor-ops.ts';
import {installSubjectLinkTool, openSubjectLinkTool} from './subject-link-tool.ts';
import {openCiteTool} from './cite-tool.ts';

let lookupResult: Promise<Response>;
vi.mock('../../modules/fetch.ts', () => ({
  GET: () => Promise.resolve(new Response(JSON.stringify({subjects: []}))),
  POST: () => lookupResult,
}));

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function createEditor(initialValue: string) {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const editor = new Editor({el, initialEditType: 'markdown', usageStatistics: false});
  installInternalLinkView(editor);
  installLosslessMarkdownTracker(editor, textarea);
  installSubjectLinkTool(editor, el);
  return editor;
}

afterEach(() => {
  document.body.innerHTML = '';
});

test('citing an address pasted in the link tool keeps the selected text (review 2 #3)', async () => {
  lookupResult = Promise.resolve(new Response('', {status: 404}));
  const editor = createEditor('India landed on the Moon in 2023.');
  const view = currentView(editor);
  const from = findText(view, 'India');
  const to = from + 'India landed on the Moon'.length;
  view.dispatch(view.state.tr.setSelection(view.state.selection.constructor.create(view.state.doc, from, to)));
  openSubjectLinkTool(editor);

  const field = document.querySelector<HTMLInputElement>('.citation-subject-search input[type="search"]');
  field.dispatchEvent(new Event('paste'));
  field.value = 'https://www.example.org/chandrayaan';
  field.dispatchEvent(new Event('input'));
  await wait(600);

  const cite = document.querySelector<HTMLButtonElement>('.citation-link-url [data-action="cite"]');
  expect(cite).not.toBeNull();
  cite.click();
  await wait(0);
  document.querySelector<HTMLButtonElement>('.citation-dialog [data-action="insert"]').click();
  expect(editor.getMarkdown()).toMatch(/^India landed on the Moon\[\^1\] in 2023\./);
});

test('a URL being typed after "[[" does not take the focus (review 2 #16)', async () => {
  const editor = createEditor('Text ');
  const view = currentView(editor);
  const end = view.state.doc.content.size - 1;
  view.dispatch(view.state.tr.insertText('[[https://e', end));
  view.dom.dispatchEvent(new Event('input', {bubbles: true}));
  await wait(0);
  expect(document.querySelector('.citation-subject-search')).not.toBeNull(); // the "[[" list
  await wait(600);
  expect(document.querySelector('.citation-link-url')).toBeNull();
  // Enter offers to cite it
  view.dom.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}));
  await wait(0);
  expect(document.querySelector('.citation-link-url')).not.toBeNull();
});

test('a lookup keeps the field being typed in and the type the writer chose (review 2 #6, #15)', async () => {
  let answer: (r: Response) => void;
  lookupResult = new Promise((resolve) => {
    answer = resolve;
  });
  const editor = createEditor('Text.');
  await wait(0); // the editor's autofocus
  const view = currentView(editor);
  const at = view.state.doc.content.size - 1;
  openCiteTool(editor, {from: at, to: at, query: 'https://www.example.org/a', manual: true});

  const title = document.querySelector<HTMLInputElement>('.citation-field[data-key="title"] input');
  title.focus();
  title.value = 'My tit';
  title.dispatchEvent(new Event('input'));
  title.setSelectionRange(6, 6);
  answer(new Response(JSON.stringify({type: 'website', site: 'Example', published: '2024-01-02'})));
  await wait(0);
  await wait(0);
  const again = document.querySelector<HTMLInputElement>('.citation-field[data-key="title"] input');
  expect(again.value).toBe('My tit');
  expect(document.activeElement).toBe(again);
  expect(again.selectionStart).toBe(6);

  // the writer picks Book while a lookup answering "news" is pending
  lookupResult = new Promise((resolve) => {
    answer = resolve;
  });
  const url = document.querySelector<HTMLInputElement>('.citation-field[data-key="url"] input');
  url.value = 'https://www.example.org/b';
  url.dispatchEvent(new Event('change'));
  const book = Array.from(document.querySelectorAll<HTMLButtonElement>('.citation-type-tab')).find((b) => b.textContent.includes('type_book'));
  book.click();
  expect(document.activeElement?.textContent).toContain('type_book');
  answer(new Response(JSON.stringify({type: 'news', title: 'News title'})));
  await wait(0);
  await wait(0);
  expect(document.querySelector('.citation-type-tab[aria-checked="true"]').textContent).toContain('type_book');
});

test('the editor loses its combobox state when the list closes from outside (review 2 #17)', async () => {
  const editor = createEditor('Text ');
  await wait(0);
  const view = currentView(editor);
  view.dispatch(view.state.tr.insertText('[[Apo', view.state.doc.content.size - 1));
  view.dom.dispatchEvent(new Event('input', {bubbles: true}));
  await wait(300);
  expect(view.dom.getAttribute('aria-expanded')).toBe('true');
  document.body.dispatchEvent(new PointerEvent('pointerdown', {bubbles: true}));
  expect(document.querySelector('.citation-subject-search')).toBeNull();
  expect(view.dom.hasAttribute('aria-expanded')).toBe(false);
  expect(view.dom.hasAttribute('aria-controls')).toBe(false);
});
