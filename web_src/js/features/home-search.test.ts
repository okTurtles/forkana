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

test('search button appears only while the field holds text', () => {
  document.body.innerHTML = `
    <form id="home-search-form">
      <input id="home-search-input" name="q" type="text"/>
      <button id="home-search-button" class="tw-hidden">Search</button>
    </form>
    <div id="home-search-suggestions" class="tw-hidden"></div>
  `;
  initHomeSearch();
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
