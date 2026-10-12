import {initSubjectTitleHint, subjectTitleHint} from './subject-title-hint.ts';

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

test('initSubjectTitleHint shows the normalized title literally', () => {
  document.body.innerHTML = `<div class="field"><input name="subject">
    <span class="tw-hidden" data-subject-title-hint data-msg-forbidden-char="forbidden"
      data-msg-normalized="Saved as “%s”."></span></div>`;
  initSubjectTitleHint();
  const input = document.querySelector<HTMLInputElement>('input');
  const hint = document.querySelector<HTMLElement>('[data-subject-title-hint]');
  const type = (value: string) => {
    input.value = value;
    input.dispatchEvent(new Event('input'));
    return hint.textContent;
  };
  // "$" is an allowed title character, so replacement patterns must not be expanded
  for (const pattern of ['$&', '$$', '$`', `$'`]) {
    expect(type(`iphone ${pattern} moon`)).toBe(`Saved as “Iphone ${pattern} moon”.`);
  }
  expect(hint.classList.contains('tw-hidden')).toBe(false);
  expect(type('a#b')).toBe('forbidden');
  expect(hint.classList.contains('red')).toBe(true);
  type('Moon');
  expect(hint.classList.contains('tw-hidden')).toBe(true);
});
