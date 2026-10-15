import {fomanticQuery} from '../modules/fomantic/base.ts';
import {POST} from '../modules/fetch.ts';
import {showErrorToast, showInfoToast} from '../modules/toast.ts';
import {addDelegatedEventListener, hideElem, showElem, toggleElem} from '../utils/dom.ts';
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

// Labels the invite button with the picked user, it stays disabled until one is picked.
function syncInviteSubmit(name: string | null): void {
  const button = document.querySelector<HTMLButtonElement>('#article-collaborator-invite-submit');
  if (!button) return;
  const enabled = Boolean(name);
  // a replacer function inserts the name literally, a string would expand "$&" and the like
  button.textContent = enabled ?
    (button.getAttribute('data-label-invite') ?? '').replace('%s', () => name) :
    (button.getAttribute('data-label-select') ?? '');
  button.classList.toggle('disabled', !enabled);
  button.disabled = !enabled;
}

type ArticleUserSearchParts = {
  input: HTMLInputElement,
  prompt: HTMLElement,
  selection: HTMLElement,
  avatar: HTMLImageElement,
  name: HTMLElement,
  clear: HTMLElement,
};

// Returns the parts of a user search, or null when any is missing, so callers can use
// every part without guarding it.
function articleUserSearchParts(elSearch: HTMLElement): ArticleUserSearchParts | null {
  const input = elSearch.querySelector<HTMLInputElement>('input.prompt');
  const selection = elSearch.querySelector<HTMLElement>('.article-user-search-selection');
  if (!input || !selection) return null;
  const prompt = input.closest<HTMLElement>('.ui.input');
  const avatar = selection.querySelector<HTMLImageElement>('.article-user-search-avatar');
  const name = selection.querySelector<HTMLElement>('.article-user-search-name');
  const clear = selection.querySelector<HTMLElement>('.article-user-search-clear');
  if (!prompt || !avatar || !name || !clear) return null;
  return {input, prompt, selection, avatar, name, clear};
}

// Resets a user search back to its searchable state, called when the user is cleared
// and every time its modal is reopened.
function resetArticleUserSearch(elSearch: HTMLElement): void {
  const parts = articleUserSearchParts(elSearch);
  if (!parts) return;
  parts.input.value = '';
  parts.avatar.src = '';
  parts.name.textContent = '';
  hideElem(parts.selection);
  showElem(parts.prompt);
}

// Turns a user field into a search box listing the users returned by its search url. A
// user is shown by their full name, falling back to the username, while the submitted
// value is always the username, which is what the backend resolves the user by.
// onChange receives the displayed name of the picked user, or null when none is picked.
function bindArticleUserSearch(elSearch: HTMLElement, onChange: (name: string | null) => void): void {
  const parts = articleUserSearchParts(elSearch);
  if (!parts) return;
  const searchURL = elSearch.getAttribute('data-search-url');

  fomanticQuery(elSearch).search({
    minCharacters: 2,
    maxResults: 5,
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
            // is kept separately for the selected-user display
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
      if (!result?.login) {
        onChange(null);
        return;
      }
      // the search fills the input with the displayed title, the form has to carry the
      // username instead
      parts.input.value = result.login;
      parts.avatar.src = result.image ?? '';
      parts.name.textContent = result.rawName ?? '';
      hideElem(parts.prompt);
      showElem(parts.selection);
      onChange(result.rawName || result.login);
    },
  });

  parts.clear.addEventListener('click', () => {
    resetArticleUserSearch(elSearch);
    onChange(null);
    parts.input.focus();
  });

  parts.input.addEventListener('input', () => onChange(null));
}

function initArticleUserSearches(): void {
  const elTransfer = document.querySelector<HTMLElement>('#article-transfer-owner-search');
  if (elTransfer) {
    bindArticleUserSearch(elTransfer, (name) => {
      transferOwnerSelected = Boolean(name);
      syncTransferConfirmInput();
    });
  }
  const elInvite = document.querySelector<HTMLElement>('#article-collaborator-invite-search');
  if (elInvite) bindArticleUserSearch(elInvite, syncInviteSubmit);
}

