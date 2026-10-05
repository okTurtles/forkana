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
};

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

test('it is only ever a message: the articles live in the Compare box', () => {
  const {root} = mount({state: 'ready', showCompareNow: true});
  expect(root.querySelectorAll('li, a')).toHaveLength(0);
});

test('"Compare now" is offered once two are picked and the Compare box is closed (figma hides it while open)', async () => {
  expect(mount({state: 'ready'}).root.querySelector('.compare-announcement-primary')).toBeNull();
  expect(mount({state: 'one', showCompareNow: true}).root.querySelector('.compare-announcement-primary')).toBeNull();
  const {root, events} = mount({state: 'ready', showCompareNow: true});
  root.querySelector<HTMLButtonElement>('.compare-announcement-primary').click();
  await nextTick();
  expect(events).toEqual(['compare']);
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
