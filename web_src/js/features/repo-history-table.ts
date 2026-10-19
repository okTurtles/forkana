import {createApp, h, reactive} from 'vue';
import CompareAnnouncement, {type CompareAnnouncementMessages} from '../components/graph/CompareAnnouncement.vue';
import {extractArticleSummary} from '../components/graph/article-summary.ts';
import {GET} from '../modules/fetch.ts';
import {matchesSelection, type RepoSelection} from '../modules/repo-selection.ts';
import {COMPARE_MODE_STATE_EVENT, compareAnnouncementFor, type CompareModeState} from '../modules/compare-mode-request.ts';

/* The subject's Table view (custom/templates/shared/repo/table.tmpl): figma "Table view /
   Articles" (641:61176) and "Table view / Comparing articles" (641:61039).

   - Paged, with figma's footer (DataTable/Footer 641:61303): "8 of 12" and Previous / Next.
   - One row open at a time: its summary (the repository description, or else the first
     paragraph of the article's README, as the Bubble view shows it) and "Go to article".
     Opening a row selects its article.
   - The selected article's row is highlighted (641:61208).
   - Compare mode of its own (#437): the header's Compare button, pressed on the Table view,
     keeps the table on screen; the rows' checkboxes pick the two articles to compare and
     the Compare banner (CompareAnnouncement.vue) says how many are picked. Checking a box
     outside compare mode enters it with that article picked. */

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

/** The footer's counter, figma's "8 of 12": how far into the articles this page reaches. */
export function tableCounterText(page: number, size: number, total: number): string {
  return `${tablePageRange(page, size, total)[1]} of ${total}`;
}

