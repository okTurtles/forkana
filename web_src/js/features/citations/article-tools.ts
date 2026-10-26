// The link and cite tools of the article editor (issue #399): the toolbar's link button links
// to a Forkana subject (Figma: tooltip "Link to a Forkana subject", shortcut "[["), and a Cite
// button cites a source (tooltip "Cite a source", Ctrl ⇧ K). See the toolbar of the "New
// Article" frames (6597:45386).

import {citationText} from './i18n.ts';
import {installSubjectLinkTool, openSubjectLinkTool} from './subject-link-tool.ts';
import {openCiteTool} from './cite-tool.ts';
import type {ToolsEditor} from './editor-ops.ts';
import {svg} from '../../svg.ts';
import {html} from '../../utils/html.ts';

// The cite icon of the design ("Icon 4", 6597:45391): a pair of square brackets.
const citeIcon = html`<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" class="svg"><path fill="currentColor" d="M3 1.75C3 1.336 3.336 1 3.75 1h2.5a.75.75 0 0 1 0 1.5H4.5v11h1.75a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Zm10 0v12.5a.75.75 0 0 1-.75.75h-2.5a.75.75 0 0 1 0-1.5h1.75v-11H9.75a.75.75 0 0 1 0-1.5h2.5a.75.75 0 0 1 .75.75Z"/></svg>`;

type ToolbarButton = {name: string, tooltip: string, el: HTMLButtonElement};

function toolbarButton(name: string, tooltip: string, icon: string): ToolbarButton {
  const el = document.createElement('button');
  el.type = 'button';
  el.className = `toastui-editor-toolbar-icons forkana-toolbar-icon ${name}`;
  el.setAttribute('aria-label', tooltip);
  el.innerHTML = icon;
  return {name, tooltip, el};
}

// articleToolbarItems returns the article editor's toolbar: the default one, with the link
// button replaced by the subject link and cite buttons.
export function articleToolbarItems(defaultItems: string[][]): {items: Array<Array<string | ToolbarButton>>, link: ToolbarButton, cite: ToolbarButton} {
  const link = toolbarButton('forkana-subject-link', citationText('tool_link'), svg('octicon-link', 16));
  const shortcut = /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘⇧K' : 'Ctrl+Shift+K';
  const cite = toolbarButton('forkana-cite', `${citationText('tool_cite')} (${shortcut})`, citeIcon);
  const items = defaultItems.map((group) => group.flatMap((item): Array<string | ToolbarButton> => (item === 'link' ? [link, cite] : [item])));
  return {items, link, cite};
}

export function installArticleTools(editor: ToolsEditor, container: HTMLElement, buttons: {link: ToolbarButton, cite: ToolbarButton}): void {
  // a toolbar click must not take the caret out of the document
  for (const b of [buttons.link, buttons.cite]) b.el.addEventListener('mousedown', (e) => e.preventDefault());
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
}
