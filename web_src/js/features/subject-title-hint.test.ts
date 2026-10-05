import {isNewSubjectTitleValid} from './subject-title-hint.ts';

// Copy of modules/subjecttitle.Pattern; the Go test TestPattern checks the same cases with RE2.
const pattern = String.raw`\s*[\p{L}\p{Nd}][\p{L}\p{M}\p{Nd}\s'’\-]*`;

test('isNewSubjectTitleValid', () => {
  for (const title of ['Moon', 'The Moon', 'Gaudí', 'Zalg\'o', 'O’Brien', 'Jean-Paul Sartre', '1984',
    'Ελλάδα', '東京', 'Москва', 'हिन्दी', 'Rock \'n\' Roll', '  The   Moon  ', '']) {
    expect(isNewSubjectTitleValid(title, pattern)).toBe(true);
  }
  for (const title of [';alskdjf', 'Test: Gaudí', 'Moon!', 'C++', 'AT&T', 'Foo/Bar', 'Foo_Bar',
    'Foo.Bar', '<script>', 'Hello 😀', '-Moon', '\'Moon', '’Moon', '\u0301Moon']) {
    expect(isNewSubjectTitleValid(title, pattern)).toBe(false);
  }
});
