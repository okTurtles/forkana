import type {DOMEvent} from '../utils/dom.ts';

// Explore pages (repos, subjects, users) share the same borderless filter/sort search bar
// design, each rendered as its own <form> with one of these ids.
const searchFormSelectors = '#repo-search-form, #subject-search-form, #user-search-form';

export function initExploreSearch() {
  for (const searchForm of document.querySelectorAll<HTMLFormElement>(searchFormSelectors)) {
    searchForm.addEventListener('change', (e: DOMEvent<Event, HTMLInputElement>) => {
      e.preventDefault();

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
}
