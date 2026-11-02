// Relative images in the REAL Toast UI Editor built like the article editor (issue #167): the
// Visual editor shows them from the repository, and the markdown keeps the path as written.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {installLosslessMarkdownTracker} from './losslessMarkdown.ts';
import {createBase64WidgetRule, installBase64WidgetPatch} from './base64ImageWidget.ts';
import {articleEditorMarkdownOptions} from './toastEditorOptions.ts';
import {installRawHtmlGuard, rawInlineHtmlWidgetRule} from './rawHtmlGuard.ts';
import {installImageSrcResolver, resolveRelativeSrc} from './editorImageSrc.ts';

const FILE_URL = 'http://localhost:3000/user2/repo1/media/branch/main/README.md';
const BASE = 'http://localhost:3000/user2/repo1/media/branch/main';

const SOURCE = [
  '# Images',
  '',
  '<p align="center"><img src="block.png" alt="block"></p>',
  '',
  'External <img src="https://example.com/x.png" alt="ext"> image.',
  '',
  'Markdown ![md](pic.png) image.',
  '',
  'Inline <img src="./img/inline.png" alt="inline"> image.',
  '',
  'Last paragraph.',
  '',
].join('\n');

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

// The article editor's construction order (toast-editor.ts).
function createEditor(initialValue: string) {
  const el = document.createElement('div');
  document.body.append(el);
  const textarea = document.createElement('textarea');
  textarea.value = initialValue;
  document.body.append(textarea);
  const ref = {current: null as any};
  const widgetRules: unknown[] = [createBase64WidgetRule(() => ref.current), rawInlineHtmlWidgetRule];
  const editor = new Editor({
    el,
    initialEditType: 'wysiwyg',
    usageStatistics: false,
    widgetRules,
    ...articleEditorMarkdownOptions,
    customHTMLRenderer: {
      image(node: any, {skipChildren, getChildrenText}: any) {
        skipChildren();
        return [
          {type: 'openTag', tagName: 'img', attributes: {src: resolveRelativeSrc(node.destination, FILE_URL), alt: getChildrenText(node)}},
          {type: 'closeTag', tagName: 'img'},
        ];
      },
    },
  });
  ref.current = editor;
  installRawHtmlGuard(editor, widgetRules);
  installImageSrcResolver(editor, FILE_URL);
  installBase64WidgetPatch(editor);
  installLosslessMarkdownTracker(editor, textarea);
  return {editor, el};
}

function visualImages(el: HTMLElement): Record<string, string> {
  const imgs = el.querySelectorAll<HTMLImageElement>('.toastui-editor-ww-container .ProseMirror img:not(.ProseMirror-separator)');
  return Object.fromEntries(Array.from(imgs, (img) => [img.getAttribute('alt'), img.getAttribute('src')]));
}

afterEach(() => {
  document.body.innerHTML = '';
});

test('the Visual editor shows relative images from the repository', async () => {
  const {editor, el} = createEditor(SOURCE);
  await flush();
  expect(visualImages(el)).toEqual({
    md: `${BASE}/pic.png`,
    inline: `${BASE}/img/inline.png`,
    block: `${BASE}/block.png`,
    ext: 'https://example.com/x.png',
  });
  expect(editor.getMarkdown()).toBe(SOURCE);
});

// (without the HTML block: the three-way merge of a Visual edit rewrites an HTML block of
// an edited article as Toast UI serializes it, whatever its images)
const EDITED_SOURCE = SOURCE.replace('<p align="center"><img src="block.png" alt="block"></p>\n\n', '');

test('editing next to the images keeps their paths as written', async () => {
  const {editor} = createEditor(EDITED_SOURCE);
  await flush();
  const view = editor.wwEditor.view;
  for (const text of ['Inline ', 'Markdown ', 'Last paragraph.']) {
    let pos = -1;
    view.state.doc.descendants((node: any, at: number) => {
      if (pos < 0 && node.isText && node.text === text) pos = at;
      return pos < 0;
    });
    expect(pos).toBeGreaterThan(-1);
    view.dispatch(view.state.tr.insertText('X', pos, pos));
  }
  await flush();
  // (an edit in the last paragraph drops the final newline, as for any article)
  expect(editor.getMarkdown().trimEnd()).toBe(EDITED_SOURCE
    .replace('Markdown ', 'XMarkdown ')
    .replace('Inline ', 'XInline ')
    .replace('Last paragraph.', 'XLast paragraph.')
    .trimEnd());
});
