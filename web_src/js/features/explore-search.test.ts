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
// The explore pages also render the tab navbar, which carries the keyword over in its links; the
// profile and admin pages render the same search bar without it. Pass withTabs for the former.
function mountSearchForm(renderedKeyword: string, withTabs: boolean = false): HTMLFormElement {
  document.body.innerHTML = `
    ${withTabs ? '<a data-explore-tab-link href="/explore/subjects">Subjects</a>' : ''}
    <form id="subject-search-form">
      <input type="search" name="q" value="${renderedKeyword}">
      <input type="radio" name="sort" value="newest">
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

// Pick one of the form's radios and let it fire the "change" the browser fires on a click.
function pickRadio(searchForm: HTMLFormElement, name: string, value: string): void {
  const radio = searchForm.querySelector<HTMLInputElement>(`input[name="${name}"][value="${value}"]`);
  radio.checked = true;
  radio.dispatchEvent(new Event('change', {bubbles: true}));
}

test('a sort picked while a new keyword is typed searches from scratch too', () => {
  const searchForm = mountSearchForm('mars', true);
  (searchForm.elements.namedItem('q') as HTMLInputElement).value = 'forest';

  pickRadio(searchForm, 'sort', 'newest');
  const params = new URLSearchParams(navigatedTo);
  expect(params.get('q')).toEqual('forest');
  // the sort just picked is the point of the gesture, the previous filters are not
  expect(params.get('sort')).toEqual('newest');
  expect(params.has('fork')).toBe(false);
});

test('a sort picked without touching the keyword keeps the filters', () => {
  const searchForm = mountSearchForm('mars', true);

  pickRadio(searchForm, 'sort', 'newest');
  const params = new URLSearchParams(navigatedTo);
  expect(params.get('q')).toEqual('mars');
  expect(params.get('sort')).toEqual('newest');
  expect(params.get('fork')).toEqual('1');
});

test('leaving an edited keyword field searches from scratch where that navigates', () => {
  // without the explore navbar (profiles, the admin repository list) the keyword field navigates
  // on "change", and that navigation is a new search like any other
  const searchForm = mountSearchForm('mars');
  const keywordField = searchForm.elements.namedItem('q') as HTMLInputElement;
  keywordField.value = 'forest';

  keywordField.dispatchEvent(new Event('change', {bubbles: true}));
  expect(navigatedTo).toEqual('q=forest');
});

test('the clear filters button still keeps the sort', () => {
  const searchForm = mountSearchForm('mars', true);
  searchForm.insertAdjacentHTML('beforeend', '<input type="radio" name="clear-filter" value="">');
  (searchForm.elements.namedItem('q') as HTMLInputElement).value = 'forest';

  pickRadio(searchForm, 'clear-filter', '');
  const params = new URLSearchParams(navigatedTo);
  expect(params.has('fork')).toBe(false);
  expect(params.get('sort')).toEqual('oldest');
  expect(params.has('clear-filter')).toBe(false);
});
