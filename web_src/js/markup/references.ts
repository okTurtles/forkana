// References of an article (issue #399): the rendered Markdown footnotes, numbered "[1]" in
// the text and listed under "References" (see modules/markup/common/footnote.go).
//
//   - hovering or focusing a "[n]" marker shows a card with the type of the source, the
//     citation, whether it is an archived copy, and "Visit source" / "Go to reference"
//     (Figma "Hover card", 6597:45562); on a phone, tapping it opens the same card in a
//     bottom sheet (6597:45407);
//   - in an article, the References heading counts the sources and their publishers and a
//     summary card shows where the evidence comes from, with chips filtering the list
//     (6597:45566, "Source summary" 6597:45570).
//
// Jumping between a marker and its reference, and highlighting the target, is plain anchor
// navigation styled with :target (citations.custom.css). The hover card is a tooltip of the
// marker: keyboard users reach the same citation and links in the References list.

import {citationText} from '../features/citations/i18n.ts';
import {classifyCitation, sourceTypes, type CitationInfo, type SourceType} from '../features/citations/citation-format.ts';
import {svg, type SvgName} from '../svg.ts';
import {createElementFromHTML} from '../utils/dom.ts';
import {html} from '../utils/html.ts';

type Reference = {
  number: number,
  item: HTMLLIElement,
  citation: HTMLElement, // a copy of the citation, without its back-links
  uses: number,
  info: CitationInfo,
};

const typeIcons: Record<SourceType, SvgName> = {
  website: 'octicon-globe',
  news: 'octicon-project-roadmap',
  journal: 'octicon-log',
  book: 'octicon-book',
  other: 'octicon-pencil',
};

// The colors of the "Source summary" bar (6597:45570): Journal, Book and Other share one.
const typeColors: Record<SourceType, string> = {
  website: 'var(--color-citation-website)',
  news: 'var(--color-citation-news)',
  journal: 'var(--color-citation-other)',
  book: 'var(--color-citation-other)',
  other: 'var(--color-citation-other)',
};

function typeName(type: SourceType): string {
  return citationText(`type_${type}`);
}

function referenceOf(item: HTMLLIElement, number: number): Reference {
  const citation = document.createElement('div');
  for (const child of item.childNodes) citation.append(child.cloneNode(true));
  const backrefs = citation.querySelector('.footnote-backrefs');
  const uses = backrefs ? Math.max(1, backrefs.querySelectorAll('a').length) : 1;
  backrefs?.remove();
  // the copy may hold several paragraphs: keep them inline in a card
  for (const p of citation.querySelectorAll('p')) p.replaceWith(...p.childNodes, ' ');
  return {number, item, citation, uses, info: classifyCitation(citation)};
}

function referencesOf(footnotes: Element): Map<string, Reference> {
  const refs = new Map<string, Reference>();
  let number = 0;
  for (const item of footnotes.querySelectorAll<HTMLLIElement>(':scope > ol > li[id]')) {
    number++;
    // the list numbers itself from this, not from a counter: a filter hides some items
    item.setAttribute('data-number', String(number));
    refs.set(item.id, referenceOf(item, number));
  }
  return refs;
}

// revealReference shows all the references again when a filter hides the one a link jumps to.
function revealReference(ref: Reference): void {
  if (ref.item.hidden) ref.item.closest('.footnotes')?.querySelector<HTMLButtonElement>('.citation-filter')?.click();
}

function referenceTitle(ref: Reference): string {
  return citationText('type_reference', typeName(ref.info.type), ref.number);
}

// cardContent builds the content shared by the hover card and the bottom sheet.
function cardContent(ref: Reference, mobile: boolean): HTMLElement {
  const card = createElementFromHTML(html`
    <div class="citation-card${mobile ? ' citation-card-sheet' : ''}">
      <div class="citation-card-meta">
        <span class="citation-card-type"></span>
      </div>
      <div class="citation-card-text"></div>
      <div class="citation-card-actions">
        <span class="citation-card-links"></span>
        <span class="citation-card-uses"></span>
      </div>
    </div>
  `);
  const type = card.querySelector('.citation-card-type');
  type.innerHTML = svg(typeIcons[ref.info.type], mobile ? 14 : 12);
  type.append(referenceTitle(ref));
  if (ref.info.archived) {
    card.querySelector('.citation-card-meta').append(createElementFromHTML(
      html`<span class="citation-chip citation-chip-archived">${citationText('archived_copy')}</span>`));
  }
  const text = card.querySelector('.citation-card-text');
  for (const child of ref.citation.childNodes) text.append(child.cloneNode(true));

  const links = card.querySelector('.citation-card-links');
  if (ref.info.sourceURL) {
    links.append(createElementFromHTML(html`<a class="citation-card-action" href="${ref.info.sourceURL}" target="_blank" rel="nofollow noopener">${citationText(mobile ? 'open_source' : 'visit_source')}</a>`));
  }
  const goto = createElementFromHTML(html`<a class="citation-card-action" href="#${ref.item.id}" data-citation-goto>${citationText('go_to_reference')}</a>`);
  goto.addEventListener('click', () => revealReference(ref));
  links.append(goto);
  card.querySelector('.citation-card-uses').textContent = ref.uses === 1 ? citationText('used_once') : citationText('used_times', ref.uses);
  return card;
}

