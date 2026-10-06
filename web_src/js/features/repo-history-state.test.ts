import {parseSubjectLocation, pickInitialSelection, selectionFromHistoryState, withCarriedQuery, type InitialSelectionInput} from './repo-history-state.ts';

const alice = {owner: 'alice', repo: 'moon', subject: 'Moon', archived: false, link: '/subject/Moon/alice'};
const bob = {owner: 'bob', repo: 'moon', subject: 'Moon', archived: true, link: '/subject/Moon/bob/2'};
const candidates = [alice, bob];

const fresh: InitialSelectionInput = {serverArticle: undefined, historySelection: undefined, urlSelection: null, candidates};

test('#402: a subject opened afresh starts with nothing selected', () => {
  expect(pickInitialSelection(fresh)).toBeNull();
});

test('on the Article view, the article the server rendered is the selection', () => {
  expect(pickInitialSelection({...fresh, serverArticle: bob, historySelection: alice, urlSelection: alice})).toEqual(bob);
  // ...and when it rendered none, nothing is selected (the view asks for a selection)
  expect(pickInitialSelection({...fresh, serverArticle: null, historySelection: alice, urlSelection: alice})).toBeNull();
});

test('#405 items 2 and 5: the history entry restores what was selected when it was left', () => {
  expect(pickInitialSelection({...fresh, historySelection: {owner: 'bob', repo: 'moon'}})).toEqual(bob);
  // ...including nothing, even when the url says otherwise
  expect(pickInitialSelection({...fresh, historySelection: null, urlSelection: {owner: 'bob', repo: 'moon'}})).toBeNull();
});

test('#406: the selected parameter (carried by the compare page tabs) selects that article', () => {
  expect(pickInitialSelection({...fresh, urlSelection: {owner: 'Bob', repo: 'Moon'}})).toEqual(bob);
});

test('a parameter naming no article of the subject selects nothing', () => {
  expect(pickInitialSelection({...fresh, urlSelection: {owner: 'carol', repo: 'moon'}})).toBeNull();
});

test('#405 item 4: the only article of a subject is selected', () => {
  expect(pickInitialSelection({...fresh, candidates: [alice]})).toEqual(alice);
});

test('selectionFromHistoryState tells "no state" from "nothing selected"', () => {
  expect(selectionFromHistoryState(null)).toBeUndefined();
  expect(selectionFromHistoryState({} as any)).toBeUndefined();
  expect(selectionFromHistoryState({view: 'table', owner: null, repo: null})).toBeNull();
  expect(selectionFromHistoryState({view: 'table', owner: 'bob', repo: 'moon', subject: 'Moon', archived: true, link: '/subject/Moon/bob/2'})).toEqual(bob);
});

test('the view urls keep the rest of the query (the Table view\'s sort), not its state parameters', () => {
  const tableUrl = '/subject/Moon?view=table';
  expect(withCarriedQuery(tableUrl, '?view=table&sort=latest&selected=alice%2Fmoon')).toBe('/subject/Moon?view=table&sort=latest');
  expect(withCarriedQuery('/subject/Moon?view=bubble', '?view=article&mode=edit&compare=1&sort=most_contrib')).toBe('/subject/Moon?view=bubble&sort=most_contrib');
  // the url's own parameters win
  expect(withCarriedQuery('/subject/Moon?view=table&sort=latest', '?sort=least_contrib')).toBe('/subject/Moon?view=table&sort=latest');
  expect(withCarriedQuery(tableUrl, '')).toBe(tableUrl);
});

test('the urls of the subject page say which view and which article they show', () => {
  expect(parseSubjectLocation('/subject/Moon%20Landing?view=table', '')).toEqual({view: 'table', mode: 'read', owner: null, subject: null, repo: null});
  expect(parseSubjectLocation('/subject/Moon%20Landing/alice/2?mode=edit', '')).toEqual({view: 'article', mode: 'edit', owner: 'alice', subject: 'Moon Landing', repo: null});
  // the permanent article url (an archived article's) names its repository, on the
  // Article view unless the url says otherwise (#425 review 4)
  expect(parseSubjectLocation('/alice/moon-landing', '')).toEqual({view: 'article', mode: 'read', owner: 'alice', subject: null, repo: 'moon-landing'});
  expect(parseSubjectLocation('/explore/articles/history/alice/moon-landing', '')).toEqual({view: 'bubble', mode: 'read', owner: 'alice', subject: null, repo: 'moon-landing'});
  expect(parseSubjectLocation('/sub/alice/moon-landing?view=article&mode=history', '/sub')).toEqual({view: 'article', mode: 'history', owner: 'alice', subject: null, repo: 'moon-landing'});
});
