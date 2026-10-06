import {subjectTitleHint} from './subject-title-hint.ts';

test('subjectTitleHint', () => {
  // valid titles that are stored as typed: no hint
  for (const title of ['Moon', ';alskdjf', 'Test: Gaudí', 'AC/DC', 'C++', 'Python (programming language)', '  Moon  ', '']) {
    expect({title, ...subjectTitleHint(title)}).toEqual({title, problem: '', normalized: ''});
  }
  // valid titles that a new subject stores normalized
  expect(subjectTitleHint('iPhone')).toEqual({problem: '', normalized: 'IPhone'});
  expect(subjectTitleHint('Foo_Bar')).toEqual({problem: '', normalized: 'Foo Bar'});
  // invalid titles
  expect(subjectTitleHint('a#b').problem).toBe('forbidden_char');
  expect(subjectTitleHint('%41').problem).toBe('percent_encoding');
  expect(subjectTitleHint('AT&amp;T').problem).toBe('html_entity');
  expect(subjectTitleHint('~~~').problem).toBe('tildes');
  expect(subjectTitleHint('../Foo').problem).toBe('relative_path');
  expect(subjectTitleHint(':Foo').problem).toBe('leading_colon');
  expect(subjectTitleHint('é'.repeat(200)).problem).toBe('too_long');
});
