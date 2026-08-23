import {fomanticQuery} from '../modules/fomantic/base.ts';
import {addDelegatedEventListener, hideElem, showElem} from '../utils/dom.ts';
import {htmlEscape} from '../utils/html.ts';

// The transfer modal additionally requires an owner picked from the search results,
// the button state depends on both the confirmation input and that selection.
let transferOwnerSelected = false;

// The delegated listeners are bound to the document, so they survive a re-render and
// must not be bound again when the settings markup is reloaded.
let delegatedListenersBound = false;

// Enables the target button only when the typed value matches the expected one
// exactly, the comparison is case-sensitive on purpose.
function syncConfirmInput(input: HTMLInputElement): void {
  const target = document.querySelector<HTMLButtonElement>(input.getAttribute('data-article-confirm-target'));
  if (!target) return;
  let enabled = input.value === input.getAttribute('data-article-confirm-value');
  if (enabled && target.getAttribute('data-requires-owner-selection') === 'true') enabled = transferOwnerSelected;
  target.classList.toggle('disabled', !enabled);
  target.disabled = !enabled;
}

function syncTransferConfirmInput(): void {
  const input = document.querySelector<HTMLInputElement>('#article-transfer-confirm-name');
  if (input) syncConfirmInput(input);
}

// Resets the "new owner" field back to its searchable state, called when the owner is
// cleared and every time the transfer modal is reopened. The settings markup is
// re-rendered in place and the modal is absent for archived articles, so the elements
// are resolved on every call instead of being captured once.
function resetTransferOwnerSearch(): void {
  transferOwnerSelected = false;
  const elSearch = document.querySelector<HTMLElement>('#article-transfer-owner-search');
  if (!elSearch) return;

  const elInput = elSearch.querySelector<HTMLInputElement>('input[name="new_owner_name"]');
  const elSelection = elSearch.querySelector<HTMLElement>('#article-transfer-owner-selection');
  elInput.value = '';
  elSelection.querySelector<HTMLImageElement>('#article-transfer-owner-avatar').src = '';
  elSelection.querySelector<HTMLElement>('#article-transfer-owner-name').textContent = '';
  hideElem(elSelection);
  showElem(elInput.closest<HTMLElement>('.ui.input'));
  syncTransferConfirmInput();
}

// Turns the "new owner" field into a search box listing the users the article can be
// transferred to. A user is shown by their full name, falling back to the username,
// while the submitted value is always the username, which is what the backend resolves
// the recipient by.
function initArticleTransferOwnerSearch(): void {
  const elSearch = document.querySelector<HTMLElement>('#article-transfer-owner-search');
  if (!elSearch) return;

  const searchURL = elSearch.getAttribute('data-search-url');
  const elInput = elSearch.querySelector<HTMLInputElement>('input[name="new_owner_name"]');
  const elPrompt = elInput.closest<HTMLElement>('.ui.input');
  const elSelection = elSearch.querySelector<HTMLElement>('#article-transfer-owner-selection');
  const elAvatar = elSelection.querySelector<HTMLImageElement>('#article-transfer-owner-avatar');
  const elName = elSelection.querySelector<HTMLElement>('#article-transfer-owner-name');

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
          const name = user.full_name || user.login;
          results.push({
            // the result template renders "title" as HTML (preserveHTML), the raw value
            // is kept separately for the selected-owner display
            title: htmlEscape(name),
            rawName: name,
            login: user.login,
            image: user.avatar_url,
          });
        }
        return {results};
      },
    },
    onSelect(result: any) {
      transferOwnerSelected = Boolean(result?.login);
      if (!transferOwnerSelected) return;
      // the search fills the input with the displayed title, the form has to carry the
      // username instead
      elInput.value = result.login;
      elAvatar.src = result.image ?? '';
      elName.textContent = result.rawName ?? '';
      hideElem(elPrompt);
      showElem(elSelection);
      syncTransferConfirmInput();
    },
  });

  elSelection.querySelector('#article-transfer-owner-clear').addEventListener('click', () => {
    resetTransferOwnerSearch();
    elInput.focus();
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

  // the settings markup is re-rendered when switching modes in the history view, so the
  // search is bound against the current elements on every call
  initArticleTransferOwnerSearch();

  if (delegatedListenersBound) return;
  delegatedListenersBound = true;

  addDelegatedEventListener(document, 'click', '[data-article-settings-modal]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    const modal = document.querySelector<HTMLElement>(el.getAttribute('data-article-settings-modal'));
    if (!modal) return;
    // a modal can be reopened, so the confirmation and the owner must be entered again
    resetTransferOwnerSearch();
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
