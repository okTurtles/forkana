import {
  canPickForCompare, compareAnnouncementFor, compareAvailableFor, replayCompareModeRequest, requestCompareMode,
  resetCompareModeRequest, takeCompareModeRequest,
} from './compare-mode-request.ts';

// the request is module state: every test starts without one
beforeEach(() => {
  resetCompareModeRequest();
});

test('a Compare press made before the bubble view mounted is taken exactly once', () => {
  expect(takeCompareModeRequest()).toBe(false);
  requestCompareMode();
  expect(takeCompareModeRequest()).toBe(true);
  expect(takeCompareModeRequest()).toBe(false);
});

test('a Compare press that mounted the graph is replayed once the graph is loaded, not before', async () => {
  const calls: string[] = [];
  let loaded = false;
  const load = async () => {
    calls.push('load');
    await Promise.resolve();
    loaded = true;
  };
  const press = () => calls.push(loaded ? 'press after load' : 'press before load');

  requestCompareMode();
  await replayCompareModeRequest(load, press);
  expect(calls).toEqual(['load', 'press after load']);
  expect(takeCompareModeRequest()).toBe(false);

  // no request, no press
  calls.length = 0;
  loaded = false;
  await replayCompareModeRequest(load, press);
  expect(calls).toEqual(['load']);
});

test('compare mode is offered while loading, after a failed load, and with two live articles', () => {
  expect(compareAvailableFor({loading: true, failed: false, liveArticles: 0})).toBe(true);
  // a graph that failed to load says nothing about the subject: not "No forks yet"
  expect(compareAvailableFor({loading: false, failed: true, liveArticles: 0})).toBe(true);
  expect(compareAvailableFor({loading: false, failed: false, liveArticles: 2})).toBe(true);
  // one live article (a deleted root and one fork, say) has nothing to compare
  expect(compareAvailableFor({loading: false, failed: false, liveArticles: 1})).toBe(false);
});

test('a deleted article or the hidden root cannot be picked for a comparison', () => {
  expect(canPickForCompare({})).toBe(true);
  expect(canPickForCompare({isTombstoned: true})).toBe(false);
  expect(canPickForCompare({isHidden: true, isTombstoned: true})).toBe(false);
});

test('a compare action is announced once, the picked article first, then the banner\'s message', () => {
  expect(compareAnnouncementFor('alice/moon', '1/2 selected – select one more to compare.'))
    .toBe('alice/moon: 1/2 selected – select one more to compare.');
  expect(compareAnnouncementFor(null, 'Select 2 articles to compare (0/2 selected).'))
    .toBe('Select 2 articles to compare (0/2 selected).');
});
