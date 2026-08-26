import {highlightKeyword} from './home-search.ts';

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
