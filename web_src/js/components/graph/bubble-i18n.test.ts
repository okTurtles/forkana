import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {bubbleMessages, formatMessage, resetBubbleMessages, t, tn} from './bubble-i18n.ts';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '../../../..');
const repoFile = (rel: string) => readFileSync(join(repoRoot, rel), 'utf8');

/* custom en-US as Gitea reads it: "section.key" -> value, wrapping quotes stripped. */
function customEnglish(): Map<string, string> {
  const keys = new Map<string, string>();
  let section = '';
  for (const raw of repoFile('custom/options/locale/locale_en-US.ini').split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#') || line.startsWith(';')) continue;
    if (line.startsWith('[')) {
      section = line.slice(1, -1).trim();
      continue;
    }
    const eq = line.indexOf('=');
    const key = line.slice(0, eq).trim();
    let value = line.slice(eq + 1).trim();
    if (value.length >= 2 && (value[0] === '`' || value[0] === '"') && value.at(-1) === value[0]) value = value.slice(1, -1);
    keys.set(section ? `${section}.${key}` : key, value);
  }
  return keys;
}

test('every string has its custom en-US text as fallback', () => {
  const english = customEnglish();
  for (const [key, fallback] of Object.entries(bubbleMessages)) {
    expect({key, english: english.get(key)}).toEqual({key, english: fallback});
  }
});

test('the bubble template sends every string, and only those', () => {
  const template = repoFile('custom/templates/shared/repo/bubble.tmpl');
  const sent = Array.from(template.matchAll(/"([\w.]+)" \(ctx\.Locale\.Tr "([\w.]+)"\)/g));
  for (const [, name, key] of sent) expect(name).toBe(key);
  expect(sent.map(([, name]) => name).sort()).toEqual(Object.keys(bubbleMessages).sort());
});

test('formatMessage fills Go verbs', () => {
  expect(formatMessage('%s Contributors', [3])).toBe('3 Contributors');
  expect(formatMessage('%[2]s の %[1]s', ['a', 'b'])).toBe('b の a');
  expect(formatMessage('Selected %[1]s with %[2]s', ['x', '2 contributors'])).toBe('Selected x with 2 contributors');
  expect(formatMessage('100%% sure', [])).toBe('100% sure');
});

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
