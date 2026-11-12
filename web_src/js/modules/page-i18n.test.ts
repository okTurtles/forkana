import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {formatMessage, isSingular} from './page-i18n.ts';
import {bubbleMessages} from '../components/graph/bubble-i18n.ts';
import {conflictMessages} from '../features/repo-conflict-i18n.ts';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '../../..');
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

/* Each list of script strings, with the template that sends their translations. */
const lists: Array<[string, Record<string, string>, string]> = [
  ['Bubble view', bubbleMessages, 'custom/templates/shared/repo/bubble.tmpl'],
  ['conflict resolution', conflictMessages, 'custom/templates/repo/diff/conflicts_box.tmpl'],
];

test.each(lists)('%s: every string has its custom en-US text as fallback', (_name, messages) => {
  const english = customEnglish();
  for (const [key, fallback] of Object.entries(messages)) {
    expect({key, english: english.get(key)}).toEqual({key, english: fallback});
  }
});

test.each(lists)('%s: the template sends every string, and only those', (_name, messages, template) => {
  const sent = Array.from(repoFile(template).matchAll(/"([\w.]+)" \(ctx\.Locale\.Tr "([\w.]+)"\)/g));
  for (const [, name, key] of sent) expect(name).toBe(key);
  expect(sent.map(([, name]) => name).sort()).toEqual(Object.keys(messages).sort());
});

test('formatMessage fills Go verbs', () => {
  expect(formatMessage('%s Contributors', [3])).toBe('3 Contributors');
  expect(formatMessage('%[2]s の %[1]s', ['a', 'b'])).toBe('b の a');
  expect(formatMessage('%[1]d of %[2]d conflicts', [1, 3])).toBe('1 of 3 conflicts');
  expect(formatMessage('100%% sure', [])).toBe('100% sure');
});

test('isSingular follows the page language', () => {
  document.documentElement.lang = 'fr-FR';
  expect(isSingular(0)).toBe(true);
  expect(isSingular(2)).toBe(false);
  document.documentElement.lang = 'en-US';
  expect(isSingular(0)).toBe(false);
  expect(isSingular(1)).toBe(true);
  document.documentElement.lang = '';
});
