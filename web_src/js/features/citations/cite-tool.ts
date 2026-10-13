// The Cite tool of the article editor (issue #399): the "Cite a source" popover (Figma
// 6597:45615) and the "Add citation" dialog (6597:44669, types: Website 6597:44491, News
// 6597:44670, Journal 6597:44848, Book 6597:45026, Other 6597:45204).
//
// One field: the writer pastes a URL, a DOI or an ISBN, Forkana guesses the type (a DOI means
// Journal, an ISBN Book, a known news site News, anything else Website) and fills in the
// details it finds (/-/citation/lookup); the dialog shows them, with a live preview of the
// reference, for the writer to check and complete. Citing a source already in the article
// reuses its number instead of adding a second reference. The citation is inserted as a
// Markdown footnote: "[^n]" at the caret, "[^n]: …" at the end of the article.

import {citationText} from './i18n.ts';
import {
  detectSourceType,
  formatCitation,
  missingRequiredFields,
  normalizeDOI,
  normalizeISBN,
  parseHttpURL,
  requiredFields,
  sourceTypes,
  type CitationFields,
  type SourceType,
} from './citation-format.ts';
import {footnoteDefinitions, footnoteSources, nextFootnoteLabel, plainCitationSummary, referenceNumberAt, type FootnoteSource} from './footnote-markdown.ts';
import {caretRect, currentView, focusEditor, insertFootnote, isInCode, textBefore, type ToolsEditor} from './editor-ops.ts';
import {showErrorToast} from '../../modules/toast.ts';
import {closePopover, openPopover} from './popover.ts';
import {POST} from '../../modules/fetch.ts';
import {svg, type SvgName} from '../../svg.ts';
import {createElementFromHTML} from '../../utils/dom.ts';
import {html, htmlEscape, htmlRaw} from '../../utils/html.ts';

export type CiteOptions = {
  from: number,
  to: number,
  query?: string, // a URL, DOI or ISBN to look up
  manual?: boolean, // open the dialog directly
};

const typeIcons: Record<SourceType, SvgName> = {
  website: 'octicon-globe',
  news: 'octicon-project-roadmap',
  journal: 'octicon-log',
  book: 'octicon-book',
  other: 'octicon-pencil',
};

type FieldSpec = {
  key: keyof CitationFields,
  label: string, // locale key
  half?: boolean,
  placeholder?: string, // locale key, or a literal starting with "="
  type?: string,
};

// The fields of each type, in the order of the design.
const typeFields: Record<SourceType, FieldSpec[]> = {
  website: [
    {key: 'url', label: 'field_url', type: 'url', placeholder: '=https://'},
    {key: 'title', label: 'field_title'},
    {key: 'site', label: 'field_website', half: true},
    {key: 'published', label: 'field_published', half: true, placeholder: 'placeholder_unknown'},
  ],
  news: [
    {key: 'url', label: 'field_url', type: 'url', placeholder: '=https://'},
    {key: 'title', label: 'field_headline'},
    {key: 'site', label: 'field_publication', half: true},
    {key: 'published', label: 'field_published', half: true, placeholder: 'placeholder_unknown'},
    {key: 'authors', label: 'field_authors', placeholder: 'placeholder_authors'},
  ],
  journal: [
    {key: 'doi', label: 'field_doi_or_url'},
    {key: 'title', label: 'field_article_title'},
    {key: 'authors', label: 'field_authors', placeholder: 'placeholder_authors'},
    {key: 'journal', label: 'field_journal', half: true},
    {key: 'published', label: 'field_year', half: true},
    {key: 'volumeIssue', label: 'field_volume_issue', half: true, placeholder: '=167 / 3918'},
    {key: 'pages', label: 'field_pages', half: true},
  ],
  book: [
    {key: 'isbn', label: 'field_isbn'},
    {key: 'title', label: 'field_title'},
    {key: 'authors', label: 'field_book_authors', half: true, placeholder: 'placeholder_authors'},
    {key: 'published', label: 'field_year', half: true},
    {key: 'publisher', label: 'field_publisher', half: true},
    {key: 'edition', label: 'field_edition', half: true},
    {key: 'link', label: 'field_link_optional', type: 'url', placeholder: '=archive.org, openlibrary.org, goodreads.com'},
  ],
  other: [
    {key: 'citation', label: 'field_citation'},
    {key: 'link', label: 'field_link_optional', type: 'url', placeholder: '=https://'},
  ],
};

