// The link-to-subject tool of the article editor (issue #399, Figma "Subject search"
// 6597:45382, author list 6597:45384, "Link popover – URL detected" 6597:45396).
//
// There are three ways to open it, all showing the same list where the writer is typing:
//   - typing "[[" (Wikipedia's syntax): the list filters as the writer keeps typing; Enter
//     inserts the highlighted subject as a link and the brackets disappear; Esc closes the
//     list and leaves the typed words; Tab on a subject lists its articles, to link to one
//     author's version (subject/Apollo 11/tom66);
//   - the toolbar's link button, which types the "[[" itself, or, when text is selected,
//     searches for it in a field and turns it into the link;
// Results are subjects, with how many articles they hold. "Link to … as a new subject" links
// to a subject nobody created yet (a red link). A web address typed or pasted there is an
// outside source: the tool offers to cite it instead, which is the default, or to insert it
// as a plain link.

import {citationText} from './i18n.ts';
import {caretRect, currentView, focusEditor, insertLink, textBefore, textBeforeCaretInBlock, type ToolsEditor} from './editor-ops.ts';
import {closePopover, isPhoneLayout, openPopover, type Popover} from './popover.ts';
import {nextFootnoteLabel, referenceNumberAt} from './footnote-markdown.ts';
import {parseHttpURL} from './citation-format.ts';
import {openCiteTool} from './cite-tool.ts';
import {GET} from '../../modules/fetch.ts';
import {svg} from '../../svg.ts';
import {createElementFromHTML} from '../../utils/dom.ts';
import {html, htmlRaw} from '../../utils/html.ts';

type SubjectResult = {name: string, articles: number | null};
type Row =
  | {kind: 'subject', subject: SubjectResult} |
  {kind: 'new', name: string} |
  {kind: 'author', subject: string, owner: string};

type Session = {
  editor: ToolsEditor,
  // "brackets": the query is the text typed after "[[" at `start`; "field": the query is in
  // the popover's search field and the link replaces [from, to)
  mode: 'brackets' | 'field',
  start: number,
  from: number,
  to: number,
  label: string, // the selected text a "field" session links, "" otherwise
  query: string,
  rows: Row[],
  active: number,
  authorsOf: string | null, // the subject whose articles are listed
  popover: Popover | null,
  searchSeq: number,
};

let session: Session | null = null;

