import {
  clearLegacyStoredSelection, matchesSelection, normalizeSelection, resolveSelection,
  selectionFromParam, selectionToParam, withSelectionParam,
} from './repo-selection.ts';

const candidates = [
  {owner: 'alice', repo: 'moon', subject: 'Moon', archived: false, link: '/subject/Moon/alice'},
  {owner: 'bob', repo: 'moon', subject: 'Moon', archived: true, link: '/subject/Moon/bob/2'},
];

test('the url parameter round-trips owner and repo', () => {
  expect(selectionToParam({owner: 'bob', repo: 'moon'})).toBe('bob/moon');
  expect(selectionFromParam('bob/moon')).toEqual({owner: 'bob', repo: 'moon', subject: null, archived: false, link: null});
});

test('a malformed url parameter selects nothing', () => {
  for (const value of [null, '', 'bob', '/moon', 'bob/', 'bob/moon/extra']) {
    expect(selectionFromParam(value)).toBeNull();
  }
});

test('withSelectionParam sets or removes the parameter and keeps the rest of the query', () => {
  expect(withSelectionParam('/subject/Moon?view=table', {owner: 'bob', repo: 'moon'})).toBe('/subject/Moon?view=table&selected=bob%2Fmoon');
  expect(withSelectionParam('/subject/Moon?view=table&selected=bob%2Fmoon', null)).toBe('/subject/Moon?view=table');
  expect(withSelectionParam('/subject/Moon?selected=alice%2Fmoon&view=bubble', {owner: 'bob', repo: 'moon'})).toBe('/subject/Moon?selected=bob%2Fmoon&view=bubble');
});

test('matchesSelection ignores the case of owner and repo names', () => {
  expect(matchesSelection({owner: 'Bob', repo: 'Moon'}, {owner: 'bob', repo: 'moon'})).toBe(true);
  expect(matchesSelection({owner: 'bob', repo: 'moon'}, {owner: 'alice', repo: 'moon'})).toBe(false);
  expect(matchesSelection(null, {owner: 'alice', repo: 'moon'})).toBe(false);
});

test('resolveSelection completes a selection from the subject articles', () => {
  expect(resolveSelection({owner: 'BOB', repo: 'moon'}, candidates)).toEqual(candidates[1]);
  expect(resolveSelection({owner: 'carol', repo: 'moon'}, candidates)).toBeNull();
  expect(resolveSelection(null, candidates)).toBeNull();
});

test('normalizeSelection rejects a selection without an article', () => {
  expect(normalizeSelection({owner: '', repo: 'moon'})).toBeNull();
  expect(normalizeSelection({owner: 'bob', repo: '', subject: 'Moon'})).toEqual({owner: 'bob', repo: 'Moon', subject: 'Moon', archived: false, link: null});
});

test('clearLegacyStoredSelection removes the selection earlier versions stored', () => {
  const items = new Map<string, string>([['selectedArticleOwner', 'bob'], ['selectedArticleRepo', 'moon'], ['other', 'kept']]);
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => items.set(key, value),
    removeItem: (key: string) => items.delete(key),
  });
  try {
    clearLegacyStoredSelection();
    expect(Array.from(items.keys())).toEqual(['other']);
  } finally {
    vi.unstubAllGlobals();
  }
});
