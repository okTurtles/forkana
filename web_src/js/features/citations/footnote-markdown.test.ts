import {
  appendFootnoteDefinition,
  footnoteDefinitions,
  footnoteSources,
  nextFootnoteLabel,
  plainCitationSummary,
  referenceNumberAt,
  referencedLabels,
} from './footnote-markdown.ts';

const article = [
  'Luna 2 reached the Moon first.[^luna] Apollo 11 landed in 1969.[^1]',
  '',
  'Apollo 11 again.[^1] Inline `[^code]` is not a reference.',
  '',
  '```',
  '[^fenced]: not a definition, nor [^fenced] a reference',
  '```',
  '',
  '[^1]: ["Manned Space Chronology: Apollo_11"](https://spaceline.org/x). *spaceline.org*. Retrieved 6 February 2008.',
  '[^luna]: "Luna 2". *NASA–NSSDC*.',
  '[^unused]: never cited',
].join('\n');

test('footnoteDefinitions and referencedLabels', () => {
  expect(Array.from(footnoteDefinitions(article).keys())).toEqual(['1', 'luna', 'unused']);
  expect(referencedLabels(article)).toEqual(['luna', '1']);
});

test('footnoteSources are numbered by first use', () => {
  expect(footnoteSources(article).map((s) => [s.label, s.number])).toEqual([['luna', 1], ['1', 2]]);
});

test('nextFootnoteLabel', () => {
  expect(nextFootnoteLabel(article)).toBe('2');
  expect(nextFootnoteLabel('')).toBe('1');
  expect(nextFootnoteLabel('a[^3] b[^7]\n\n[^3]: x\n[^7]: y')).toBe('8');
});

test('referenceNumberAt', () => {
  expect(referenceNumberAt('Luna 2[^luna] and', 'new')).toBe(2);
  expect(referenceNumberAt('Luna 2[^luna] and Apollo[^1] and', 'luna')).toBe(1);
  expect(referenceNumberAt('', 'new')).toBe(1);
});

test('appendFootnoteDefinition', () => {
  expect(appendFootnoteDefinition('Text.', '1', 'A.')).toBe('Text.\n\n[^1]: A.\n');
  expect(appendFootnoteDefinition('Text.\n\n[^1]: A.\n', '2', 'B.')).toBe('Text.\n\n[^1]: A.\n[^2]: B.\n');
  expect(appendFootnoteDefinition('', '1', 'A.')).toBe('[^1]: A.\n');
});

test('plainCitationSummary', () => {
  expect(plainCitationSummary('"Luna 2". *NASA–NSSDC*.')).toEqual({title: '"Luna 2"', publisher: 'NASA–NSSDC'});
  expect(plainCitationSummary('["Manned Space Chronology: Apollo\\_11"](https://x). *spaceline.org*.'))
    .toEqual({title: '"Manned Space Chronology: Apollo_11"', publisher: 'spaceline.org'});
  expect(plainCitationSummary('Chaikin, Andrew (1994). *A Man on the Moon* (1st ed.). Viking.'))
    .toEqual({title: 'A Man on the Moon', publisher: ''});
});
