import {pickInitialSelection, selectionFromHistoryState, type InitialSelectionInput} from './repo-history-state.ts';

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
