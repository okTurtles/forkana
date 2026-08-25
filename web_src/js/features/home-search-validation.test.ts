import {initHomeSearchValidation} from './home-search-validation.ts';

function setupHomeSearch(): {form: HTMLFormElement, input: HTMLInputElement, error: HTMLElement} {
  document.body.innerHTML = `
    <form id="home-search-form" action="/explore/articles" method="get">
      <input id="search" name="q" type="text">
      <input type="hidden" name="sort" value="score">
    </form>
    <div id="home-search-error" class="home-search-error tw-hidden" role="alert">Type correct subject</div>
  `;
  initHomeSearchValidation();
  return {
    form: document.querySelector('#home-search-form'),
    input: document.querySelector('#search'),
    error: document.querySelector('#home-search-error'),
  };
}

// jsdom does not implement form submission, it only fires the event, so the test asserts on
// "defaultPrevented" to tell a blocked search from one that goes to the explore page.
function submit(form: HTMLFormElement): boolean {
  const event = new Event('submit', {bubbles: true, cancelable: true});
  form.dispatchEvent(event);
  return event.defaultPrevented;
}

test('blocks a blank search and shows the error message', () => {
  const {form, input, error} = setupHomeSearch();

  for (const value of ['', ' ', '   \t ']) {
    input.value = value;
    expect(submit(form)).toBe(true);
    expect(error.classList.contains('tw-hidden')).toBe(false);
    expect(input.classList.contains('home-search-input-invalid')).toBe(true);
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.getAttribute('aria-describedby')).toBe('home-search-error');
  }
});

test('typing clears the error message again', () => {
  const {form, input, error} = setupHomeSearch();

  input.value = ' ';
  submit(form);
  input.value = 'forest';
  input.dispatchEvent(new Event('input', {bubbles: true}));

  expect(error.classList.contains('tw-hidden')).toBe(true);
  expect(input.classList.contains('home-search-input-invalid')).toBe(false);
  expect(input.getAttribute('aria-invalid')).toBe(null);
  expect(input.getAttribute('aria-describedby')).toBe(null);
});

test('submits a search with a keyword', () => {
  const {form, input, error} = setupHomeSearch();

  input.value = 'forest';
  expect(submit(form)).toBe(false);
  expect(error.classList.contains('tw-hidden')).toBe(true);
});
