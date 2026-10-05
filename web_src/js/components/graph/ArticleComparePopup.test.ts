import {createApp} from 'vue';
import ArticleComparePopup from './ArticleComparePopup.vue';

function mountPopup(articles: Array<Record<string, any>>, subject = 'Moon Landing', extra: Record<string, any> = {}): HTMLElement {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(ArticleComparePopup, {articles, subject, ...extra});
  app.mount(root);
  onTestFinished(() => {
    app.unmount();
    root.remove();
  });
  return root;
}

function articleHrefs(root: HTMLElement): string[] {
  return Array.from(root.querySelectorAll<HTMLAnchorElement>('.compare-article-name'), (a) => a.getAttribute('href') || '');
}

test('links to the article url the server built', () => {
  const root = mountPopup([
    {id: '1', repoOwner: 'user2', repoName: 'moon-landing', repoSubject: 'Moon Landing', articleLink: '/sub/subject/Moon%20Landing/user2/2', contributors: 1, children: []},
  ]);
  expect(articleHrefs(root)).toEqual(['/sub/subject/Moon%20Landing/user2/2']);
});

test('falls back to the permanent repository url, not the subject', () => {
  const appSubUrl = window.config.appSubUrl;
  window.config.appSubUrl = '/sub';
  onTestFinished(() => {
    window.config.appSubUrl = appSubUrl;
  });
  const root = mountPopup([
    {id: '1', repoOwner: 'user2', repoName: 'moon-landing-2', repoSubject: 'Moon Landing', contributors: 1, children: []},
    {id: '2', fullName: 'user 4/moon-landing', contributors: 1, children: []},
  ]);
  expect(articleHrefs(root)).toEqual(['/sub/user2/moon-landing-2', '/sub/user%204/moon-landing']);
});

const twoArticles = [
  {id: '1', repoOwner: 'alice', repoName: 'moon', contributors: 335, children: ['x'], updatedAt: '2025-06-10T00:00:00Z'},
  {id: '2', repoOwner: 'bob', repoName: 'moon', contributors: 1, children: [], updatedAt: '2025-06-11T00:00:00Z'},
];

test('is a popover, not a modal: no backdrop', () => {
  const root = mountPopup(twoArticles);
  expect(root.querySelector('.compare-popover')?.getAttribute('aria-modal')).toBe('false');
  expect(root.querySelector('.compare-popup-overlay')).toBeNull();
});

test('beside the bubbles it has a caret on the facing edge, at the given height; under the graph it has none', () => {
  const right = mountPopup(twoArticles, 'Moon', {placement: 'right', caretY: 120});
  const caret = right.querySelector<SVGElement>('.compare-popover-caret');
  expect(caret).not.toBeNull();
  expect(caret.style.top).toBe('113px');   // centred: 120 - 14 / 2
  expect(right.querySelector('.compare-popover.is-right')).not.toBeNull();
  const below = mountPopup(twoArticles, 'Moon', {placement: 'below'});
  expect(below.querySelector('.compare-popover-caret')).toBeNull();
});

test('each article shows its details, which its chevron folds and unfolds', async () => {
  const root = mountPopup(twoArticles);
  const details = () => Array.from(root.querySelectorAll('.compare-article-meta'), (el) => Array.from(el.children, (line) => line.textContent.trim()).join(' '));
  expect(details()).toEqual([
    '335 Contributors 1 Fork Last updated: 2025-06-10',
    '1 Contributor 0 Forks Last updated: 2025-06-11',
  ]);
  root.querySelector<HTMLButtonElement>('.compare-popover-details .compare-popover-chevron').click();
  await Promise.resolve();
  expect(details()).toEqual(['1 Contributor 0 Forks Last updated: 2025-06-11']);
  const unfold = root.querySelector<HTMLButtonElement>('.compare-popover-row .compare-popover-chevron');
  expect(unfold.getAttribute('aria-expanded')).toBe('false');
  unfold.click();
  await Promise.resolve();
  expect(details()).toHaveLength(2);
});