let openCard: {card: HTMLElement, marker: HTMLElement, hideTimer?: ReturnType<typeof setTimeout>} | null = null;
let cardSeq = 0;

// Escape dismisses the card, however it was opened (WCAG 1.4.13)
function onCardKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') closeCard();
}

function closeCard(): void {
  if (!openCard) return;
  openCard.marker.classList.remove('is-active');
  openCard.marker.querySelector('a')?.removeAttribute('aria-describedby');
  openCard.card.remove();
  openCard = null;
  document.removeEventListener('keydown', onCardKey);
}

function placeCard(card: HTMLElement, marker: HTMLElement): void {
  const rect = marker.getBoundingClientRect();
  const width = Math.min(400, document.documentElement.clientWidth - 32);
  card.style.width = `${width}px`;
  const left = Math.max(16, Math.min(rect.left + window.scrollX, window.scrollX + document.documentElement.clientWidth - width - 16));
  card.style.left = `${left}px`;
  card.style.top = `${rect.bottom + window.scrollY + 6}px`;
}

// scheduleHideFor hides the marker's card soon, unless the pointer comes back to the marker
// or moves into the card.
function scheduleHideFor(marker: HTMLElement): void {
  const state = openCard;
  if (state?.marker !== marker) return;
  clearTimeout(state.hideTimer);
  state.hideTimer = setTimeout(() => {
    if (openCard === state) closeCard();
  }, 250);
}

function showHoverCard(marker: HTMLElement, ref: Reference): void {
  if (openCard?.marker === marker) {
    clearTimeout(openCard.hideTimer);
    return;
  }
  closeCard();
  const card = cardContent(ref, false);
  card.classList.add('citation-hover-card');
  card.setAttribute('role', 'tooltip');
  card.id = `citation-hover-card-${++cardSeq}`;
  document.body.append(card);
  placeCard(card, marker);
  marker.classList.add('is-active');
  marker.querySelector('a')?.setAttribute('aria-describedby', card.id);
  document.addEventListener('keydown', onCardKey);
  const state = {card, marker} as NonNullable<typeof openCard>;
  openCard = state;
  card.addEventListener('mouseenter', () => clearTimeout(state.hideTimer));
  card.addEventListener('mouseleave', () => scheduleHideFor(marker));
  card.querySelector('[data-citation-goto]')?.addEventListener('click', () => closeCard());
}

function showBottomSheet(marker: HTMLElement, ref: Reference): void {
  closeCard();
  const sheet = createElementFromHTML(html`
    <div class="citation-sheet-backdrop">
      <div class="citation-sheet" role="dialog" aria-modal="true" aria-label="${referenceTitle(ref)}">
        <div class="citation-sheet-handle"></div>
      </div>
    </div>
  `);
  sheet.querySelector('.citation-sheet').append(cardContent(ref, true));
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') close(); // eslint-disable-line @typescript-eslint/no-use-before-define
  };
  const close = () => {
    const hadFocus = sheet.contains(document.activeElement);
    sheet.remove();
    marker.classList.remove('is-active');
    document.removeEventListener('keydown', onKey);
    // back to the marker, unless the writer jumped to the reference
    if (hadFocus || document.activeElement === document.body) marker.querySelector('a')?.focus({preventScroll: true});
  };
  sheet.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;
    if (target === sheet) close();
    if (target.closest('[data-citation-goto]')) {
      sheet.remove(); // the link moves the focus to the reference
      marker.classList.remove('is-active');
      document.removeEventListener('keydown', onKey);
    }
  });
  document.addEventListener('keydown', onKey);
  marker.classList.add('is-active');
  document.body.append(sheet);
  sheet.querySelector<HTMLElement>('.citation-card-action')?.focus();
}

function isPhone(): boolean {
  return window.matchMedia('(max-width: 767.98px), (pointer: coarse)').matches;
}

function initReferenceMarkers(container: HTMLElement, refs: Map<string, Reference>): void {
  for (const sup of container.querySelectorAll<HTMLElement>('sup.footnote-ref')) {
    const link = sup.querySelector<HTMLAnchorElement>('a[href^="#"]');
    if (!link) continue;
    const ref = refs.get(decodeURIComponent(link.getAttribute('href').slice(1)));
    if (!ref) continue;
    let showTimer: ReturnType<typeof setTimeout>;
    link.addEventListener('mouseenter', () => {
      if (isPhone()) return;
      showTimer = setTimeout(() => showHoverCard(sup, ref), 150);
    });
    link.addEventListener('mouseleave', () => {
      clearTimeout(showTimer);
      scheduleHideFor(sup);
    });
    link.addEventListener('focus', () => {
      if (!isPhone()) showHoverCard(sup, ref);
    });
    link.addEventListener('blur', () => scheduleHideFor(sup));
    link.addEventListener('click', (e) => {
      if (!isPhone()) {
        closeCard();
        return; // jump to the reference
      }
      e.preventDefault();
      showBottomSheet(sup, ref);
    });
    // a filter may hide the reference the marker jumps to
    link.addEventListener('click', () => revealReference(ref));
  }
}

