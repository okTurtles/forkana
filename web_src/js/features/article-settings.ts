import {fomanticQuery} from '../modules/fomantic/base.ts';
import {addDelegatedEventListener} from '../utils/dom.ts';
import {htmlEscape} from '../utils/html.ts';

// The transfer modal additionally requires an owner picked from the search results,
// the button state depends on both the confirmation input and that selection.
let transferOwnerSelected = false;

// Enables the target button only when the typed value matches the expected one
// exactly, the comparison is case-sensitive on purpose.
function syncConfirmInput(input: HTMLInputElement): void {
  const target = document.querySelector<HTMLButtonElement>(input.getAttribute('data-article-confirm-target'));
  if (!target) return;
  let enabled = input.value === input.getAttribute('data-article-confirm-value');
  if (enabled && target.id === 'article-transfer-submit') enabled = transferOwnerSelected;
  target.classList.toggle('disabled', !enabled);
  target.disabled = !enabled;
}

function syncTransferConfirmInput(): void {
  const input = document.querySelector<HTMLInputElement>('#article-transfer-confirm-name');
  if (input) syncConfirmInput(input);
}

// Turns the "new owner" field into a search box listing the users the article can be
// transferred to. The selected value is the user's full name, which is what the
// backend resolves the recipient by.
function initArticleTransferOwnerSearch(): void {
  const elSearch = document.querySelector<HTMLElement>('#article-transfer-owner-search');
  if (!elSearch) return;

  const searchURL = elSearch.getAttribute('data-search-url');
  const elInput = elSearch.querySelector<HTMLInputElement>('input[name="new_owner_name"]');
  fomanticQuery(elSearch).search({
    minCharacters: 3,
    maxResults: 3,
    cache: true,
    throttle: 300,
    showNoResults: false,
    apiSettings: {
      url: `${searchURL}?q={query}`,
      onResponse(response: any) {
        const results = [];
        for (const user of response.data) {
          results.push({
            title: user.full_name || user.login,
            description: htmlEscape(user.login),
          });
        }
        return {results};
      },
    },
    onSelect(result: any) {
      transferOwnerSelected = Boolean(result?.title);
      syncTransferConfirmInput();
    },
  });

  elInput.addEventListener('input', () => {
    transferOwnerSelected = false;
    syncTransferConfirmInput();
  });
}

// Opens the Article settings modals (transfer/archive/delete). Submission is
// handled by the forms inside the modals.
export function initArticleSettings(): void {
  if (!document.querySelector('#article-settings-general')) return;

  initArticleTransferOwnerSearch();

  addDelegatedEventListener(document, 'click', '[data-article-settings-modal]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    const modal = document.querySelector<HTMLElement>(el.getAttribute('data-article-settings-modal'));
    if (!modal) return;
    // a modal can be reopened, so the confirmation and the owner must be entered again
    transferOwnerSelected = false;
    for (const input of modal.querySelectorAll<HTMLInputElement>('input[name="new_owner_name"], [data-article-confirm-value]')) {
      input.value = '';
    }
    for (const input of modal.querySelectorAll<HTMLInputElement>('[data-article-confirm-value]')) {
      syncConfirmInput(input);
    }
    fomanticQuery(modal).modal('show');
  });

  addDelegatedEventListener(document, 'input', '[data-article-confirm-value]', (el: HTMLInputElement) => {
    syncConfirmInput(el);
  });
}
