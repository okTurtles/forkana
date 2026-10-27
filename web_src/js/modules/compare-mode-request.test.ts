import {
  canPickForCompare, compareAnnouncementFor, compareAvailableFor, replayCompareModeRequest, requestCompareMode,
  readCompareMessages, resetCompareModeRequest, subjectCompareUrl, takeCompareModeRequest,
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

test('the banner messages come from the page, with overrides and English fallbacks', () => {
  const root = document.createElement('div');
  root.setAttribute('data-msg-one', 'one picked');
  root.setAttribute('data-msg-no-forks', 'no forks');
  const messages = readCompareMessages(root, {one: `the table's one`, ready: ''});
  expect(messages.one).toBe(`the table's one`);
  expect(messages.unavailable).toBe('no forks');
  expect(messages.ready).toBe('2/2 selected – ready to compare.');
  expect(readCompareMessages(null).dismissNotice).toBe('Dismiss');
});

test('"No forks yet" links "Fork this article" to the page\'s fork url (#451)', () => {
  const root = document.createElement('div');
  root.setAttribute('data-msg-no-forks', 'No forks yet. %s to start comparing.');
  root.setAttribute('data-msg-fork-link', 'Fork $& this');
  root.setAttribute('data-fork-url', '/user/login?redirect_to=%2Fsubject%2FMoon');
  const messages = readCompareMessages(root);
  // the plain message (the screen reader's) has the link's text in place of the %s
  expect(messages.unavailable).toBe('No forks yet. Fork $& this to start comparing.');
  expect(messages.unavailableLink).toEqual({
    before: 'No forks yet. ', text: 'Fork $& this', href: '/user/login?redirect_to=%2Fsubject%2FMoon', after: ' to start comparing.',
  });
  // no fork url on the page: plain text
  root.removeAttribute('data-fork-url');
  expect(readCompareMessages(root).unavailableLink).toBeNull();
  expect(readCompareMessages(null).unavailable).toBe('No forks yet. Compare needs at least 2 articles. Fork this article to start comparing.');
});

test('a translation without exactly one %s gets no link, and no literal %s', () => {
  const root = document.createElement('div');
  root.setAttribute('data-fork-url', '/subject/Moon/alice?view=article&mode=edit');
  root.setAttribute('data-msg-no-forks', 'No forks. %s, really %s.');
  expect(readCompareMessages(root).unavailableLink).toBeNull();
  expect(readCompareMessages(root).unavailable).toBe('No forks. Fork this article, really Fork this article.');
  root.setAttribute('data-msg-no-forks', 'No forks yet.');
  expect(readCompareMessages(root).unavailableLink).toBeNull();
  expect(readCompareMessages(root).unavailable).toBe('No forks yet.');
});

test(`the compare page of two owners' articles`, () => {
  expect(subjectCompareUrl('/sub', 'Moon Landing', 'jane z', 'peter')).toBe('/sub/subject/Moon%20Landing/compare/jane%20z...peter');
});
