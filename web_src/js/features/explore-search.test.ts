import {buildSearchParams, initExploreSearch} from './explore-search.ts';

// Where the feature navigated to, captured instead of letting happy-dom leave the page under test.
let navigatedTo: string | null = null;

beforeEach(() => {
  navigatedTo = null;
  Object.defineProperty(window.location, 'search', {
    configurable: true,
    get: () => navigatedTo ?? '',
    set: (value: string) => { navigatedTo = value },
  });
});

afterEach(() => {
  delete (window.location as any).search;
});

// A search bar as the explore pages render it: the keyword field carries the keyword the page was
// rendered for as its "value" attribute (so the field's defaultValue), and the sort and the filters
// are radios inside that same form, checked to match the results currently on screen.
function mountSearchForm(renderedKeyword: string): HTMLFormElement {
  document.body.innerHTML = `
    <form id="subject-search-form">
      <input type="search" name="q" value="${renderedKeyword}">
      <input type="radio" name="sort" value="oldest" checked>
      <input type="radio" name="fork" value="1" checked>
      <button type="submit">Search</button>
    </form>
  `;
  const searchForm = document.querySelector<HTMLFormElement>('#subject-search-form');
  initExploreSearch();
  return searchForm;
}

// Type a keyword into the mounted form and press Enter. Returns the submit event, whose
// defaultPrevented tells whether the feature took over or left the submit to the browser.
function submitKeyword(searchForm: HTMLFormElement, keyword: string): Event {
  (searchForm.elements.namedItem('q') as HTMLInputElement).value = keyword;
  const e = new Event('submit', {bubbles: true, cancelable: true});
  searchForm.dispatchEvent(e);
  return e;
}

test('a changed keyword searches from scratch', () => {
  const searchForm = mountSearchForm('mars');
  // the form does hold the sort and the filter of the results on screen
  const rendered = buildSearchParams(searchForm);
  expect(rendered.get('sort')).toEqual('oldest');
  expect(rendered.get('fork')).toEqual('1');

  const e = submitKeyword(searchForm, 'forest');
  // the native submit, which would have carried that arrangement over, is cancelled
  expect(e.defaultPrevented).toBe(true);
  expect(navigatedTo).toEqual('q=forest');
});

test('re-submitting the same keyword is left to the browser', () => {
  const searchForm = mountSearchForm('mars');
  // a sort or a filter just picked in this form is only recorded here, so it has to be submitted
  expect(submitKeyword(searchForm, 'mars').defaultPrevented).toBe(false);
  expect(navigatedTo).toBe(null);
  // surrounding whitespace does not make it a different keyword either
  expect(submitKeyword(searchForm, '  mars  ').defaultPrevented).toBe(false);
  expect(navigatedTo).toBe(null);
});

test('clearing the keyword clears the search, not the arrangement', () => {
  const searchForm = mountSearchForm('mars');
  const e = submitKeyword(searchForm, '');
  expect(e.defaultPrevented).toBe(true);
  // "q" is dropped rather than submitted empty, and the sort and the filters survive
  const params = new URLSearchParams(navigatedTo);
  expect(params.has('q')).toBe(false);
  expect(params.get('sort')).toEqual('oldest');
  expect(params.get('fork')).toEqual('1');
});
