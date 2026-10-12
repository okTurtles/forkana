import {checkSubjectTitle, normalizeSubjectTitle} from './subject-title.ts';
import cases from '../../../modules/subjecttitle/testdata/cases.json';

// The same cases are run against the Go rule by modules/subjecttitle/subjecttitle_test.go, so
// the two implementations cannot drift apart. Each result is paired with its input so that a
// failure names the case.

test('normalizeSubjectTitle (shared cases)', () => {
  const got = cases.normalize.map((c) => [c.in, normalizeSubjectTitle(c.in)]);
  expect(got).toEqual(cases.normalize.map((c) => [c.in, c.out]));
});

test('checkSubjectTitle (shared cases)', () => {
  const titles = cases.check.map((c) => ('repeat' in c ? c.title.repeat(c.repeat) : c.title));
  // the check cases are already normalized
  expect(titles.map(normalizeSubjectTitle)).toEqual(titles);
  const got = titles.map((title) => [title, checkSubjectTitle(title)]);
  expect(got).toEqual(cases.check.map((c, i) => [titles[i], c.problem]));
});

test('checkSubjectTitle rejects lone surrogates', () => {
  // the JavaScript counterpart of invalid UTF-8, which JSON cannot hold
  expect(checkSubjectTitle('Foo\uD800')).toBe('forbidden_char');
  expect(checkSubjectTitle('Foo\uDC00Bar')).toBe('forbidden_char');
  expect(checkSubjectTitle('Hello \u{1F600}')).toBe('');
});

test('the HTML maxlength never rejects a valid title', () => {
  // maxlength="255" counts UTF-16 code units, and a code unit is at least one UTF-8 byte, so
  // any title of at most 255 bytes fits; longer ones are left to the hint and the server
  for (const title of ['A'.repeat(255), 'É'.repeat(127), '東'.repeat(85), '\u{1F600}'.repeat(63)]) {
    expect(checkSubjectTitle(title)).toBe('');
    expect(title.length).toBeLessThanOrEqual(255);
  }
});
