import {resetBubbleMessages, t, tn} from './bubble-i18n.ts';

// The list's fallbacks and template are checked in ../../modules/page-i18n.test.ts.

test('t and tn read the page translations and its plural rules', () => {
  document.body.innerHTML = `<div id="bubble-view-root" data-i18n='${JSON.stringify({
    'repo.bubble.contributor_1': 'Contributeur',
    'repo.bubble.contributor_n': 'Contributeurs',
  })}'></div>`;
  document.documentElement.lang = 'fr-FR';
  resetBubbleMessages();
  expect(tn(1, 'repo.bubble.contributor_1', 'repo.bubble.contributor_n')).toBe('Contributeur');
  expect(tn(0, 'repo.bubble.contributor_1', 'repo.bubble.contributor_n')).toBe('Contributeur'); // French: 0 is singular
  expect(tn(2, 'repo.bubble.contributor_1', 'repo.bubble.contributor_n')).toBe('Contributeurs');
  expect(t('repo.bubble.legend_article')).toBe('Article'); // not sent: the English
  document.documentElement.lang = '';
  document.body.innerHTML = '';
  resetBubbleMessages();
  expect(tn(0, 'repo.bubble.contributor_1', 'repo.bubble.contributor_n')).toBe('Contributors');
});