// What /-/citation/lookup returns (services/citation.Source).
type LookupSource = {
  type: SourceType,
  url?: string,
  title?: string,
  site?: string,
  authors?: string[],
  published?: string,
  journal?: string,
  volume?: string,
  issue?: string,
  pages?: string,
  doi?: string,
  isbn?: string,
  publisher?: string,
};

export function fieldsFromLookup(src: LookupSource): CitationFields {
  const f: CitationFields = {
    url: src.url,
    title: src.title,
    site: src.site,
    authors: src.authors?.join('; '),
    published: src.published,
    journal: src.journal,
    volumeIssue: [src.volume, src.issue].filter(Boolean).join(' / ') || undefined,
    pages: src.pages,
    doi: src.doi,
    isbn: src.isbn,
    publisher: src.publisher,
  };
  if (src.type === 'book' && src.url) f.link = src.url;
  for (const k of Object.keys(f) as Array<keyof CitationFields>) {
    if (!f[k]) delete f[k];
  }
  return f;
}

// fieldsFromQuery puts what the writer pasted in the field of its type.
function fieldsFromQuery(type: SourceType, query: string): CitationFields {
  if (type === 'journal') return {doi: normalizeDOI(query) || query};
  if (type === 'book') return {isbn: query.trim().replace(/^isbn(?:-1[03])?:?\s*/i, '')};
  if (type === 'other') return parseHttpURL(query) ? {link: query} : {citation: query};
  return {url: query};
}

// citationPreviewHTML renders the Markdown of a citation the way the References list shows it
// (only what formatCitation writes: links, emphasis, autolinks and escapes).
export function citationPreviewHTML(markdown: string): string {
  const escapes: string[] = [];
  let s = markdown.replace(/\\(.)/g, (_, c: string) => {
    escapes.push(c);
    return `\u0000${escapes.length - 1}\u0000`;
  });
  s = htmlEscape(s);
  const restore = (t: string) => t.replace(/\0(\d+)\0/g, (_, i: string) => htmlEscape(escapes[Number(i)]));
  // the text is HTML-escaped already: its pieces go in raw
  const link = (url: string, text: string) => html`<a class="external-link" href="${htmlRaw(/^https?:/i.test(url) ? url : '#')}" target="_blank" rel="noopener">${htmlRaw(text)}</a>`;
  // links first: their "(<…>)" destination would look like an autolink
  s = s.replace(/\[([^\]]*)\]\((?:&lt;((?:(?!&gt;).)*)&gt;|([^)\s]*))\)/g, (_, text: string, a: string, b: string) => link(a ?? b ?? '', text));
  s = s.replace(/&lt;(https?:\/\/(?:[^\s&]|&amp;)+)&gt;/g, (_, url: string) => link(url, url));
  s = s.replace(/\*([^*]+)\*/g, (_, text: string) => html`<em>${htmlRaw(text)}</em>`);
  return restore(s);
}

let lookupSeq = 0;

async function lookup(query: string): Promise<LookupSource | null> {
  try {
    const resp = await POST(`${window.config.appSubUrl}/-/citation/lookup`, {data: new URLSearchParams({q: query})});
    if (!resp.ok) return null;
    return await resp.json();
  } catch {
    return null;
  }
}

function insertReference(editor: ToolsEditor, opts: CiteOptions, source: FootnoteSource): void {
  closePopover();
  insertFootnote(editor, opts.from, opts.to, source.label);
}

function reuseRow(source: FootnoteSource, separator: string): HTMLElement {
  const {title, publisher} = plainCitationSummary(source.text);
  return createElementFromHTML(html`
    <button type="button" class="citation-reuse-row">
      <span class="citation-reuse-number">[${source.number}]</span>
      <span class="citation-reuse-text">${title}${htmlRaw(publisher ? html`<span class="citation-reuse-publisher">${separator}${publisher}</span>` : '')}</span>
    </button>
  `);
}