// initReferenceSummary adds the source count to the References heading and the "Where this
// article's evidence comes from" card with its filter chips.
function initReferenceSummary(footnotes: HTMLElement, refs: Reference[]): void {
  const heading = footnotes.querySelector<HTMLElement>(':scope > .footnotes-heading');
  if (!heading || !refs.length || footnotes.querySelector('.citation-summary')) return;

  const publishers = new Map<string, number>();
  for (const ref of refs) {
    const key = ref.info.publisher.toLowerCase();
    if (key) publishers.set(key, (publishers.get(key) ?? 0) + 1);
  }
  const sourceCount = refs.length === 1 ? citationText('source_count_one') : citationText('source_count', refs.length);
  const publisherCount = publishers.size === 1 ? citationText('publisher_count_one') : citationText('publisher_count', publishers.size);
  const header = createElementFromHTML(html`<div class="footnotes-header"><span class="footnotes-count">${citationText('sources_from_publishers', sourceCount, publisherCount)}</span></div>`);
  heading.replaceWith(header);
  header.prepend(heading);

  const counts = new Map<SourceType, number>();
  for (const ref of refs) counts.set(ref.info.type, (counts.get(ref.info.type) ?? 0) + 1);
  const top = Math.max(0, ...publishers.values());
  const dominated = top >= 2 && top / refs.length > 0.3;

  const card = createElementFromHTML(html`
    <div class="citation-summary">
      <div class="citation-summary-title">${citationText('evidence_title')}</div>
      <div class="citation-summary-bar" aria-hidden="true"></div>
      <div class="citation-summary-legend">
        <span class="citation-summary-types"></span>
        <span class="citation-summary-balance ${dominated ? 'is-dominated' : ''}">${dominated ? citationText('publisher_dominates', top, refs.length) : citationText('publishers_balanced')}</span>
      </div>
      <div class="citation-summary-filters" role="group"></div>
    </div>
  `);
  const bar = card.querySelector('.citation-summary-bar');
  for (const type of sourceTypes) {
    const n = counts.get(type) ?? 0;
    if (!n) continue;
    const segment = document.createElement('span');
    segment.style.flexGrow = String(n);
    segment.style.background = typeColors[type];
    bar.append(segment);
  }
  const legend = card.querySelector('.citation-summary-types');
  const otherCounts = (['journal', 'book', 'other'] as SourceType[]).map((t) => `${typeName(t)} ${counts.get(t) ?? 0}`).join(', ');
  for (const [color, label] of [
    [typeColors.website, `${typeName('website')} ${counts.get('website') ?? 0}`],
    [typeColors.news, `${typeName('news')} ${counts.get('news') ?? 0}`],
    [typeColors.other, otherCounts],
  ]) {
    legend.append(createElementFromHTML(html`<span class="citation-summary-key"><span class="citation-summary-swatch" style="background: ${color}"></span>${label}</span>`));
  }

  // filter chips: all, then each type present, then archived copies
  const filters = card.querySelector('.citation-summary-filters');
  const chips: Array<{label: string, accepts: (ref: Reference) => boolean}> = [
    {label: citationText('filter_all', refs.length), accepts: () => true},
  ];
  for (const type of sourceTypes) {
    if (counts.get(type)) chips.push({label: typeName(type), accepts: (ref) => ref.info.type === type});
  }
  if (refs.some((ref) => ref.info.archived)) chips.push({label: citationText('filter_archived'), accepts: (ref) => ref.info.archived});
  for (const [i, chip] of chips.entries()) {
    const button = createElementFromHTML<HTMLButtonElement>(html`<button type="button" class="citation-chip citation-filter" aria-pressed="${i === 0 ? 'true' : 'false'}">${chip.label}</button>`);
    button.addEventListener('click', () => {
      for (const b of filters.querySelectorAll('button')) b.setAttribute('aria-pressed', String(b === button));
      for (const ref of refs) ref.item.hidden = !chip.accepts(ref);
    });
    filters.append(button);
  }
  header.after(card);
}

export function initMarkupReferences(container: HTMLElement): void {
  const footnotes = container.querySelector<HTMLElement>('.footnotes');
  if (!footnotes) return;
  const refs = referencesOf(footnotes);
  initReferenceMarkers(container, refs);
  // the source summary is for articles, not for footnotes in comments
  if (container.classList.contains('file-view')) initReferenceSummary(footnotes, Array.from(refs.values()));
}
