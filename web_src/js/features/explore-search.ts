import {queryElems} from '../utils/dom.ts';
import type {DOMEvent} from '../utils/dom.ts';

// Explore pages (repos, subjects, users) share the same borderless filter/sort search bar
// design, each rendered as its own <form> with one of these ids.
const searchFormSelectors = '#repo-search-form, #subject-search-form, #user-search-form';

// Tab links of the explore navbar (custom/templates/explore/navbar.tmpl). The active tab is
// server-rendered with the keyword of the page on screen, the other tabs with no keyword at all.
const tabLinkSelector = 'a[data-explore-tab-link]';

// Search field of the explore page, every tab searches with the same "q" parameter.
const searchInputSelector = '.page-content.explore input[type="search"][name="q"]';

// Each explore tab keeps its own search (#420): switching to another tab shows that tab's own last
// search, or its plain page if it has none, and coming back restores this one, including any text
// typed into the field but not searched yet. Nothing is ever searched with another tab's text.
//
// The searches live in sessionStorage, per tab of the explore navbar (keyed by its path):
//  - "query" is the keyword that tab was last rendered for, which its link points back to;
//  - "draft" is whatever its field held when it was left, put back into the field (without
//    searching it) when the user comes back to that tab through the navbar.
// sessionStorage rather than extra query parameters on the tab links: the state belongs to this
// browser tab only, works for anonymous users, needs nothing from the server, and keeps the
// addresses of the explore pages the plain, shareable "?q=..." ones.
type TabSearch = {query: string, draft: string};

const storageKeyPrefix = 'explore-search:';

// Set when a tab link is followed, to the path of the tab it leads to, so that tab knows it has
// been switched to (and may restore its draft), as opposed to being loaded from an address, the
// home page search, a bookmark or a pagination link, where the address alone says what to show.
const switchStorageKey = `${storageKeyPrefix}switched-to`;

function storageGet(key: string): string | null {
  try {
    return window.sessionStorage.getItem(key);
  } catch {
    return null; // storage disabled: the other tabs simply open unsearched
  }
}

function storageSet(key: string, value: string): void {
  try {
    window.sessionStorage.setItem(key, value);
  } catch {}
}

function storageRemove(key: string): void {
  try {
    window.sessionStorage.removeItem(key);
  } catch {}
}

function tabPath(tabLink: HTMLAnchorElement): string {
  // the "href" property of an anchor is already resolved against the document
  return new URL(tabLink.href).pathname;
}

function loadTabSearch(path: string): TabSearch | null {
  try {
    const value = JSON.parse(storageGet(storageKeyPrefix + path) ?? 'null');
    if (typeof value?.query !== 'string' || typeof value?.draft !== 'string') return null;
    return value;
  } catch {
    return null;
  }
}

function saveTabSearch(path: string, search: TabSearch): void {
  storageSet(storageKeyPrefix + path, JSON.stringify(search));
}

// Point every inactive tab link at the keyword last searched on that tab (or at its plain page).
// Only "q" is set: sort and filter values are tab specific, and an unsupported "sort" makes the
// users tab return 404.
function updateInactiveTabLinks(): void {
  queryElems<HTMLAnchorElement>(document, `${tabLinkSelector}:not(.active)`, (tabLink) => {
    const url = new URL(tabLink.href);
    const query = loadTabSearch(url.pathname)?.query;
    if (query) {
      url.searchParams.set('q', query);
    } else {
      url.searchParams.delete('q');
    }
    tabLink.href = url.toString();
  });
}

function initExploreTabSearches(): void {
  const activeTabLink = document.querySelector<HTMLAnchorElement>(`${tabLinkSelector}.active`);
  const searchInput = document.querySelector<HTMLInputElement>(searchInputSelector);
  if (!activeTabLink || !searchInput) return;
  const activePath = tabPath(activeTabLink);

  // the page's keyword is the one the server filled into the field, whatever the browser restored
  const renderedKeyword = searchInput.defaultValue.trim();
  const switchedHere = storageGet(switchStorageKey) === activePath;
  storageRemove(switchStorageKey);

  const stored = loadTabSearch(activePath);
  if (switchedHere && stored?.query === renderedKeyword && stored.draft !== renderedKeyword) {
    searchInput.value = stored.draft; // shown as it was left, searched on Enter like any typed text
  }
  const saveDraft = () => saveTabSearch(activePath, {query: renderedKeyword, draft: searchInput.value.trim()});
  saveDraft();
  searchInput.addEventListener('input', saveDraft);

  updateInactiveTabLinks();
  // a page restored from the back/forward cache keeps the links it had when it was left, while the
  // other tabs may have searched something else since
  window.addEventListener('pageshow', (e: PageTransitionEvent) => {
    if (e.persisted) updateInactiveTabLinks();
  });

  queryElems<HTMLAnchorElement>(document, tabLinkSelector, (tabLink) => {
    tabLink.addEventListener('click', () => storageSet(switchStorageKey, tabPath(tabLink)));
  });
}

// The filters offered beside the search field. The "clear filters" button drops exactly these.
const filterParamNames = ['archived', 'fork', 'mirror', 'template', 'private', 'repo_role'];

// Drop the filters, leaving the sort order alone. This is what the "clear filters" button does.
function dropFilterParams(params: URLSearchParams): void {
  for (const name of filterParamNames) params.delete(name);
}