const BRACKETS_RE = /\[\[([^[\]\n]{0,80})$/;

function endSession(): void {
  const s = session;
  session = null;
  if (s?.popover) s.popover.close();
}

// subjectLinkURL is the link destination of a subject (and author): the subject name as
// written, spaces included (the article format of issue #399), with only the characters that
// would break the link or the path escaped.
export function subjectLinkURL(subject: string, owner?: string): string {
  let name = subject.replaceAll('%', '%25').replaceAll('/', '%2F').replace(/[\n\\]/g, encodeURIComponent);
  let depth = 0;
  let balanced = true;
  for (const c of name) {
    if (c === '(') depth++;
    if (c === ')' && --depth < 0) balanced = false;
  }
  if (!balanced || depth !== 0) name = name.replaceAll('(', '%28').replaceAll(')', '%29');
  return owner ? `subject/${name}/${encodeURIComponent(owner)}` : `subject/${name}`;
}

// internalSubjectOfURL returns the subject (and author) a URL of this instance points to.
export function internalSubjectOfURL(raw: string): {subject: string, owner: string} | null {
  const u = parseHttpURL(raw);
  if (!u || u.origin !== window.location.origin) return null;
  const prefix = `${window.config.appSubUrl}/subject/`;
  if (!u.pathname.startsWith(prefix)) return null;
  const [subject, owner] = u.pathname.slice(prefix.length).split('/').map((s) => {
    try {
      return decodeURIComponent(s);
    } catch {
      return s;
    }
  });
  return subject ? {subject, owner: owner ?? ''} : null;
}

function range(s: Session): {from: number, to: number} {
  if (s.mode === 'field') return {from: s.from, to: s.to};
  return {from: s.start, to: currentView(s.editor).state.selection.from};
}

function chooseRow(s: Session, row: Row): void {
  const {from, to} = range(s);
  let label: string;
  let url: string;
  if (row.kind === 'author') {
    label = s.label || row.subject;
    url = subjectLinkURL(row.subject, row.owner);
  } else {
    const name = row.kind === 'subject' ? row.subject.name : row.name;
    label = s.label || name;
    url = subjectLinkURL(name);
  }
  endSession();
  insertLink(s.editor, from, to, label, url);
}

function articlesText(n: number | null): string {
  if (n === null) return '';
  if (n === 0) return citationText('subject_articles_none');
  return n === 1 ? citationText('subject_articles_one') : citationText('subject_articles', n);
}

function renderRows(s: Session): void {
  const list = s.popover?.el.querySelector('.citation-subject-results');
  if (!list) return;
  list.textContent = '';
  if (!s.rows.length) {
    list.append(createElementFromHTML(html`<div class="citation-subject-empty">${s.authorsOf ? citationText('no_articles') : citationText('search_subjects')}</div>`));
    return;
  }
  for (const [i, row] of s.rows.entries()) {
    let el: HTMLElement;
    if (row.kind === 'subject') {
      el = createElementFromHTML(html`
        <div class="citation-subject-row" role="option">
          ${htmlRaw(svg('octicon-goal', 16, 'citation-subject-icon'))}
          <span class="citation-subject-text"><span class="citation-subject-name">${row.subject.name}</span><span class="citation-subject-meta">${articlesText(row.subject.articles)}</span></span>
          ${htmlRaw(row.subject.articles ? svg('octicon-chevron-right', 12, 'citation-subject-chevron') : '')}
        </div>
      `);
    } else if (row.kind === 'author') {
      el = createElementFromHTML(html`
        <div class="citation-subject-row" role="option">
          ${htmlRaw(svg('octicon-goal', 16, 'citation-subject-icon'))}
          <span class="citation-subject-text"><span class="citation-subject-name">${row.subject}</span><span class="citation-subject-meta">${citationText('by_author', row.owner)}</span></span>
        </div>
      `);
    } else {
      list.append(createElementFromHTML(html`<div class="citation-subject-divider" role="separator"></div>`));
      el = createElementFromHTML(html`
        <div class="citation-subject-row citation-subject-new" role="option">
          <span class="citation-subject-plus">+</span>
          <span class="citation-subject-text"><span class="citation-subject-name">${citationText('new_subject', row.name)}</span></span>
        </div>
      `);
    }
    el.classList.toggle('is-active', i === s.active);
    el.setAttribute('aria-selected', String(i === s.active));
    el.addEventListener('pointerdown', (e) => e.preventDefault()); // keep the editor's caret
    el.addEventListener('click', () => chooseRow(s, row));
    list.append(el);
  }
}

async function showAuthors(s: Session, subject: string): Promise<void> {
  s.authorsOf = subject;
  s.rows = [];
  s.active = 0;
  renderRows(s);
  const seq = ++s.searchSeq;
  try {
    const resp = await GET(`${window.config.appSubUrl}/explore/subjects/articles?subject=${encodeURIComponent(subject)}`);
    const data = await resp.json();
    if (session !== s || seq !== s.searchSeq) return;
    s.rows = (data.articles ?? []).map((a: {owner: string}) => ({kind: 'author', subject: data.subject ?? subject, owner: a.owner}));
  } catch {
    s.rows = [];
  }
  s.popover?.el.classList.add('is-authors');
  renderRows(s);
}

function showSubjects(s: Session): void {
  s.authorsOf = null;
  s.popover?.el.classList.remove('is-authors');
  search(s);
}

// showURLDetected turns the popover into "Add a citation": the query is a web address.
function showURLDetected(s: Session, url: string): void {
  const {from, to} = range(s);
  const editor = s.editor;
  const label = s.label;
  const markdown = editor.getMarkdown();
  const view = currentView(editor);
  const number = referenceNumberAt(textBefore(view, from), nextFootnoteLabel(markdown));
  endSession();
  const el = createElementFromHTML(html`
    <div class="citation-link-url" role="dialog" aria-label="${citationText('add_a_citation')}">
      <div class="citation-popover-header">
        <span class="citation-popover-title">${citationText('add_a_citation')}</span>
        <button type="button" class="citation-popover-close" aria-label="${citationText('close')}">${htmlRaw(svg('octicon-x', 16))}</button>
      </div>
      <label class="citation-text-input">
        <span class="citation-text-input-label">${citationText('link')}</span>
        <span class="citation-text-input-box">${htmlRaw(svg('octicon-link', 16))}<input type="url" value="${url}"></span>
      </label>
      <div class="citation-banner"><span><strong>${citationText('outside_source')}</strong> ${citationText('outside_source_desc', number)}</span></div>
      <div class="citation-popover-footer">
        <button type="button" class="citation-button citation-button-invisible" data-action="plain">${citationText('insert_plain_link')}</button>
        <button type="button" class="citation-button citation-button-primary" data-action="cite">${citationText('add_citation')}</button>
      </div>
    </div>
  `);
  const popover = openPopover(el, caretRect(view));
  const input = el.querySelector('input');
  el.querySelector('.citation-popover-close').addEventListener('click', () => popover.close());
  el.querySelector('[data-action="plain"]').addEventListener('click', () => {
    const target = input.value.trim() || url;
    popover.close();
    insertLink(editor, from, to, label || target, target);
  });
  const cite = () => {
    const target = input.value.trim() || url;
    popover.close();
    openCiteTool(editor, {from, to, query: target, manual: true});
  };
  el.querySelector('[data-action="cite"]').addEventListener('click', cite);
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      cite();
    }
  });
  el.querySelector<HTMLButtonElement>('[data-action="cite"]').focus();
}

