import {readStoredSelection, writeStoredSelection} from './repo-selection.ts';

// the test environment does not always provide window.localStorage, so give it a plain one
beforeEach(() => {
  const items = new Map<string, string>();
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => items.set(key, value),
    removeItem: (key: string) => items.delete(key),
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('writeStoredSelection does not persist the article link', () => {
  window.localStorage.setItem('selectedArticleLink', '/subject/foo/user2/2');
  writeStoredSelection({owner: 'user2', repo: 'foo-2', subject: 'foo', archived: true, link: '/subject/foo/user2/3'});
  expect(window.localStorage.getItem('selectedArticleLink')).toBeNull();
  expect(readStoredSelection()).toEqual({owner: 'user2', repo: 'foo-2', subject: 'foo', archived: true, link: null});
});

test('readStoredSelection ignores a link stored by an earlier version', () => {
  writeStoredSelection({owner: 'user2', repo: 'foo', subject: 'foo'});
  window.localStorage.setItem('selectedArticleLink', '/subject/foo/user2/2');
  expect(readStoredSelection()?.link).toBeNull();
});
