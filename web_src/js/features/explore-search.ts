import {addDelegatedEventListener, queryElems} from '../utils/dom.ts';
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

// True while a click on an explore tab link is being processed. Clicking a tab blurs the search
// field, which fires its "change" event and would otherwise navigate to the current tab and cancel
// the tab switch.
let switchingTab = false;

// Update the "q" parameter of the explore tab links so switching tabs keeps whatever is currently
// in the search field, without relying on any browser storage (explore is available to anonymous
// users, and switching tabs is a plain server-rendered page load).
function updateTabLinks(keyword: string): void {
  queryElems<HTMLAnchorElement>(document, tabLinkSelector, (tabLink) => {
    const url = new URL(tabLink.href, window.location.href);
    if (keyword) {
      url.searchParams.set('q', keyword);
    } else {
      url.searchParams.delete('q');
    }
    tabLink.href = url.toString();
  });
}

export function initExploreSearch() {
  for (const searchForm of document.querySelectorAll<HTMLFormElement>(searchFormSelectors)) {
    searchForm.addEventListener('change', (e: DOMEvent<Event, HTMLInputElement>) => {
      e.preventDefault();

      // the tab link click that caused this change event takes precedence, that link already
      // carries the current keyword in its own URL
      if (switchingTab) return;

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

  if (!document.querySelector(tabLinkSelector)) return;

  queryElems<HTMLInputElement>(document, searchInputSelector, (searchInput) => {
    searchInput.addEventListener('input', () => updateTabLinks(searchInput.value.trim()));
  });

  // "pointerdown" fires before the search field loses focus, so the flag is set before the
  // resulting "change" event. It is reset in a timeout, which runs after that event.
  addDelegatedEventListener(document, 'pointerdown', tabLinkSelector, () => {
    switchingTab = true;
    setTimeout(() => {
      switchingTab = false;
    }, 0);
  });
}
