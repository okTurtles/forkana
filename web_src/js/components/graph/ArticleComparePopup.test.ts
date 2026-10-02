import {createApp} from 'vue';
import ArticleComparePopup from './ArticleComparePopup.vue';

function mountPopup(articles: Array<Record<string, any>>, subject = 'Moon Landing'): HTMLElement {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(ArticleComparePopup, {articles, subject});
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