/** The compare page of two articles of a subject, as the Bubble view opens it. */
export function tableCompareUrl(appSubUrl: string, subject: string, first: RepoSelection, second: RepoSelection): string {
  return `${appSubUrl}/subject/${encodeURIComponent(subject)}/compare/${encodeURIComponent(first.owner)}...${encodeURIComponent(second.owner)}`;
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
/** The states of CompareAnnouncement.vue (its CompareAnnouncementState). */
export type TableCompareState = 'select' | 'one' | 'ready' | 'unavailable';

export function tableCompareState(on: boolean, picks: number, unavailableShown: boolean): TableCompareState | null {
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

function readMessages(): CompareAnnouncementMessages {
  const bubbleRoot = document.querySelector('#compare-announcement-root');
  const tableRoot = document.querySelector('#table-compare-announcement-root');
  const bubble = (name: string) => bubbleRoot?.getAttribute(`data-msg-${name}`);
  return {
    select: bubble('select') || 'Select 2 articles to compare (0/2 selected).',
    one: tableRoot?.getAttribute('data-msg-one') || bubble('one') || '1/2 article selected. Select another to compare.',
    ready: bubble('ready') || '2/2 selected – ready to compare.',
    unavailable: bubble('no-forks') || 'No forks yet. Compare needs at least 2 articles. Fork this article to start comparing.',
    compareNow: bubble('compare-now') || 'Compare now',
    dismiss: bubble('dismiss') || 'Exit compare mode',
    dismissNotice: bubble('dismiss-notice') || 'Dismiss',
  };
}

export function initHistoryTable(opts: TableOptions): HistoryTable | null {
  const {section, appSubUrl, selectionOf, getSelection, select, openArticle} = opts;
  const table = section.querySelector<HTMLTableElement>('#articles-table');
  if (!table) return null;

  const rows = Array.from(table.querySelectorAll<HTMLTableRowElement>('tr.article-row'));
  const detailOf = (row: HTMLTableRowElement) => {
    const id = row.querySelector('.row-toggle')?.getAttribute('data-target');
    return id ? section.querySelector<HTMLTableRowElement>(`#${CSS.escape(id)}`) : null;
  };
  const isTombstone = (row: Element) => row.getAttribute('data-tombstone') === 'true';
  const liveRows = rows.filter((row) => !isTombstone(row)).length;
  const compareAvailable = liveRows >= 2;

  const pageSize = Number(section.querySelector('.history-table')?.getAttribute('data-page-size')) || TABLE_PAGE_SIZE;
  let page = 1;
  const counterEl = section.querySelector<HTMLElement>('[data-role="table-counter"]');
  const prevBtn = section.querySelector<HTMLButtonElement>('[data-role="table-prev"]');
  const nextBtn = section.querySelector<HTMLButtonElement>('[data-role="table-next"]');
  const statusEl = section.querySelector<HTMLElement>('[data-role="table-status"]');
  const messages = readMessages();

  let active = false;
  const compare = reactive({on: false, picks: [] as RepoSelection[], unavailableShown: false});
  // the Bubble view's compare state, as it last reported it: the button shows it again
  // when the Table view is left
  let bubbleState: CompareModeState | null = null;
  let dispatching = false;

  function showPage(next: number) {
    page = Math.min(Math.max(1, next), tablePageCount(rows.length, pageSize));
    const [start, end] = tablePageRange(page, pageSize, rows.length);
    for (const [idx, row] of rows.entries()) {
      const off = idx < start || idx >= end;
      row.toggleAttribute('hidden', off);
      detailOf(row)?.toggleAttribute('hidden', off);
    }
    if (counterEl) counterEl.textContent = tableCounterText(page, pageSize, rows.length);
    if (prevBtn) prevBtn.disabled = page <= 1;
    if (nextBtn) nextBtn.disabled = page >= tablePageCount(rows.length, pageSize);
  }

  function rowOf(selection: RepoSelection | null): number {
    if (!selection) return -1;
    return rows.findIndex((row) => matchesSelection(selectionOf(row), selection));
  }

  function syncRows() {
    const selection = getSelection();
    for (const row of rows) {
      const article = selectionOf(row);
      const picked = compare.picks.some((p) => matchesSelection(p, article));
      const checkbox = row.querySelector<HTMLInputElement>('.row-check');
      if (checkbox) checkbox.checked = picked;
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

  async function loadSummary(detail: HTMLElement) {
    const el = detail.querySelector<HTMLElement>('[data-role="summary"]');
    if (!el || el.textContent.trim()) return;
    const src = el.getAttribute('data-summary-src');
    const empty = el.getAttribute('data-empty') || '';
    if (!src) {
      el.textContent = empty;
      return;
    }
    if (el.classList.contains('is-loading')) return;
    el.classList.add('is-loading');
    try {
      const res = await GET(src);
      // a 404 (no README) is final; a server error may be retried by opening the row again
      if (!res.ok) {
        if (res.status < 500) el.removeAttribute('data-summary-src');
        el.textContent = empty;
        return;
      }
      el.removeAttribute('data-summary-src');
      el.textContent = extractArticleSummary(await res.text()) || empty;
    } catch {
      el.textContent = empty;
    } finally {
      el.classList.remove('is-loading');
    }
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

  function goToComparison() {
    if (compare.picks.length !== 2) return;
    const [first, second] = compare.picks;
    const subject = first.subject || second.subject || opts.subject;
    if (!subject) return;
    window.location.href = tableCompareUrl(appSubUrl, subject, first, second);
  }

  // The Compare banner, under the navbar like the Bubble view's (its root is shown only
  // with the Table view: web_src/css/features/subject-views.css).
  const bannerRoot = document.querySelector<HTMLElement>('#table-compare-announcement-root');
  if (bannerRoot) {
    createApp({
      render() {
        const state = tableCompareState(compare.on, compare.picks.length, compare.unavailableShown);
        if (!state) return null;
        return h(CompareAnnouncement, {
          state,
          messages,
          showCompareNow: true,
          onDismiss: () => {
            if (compare.on) toggleCompare();
            else compare.unavailableShown = false;
          },
          onCompare: goToComparison,
        });
      },
    }).mount(bannerRoot);
  }

  // The Bubble view reports its own compare state whenever it changes, even while hidden:
  // that is the button's state when the Table view is not on screen.
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
    if (row) openArticle(row);
  });

  table.addEventListener('change', (event) => {
    const checkbox = event.target as HTMLInputElement;
    if (!checkbox.classList?.contains('row-check')) return;
    const row = checkbox.closest<HTMLTableRowElement>('tr.article-row');
    const article = row ? selectionOf(row) : null;
    if (!row || !article || isTombstone(row)) {
      checkbox.checked = false;
      return;
    }
    if (checkbox.checked && !compare.on) {
      if (!compareAvailable) {
        checkbox.checked = false;
        compare.unavailableShown = true;
        announce(null);
        return;
      }
      compare.on = true;
      compare.unavailableShown = false;
      syncButton();
    }
    const before = compare.picks.length;
    compare.picks = nextComparePicks(compare.picks, article, checkbox.checked);
    syncRows();
    if (compare.picks.length !== before) announce(`${article.owner}/${article.repo}`);
  });

  prevBtn?.addEventListener('click', () => showPage(page - 1));
  nextBtn?.addEventListener('click', () => showPage(page + 1));

  // the page holding the selected article, else the first
  const selectedIdx = rowOf(getSelection());
  showPage(selectedIdx >= 0 ? tablePageOfRow(selectedIdx, pageSize) : 1);
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
        if (idx >= 0) showPage(tablePageOfRow(idx, pageSize));
        syncButton();
      } else {
        dispatchState(bubbleState ?? {on: false, available: compareAvailable});
      }
    },
    toggleCompare,
  };
}