// Return the results to the way the plain, unfiltered page arranges them: no filters and no sort.
//
// Both are rendered inside the same form as the keyword field, so a native submit carries them
// along with whatever has just been typed. The page number needs no dropping: pagination is
// rendered as links outside the form, so a query string rebuilt from the form alone never has one.
function resetArrangement(params: URLSearchParams): void {
  dropFilterParams(params);
  params.delete('sort');
}

// The form's keyword field, when it has one.
function getKeywordField(searchForm: HTMLFormElement): HTMLInputElement | null {
  const field = searchForm.elements.namedItem('q');
  return field instanceof HTMLInputElement ? field : null;
}

// Current content of the form's keyword field, or "" when it has none.
function getKeyword(searchForm: HTMLFormElement): string {
  return getKeywordField(searchForm)?.value.trim() ?? '';
}

// The keyword the page on screen was rendered for: the field's "value" attribute, which the server
// filled in and which the browser keeps as "defaultValue" however the field is edited afterwards.
function getRenderedKeyword(searchForm: HTMLFormElement): string {
  return getKeywordField(searchForm)?.defaultValue.trim() ?? '';
}

// Whether the field holds a search of its own that the page on screen is not the answer to. An
// emptied field is not one: it asks for the unsearched page, which #335 returns without touching
// the way the results are arranged.
function isNewKeyword(searchForm: HTMLFormElement): boolean {
  const keyword = getKeyword(searchForm);
  return Boolean(keyword) && keyword !== getRenderedKeyword(searchForm);
}

// Query string the form should navigate to.
//
// An empty keyword is dropped instead of being submitted as "q=": the server treats both the same
// way, but only the shorter URL is the address of the plain, unsearched page, so clearing the field
// leaves a link worth bookmarking and sharing rather than one that searches for nothing.
export function buildSearchParams(searchForm: HTMLFormElement): URLSearchParams {
  const params = new URLSearchParams();
  for (const [key, value] of new FormData(searchForm).entries()) {
    params.set(key, value.toString());
  }
  params.delete('clear-filter');
  if (!getKeyword(searchForm)) params.delete('q');
  return params;
}

export function initExploreSearch() {
  // The explore navbar keeps a search per tab (see initExploreTabSearches), which also
  // changes how the keyword field itself is submitted, see below. The same search bar is rendered
  // without that navbar on other pages (profiles, organizations, the admin repository list).
  const hasExploreTabs = Boolean(document.querySelector(tabLinkSelector));

  for (const searchForm of document.querySelectorAll<HTMLFormElement>(searchFormSelectors)) {
    searchForm.addEventListener('change', (e: DOMEvent<Event, HTMLInputElement>) => {
      e.preventDefault();

      // The keyword field fires "change" once it loses focus with an edited value, no matter what
      // the user does next: click a tab, move the focus towards one with the keyboard (which first
      // steps through the search button), or click anywhere else. Reloading the current tab from
      // here cancels that tab switch and takes the focus with it, so on the explore pages the
      // keyword field keeps its own submit paths (the Enter key and the search button), and what
      // has been typed is kept as this tab's draft instead (see initExploreTabSearches).
      if (hasExploreTabs && e.target.name === 'q') return;

      const params = buildSearchParams(searchForm);
      if (e.target.name === 'clear-filter') {
        dropFilterParams(params);
      } else if (isNewKeyword(searchForm)) {
        // A keyword edited but not submitted rides along with this control change, which makes the
        // navigation a fresh search: it gets the fresh arrangement a submitted one gets, except for
        // the control the user has just picked, which is the whole point of the gesture. Without
        // this, picking a sort after typing a new keyword would search for it through the filters
        // the previous results were left in, which is the bug this file exists to keep fixed.
        resetArrangement(params);
        if (e.target.name !== 'q') params.set(e.target.name, e.target.value);
      }

      window.location.search = params.toString();
    });

    // Emptying the keyword field returns the page to its unsearched state. Two gestures get there:
    //
    //  - the "search" event, which Blink and WebKit fire when the field's own clear button (the "x"
    //    that <input type="search"> renders) or the Escape key empties it. Nothing else notices
    //    that gesture: it produces an "input" event indistinguishable from typing, and no "change"
    //    until the field is left, so the results used to sit on screen beside an empty search box.
    //  - submitting an empty field, via Enter or the search button. Firefox reaches the reset only
    //    this way: it renders no clear button and does not implement the "search" event.
    //
    // Both only intervene while the field is empty, and both drop only "q". Sort and the filters are
    // separate controls, so they survive: clearing the search clears the search, not the way the
    // results are arranged.
    searchForm.addEventListener('search', () => {
      if (getKeyword(searchForm)) return;
      window.location.search = buildSearchParams(searchForm).toString();
    });

    // Submitting a search that differs from the one on screen starts a fresh one, so it takes the
    // results back to their default arrangement. The sort radios and the filter radios sit inside
    // this same form with the current page's choices checked, so a native submit would otherwise
    // carry them over and arrange a brand new set of results the way the previous one was left.
    //
    // Re-submitting the keyword already on screen keeps them: there the form is the only record of
    // a sort or a filter the user has just picked, and dropping it would undo that choice.
    searchForm.addEventListener('submit', (e: Event) => {
      if (getKeyword(searchForm) && !isNewKeyword(searchForm)) return; // submit it natively
      e.preventDefault();
      const params = buildSearchParams(searchForm); // an empty keyword drops "q" and nothing else
      if (isNewKeyword(searchForm)) resetArrangement(params);
      window.location.search = params.toString();
    });
  }

  if (hasExploreTabs) initExploreTabSearches();
}
