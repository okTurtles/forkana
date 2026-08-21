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

      const params = new URLSearchParams();
      for (const [key, value] of new FormData(searchForm).entries()) {
        params.set(key, value.toString());
      }
      if (e.target.name === 'clear-filter') {
        params.delete('archived');
        params.delete('fork');
        params.delete('mirror');
        params.delete('template');
        params.delete('private');
        params.delete('repo_role');
      }

      params.delete('clear-filter');
      window.location.search = params.toString();
    });
  }

  if (!tabsCarryKeyword) return;

  queryElems<HTMLInputElement>(document, searchInputSelector, (searchInput) => {
    searchInput.addEventListener('input', () => updateTabLinks(searchInput.value.trim()));
  });
}