// Puts a reopened modal back in its initial state: the confirmation, the picked user and
// the access level must be entered again.
function resetArticleSettingsModal(modal: HTMLElement): void {
  for (const elSearch of modal.querySelectorAll<HTMLElement>('.article-user-search')) {
    resetArticleUserSearch(elSearch);
  }
  transferOwnerSelected = false;
  syncTransferConfirmInput();
  syncInviteSubmit(null);

  // the hidden input carries the last selection, the default comes from the markup
  const elRole = modal.querySelector<HTMLElement>('#article-collaborator-role');
  const defaultRole = elRole?.getAttribute('data-default-value');
  if (elRole && defaultRole) fomanticQuery(elRole).dropdown('set selected', defaultRole);

  for (const input of modal.querySelectorAll<HTMLInputElement>('[data-article-confirm-value]')) {
    input.value = '';
    syncConfirmInput(input);
  }
}

// Shows one settings section (general, collaborators) and hides the others, both are
// rendered so switching does not reload the article.
function showArticleSettingsSection(section: string): void {
  for (const item of document.querySelectorAll('[data-article-settings-section]')) {
    item.classList.toggle('active', item.getAttribute('data-article-settings-section') === section);
  }
  for (const panel of document.querySelectorAll('[data-article-settings-panel]')) {
    toggleElem(panel, panel.getAttribute('data-article-settings-panel') === section);
  }
}

function showRemoveCollaboratorsModal(): void {
  const modal = document.querySelector<HTMLElement>('#article-collaborators-remove-modal');
  if (modal) fomanticQuery(modal).modal('show');
}

