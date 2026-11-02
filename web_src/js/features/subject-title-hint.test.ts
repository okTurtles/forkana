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

test('initSubjectTitleHint blocks submitting an invalid title (#401)', () => {
  document.body.innerHTML = `<form><div class="field"><input name="subject">
    <span class="tw-hidden" data-subject-title-hint data-msg-forbidden-char="No # here"
      data-msg-normalized="Saved as “%s”."></span></div></form>`;
  initSubjectTitleHint();
  const form = document.querySelector('form');
  const input = document.querySelector<HTMLInputElement>('input');
  const type = (value: string) => {
    input.value = value;
    input.dispatchEvent(new Event('input'));
  };
  type('a#b');
  expect(input.validationMessage).toBe('No # here');
  expect(form.checkValidity()).toBe(false);
  // apostrophes, punctuation and other scripts are allowed
  for (const title of [`'''`, `''''''''''''''''`, ';alskdjf', 'Test: Gaudí', '東京', 'iPhone']) {
    type(title);
    expect(input.validationMessage).toBe('');
    expect(form.checkValidity()).toBe(true);
  }
});

test('initSubjectTitleHint lets the prefilled existing subject through', () => {
  document.body.innerHTML = `<form><div class="field"><input name="subject" value="Legacy #Gaudí">
    <span class="tw-hidden" data-subject-title-hint data-existing-subject="Legacy #Gaudí"
      data-msg-forbidden-char="No # here"></span></div></form>`;
  initSubjectTitleHint();
  const input = document.querySelector<HTMLInputElement>('input');
  const hint = document.querySelector<HTMLElement>('[data-subject-title-hint]');
  expect(input.validationMessage).toBe('');
  expect(hint.classList.contains('tw-hidden')).toBe(true);
  input.value = 'Legacy #Gaudí 2';
  input.dispatchEvent(new Event('input'));
  expect(input.validationMessage).toBe('No # here');
});
