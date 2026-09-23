import {highlightKeyword, initHomeSearch} from './home-search.ts';

test('highlightKeyword', () => {
  expect(highlightKeyword('Moons of Saturn', 'Moon')).toEqual('<b>Moon</b>s of Saturn');
  // the match is case-insensitive, but the subject keeps its own spelling
  expect(highlightKeyword('Moonshine', 'moon')).toEqual('<b>Moon</b>shine');
  // subjects that only contain the keyword are highlighted where they match
  expect(highlightKeyword('Full Moon Party', 'moon')).toEqual('Full <b>Moon</b> Party');
  // a subject matched by similarity rather than by substring stays as it is
  expect(highlightKeyword('Moonshine', 'mooon')).toEqual('Moonshine');
  expect(highlightKeyword('Moonshine', '')).toEqual('Moonshine');
  // names are escaped, they end up in the dropdown as HTML
  expect(highlightKeyword('<script>', 'scr')).toEqual('&lt;<b>scr</b>ipt&gt;');
});

// One DOM per test: setting the value BEFORE init is how a browser restoring a
// keyword on back-navigation presents the page to the script.
function initHomeSearchDom(keyword = ''): void {
  document.body.innerHTML = `
    <form id="home-search-form">
      <input id="home-search-input" name="q" type="text"/>
      <button id="home-search-button" class="tw-hidden">Search</button>
    </form>
    <div id="home-search-suggestions" class="tw-hidden"></div>
  `;
  document.querySelector<HTMLInputElement>('#home-search-input').value = keyword;
  initHomeSearch();
}

test('search button appears only while the field holds text', () => {
  initHomeSearchDom();
  const input = document.querySelector<HTMLInputElement>('#home-search-input');
  const button = document.querySelector<HTMLElement>('#home-search-button');

  input.value = 'mars';
  input.dispatchEvent(new Event('input'));
  expect(button.classList.contains('tw-hidden')).toBe(false);
  expect(input.classList.contains('home-search-input-with-button')).toBe(true);

  // whitespace alone is not a keyword, the empty-search validation would reject it anyway
  input.value = '   ';
  input.dispatchEvent(new Event('input'));
  expect(button.classList.contains('tw-hidden')).toBe(true);
  expect(input.classList.contains('home-search-input-with-button')).toBe(false);
});

test('search button is shown right away when the browser restored a keyword', () => {
  initHomeSearchDom('mars');
  expect(document.querySelector('#home-search-button').classList.contains('tw-hidden')).toBe(false);
});
