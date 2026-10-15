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
  const defined = new Set(['luna', '1']);
  expect(referenceNumberAt('Luna 2[^luna] and', 'new', defined)).toBe(2);
  expect(referenceNumberAt('Luna 2[^luna] and Apollo[^1] and', 'luna', defined)).toBe(1);
  expect(referenceNumberAt('', 'new', defined)).toBe(1);
  // a reference without a definition gets no number when rendered (review 2 #4)
  expect(referenceNumberAt('A[^todo] b[^1] c', '2', defined)).toBe(2);
});

test('a "[^1]:" in running text is a reference (review 2 #22)', () => {
  expect(referencedLabels('See[^1]: details.\n\n[^2]: not a reference')).toEqual(['1']);
});

test('appendFootnoteDefinition', () => {
  expect(appendFootnoteDefinition('Text.', '1', 'A.')).toBe('Text.\n\n[^1]: A.\n');
  expect(appendFootnoteDefinition('Text.\n\n[^1]: A.\n', '2', 'B.')).toBe('Text.\n\n[^1]: A.\n[^2]: B.\n');
  expect(appendFootnoteDefinition('', '1', 'A.')).toBe('[^1]: A.\n');
  // a code block left open is closed first (review 2 #21)
  expect(appendFootnoteDefinition('Text\n```js\ncode', '1', 'A.')).toBe('Text\n```js\ncode\n```\n\n[^1]: A.\n');
  expect(appendFootnoteDefinition('Text\n~~~~\ncode\n', '1', 'A.')).toBe('Text\n~~~~\ncode\n~~~~\n\n[^1]: A.\n');
});

test('plainCitationSummary', () => {
  expect(plainCitationSummary('"Luna 2". *NASA–NSSDC*.')).toEqual({title: '"Luna 2"', publisher: 'NASA–NSSDC'});
  expect(plainCitationSummary('["Manned Space Chronology: Apollo\\_11"](https://x). *spaceline.org*.'))
    .toEqual({title: '"Manned Space Chronology: Apollo_11"', publisher: 'spaceline.org'});
  expect(plainCitationSummary('Chaikin, Andrew (1994). *A Man on the Moon* (1st ed.). Viking.'))
    .toEqual({title: 'A Man on the Moon', publisher: ''});
  // quotes in a title, escaped markers in the text (review 2 #23)
  expect(plainCitationSummary('["The "Moon" hoax"](https://x). *Site*.')).toEqual({title: '"The "Moon" hoax"', publisher: 'Site'});
  expect(plainCitationSummary('"The "Moon" hoax". Retrieved 1 October 2026.').title).toBe('"The "Moon" hoax"');
  expect(plainCitationSummary('Notes on 5\\*3 and a\\_b.').title).toBe('Notes on 5*3 and a_b.');
});
