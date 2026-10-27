// The article editor's toolbar. Its buttons are those of figma "Article view / Logged out -
// Edit" (7368:264887, _Markdown toolbar 7368:265005): heading, bold, italic | quote, code,
// link | bulleted, numbered and task lists | a fourth group, which in the figma is mention,
// cross-reference and saved replies with a dropdown. An article has no mentions or saved
// replies, so that group holds what the article editor has instead:
// - Cite a source (issue #399, tooltip "Cite a source", Ctrl ⇧ K), with the figma's
//   cross-reference icon: a citation is the article's reference to a source;
// - the figma's 40px reply-and-dropdown button, as a menu with the editor's code block,
//   image and table, which the figma's toolbar has no room for.
// The link button links to a Forkana subject (tooltip "Link to a Forkana subject", shortcut
// "[[") or, given an address, to it.

import {citationText} from './i18n.ts';
import {installSubjectLinkTool, openSubjectLinkTool} from './subject-link-tool.ts';
import {openCiteTool} from './cite-tool.ts';
import type {ToolsEditor} from './editor-ops.ts';
import {svg, type SvgName} from '../../svg.ts';

type ToolbarButton = {name: string, tooltip: string, el: HTMLButtonElement, popup?: {body: HTMLElement, className: string}};

function toolbarButton(name: string, tooltip: string, icon: string): ToolbarButton {
  const el = document.createElement('button');
  el.type = 'button';
  el.className = `toastui-editor-toolbar-icons forkana-toolbar-icon ${name}`;
  el.setAttribute('aria-label', tooltip);
  el.innerHTML = icon;
  return {name, tooltip, el};
}

type MoreMenu = ToolbarButton & {items: Record<'codeblock' | 'image' | 'table', HTMLButtonElement>};

// The 40px button with the reply icon and a dropdown triangle (Button: Icon 24px
// I7368:265009;1597:117 with "Action: Dropdown"), and the menu Toast UI shows under it.
function moreMenu(): MoreMenu {
  const button = toolbarButton('forkana-more', citationText('tool_more'), `${svg('octicon-reply', 16)}${svg('octicon-triangle-down', 16)}`);
  button.el.setAttribute('aria-haspopup', 'menu');
  const body = document.createElement('div');
  body.className = 'forkana-more-menu';
  body.setAttribute('role', 'menu');
  const item = (key: string, icon: SvgName) => {
    const el = document.createElement('button');
    el.type = 'button';
    el.className = 'forkana-more-item';
    el.setAttribute('role', 'menuitem');
    el.innerHTML = `${svg(icon, 16)}<span></span>`;
    el.querySelector('span').textContent = citationText(key);
    body.append(el);
    return el;
  };
  const items = {
    codeblock: item('tool_more_code_block', 'octicon-file-code'),
    image: item('tool_more_image', 'octicon-image'),
    table: item('tool_more_table', 'octicon-table'),
  };
  return {...button, popup: {body, className: 'forkana-more-popup'}, items};
}

// articleToolbarItems returns the article editor's toolbar (see the top of this file): the
// first three groups of the default toolbar, its link button being the subject link, and the
// cite button and the menu as the fourth group, in place of the default's code block, image
// and table, which the menu holds.
export function articleToolbarItems(defaultItems: string[][]): {items: Array<Array<string | ToolbarButton>>, link: ToolbarButton, cite: ToolbarButton, more: MoreMenu} {
  const link = toolbarButton('forkana-subject-link', citationText('tool_link'), svg('octicon-link', 16));
  const shortcut = /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘⇧K' : 'Ctrl+Shift+K';
  const cite = toolbarButton('forkana-cite', `${citationText('tool_cite')} (${shortcut})`, svg('octicon-cross-reference', 16));
  const more = moreMenu();
  const items: Array<Array<string | ToolbarButton>> = defaultItems.slice(0, 3).map((group) => group.map((item) => (item === 'link' ? link : item)));
  items.push([cite, more]);
  return {items, link, cite, more};
}

type ArticleToolsEditor = ToolsEditor & {
  exec(name: string, payload?: Record<string, unknown>): void,
  eventEmitter: {emit(event: string, ...args: any[]): void},
};

export function installArticleTools(editor: ArticleToolsEditor, container: HTMLElement, buttons: {link: ToolbarButton, cite: ToolbarButton, more: MoreMenu}): void {
  // a toolbar click must not take the caret out of the document
  for (const b of [buttons.link, buttons.cite, buttons.more]) b.el.addEventListener('mousedown', (e) => e.preventDefault());
  buttons.link.el.addEventListener('click', () => openSubjectLinkTool(editor));
  buttons.cite.el.addEventListener('click', () => openCiteTool(editor));
  container.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.code === 'KeyK') {
      e.preventDefault();
      e.stopPropagation();
      openCiteTool(editor);
    }
  }, true);
  installSubjectLinkTool(editor, container);

  // The menu's items: Toast UI's own commands. The image is uploaded through the editor's
  // image hook, as a pasted or dropped one is.
  const {items} = buttons.more;
  const closeMenu = () => editor.eventEmitter.emit('closePopup');
  for (const el of Object.values(items)) el.addEventListener('mousedown', (e) => e.preventDefault());
  items.codeblock.addEventListener('click', () => {
    closeMenu();
    editor.exec('codeBlock');
  });
  items.table.addEventListener('click', () => {
    closeMenu();
    editor.exec('addTable', {rowCount: 3, columnCount: 2, data: []});
  });
  const fileInput = document.createElement('input');
  fileInput.type = 'file';
  fileInput.accept = 'image/*';
  fileInput.hidden = true;
  container.append(fileInput);
  items.image.addEventListener('click', () => {
    closeMenu();
    fileInput.value = '';
    fileInput.click();
  });
  fileInput.addEventListener('change', () => {
    const file = fileInput.files?.[0];
    if (!file) return;
    editor.eventEmitter.emit('addImageBlobHook', file, (imageUrl: string, altText?: string) => {
      editor.exec('addImage', {imageUrl, altText: altText || file.name || 'image'});
    }, 'ui');
  });
}