// openCiteDialog opens "Add citation".
function openCiteDialog(editor: ToolsEditor, opts: CiteOptions): void {
  closePopover();
  const markdown = editor.getMarkdown();
  const sources = footnoteSources(markdown);
  const label = nextFootnoteLabel(markdown);
  const number = referenceNumberAt(textBefore(currentView(editor), opts.from), label, new Set(footnoteDefinitions(markdown).keys()));
  const query = opts.query?.trim() ?? '';
  let type: SourceType = (query && detectSourceType(query)) || 'website';
  // once the writer picked a type, a lookup fills in its fields but does not change it
  let typeChosen = false;
  const values: Record<SourceType, CitationFields> = {website: {}, news: {}, journal: {}, book: {}, other: {}};
  if (query) values[type] = fieldsFromQuery(type, query);
  // the fields a lookup filled in, as "type:field": a later lookup may replace them, the
  // writer's own values it may not
  const filled = new Set<string>();
  let location = '';

  const dialog = createElementFromHTML(html`
    <div class="citation-dialog-backdrop">
      <div class="citation-dialog" role="dialog" aria-modal="true" aria-labelledby="citation-dialog-title">
        <div class="citation-dialog-header">
          <span id="citation-dialog-title">${citationText('add_citation')}</span>
          <button type="button" class="citation-popover-close" aria-label="${citationText('close')}">${htmlRaw(svg('octicon-x', 16))}</button>
        </div>
        ${htmlRaw(sources.length ? html`
          <div class="citation-dialog-section citation-dialog-reuse">
            <div class="citation-dialog-section-title">${citationText('dialog_reuse')}</div>
            <input type="search" class="citation-field-input" placeholder="${citationText('dialog_search_references')}">
            <div class="citation-reuse-list"></div>
          </div>
        ` : '')}
        <div class="citation-dialog-section">
          <div class="citation-dialog-section-title">${citationText('dialog_new_source')}</div>
          <div class="citation-type-picker" role="radiogroup"></div>
        </div>
        <div class="citation-lookup-status" hidden></div>
        <div class="citation-dialog-fields"></div>
        <div class="citation-dialog-preview">
          <div class="citation-dialog-preview-title">${citationText('preview_title')}</div>
          <div class="citation-dialog-preview-text"></div>
        </div>
        <div class="citation-dialog-error" hidden>${citationText('required_missing')}</div>
        <div class="citation-dialog-footer">
          <label class="citation-field citation-field-location">
            <span class="citation-field-label">${citationText('field_location')}</span>
            <input class="citation-field-input" placeholder="${citationText('placeholder_location')}">
          </label>
          <button type="button" class="citation-button citation-button-large" data-action="cancel">${citationText('cancel')}</button>
          <button type="button" class="citation-button citation-button-large citation-button-primary" data-action="insert">${citationText('insert_reference', number)}</button>
        </div>
      </div>
    </div>
  `);

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      close();
    }
  }
  function close() {
    dialog.remove();
    document.removeEventListener('keydown', onKey, true);
    focusEditor(editor);
  }
  document.addEventListener('keydown', onKey, true);
  dialog.addEventListener('pointerdown', (e) => {
    if (e.target === dialog) close();
  });
  dialog.querySelector('.citation-popover-close').addEventListener('click', close);
  dialog.querySelector('[data-action="cancel"]').addEventListener('click', close);

  // reuse a source
  const reuseList = dialog.querySelector('.citation-reuse-list');
  const reuseSearch = dialog.querySelector<HTMLInputElement>('.citation-dialog-reuse input');
  function renderReuse() {
    if (!reuseList) return;
    reuseList.textContent = '';
    const q = reuseSearch.value.trim().toLowerCase();
    for (const source of sources.filter((s) => !q || s.text.toLowerCase().includes(q)).slice(0, 4)) {
      const row = reuseRow(source, ', ');
      row.addEventListener('click', () => {
        close();
        insertFootnote(editor, opts.from, opts.to, source.label);
      });
      reuseList.append(row);
    }
  }
  reuseSearch?.addEventListener('input', renderReuse);
  renderReuse();

  const preview = dialog.querySelector('.citation-dialog-preview-text');
  const fieldsEl = dialog.querySelector('.citation-dialog-fields');
  const status = dialog.querySelector<HTMLElement>('.citation-lookup-status');
  const error = dialog.querySelector<HTMLElement>('.citation-dialog-error');
  const currentFields = (): CitationFields => ({...values[type], location});
  function updatePreview() {
    const f = currentFields();
    const filledEnough = missingRequiredFields(type, f).length === 0;
    preview.innerHTML = filledEnough || Object.values(values[type]).some(Boolean) ? citationPreviewHTML(formatCitation(type, f)) : '';
  }

  async function runLookup(q: string) {
    const seq = ++lookupSeq;
    status.hidden = false;
    status.className = 'citation-lookup-status is-loading';
    status.textContent = `✦ ${citationText('reading_page')}`;
    const src = await lookup(q);
    if (seq !== lookupSeq || !dialog.isConnected) return;
    if (!src) {
      status.className = 'citation-lookup-status is-failed';
      status.textContent = citationText('lookup_failed');
      return;
    }
    status.hidden = true;
    const found = fieldsFromLookup(src);
    const t = !typeChosen && sourceTypes.includes(src.type) ? src.type : type;
    // what the previous lookup filled in belongs to the previous source
    for (const key of Array.from(filled)) {
      const [ft, fk] = key.split(':') as [SourceType, keyof CitationFields];
      if (ft === t && !(fk in found)) {
        delete values[ft][fk];
        filled.delete(key);
      }
    }
    for (const [k, v] of Object.entries(found) as Array<[keyof CitationFields, string]>) {
      if (!values[t][k]?.trim() || filled.has(`${t}:${k}`)) {
        values[t][k] = v;
        filled.add(`${t}:${k}`);
      }
    }
    if (t !== type) {
      for (const [k, v] of Object.entries(values[type]) as Array<[keyof CitationFields, string]>) {
        if (v && !values[t][k]) values[t][k] = v;
      }
      type = t;
      renderPicker();
    }
    renderFields();
  }

  function renderFields() {
    // a lookup may render the fields again while the writer types in one: keep the caret
    const active = document.activeElement instanceof HTMLInputElement && fieldsEl.contains(document.activeElement) ? document.activeElement : null;
    const activeKey = active?.closest<HTMLElement>('.citation-field')?.getAttribute('data-key');
    const caret = active?.selectionStart ?? null;
    fieldsEl.textContent = '';
    const required = new Set(requiredFields[type]);
    for (const spec of typeFields[type]) {
      const placeholder = spec.placeholder?.startsWith('=') ? spec.placeholder.slice(1) : spec.placeholder ? citationText(spec.placeholder) : '';
      const filledLabel = filled.has(`${type}:${spec.key}`) ? citationText(spec.key === 'title' || spec.key === 'url' ? 'filled_from_page' : 'filled_auto') : '';
      const field = createElementFromHTML(html`
        <label class="citation-field${spec.half ? ' citation-field-half' : ''}" data-key="${spec.key}">
          <span class="citation-field-label-row">
            <span class="citation-field-label">${citationText(spec.label)}${required.has(spec.key) ? ' *' : ''}</span>
            ${htmlRaw(filledLabel ? html`<span class="citation-field-filled">${filledLabel}</span>` : '')}
          </span>
          <input class="citation-field-input" type="${spec.type ?? 'text'}" placeholder="${placeholder}" value="${values[type][spec.key] ?? ''}">
        </label>
      `);
      const input = field.querySelector('input');
      input.addEventListener('input', () => {
        values[type][spec.key] = input.value;
        if (filled.delete(`${type}:${spec.key}`)) field.querySelector('.citation-field-filled')?.remove();
        error.hidden = true;
        updatePreview();
      });
      // a DOI or an ISBN pasted in its field fills in the rest
      if (spec.key === 'doi' || spec.key === 'isbn' || (spec.key === 'url' && type !== 'other')) {
        input.addEventListener('change', () => {
          const v = input.value.trim();
          if (v && (spec.key !== 'url' || parseHttpURL(v)) && (spec.key !== 'doi' || normalizeDOI(v) || parseHttpURL(v)) && (spec.key !== 'isbn' || normalizeISBN(v))) {
            runLookup(v);
          }
        });
      }
      fieldsEl.append(field);
    }
    updatePreview();
    const again = activeKey ? fieldsEl.querySelector<HTMLInputElement>(`.citation-field[data-key="${activeKey}"] input`) : null;
    if (again) {
      again.focus();
      try {
        if (caret !== null) again.setSelectionRange(caret, caret);
      } catch {
        // an input type="url" has no selection
      }
    }
  }

  const picker = dialog.querySelector('.citation-type-picker');
  function renderPicker() {
    picker.textContent = '';
    for (const t of sourceTypes) {
      const tab = createElementFromHTML<HTMLButtonElement>(html`
        <button type="button" class="citation-type-tab" role="radio" aria-checked="${t === type ? 'true' : 'false'}">
          ${htmlRaw(svg(typeIcons[t], 12))}<span>${citationText(`type_${t}`)}</span>
        </button>
      `);
      tab.addEventListener('click', () => {
        typeChosen = true;
        if (t === type) return;
        // carry what the writer already typed that the other type also has
        for (const [k, v] of Object.entries(values[type]) as Array<[keyof CitationFields, string]>) {
          if (v && typeFields[t].some((spec) => spec.key === k) && !values[t][k]) values[t][k] = v;
        }
        type = t;
        renderPicker();
        renderFields();
        // the tab was rendered again: give the focus to the new one
        picker.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
      });
      picker.append(tab);
    }
  }

  const locationInput = dialog.querySelector<HTMLInputElement>('.citation-field-location input');
  locationInput.addEventListener('input', () => {
    location = locationInput.value;
    updatePreview();
  });

  dialog.querySelector('[data-action="insert"]').addEventListener('click', () => {
    const f = currentFields();
    if (missingRequiredFields(type, f).length) {
      error.hidden = false;
      return;
    }
    close();
    insertFootnote(editor, opts.from, opts.to, label, formatCitation(type, f));
  });

  renderPicker();
  renderFields();
  document.body.append(dialog);
  if (query) runLookup(query);
  dialog.querySelector<HTMLInputElement>('.citation-dialog-fields input')?.focus();
}

