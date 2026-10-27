import {Transition, createApp, h, reactive} from 'vue';
import CompareAnnouncement from '../components/graph/CompareAnnouncement.vue';
import {collapseInfoBar, expandInfoBar} from '../modules/info-bar-transition.ts';
import {fetchReadmeSummary} from '../components/graph/article-summary.ts';
import {matchesSelection, type RepoSelection} from '../modules/repo-selection.ts';
import {getCurrentLocale} from '../utils.ts';
import {
  COMPARE_MODE_STATE_EVENT, compareAnnouncementFor, readCompareMessages, subjectCompareUrl,
  type CompareAnnouncementState, type CompareModeState,
} from '../modules/compare-mode-request.ts';

/* The subject's Table view (custom/templates/shared/repo/table.tmpl): figma "Table view /
   Articles" (641:61176) and "Table view / Comparing articles" (641:61039).

   - Paged, with figma's footer (DataTable/Footer 641:61303): "8 of 12" and Previous / Next.
   - One row open at a time: its summary (the repository description, or else the first
     paragraph of the article's README, as the Bubble view shows it) and "Go to article".
     Opening a row selects its article.
   - The selected article's row is highlighted (641:61208).
   - Compare mode of its own (#437): the header's Compare button, pressed on the Table view,
     keeps the table on screen; the rows (their checkboxes, or the rows themselves) pick
     the two articles to compare and the Compare banner (CompareAnnouncement.vue) says how
     many are picked. Checking a box outside compare mode enters it with that article
     picked. */

/** Figma's table shows 8 rows a page ("8 of 12", 641:61303). */
export const TABLE_PAGE_SIZE = 8;

/** How many pages `total` rows take, one at least (an empty table still has its page). */
export function tablePageCount(total: number, size: number): number {
  return Math.max(1, Math.ceil(total / size));
}

/** The page (1-based) showing row `index` (0-based). */
export function tablePageOfRow(index: number, size: number): number {
  return Math.floor(Math.max(0, index) / size) + 1;
}

/** The rows [start, end) shown on `page`, which is clamped to the existing pages. */
export function tablePageRange(page: number, size: number, total: number): [number, number] {
  const clamped = Math.min(Math.max(1, page), tablePageCount(total, size));
  const start = (clamped - 1) * size;
  return [start, Math.min(total, start + size)];
}

/** Fills a localized message's positional placeholders, written as in the rest of the
   locale file (%[1]d, %[2]s; Go's fmt), for a message the server sends unformatted: the
   page renders it without arguments, which leaves it as it is. */
export function fillMessage(template: string, ...values: Array<number | string>): string {
  return template.replace(/%\[(\d+)\][ds]/g, (all, n: string) => (Number(n) <= values.length ? String(values[Number(n) - 1]) : all));
}

/** The footer's counter, figma's "8 of 12": how far into the articles this page reaches,
   in the localized `template` ("%[1]d of %[2]d"). */
export function tableCounterText(template: string, page: number, size: number, total: number): string {
  return fillMessage(template, tablePageRange(page, size, total)[1], total);
}

/** Figma's date, "03 Feb 2025" (641:61205): day, short month and year in that order
   whatever the locale's own order, in `lang`'s month names and the reader's time zone
   (or `timeZone`). */
export function formatTableDate(date: Date, lang?: string, timeZone?: string): string {
  const parts = new Intl.DateTimeFormat(lang || undefined, {day: '2-digit', month: 'short', year: 'numeric', timeZone}).formatToParts(date);
  const part = (type: string) => parts.find((p) => p.type === type)?.value ?? '';
  return `${part('day')} ${part('month')} ${part('year')}`;
}

/** The compare picks after a checkbox of `article` is (un)checked: two at most, in the
   order they were picked. A third pick is refused (the picks stay as they were). */
export function nextComparePicks(picks: RepoSelection[], article: RepoSelection, checked: boolean): RepoSelection[] {
  const without = picks.filter((p) => !matchesSelection(p, article));
  if (!checked) return without;
  if (without.length !== picks.length) return picks;
  return picks.length >= 2 ? picks : [...picks, article];
}

/** The banner's state: the picks while compare mode is on, else the "No forks yet"
   notice when it was asked for, else none. */