async function search(s: Session): Promise<void> {
  const query = s.query.trim();
  const seq = ++s.searchSeq;
  if (parseHttpURL(query)) {
    const internal = internalSubjectOfURL(query);
    if (!internal) {
      showURLDetected(s, query);
      return;
    }
    s.rows = internal.owner ?
      [{kind: 'author', subject: internal.subject, owner: internal.owner}] :
      [{kind: 'subject', subject: {name: internal.subject, articles: null}}];
    s.active = 0;
    renderRows(s);
    return;
  }
  if (!query) {
    s.rows = [];
    renderRows(s);
    return;
  }
  let results: SubjectResult[] = [];
  try {
    const resp = await GET(`${window.config.appSubUrl}/explore/subjects/suggestions?detail=1&q=${encodeURIComponent(query)}`);
    const data = await resp.json();
    results = data.details ?? (data.subjects ?? []).map((name: string): SubjectResult => ({name, articles: null}));
  } catch {
    results = [];
  }
  if (session !== s || seq !== s.searchSeq || s.authorsOf) return;
  s.rows = results.map((subject) => ({kind: 'subject', subject}));
  if (!results.some((r) => r.name.toLowerCase() === query.toLowerCase())) s.rows.push({kind: 'new', name: query});
  s.active = 0;
  renderRows(s);
}

let searchTimer: ReturnType<typeof setTimeout>;

function scheduleSearch(s: Session): void {
  clearTimeout(searchTimer);
  // an address turns the list into "Add a citation": wait until it looks complete
  searchTimer = setTimeout(() => search(s), parseHttpURL(s.query.trim()) ? 500 : 150);
}

function openSession(s: Session): void {
  const el = createElementFromHTML(html`
    <div class="citation-subject-search" role="listbox" aria-label="${citationText('tool_link')}">
      ${htmlRaw(s.mode === 'field' || isPhoneLayout() ? html`
        <div class="citation-popover-header citation-subject-header">
          <span class="citation-popover-title">${citationText('tool_link')}</span>
          <button type="button" class="citation-popover-close" aria-label="${citationText('close')}">${htmlRaw(svg('octicon-x', 16))}</button>
        </div>
        <label class="citation-text-input citation-subject-field">
          <span class="citation-text-input-box">${htmlRaw(svg('octicon-search', 16))}<input type="search" placeholder="${citationText('search_subjects')}" value="${s.query}"></span>
        </label>
      ` : '')}
      <div class="citation-subject-results"></div>
    </div>
  `);
  s.popover = openPopover(el, caretRect(currentView(s.editor)), () => {
    if (session === s) session = null;
  });
  el.querySelector('.citation-popover-close')?.addEventListener('click', () => endSession());
  const field = el.querySelector<HTMLInputElement>('input[type="search"]');
  if (field) {
    if (s.mode === 'brackets') {
      // on a phone, the writer types in the sheet's field: "[[" stays in the text until a
      // subject is chosen
      s.mode = 'field';
      s.from = s.start;
      s.to = currentView(s.editor).state.selection.from;
    }
    field.addEventListener('input', () => {
      s.query = field.value;
      s.authorsOf = null;
      scheduleSearch(s);
    });
    field.addEventListener('keydown', (e) => handleKey(s, e));
    setTimeout(() => field.focus(), 0);
  }
  renderRows(s);
  if (s.query) scheduleSearch(s);
}

