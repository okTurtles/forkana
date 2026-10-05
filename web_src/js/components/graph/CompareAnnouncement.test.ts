import {createApp, nextTick} from 'vue';
import CompareAnnouncement, {type CompareAnnouncementMessages} from './CompareAnnouncement.vue';
import {requestCompareMode, takeCompareModeRequest} from '../../modules/compare-mode-request.ts';

const messages: CompareAnnouncementMessages = {
  select: 'Select 2 articles to compare (0/2 selected).',
  one: '1/2 selected – select one more to compare.',
  ready: '2/2 selected – ready to compare.',
  unavailable: 'No forks yet. Compare needs at least 2 articles. Fork this article to start comparing.',
  compareNow: 'Compare now',
  dismiss: 'Exit compare mode',
  contributors: '%d contributors',
  contributor: '%d contributor',
};

const articles = [
  {id: '1', repoOwner: 'alice', repoName: 'moon', repoSubject: 'Moon', contributors: 335},
  {id: '2', repoOwner: 'bob', repoName: 'moon', repoSubject: 'Moon', contributors: 1},
];

function mount(props: Record<string, any>) {
  const root = document.createElement('div');
  document.body.append(root);
  const events: string[] = [];
  const app = createApp(CompareAnnouncement, {
    messages,
    ...props,
    onDismiss: () => events.push('dismiss'),
    onCompare: () => events.push('compare'),
  });
  app.mount(root);
  onTestFinished(() => {
    app.unmount();
    root.remove();
  });
  return {root, events};
}

const text = (root: HTMLElement) => root.querySelector('.compare-announcement-message')?.textContent.trim();

test('says what to do in each state, from the figma frames', () => {
  expect(text(mount({state: 'select'}).root)).toBe(messages.select);
  expect(text(mount({state: 'one'}).root)).toBe(messages.one);
  expect(text(mount({state: 'ready'}).root)).toBe(messages.ready);
  expect(text(mount({state: 'unavailable'}).root)).toBe(messages.unavailable);
});

test('with the Compare box beside the bubbles, the ready banner is just its message (figma hides "Compare now")', () => {
  const {root} = mount({state: 'ready', articles});
  expect(root.querySelector('.compare-announcement-primary')).toBeNull();
  expect(root.querySelectorAll('li')).toHaveLength(0);
});

test('expanded (a phone, or no room for the box), it carries both articles and "Compare now"', async () => {
  const {root, events} = mount({state: 'ready', articles, expanded: true});
  expect(Array.from(root.querySelectorAll('li'), (li) => li.textContent.replace(/\s+/g, ' ').trim())).toEqual([
    'alice / Moon335 contributors',
    'bob / Moon1 contributor',
  ]);
  root.querySelector<HTMLButtonElement>('.compare-announcement-primary').click();
  await nextTick();
  expect(events).toEqual(['compare']);
});

test('expanded only matters once two articles are picked', () => {
  const {root} = mount({state: 'one', articles: articles.slice(0, 1), expanded: true});
  expect(root.querySelector('.compare-announcement-primary')).toBeNull();
  expect(root.querySelectorAll('li')).toHaveLength(0);
});

test('the x dismisses it', async () => {
  const {root, events} = mount({state: 'select'});
  const x = root.querySelector<HTMLButtonElement>('.compare-announcement-dismiss');
  expect(x.getAttribute('aria-label')).toBe('Exit compare mode');
  x.click();
  await nextTick();
  expect(events).toEqual(['dismiss']);
});

test('a Compare press made before the bubble view mounted is taken exactly once', () => {
  expect(takeCompareModeRequest()).toBe(false);
  requestCompareMode();
  expect(takeCompareModeRequest()).toBe(true);
  expect(takeCompareModeRequest()).toBe(false);
});
