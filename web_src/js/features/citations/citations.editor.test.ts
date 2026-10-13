// The link and cite tools (issue #399) against the REAL Toast UI Editor, installed the way
// toast-editor.ts installs them: the internal link view before the lossless tracker.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from '../comp/losslessMarkdown.ts';
import {installBase64WidgetPatch} from '../comp/base64ImageWidget.ts';
import {installInternalLinkView} from './internal-link-view.ts';
import {currentView, findText, insertFootnote, insertLink, isInCode} from './editor-ops.ts';
import {subjectLinkURL} from './subject-link-tool.ts';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

const ARTICLE = [
  '# Moon landing',
  '',
  'The first crewed landing was [Apollo 11](subject/Apollo 11) in 1969.[^1]',
  '',
  'See [Tom\'s version](subject/Apollo 11/tom66) and [Mercury](subject/Mercury (planet)).',
  '',
  '[^1]: ["Apollo 11"](https://www.nasa.gov/apollo-11). *NASA*. Retrieved 1 October 2026.',
  '',
].join('\n');

function createEditor(initialValue: string, initialEditType: 'markdown' | 'wysiwyg' = 'wysiwyg') {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const editor = new Editor({el, initialEditType, usageStatistics: false});
  installBase64WidgetPatch(editor);
  installInternalLinkView(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, textarea};
}

function linksOf(view: any): Array<{text: string, url: string}> {
  const links: Array<{text: string, url: string}> = [];
  view.state.doc.descendants((node: any) => {
    const link = node.marks?.find((m: any) => m.type.name === 'link');
    if (node.isText && link) links.push({text: node.text, url: link.attrs.linkUrl});
  });
  return links;
}

test('internal links with spaces are links in the Visual editor', () => {
  const {editor} = createEditor(ARTICLE);
  expect(linksOf(editor.wwEditor.view)).toEqual([
    {text: 'Apollo 11', url: 'subject/Apollo 11'},
    {text: 'Tom\'s version', url: 'subject/Apollo 11/tom66'},
    {text: 'Mercury', url: 'subject/Mercury (planet)'},
    {text: '"Apollo 11"', url: 'https://www.nasa.gov/apollo-11'},
  ]);
  // nothing was edited: the article is unchanged
  expect(editor.getMarkdown()).toBe(ARTICLE);
});

test('internal links round-trip through the modes', async () => {
  const {editor, textarea} = createEditor(ARTICLE, 'markdown');
  editor.changeMode('wysiwyg');
  await flush();
  expect(linksOf(editor.wwEditor.view).map((l) => l.url)).toContain('subject/Apollo 11');
  editor.changeMode('markdown');
  await flush();
  expect(editor.getMarkdown()).toBe(ARTICLE);
  expect(textarea.value).toBe(ARTICLE);
});

test('a Visual edit keeps the internal links in Markdown', () => {
  const {editor} = createEditor(ARTICLE);
  const view = editor.wwEditor.view;
  const at = findText(view, 'in 1969.');
  view.dispatch(view.state.tr.insertText('on 20 July ', at + 'in '.length));
  const md = editor.getMarkdown();
  expect(md).toContain('[Apollo 11](subject/Apollo 11) in on 20 July 1969.[^1]');
  expect(md).toContain('[Mercury](subject/Mercury (planet))');
  expect(md).toContain('[^1]: ["Apollo 11"](https://www.nasa.gov/apollo-11). *NASA*. Retrieved 1 October 2026.');
});

test('insertLink in the Visual editor adds a link with the subject name', () => {
  const {editor} = createEditor('Landings: here.\n');
  const view = currentView(editor);
  const at = findText(view, 'here');
  insertLink(editor, at, at + 'here'.length, 'Apollo 11', subjectLinkURL('Apollo 11'));
  expect(editor.getMarkdown().trim()).toBe('Landings: [Apollo 11](subject/Apollo 11).');
});

test('insertLink in the Source editor writes the Markdown', () => {
  const {editor} = createEditor('Landings: [[Apol', 'markdown');
  const view = currentView(editor);
  const end = view.state.doc.content.size - 1;
  const start = end - '[[Apol'.length;
  insertLink(editor, start, end, 'Apollo 11', subjectLinkURL('Apollo 11', 'tom66'));
  expect(editor.getMarkdown()).toBe('Landings: [Apollo 11](subject/Apollo 11/tom66)');
});

test('insertFootnote in the Source editor adds the reference and its definition', () => {
  const {editor, textarea} = createEditor('Luna 2 came first.\n', 'markdown');
  const view = currentView(editor);
  const at = findText(view, 'first.') + 'first.'.length;
  insertFootnote(editor, at, at, '1', '"Luna 2". *NASA–NSSDC*.');
  expect(editor.getMarkdown()).toBe('Luna 2 came first.[^1]\n\n[^1]: "Luna 2". *NASA–NSSDC*.\n');
  expect(textarea.value).toBe(editor.getMarkdown());
  // the caret is after the reference
  expect(currentView(editor).state.selection.from).toBe(at + '[^1]'.length);
});

