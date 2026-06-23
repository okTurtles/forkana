import {hideElem, showElem} from '../utils/dom.ts';

// initUserThemeSelector wires the appearance-settings theme cards to submit the form
// when a theme is picked. Done here with a delegated listener rather than an inline
// onchange handler so it follows the codebase convention and survives a strict CSP.
function initUserThemeSelector() {
  const form = document.querySelector<HTMLFormElement>('#theme-selector-form');
  if (!form) return;
  // Progressive enhancement: with JS we auto-submit on change, so the explicit
  // submit button (the no-JS / keyboard fallback) is redundant — hide it.
  hideElem(form.querySelector('.theme-submit-fallback'));
  form.addEventListener('change', (e) => {
    if ((e.target as HTMLElement).matches('.theme-radio')) form.requestSubmit();
  });
}

export function initUserSettings() {
  initUserThemeSelector();

  if (!document.querySelector('.user.settings.profile')) return;

  const usernameInput = document.querySelector<HTMLInputElement>('#username');
  if (!usernameInput) return;
  usernameInput.addEventListener('input', function () {
    const prompt = document.querySelector('#name-change-prompt');
    const promptRedirect = document.querySelector('#name-change-redirect-prompt');
    if (this.value.toLowerCase() !== this.getAttribute('data-name').toLowerCase()) {
      showElem(prompt);
      showElem(promptRedirect);
    } else {
      hideElem(prompt);
      hideElem(promptRedirect);
    }
  });
}
