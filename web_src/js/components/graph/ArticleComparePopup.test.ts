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

test('beside the bubbles it is a popover, not a modal; the phone sheet is modal', () => {
  const side = mountPopup(twoArticles, 'Moon', {placement: 'right'});
  expect(side.querySelector('.compare-popover')?.getAttribute('aria-modal')).toBe('false');
  const sheet = mountPopup(twoArticles, 'Moon', {placement: 'sheet'});
  expect(sheet.querySelector('.compare-popover.is-sheet')?.getAttribute('aria-modal')).toBe('true');
});

test('beside the bubbles it has a caret on the facing edge, at the given height; the sheet has none', () => {
  const right = mountPopup(twoArticles, 'Moon', {placement: 'right', caretY: 120});
  const caret = right.querySelector<SVGElement>('.compare-popover-caret');
  expect(caret).not.toBeNull();
  expect(caret.style.top).toBe('113px');   // centred: 120 - 14 / 2
  expect(right.querySelector('.compare-popover.is-right')).not.toBeNull();
  const sheet = mountPopup(twoArticles, 'Moon', {placement: 'sheet'});
  expect(sheet.querySelector('.compare-popover-caret')).toBeNull();
});

test('the details are two semibold count lines and an italic date line, which the chevron folds', async () => {
  const root = mountPopup(twoArticles);
  const details = () => Array.from(root.querySelectorAll('.compare-article-meta'), (el) => Array.from(el.children, (line) => `${line.className}:${line.textContent.trim()}`));
  expect(details()).toEqual([
    ['compare-article-count:335 Contributors', 'compare-article-count:1 Fork', 'compare-article-date:Last updated: 2025-06-10'],
    ['compare-article-count:1 Contributor', 'compare-article-count:0 Forks', 'compare-article-date:Last updated: 2025-06-11'],
  ]);
  root.querySelector<HTMLButtonElement>('.compare-popover-details .compare-popover-chevron').click();
  await Promise.resolve();
  expect(details()).toHaveLength(1);
  const unfold = root.querySelector<HTMLButtonElement>('.compare-popover-row .compare-popover-chevron');
  expect(unfold.getAttribute('aria-expanded')).toBe('false');
  unfold.click();
  await Promise.resolve();
  expect(details()).toHaveLength(2);
});