// openCitePopover opens "Cite a source" at the caret.
function openCitePopover(editor: ToolsEditor, opts: CiteOptions): void {
  const sources = footnoteSources(editor.getMarkdown());
  const el = createElementFromHTML(html`
    <div class="citation-cite-popover" role="dialog" aria-label="${citationText('tool_cite')}">
      <div class="citation-popover-header">
        <span class="citation-popover-title">${citationText('tool_cite')}</span>
        <button type="button" class="citation-popover-close" aria-label="${citationText('close')}">${htmlRaw(svg('octicon-x', 16))}</button>
      </div>
      <label class="citation-text-input">
        <span class="citation-text-input-label">${citationText('cite_input_label')}</span>
        <span class="citation-text-input-box">${htmlRaw(svg('octicon-link', 16))}<input type="text" placeholder="https://"></span>
      </label>
      <div class="citation-cite-hint">${citationText('cite_fills_details')}</div>
      ${htmlRaw(sources.length ? html`
        <div class="citation-cite-reuse">
          <div class="citation-cite-reuse-title">${citationText('reuse_source')}</div>
          <div class="citation-reuse-list"></div>
        </div>
      ` : '')}
      <div class="citation-popover-footer">
        <button type="button" class="citation-button citation-button-invisible" data-action="manual">${htmlRaw(svg('octicon-pencil', 16))}${citationText('add_manually')}</button>
      </div>
    </div>
  `);
  const popover = openPopover(el, caretRect(currentView(editor)), () => focusEditor(editor));
  el.querySelector('.citation-popover-close').addEventListener('click', () => popover.close());
  const list = el.querySelector('.citation-reuse-list');
  for (const source of sources.slice(0, 5)) {
    const row = reuseRow(source, '  ');
    row.addEventListener('click', () => insertReference(editor, opts, source));
    list?.append(row);
  }
  const input = el.querySelector('input');
  const proceed = (manual: boolean) => {
    const q = input.value.trim();
    if (!manual && !detectSourceType(q)) {
      input.closest('.citation-text-input').classList.add('is-invalid');
      return;
    }
    popover.close();
    openCiteDialog(editor, {...opts, query: q || undefined});
  };
  input.addEventListener('input', () => input.closest('.citation-text-input').classList.remove('is-invalid'));
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      proceed(false);
    }
  });
  // a pasted URL, DOI or ISBN goes on at once
  input.addEventListener('paste', () => setTimeout(() => {
    if (detectSourceType(input.value.trim())) proceed(false);
  }, 0));
  el.querySelector('[data-action="manual"]').addEventListener('click', () => proceed(true));
  setTimeout(() => input.focus(), 0);
}

// openCiteTool is the toolbar's Cite button and its shortcut: the citation goes after the
// selection (or at the caret).
export function openCiteTool(editor: ToolsEditor, opts?: CiteOptions): void {
  if (!opts) {
    const {to} = currentView(editor).state.selection;
    opts = {from: to, to};
  }
  if (isInCode(editor, opts.from)) {
    // "[^1]" in code is text: the citation would never render
    showErrorToast(citationText('in_code'));
    return;
  }
  if (opts.manual) {
    openCiteDialog(editor, opts);
  } else {
    openCitePopover(editor, opts);
  }
}
