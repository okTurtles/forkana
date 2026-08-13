import {fomanticQuery} from '../modules/fomantic/base.ts';
import {addDelegatedEventListener} from '../utils/dom.ts';

// Enables the target button only when the typed value matches the expected one
// exactly, the comparison is case-sensitive on purpose.
function syncConfirmInput(input: HTMLInputElement): void {
  const target = document.querySelector<HTMLButtonElement>(input.getAttribute('data-article-confirm-target'));
  if (!target) return;
  const matches = input.value === input.getAttribute('data-article-confirm-value');
  target.classList.toggle('disabled', !matches);
  target.disabled = !matches;
}

// Opens the Article settings modals (transfer/archive/delete). The modal
// content and submission handling are not wired up yet.
export function initArticleSettings(): void {
  if (!document.querySelector('#article-settings-general')) return;

  addDelegatedEventListener(document, 'click', '[data-article-settings-modal]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    const modal = document.querySelector<HTMLElement>(el.getAttribute('data-article-settings-modal'));
    if (!modal) return;
    // a modal can be reopened, so the confirmation must be typed again every time
    for (const input of modal.querySelectorAll<HTMLInputElement>('[data-article-confirm-value]')) {
      input.value = '';
      syncConfirmInput(input);
    }
    fomanticQuery(modal).modal('show');
  });

  addDelegatedEventListener(document, 'input', '[data-article-confirm-value]', (el: HTMLInputElement) => {
    syncConfirmInput(el);
  });
}
