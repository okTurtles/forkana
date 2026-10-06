import {
  compareAvailableFor, replayCompareModeRequest, requestCompareMode, resetCompareModeRequest, takeCompareModeRequest,
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