// The collaborators table: search, role filter, selection with bulk removal and the role
// dropdown of each row. The list is short, so filtering happens on the rendered rows.
function initArticleCollaborators(): void {
  const root = document.querySelector<HTMLElement>('#article-settings-collaborators');
  if (!root) return;

  const rows = Array.from(root.querySelectorAll<HTMLElement>('tr[data-article-collaborator]'));
  const elSelectAll = root.querySelector<HTMLInputElement>('#article-collaborators-select-all');
  const elRemoveSelected = root.querySelector<HTMLButtonElement>('#article-collaborators-remove-selected');
  const elSearch = root.querySelector<HTMLInputElement>('#article-collaborators-search');
  const elNoMatch = root.querySelector<HTMLElement>('#article-collaborators-no-match');
  let roleFilter = '';

  const rowCheckbox = (row: HTMLElement) => row.querySelector<HTMLInputElement>('[data-article-collaborator-select]');
  const visibleCheckboxes = () => rows.filter((row) => !row.classList.contains('tw-hidden')).map(rowCheckbox).filter(Boolean);

  const syncSelection = () => {
    const checkboxes = visibleCheckboxes();
    const checked = checkboxes.filter((checkbox) => checkbox.checked).length;
    if (elSelectAll) {
      elSelectAll.checked = checkboxes.length > 0 && checked === checkboxes.length;
      elSelectAll.indeterminate = checked > 0 && checked < checkboxes.length;
      elSelectAll.disabled = checkboxes.length === 0;
    }
    if (elRemoveSelected) toggleElem(elRemoveSelected, checked > 0);
  };

  const applyFilter = () => {
    const query = elSearch?.value.trim().toLowerCase() ?? '';
    let shown = 0;
    for (const row of rows) {
      const matches = (!query || row.getAttribute('data-search').toLowerCase().includes(query)) &&
        (!roleFilter || row.getAttribute('data-role') === roleFilter);
      toggleElem(row, matches);
      // a collaborator filtered out of sight must not be removed along with the visible ones
      const checkbox = rowCheckbox(row);
      if (!matches && checkbox) checkbox.checked = false;
      if (matches) shown++;
    }
    if (elNoMatch) toggleElem(elNoMatch, shown === 0);
    syncSelection();
  };

  elSelectAll?.addEventListener('change', () => {
    for (const checkbox of visibleCheckboxes()) checkbox.checked = elSelectAll.checked;
    syncSelection();
  });
  for (const row of rows) rowCheckbox(row)?.addEventListener('change', syncSelection);
  elSearch?.addEventListener('input', applyFilter);

  const elRoleFilter = root.querySelector<HTMLElement>('#article-collaborators-role-filter');
  if (elRoleFilter) {
    // the filter is only an arrow next to the "Role" header, so picking a role must not
    // write the role into it; the arrow is highlighted instead while a role is filtered
    const $roleFilter = fomanticQuery(elRoleFilter);
    $roleFilter.dropdown({
      action(_text: string, value: string) {
        $roleFilter.dropdown('hide');
        roleFilter = value;
        for (const item of elRoleFilter.querySelectorAll('.menu > .item')) {
          item.classList.toggle('active', item.getAttribute('data-value') === value);
        }
        elRoleFilter.classList.toggle('tw-text-primary', Boolean(value));
        applyFilter();
      },
    });
  }

  elRemoveSelected?.addEventListener('click', showRemoveCollaboratorsModal);
  // removing a single collaborator reuses the bulk form with only their row selected
  for (const button of root.querySelectorAll<HTMLElement>('[data-article-collaborator-remove]')) {
    button.addEventListener('click', () => {
      const uid = button.getAttribute('data-article-collaborator-remove');
      for (const row of rows) {
        const checkbox = rowCheckbox(row);
        if (checkbox) checkbox.checked = checkbox.value === uid;
      }
      syncSelection();
      showRemoveCollaboratorsModal();
    });
  }

  for (const dropdownEl of root.querySelectorAll<HTMLElement>('.ui.dropdown.article-collaborator-role')) {
    const textEl = dropdownEl.querySelector<HTMLElement>(':scope > .text');
    const row = dropdownEl.closest<HTMLElement>('tr[data-article-collaborator]');
    const $dropdown = fomanticQuery(dropdownEl);
    $dropdown.dropdown({
      async action(_text: string, value: string) {
        $dropdown.dropdown('hide');
        const lastValue = dropdownEl.getAttribute('data-last-value');
        if (value === lastValue) return;
        dropdownEl.classList.add('is-loading', 'loading-icon-2px');
        try {
          const response = await POST(dropdownEl.getAttribute('data-url'), {
            data: new URLSearchParams({uid: dropdownEl.getAttribute('data-uid'), role: value}),
          });
          if (!response.ok) {
            let message = `Failed with status ${response.status}`;
            try {
              message = (await response.json()).errorMessage ?? message;
            } catch {
              // a non-JSON error page keeps the status message
            }
            throw new Error(message);
          }
          // the items carry a description, the shown text is only the role name
          textEl.textContent = dropdownEl.querySelector(`.item[data-value="${value}"]`)?.getAttribute('data-text') ?? '';
          dropdownEl.setAttribute('data-last-value', value);
          row?.setAttribute('data-role', value);
          applyFilter();
          // the server escapes the message, it is HTML
          const {message} = await response.json();
          if (message) showInfoToast(message, {useHtmlBody: true});
        } catch (err) {
          showErrorToast(err.message);
        } finally {
          dropdownEl.classList.remove('is-loading', 'loading-icon-2px');
        }
      },
    });
  }

  applyFilter();
}

// Opens the Article settings modals (invite/transfer/archive/delete) and switches the
// settings sections. Submission is handled by the forms inside the modals.
export function initArticleSettings(): void {
  if (!document.querySelector('#article-settings-root')) return;

  // the settings markup is re-rendered when switching modes in the history view, so the
  // searches and the table are bound against the current elements on every call
  initArticleUserSearches();
  initArticleCollaborators();

  if (delegatedListenersBound) return;
  delegatedListenersBound = true;

  addDelegatedEventListener(document, 'click', '[data-article-settings-modal]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    const modal = document.querySelector<HTMLElement>(el.getAttribute('data-article-settings-modal'));
    if (!modal) return;
    resetArticleSettingsModal(modal);
    fomanticQuery(modal).modal('show');
  });

  addDelegatedEventListener(document, 'click', '[data-article-settings-section]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    showArticleSettingsSection(el.getAttribute('data-article-settings-section'));
  });

  addDelegatedEventListener(document, 'input', '[data-article-confirm-value]', (el: HTMLInputElement) => {
    syncConfirmInput(el);
  });
}