// handleKey handles the list's keys; it returns whether the key was used.
function handleKey(s: Session, e: KeyboardEvent): boolean {
  if (!s.popover) return false;
  const count = s.rows.length;
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowUp':
      if (!count) return false;
      s.active = (s.active + (e.key === 'ArrowDown' ? 1 : count - 1)) % count;
      renderRows(s);
      break;
    case 'Enter':
      if (!count) return false;
      chooseRow(s, s.rows[s.active]);
      break;
    case 'Tab': {
      const row = s.rows[s.active];
      if (e.shiftKey && s.authorsOf) {
        showSubjects(s);
      } else if (row?.kind === 'subject' && row.subject.articles !== 0) {
        showAuthors(s, row.subject.name);
      } else {
        return false;
      }
      break;
    }
    case 'ArrowLeft':
      if (!s.authorsOf) return false;
      showSubjects(s);
      break;
    case 'Escape':
      // close the list and leave the text as plain words: drop the "[["
      if (s.mode === 'brackets') {
        const view = currentView(s.editor);
        const text = view.state.doc.textBetween(s.start, Math.min(s.start + 2, view.state.doc.content.size));
        endSession();
        if (text === '[[') view.dispatch(view.state.tr.delete(s.start, s.start + 2));
      } else {
        endSession();
        focusEditor(s.editor);
      }
      break;
    default:
      return false;
  }
  e.preventDefault();
  e.stopPropagation();
  return true;
}

// onEditorInput follows what the writer types: "[[" opens the list, the text after it is
// the query.
function onEditorInput(editor: ToolsEditor): void {
  if (session && session.mode === 'field') return;
  const view = currentView(editor);
  const m = BRACKETS_RE.exec(textBeforeCaretInBlock(view));
  if (!m) {
    if (session) endSession();
    return;
  }
  const start = view.state.selection.from - m[0].length;
  if (!session || session.editor !== editor || session.start !== start) {
    endSession();
    session = {editor, mode: 'brackets', start, from: start, to: start, label: '', query: m[1], rows: [], active: 0, authorsOf: null, popover: null, searchSeq: 0};
    openSession(session);
    return;
  }
  if (session.query !== m[1]) {
    session.query = m[1];
    session.authorsOf = null;
    session.popover?.el.classList.remove('is-authors');
    scheduleSearch(session);
  }
  session.popover?.reposition(caretRect(view));
}

// openSubjectLinkTool is the toolbar button: it searches for the selected text, or starts a
// "[[" at the caret.
export function openSubjectLinkTool(editor: ToolsEditor): void {
  closePopover();
  const view = currentView(editor);
  const {from, to} = view.state.selection;
  if (to > from) {
    const label = view.state.doc.textBetween(from, to, ' ');
    session = {editor, mode: 'field', start: from, from, to, label, query: label, rows: [], active: 0, authorsOf: null, popover: null, searchSeq: 0};
    openSession(session);
    return;
  }
  focusEditor(editor);
  view.dispatch(view.state.tr.insertText('[[', from, to));
  onEditorInput(editor);
}

// installSubjectLinkTool follows the editor's typing and keys for the "[[" list.
export function installSubjectLinkTool(editor: ToolsEditor, container: HTMLElement): void {
  container.addEventListener('input', () => {
    // after ProseMirror has applied the input
    queueMicrotask(() => onEditorInput(editor));
  });
  container.addEventListener('keydown', (e) => {
    if (session?.editor === editor && session.mode === 'brackets') handleKey(session, e);
  }, true);
  editor.on('changeMode', () => endSession());
}