export function tableCompareState(on: boolean, picks: number, unavailableShown: boolean): CompareAnnouncementState | null {
  if (on) return picks >= 2 ? 'ready' : picks === 1 ? 'one' : 'select';
  return unavailableShown ? 'unavailable' : null;
}

type TableOptions = {
  section: HTMLElement;
  appSubUrl: string;
  subject: string;
  selectionOf: (el: Element) => RepoSelection | null;
  getSelection: () => RepoSelection | null;
  select: (selection: RepoSelection) => void;
  openArticle: (el: Element) => void;
};

export type HistoryTable = {
  /** The page's selection changed: its row is highlighted. */
  syncSelection: () => void;
  /** The view was switched to (`true`) or away from: the header's Compare button
     follows the compare mode of the view on screen. */
  setActive: (active: boolean) => void;
  /** Turns the table's compare mode on or off (the header's Compare button). */
  toggleCompare: () => void;
};

export function initHistoryTable(opts: TableOptions): HistoryTable | null {
  const {section, appSubUrl, selectionOf, getSelection, select, openArticle} = opts;
  const table = section.querySelector<HTMLTableElement>('#articles-table');
  if (!table) return null;

  const rows = Array.from(table.querySelectorAll<HTMLTableRowElement>('tr.article-row'));
  const detailOf = (row: HTMLTableRowElement) => {
    const id = row.querySelector('.row-toggle')?.getAttribute('data-target');
    return id ? section.querySelector<HTMLTableRowElement>(`#${CSS.escape(id)}`) : null;
  };
  const checkboxOf = (row: Element) => row.querySelector<HTMLInputElement>('.row-check');
  const isTombstone = (row: Element) => row.getAttribute('data-tombstone') === 'true';
  const liveRows = rows.filter((row) => !isTombstone(row)).length;
  const compareAvailable = liveRows >= 2;

  let page = 1;
  const counterEl = section.querySelector<HTMLElement>('[data-role="table-counter"]');
  const counterMsg = counterEl?.getAttribute('data-msg-counter') || '%[1]d of %[2]d';
  const prevBtn = section.querySelector<HTMLButtonElement>('[data-role="table-prev"]');
  const nextBtn = section.querySelector<HTMLButtonElement>('[data-role="table-next"]');
  const statusEl = section.querySelector<HTMLElement>('[data-role="table-status"]');
  const pageMsg = statusEl?.getAttribute('data-msg-page') || 'Articles %[1]d to %[2]d of %[3]d';
  const messages = readCompareMessages(document.querySelector('#compare-announcement-root'), {
    one: document.querySelector('#table-compare-announcement-root')?.getAttribute('data-msg-one') || undefined,
  });

  let active = false;
  const compare = reactive({on: false, picks: [] as RepoSelection[], unavailableShown: false});
  // the Bubble view's compare state, as it last reported it: the button shows it again
  // when the Table view is left
  let bubbleState: CompareModeState | null = null;
  let dispatching = false;

  // The dates in the reader's time zone and month names, in figma's order.
  const lang = getCurrentLocale() || undefined;
  for (const time of table.querySelectorAll<HTMLTimeElement>('time[data-unix]')) {
    const unix = Number(time.getAttribute('data-unix'));
    if (unix > 0) time.textContent = formatTableDate(new Date(unix * 1000), lang);
  }

  function lastPage() {
    return tablePageCount(rows.length, TABLE_PAGE_SIZE);
  }

  function setDirection(btn: HTMLButtonElement | null, enabled: boolean) {
    // aria-disabled rather than disabled: a disabled button would drop the keyboard
    // focus of whoever just pressed it onto the first or last page
    btn?.setAttribute('aria-disabled', String(!enabled));
  }

  function showPage(next: number, announce = false) {
    page = Math.min(Math.max(1, next), lastPage());
    const [start, end] = tablePageRange(page, TABLE_PAGE_SIZE, rows.length);
    for (const [idx, row] of rows.entries()) {
      const off = idx < start || idx >= end;
      row.toggleAttribute('hidden', off);
      detailOf(row)?.toggleAttribute('hidden', off);
    }
    if (counterEl) counterEl.textContent = tableCounterText(counterMsg, page, TABLE_PAGE_SIZE, rows.length);
    setDirection(prevBtn, page > 1);
    setDirection(nextBtn, page < lastPage());
    if (announce && statusEl) statusEl.textContent = fillMessage(pageMsg, start + 1, end, rows.length);
  }

  function rowOf(selection: RepoSelection | null): number {
    if (!selection) return -1;
    return rows.findIndex((row) => matchesSelection(selectionOf(row), selection));
  }

  function syncRows() {
    const selection = getSelection();
    const full = compare.on && compare.picks.length >= 2;
    // in compare mode a click on a row picks it (see the click handler)
    table.classList.toggle('is-comparing', compare.on);
    for (const row of rows) {
      const article = selectionOf(row);
      const picked = compare.picks.some((p) => matchesSelection(p, article));
      const checkbox = checkboxOf(row);
      if (checkbox) {
        checkbox.checked = picked;
        // two picked: the other rows cannot be picked until one is let go
        checkbox.disabled = isTombstone(row) || (full && !picked);
      }
      // in compare mode the picked rows are the highlighted ones (641:61072), otherwise
      // the selected article's (641:61208)
      row.classList.toggle('is-selected', compare.on ? picked : matchesSelection(selection, article));
    }
  }

  function setOpen(row: HTMLTableRowElement, open: boolean) {
    const detail = detailOf(row);
    const btn = row.querySelector<HTMLButtonElement>('.row-toggle');
    if (!detail || !btn) return;
    detail.classList.toggle('tw-hidden', !open);
    row.classList.toggle('is-open', open);
    btn.setAttribute('aria-expanded', String(open));
    btn.querySelector('.icon-down')?.classList.toggle('tw-hidden', open);
    btn.querySelector('.icon-up')?.classList.toggle('tw-hidden', !open);
    if (open) loadSummary(detail);
  }

  // The README's summary, fetched while the source is pending: a 404 is final, a server
  // or network error keeps the source so that opening the row again retries it.
  async function loadSummary(detail: HTMLElement) {
    const el = detail.querySelector<HTMLElement>('[data-role="summary"]');
    if (!el || el.classList.contains('is-loading')) return;
    const src = el.getAttribute('data-summary-src');
    const empty = el.getAttribute('data-empty') || '';
    if (!src) {
      if (!el.textContent.trim()) el.textContent = empty;
      return;
    }
    el.classList.add('is-loading');
    const {summary, retry} = await fetchReadmeSummary(src);
    if (!retry) el.removeAttribute('data-summary-src');
    el.textContent = summary || empty;
    el.classList.remove('is-loading');
  }

  function announce(article: string | null) {
    const state = tableCompareState(compare.on, compare.picks.length, compare.unavailableShown);
    if (!statusEl || !state) return;
    statusEl.textContent = compareAnnouncementFor(article, messages[state]);
  }

  function dispatchState(state: CompareModeState) {
    dispatching = true;
    try {
      window.dispatchEvent(new CustomEvent<CompareModeState>(COMPARE_MODE_STATE_EVENT, {detail: state}));
    } finally {
      dispatching = false;
    }
  }

  function syncButton() {
    if (active) dispatchState({on: compare.on, available: compareAvailable});
  }

  function toggleCompare() {
    if (!compare.on && !compareAvailable) {
      compare.unavailableShown = !compare.unavailableShown;
    } else {
      compare.on = !compare.on;
      compare.unavailableShown = false;
      if (!compare.on) compare.picks = [];
    }
    syncRows();
    syncButton();
    announce(null);
  }

  // (Un)picks the article of `row` for the comparison; picking one outside compare mode
  // enters it.
  function pick(row: HTMLTableRowElement, checked: boolean) {
    const article = selectionOf(row);
    if (!article || isTombstone(row)) {
      syncRows();
      return;
    }
    if (checked && !compare.on) {
      if (!compareAvailable) {
        compare.unavailableShown = true;
        syncRows();
        announce(null);
        return;
      }
      compare.on = true;
      compare.unavailableShown = false;
      syncButton();
    }
    const before = compare.picks.length;
    compare.picks = nextComparePicks(compare.picks, article, checked);
    syncRows();
    if (compare.picks.length !== before) announce(`${article.owner}/${article.repo}`);
  }

  function goToComparison() {
    if (compare.picks.length !== 2) return;
    const [first, second] = compare.picks;
    const subject = first.subject || second.subject || opts.subject;
    if (!subject) return;
    window.location.href = subjectCompareUrl(appSubUrl, subject, first.owner, second.owner);
  }

  // The Compare banner, under the navbar like the Bubble view's (its root is shown only
  // with the Table view: web_src/css/features/subject-views.css).
  const bannerRoot = document.querySelector<HTMLElement>('#table-compare-announcement-root');
  if (bannerRoot) {
    createApp({
      // #469: it slides open and closed (info-bar-transition.ts), in a wrapper that takes
      // the slide, as the Bubble view's does
      render() {
        const state = tableCompareState(compare.on, compare.picks.length, compare.unavailableShown);
        return h(Transition, {
          css: false,
          onEnter: (el: Element, done: () => void) => expandInfoBar(el as HTMLElement, done),
          onLeave: (el: Element, done: () => void) => collapseInfoBar(el as HTMLElement, done),
        }, () => (state ? h('div', {class: 'info-bar-slide'}, [h(CompareAnnouncement, {
          state,
          messages,
          showCompareNow: true,
          onDismiss: () => {
            if (compare.on) toggleCompare();
            else compare.unavailableShown = false;
          },
          onCompare: goToComparison,
        })]) : null));
      },
    }).mount(bannerRoot);
  }

  // The Bubble view reports its own compare state whenever it changes, even while hidden:
  // that is the button's state when the Table view is not on screen. While the table is,
  // its own state is sent again right after. The button (copycontent.ts) shows the last
  // state it is sent: its listener is registered before this one (initCompareModeToggle
  // runs before initRepoHistory), so it gets the Bubble view's state first, then the
  // table's, sent from inside this listener.
  window.addEventListener(COMPARE_MODE_STATE_EVENT, (event) => {
    if (dispatching) return;
    bubbleState = (event as CustomEvent<CompareModeState>).detail;
    syncButton();
  });

  table.addEventListener('click', (event) => {
    const target = event.target as HTMLElement;
    const toggle = target.closest<HTMLButtonElement>('.row-toggle');
    if (toggle) {
      event.preventDefault();
      const row = toggle.closest<HTMLTableRowElement>('tr.article-row');
      if (!row) return;
      const open = !row.classList.contains('is-open');
      // one row open at a time, as figma draws it
      if (open) for (const other of rows) if (other !== row && other.classList.contains('is-open')) setOpen(other, false);
      setOpen(row, open);
      if (open) {
        const article = selectionOf(row);
        if (article) select(article);
      }
      return;
    }
    const goTo = target.closest('.go-to-article');
    if (goTo) {
      event.preventDefault();
      openArticle(goTo);
      return;
    }
    if (target.closest('.history-table-check')) return;
    const row = target.closest<HTMLTableRowElement>('tr.article-row');
    if (!row) return;
    // in compare mode a row picks its article, as a bubble does in the Bubble view,
    // instead of leaving the table and its picks
    if (compare.on) {
      const checkbox = checkboxOf(row);
      if (checkbox && !checkbox.disabled) pick(row, !checkbox.checked);
      return;
    }
    openArticle(row);
  });

  table.addEventListener('change', (event) => {
    const checkbox = event.target as HTMLInputElement;
    if (!checkbox.classList?.contains('row-check')) return;
    const row = checkbox.closest<HTMLTableRowElement>('tr.article-row');
    if (row) pick(row, checkbox.checked);
  });

  for (const [btn, step] of [[prevBtn, -1], [nextBtn, 1]] as const) {
    btn?.addEventListener('click', () => {
      if (btn.getAttribute('aria-disabled') === 'true') return;
      showPage(page + step, true);
    });
  }

  // the page holding the selected article, else the first
  const selectedIdx = rowOf(getSelection());
  showPage(selectedIdx >= 0 ? tablePageOfRow(selectedIdx, TABLE_PAGE_SIZE) : 1);
  syncRows();

  return {
    syncSelection() {
      syncRows();
    },
    setActive(next: boolean) {
      if (next === active) return;
      active = next;
      if (active) {
        // a selection made in another view: show its page
        const idx = rowOf(getSelection());
        if (idx >= 0) showPage(tablePageOfRow(idx, TABLE_PAGE_SIZE));
        syncButton();
      } else {
        dispatchState(bubbleState ?? {on: false, available: compareAvailable});
      }
    },
    toggleCompare,
  };
}
