import {queryElems} from '../utils/dom.ts';
import type {DOMEvent} from '../utils/dom.ts';

// Explore pages (repos, subjects, users) share the same borderless filter/sort search bar
// design, each rendered as its own <form> with one of these ids.
const searchFormSelectors = '#repo-search-form, #subject-search-form, #user-search-form';

// Tab links of the explore navbar (custom/templates/explore/navbar.tmpl). They are server-rendered
// with the submitted keyword already appended as "?q=...", this selector is used to also carry over
// a keyword that has been typed but not submitted yet.
const tabLinkSelector = 'a[data-explore-tab-link]';

// Search field of the explore page, every tab searches with the same "q" parameter.
const searchInputSelector = '.page-content.explore input[type="search"][name="q"]';

// Update the "q" parameter of the explore tab links so switching tabs keeps whatever is currently
// in the search field, without relying on any browser storage (explore is available to anonymous
// users, and switching tabs is a plain server-rendered page load).
function updateTabLinks(keyword: string): void {
  queryElems<HTMLAnchorElement>(document, tabLinkSelector, (tabLink) => {
    // the "href" property of an anchor is already resolved against the document
    const url = new URL(tabLink.href);
    if (keyword) {
      url.searchParams.set('q', keyword);
    } else {
      url.searchParams.delete('q');
    }
    tabLink.href = url.toString();
  });
}

// The filters offered beside the search field. The "clear filters" button drops exactly these.
const filterParamNames = ['archived', 'fork', 'mirror', 'template', 'private', 'repo_role'];

// Everything that arranges the results rather than selects them: the filters, the sort order, and
// the page reached within that arrangement. All of it is rendered inside the same form as the
// keyword field, so a native submit carries it along with whatever has just been typed.
const arrangementParamNames = [...filterParamNames, 'sort', 'page'];

// Return the results to the way the plain, unfiltered page arranges them.
function resetArrangement(params: URLSearchParams): void {
  for (const name of arrangementParamNames) params.delete(name);
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
  // The explore navbar carries the keyword over in its tab links (see updateTabLinks), which also
  // changes how the keyword field itself is submitted, see below. The same search bar is rendered
  // without that navbar on other pages (profiles, organizations, the admin repository list).
  const tabsCarryKeyword = Boolean(document.querySelector(tabLinkSelector));

  for (const searchForm of document.querySelectorAll<HTMLFormElement>(searchFormSelectors)) {
    searchForm.addEventListener('change', (e: DOMEvent<Event, HTMLInputElement>) => {
      e.preventDefault();

      // The keyword field fires "change" once it loses focus with an edited value, no matter what
      // the user does next: click a tab, move the focus towards one with the keyboard (which first
      // steps through the search button), or click anywhere else. Reloading the current tab from
      // here cancels that tab switch and takes the focus with it, so on the explore pages the
      // keyword field keeps its own submit paths (the Enter key and the search button) and the tab
      // links carry whatever has been typed over themselves.
      if (tabsCarryKeyword && e.target.name === 'q') return;

      const params = buildSearchParams(searchForm);
      if (e.target.name === 'clear-filter') {
        for (const name of filterParamNames) params.delete(name);
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
      const keyword = getKeyword(searchForm);
      if (keyword && keyword === getRenderedKeyword(searchForm)) return; // submit it natively
      e.preventDefault();
      const params = buildSearchParams(searchForm); // an empty keyword drops "q" and nothing else
      if (keyword) resetArrangement(params);
      window.location.search = params.toString();
    });
  }

  if (!tabsCarryKeyword) return;

  queryElems<HTMLInputElement>(document, searchInputSelector, (searchInput) => {
    searchInput.addEventListener('input', () => updateTabLinks(searchInput.value.trim()));
  });
}