test('insertFootnote in the Visual editor goes through the Markdown', () => {
  const {editor} = createEditor(ARTICLE);
  const view = currentView(editor);
  const at = findText(view, 'in 1969.[^1]') + 'in 1969.[^1]'.length;
  insertFootnote(editor, at, at, '2', '["Luna 2"](https://nssdc.gsfc.nasa.gov/x). *NASA–NSSDC*. Retrieved 1 October 2026.');
  const md = editor.getMarkdown();
  expect(md).toContain('[Apollo 11](subject/Apollo 11) in 1969.[^1][^2]');
  expect(md).toContain('[^1]: ["Apollo 11"](https://www.nasa.gov/apollo-11). *NASA*. Retrieved 1 October 2026.\n[^2]: ["Luna 2"](https://nssdc.gsfc.nasa.gov/x). *NASA–NSSDC*. Retrieved 1 October 2026.');
  // the rest of the article is untouched, internal links included
  expect(md).toContain('See [Tom\'s version](subject/Apollo 11/tom66) and [Mercury](subject/Mercury (planet)).');
  expect(md.startsWith('# Moon landing\n\n')).toBe(true);
  // and it still shows the internal links as links
  expect(linksOf(currentView(editor)).map((l) => l.url)).toContain('subject/Apollo 11');
});

test('reusing a source in the Visual editor only adds the reference', () => {
  const {editor} = createEditor(ARTICLE);
  const view = currentView(editor);
  const at = findText(view, 'Mercury') - 1;
  insertFootnote(editor, at, at, '1');
  const md = editor.getMarkdown();
  expect(md.match(/\[\^1\]:/g)).toHaveLength(1);
  expect(md).toContain('[^1]');
  expect(md.match(/\[\^1\]/g).length).toBe(3);
});

test('subjectLinkURL keeps the name readable', () => {
  expect(subjectLinkURL('Solar System')).toBe('subject/Solar System');
  expect(subjectLinkURL('AC/DC')).toBe('subject/AC%2FDC');
  expect(subjectLinkURL('Mercury (planet)')).toBe('subject/Mercury (planet)');
  expect(subjectLinkURL('Smile :)')).toBe('subject/Smile :%29');
  expect(subjectLinkURL('Solar System', 'pierre')).toBe('subject/Solar System/pierre');
  expect(subjectLinkURL('What? #1')).toBe('subject/What%3F %231');
  // a quoted word at the end would be the link's title (review 2 #10)
  expect(subjectLinkURL('Operation "Desert Storm"')).toBe('subject/Operation %22Desert Storm%22');
  expect(subjectLinkURL("The 'Moon'")).toBe("subject/The 'Moon%27");
});

test('a quoted title stays a title in the Visual editor (review #7)', () => {
  const md = '[Moon](subject/Moon "the satellite") and [Mercury](subject/Mercury (planet)).\n';
  const {editor} = createEditor(md);
  expect(linksOf(editor.wwEditor.view)).toEqual([
    {text: 'Moon', url: 'subject/Moon'},
    {text: 'Mercury', url: 'subject/Mercury (planet)'},
  ]);
  expect(editor.getMarkdown()).toBe(md);
});

test('a citation after inline code goes after it, not in it (review #10)', () => {
  const {editor} = createEditor('Run `make` here.\n');
  const view = currentView(editor);
  const at = findText(view, ' here');
  expect(isInCode(editor, at)).toBe(false); // at the edge of the code span
  expect(isInCode(editor, at - 2)).toBe(true); // inside it
  insertFootnote(editor, at, at, '1', 'A source.');
  const md = editor.getMarkdown();
  expect(md).toContain('Run `make`[^1] here.');
  expect(md).toContain('[^1]: A source.');
});

test('isInCode in the Source editor', () => {
  const {editor} = createEditor('Text `code` and\n\n```sh\nif [[ -f x ]]\n```\n', 'markdown');
  const view = currentView(editor);
  expect(isInCode(editor, findText(view, 'code') + 1)).toBe(true);
  expect(isInCode(editor, findText(view, ' and'))).toBe(false);
  expect(isInCode(editor, findText(view, '-f'))).toBe(true);
});

test('a reused label with "$" patterns is inserted as written (review #31)', () => {
  const {editor} = createEditor('One.[^$&]\n\n[^$&]: Source.\n');
  const view = currentView(editor);
  const at = findText(view, 'One.') + 'One.'.length;
  insertFootnote(editor, at, at, '$&');
  expect(editor.getMarkdown()).toContain('One.[^$&][^$&]');
});

test('insertLink in the Source editor keeps a plain address a link (review #17)', () => {
  const {editor} = createEditor('See x.', 'markdown');
  const view = currentView(editor);
  const at = findText(view, 'x');
  insertLink(editor, at, at + 1, 'C*-algebra*', 'https://example.com/a b(c');
  expect(editor.getMarkdown()).toBe('See [C\\*-algebra\\*](<https://example.com/a b(c>).');
});

test('insertLink in the Visual editor keeps an address with a parenthesis a link (review 2 #11)', () => {
  const {editor} = createEditor('See x.\n');
  const view = currentView(editor);
  const at = findText(view, 'x');
  insertLink(editor, at, at + 1, 'page', 'https://example.com/a(b c');
  expect(editor.getMarkdown().trim()).toBe('See [page](https://example.com/a%28b%20c).');
});
