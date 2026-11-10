import {fomanticQuery} from '../modules/fomantic/base.ts';
import {POST} from '../modules/fetch.ts';
import {showErrorToast, showInfoToast} from '../modules/toast.ts';
import {addDelegatedEventListener, hideElem, showElem, toggleElem} from '../utils/dom.ts';
import {htmlEscape} from '../utils/html.ts';

// The transfer dialog additionally requires an owner picked from the search results,
// the button state depends on both the confirmation input and that selection.
let transferOwnerSelected = false;

// The delegated listeners are bound to the document, so they survive a re-render and
// must not be bound again when the settings markup is reloaded.
let delegatedListenersBound = false;

// Fills a message of the markup's data attributes: "{name}" and "{count}" stand for the
// values, which are inserted literally (a replacer function does not expand "$&").
function fillMessage(template: string | null, values: {name?: string, count?: number}): string {
  return (template ?? '')
    .replace('{name}', () => values.name ?? '')
    .replace('{count}', () => String(values.count ?? ''));
}

// "owner / subject" with the spaces around the first slash trimmed, as the server compares
// it (articleNameConfirmed in routers/web/repo/setting/setting.go): "owner/subject" and
// "owner / subject" are the same confirmation.
export function normalizeArticleName(value: string): string {
  const slash = value.indexOf('/');
  if (slash < 0) return value.trim();
  return `${value.slice(0, slash).trim()}/${value.slice(slash + 1).trim()}`;
}

// Enables the target button only when the typed value names the article, the comparison
// is case-sensitive on purpose.
function syncConfirmInput(input: HTMLInputElement): void {
  const target = document.querySelector<HTMLButtonElement>(input.getAttribute('data-article-confirm-target'));
  if (!target) return;
  let enabled = input.value.trim() !== '' &&
    normalizeArticleName(input.value) === normalizeArticleName(input.getAttribute('data-article-confirm-value') ?? '');
  if (enabled && target.getAttribute('data-requires-owner-selection') === 'true') enabled = transferOwnerSelected;
  target.disabled = !enabled;
}

function syncTransferConfirmInput(): void {
  const input = document.querySelector<HTMLInputElement>('#article-transfer-confirm-name');
  if (input) syncConfirmInput(input);
}

type PickedUser = {name: string, login: string};

// The invite dialog (#461, figma "Invite – search" 7619:68300 and "– choose role"
// 7619:68440): until someone is picked, "Invite" is unavailable and the footer says why;
// once picked, the access level and its caption show and the button reads "Invite
// {username}".
function syncInviteDialog(user: PickedUser | null): void {
  const button = document.querySelector<HTMLButtonElement>('#article-collaborator-invite-submit');
  if (!button) return;
  const dialog = button.closest<HTMLElement>('.forkana-dialog');
  button.textContent = user ?
    fillMessage(button.getAttribute('data-label-invite'), {name: user.login}) :
    (button.getAttribute('data-label-select') ?? '');
  button.disabled = !user;

  const reason = dialog?.querySelector<HTMLElement>('[data-role="invite-reason"]');
  if (reason) toggleElem(reason, !user);
  const access = dialog?.querySelector<HTMLElement>('[data-role="invite-access"]');
  if (access) toggleElem(access, Boolean(user));
  const hint = dialog?.querySelector<HTMLElement>('[data-role="invite-hint"]');
  if (hint) hint.textContent = user ? fillMessage(hint.getAttribute('data-template'), {name: user.name}) : '';
}

type ArticleUserSearchParts = {
  input: HTMLInputElement,
  prompt: HTMLElement,
  selection: HTMLElement,
  avatar: HTMLImageElement,
  name: HTMLElement,
  login: HTMLElement,
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
  const login = selection.querySelector<HTMLElement>('.article-user-search-login');
  const clear = selection.querySelector<HTMLElement>('.article-user-search-clear');
  if (!prompt || !avatar || !name || !login || !clear) return null;
  return {input, prompt, selection, avatar, name, login, clear};
}

// Resets a user search back to its searchable state, called when the user is cleared
// and every time its dialog is reopened.
function resetArticleUserSearch(elSearch: HTMLElement): void {
  const parts = articleUserSearchParts(elSearch);
  if (!parts) return;
  parts.input.value = '';
  parts.avatar.src = '';
  parts.name.textContent = '';
  parts.login.textContent = '';
  hideElem(parts.selection);
  showElem(parts.prompt);
  fomanticQuery(elSearch).search('hide results');
}

// Turns a user field into a search box listing the users returned by its search url. A
// user is shown by their full name, falling back to the username, with the @username
// beside it, while the submitted value is always the username, which is what the
// backend resolves the user by. onChange receives the picked user, or null when none is.
function bindArticleUserSearch(elSearch: HTMLElement, onChange: (user: PickedUser | null) => void): void {
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
            // the result template renders "title" and "description" as HTML
            // (preserveHTML), the raw values are kept separately for the picked user
            title: htmlEscape(name),
            description: htmlEscape(`@${user.login}`),
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
      parts.login.textContent = `@${result.login}`;
      hideElem(parts.prompt);
      showElem(parts.selection);
      onChange({name: result.rawName || result.login, login: result.login});
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
    bindArticleUserSearch(elTransfer, (user) => {
      transferOwnerSelected = Boolean(user);
      syncTransferConfirmInput();
    });
  }
  const elInvite = document.querySelector<HTMLElement>('#article-collaborator-invite-search');
  if (elInvite) bindArticleUserSearch(elInvite, syncInviteDialog);
}

// Puts a reopened dialog back in its initial state: the confirmation, the picked user and
// the access level must be entered again.
function resetArticleSettingsModal(modal: HTMLElement): void {
  for (const elSearch of modal.querySelectorAll<HTMLElement>('.article-user-search')) {
    resetArticleUserSearch(elSearch);
  }
  transferOwnerSelected = false;
  syncTransferConfirmInput();
  syncInviteDialog(null);

  // the hidden input carries the last selection, the default comes from the markup
  const elRole = modal.querySelector<HTMLElement>('#article-collaborator-role');
  const defaultRole = elRole?.getAttribute('data-default-value');
  if (elRole && defaultRole) fomanticQuery(elRole).dropdown('set selected', defaultRole);

  for (const input of modal.querySelectorAll<HTMLInputElement>('[data-article-confirm-value]')) {
    input.value = '';
    syncConfirmInput(input);
  }
}

// Shows one settings section (general, collaborators) and hides the other, both are
// rendered so switching does not reload the article.
function showArticleSettingsSection(section: string): void {
  for (const item of document.querySelectorAll('[data-article-settings-section]')) {
    const active = item.getAttribute('data-article-settings-section') === section;
    item.classList.toggle('active', active);
    item.setAttribute('aria-pressed', String(active));
  }
  for (const panel of document.querySelectorAll('[data-article-settings-panel]')) {
    toggleElem(panel, panel.getAttribute('data-article-settings-panel') === section);
  }
  document.querySelector('#article-settings-root')?.setAttribute('data-section', section);
}

// Marks the item of a menu that matches the value, for its tick.
function markMenuItem(dropdownEl: HTMLElement, value: string): void {
  for (const item of dropdownEl.querySelectorAll('.menu > .item')) {
    const current = item.getAttribute('data-value') === value;
    item.classList.toggle('active', current);
    item.classList.toggle('selected', current);
  }
}

// the confirm button's listener for the dialog's current opening
let removeListener: AbortController | null = null;

// The remove dialog (#461, figma "Remove collaborator – confirm" 7619:68568 and "Remove
// – states" 7675:74699). Its words depend on who is removed: one person is named, a
// waiting invite is cancelled instead, several are counted. While the removal is sent,
// its button reads "Removing…" and the dialog cannot be closed; a refusal shows in a
// banner under its header and the buttons work again; a success reloads the
// collaborators, with its banner under the navbar.
function openRemoveCollaboratorsModal(rows: HTMLElement[]): void {
  const modal = document.querySelector<HTMLElement>('#article-collaborators-remove-modal');
  const form = document.querySelector<HTMLFormElement>('#article-collaborators-remove-form');
  if (!modal || !form || !rows.length) return;
  const one = rows.length === 1 ? rows[0] : null;
  const name = one?.getAttribute('data-name') ?? '';
  const pending = one?.getAttribute('data-pending') === 'true';
  const attr = (key: string) => modal.getAttribute(key);

  let title: string;
  let desc: string;
  let confirm: string;
  if (one && pending) {
    title = fillMessage(attr('data-title-invite'), {name});
    desc = fillMessage(attr('data-desc-invite'), {name});
    confirm = attr('data-confirm-invite') ?? '';
  } else if (one) {
    title = attr('data-title-one') ?? '';
    desc = fillMessage(attr('data-desc-one'), {name});
    confirm = attr('data-confirm') ?? '';
  } else {
    title = fillMessage(attr('data-title-many'), {count: rows.length});
    desc = attr('data-desc-many') ?? '';
    confirm = attr('data-confirm') ?? '';
  }
  const failed = one ? fillMessage(attr('data-failed-one'), {name}) : fillMessage(attr('data-failed-many'), {count: rows.length});

  modal.querySelector('[data-role="dialog-title"]').textContent = title;
  modal.querySelector('[data-role="remove-desc"]').textContent = desc;
  const banner = modal.querySelector<HTMLElement>('[data-role="remove-error"]');
  hideElem(banner);
  const submit = modal.querySelector<HTMLButtonElement>('#article-collaborators-remove-submit');
  submit.textContent = confirm;
  submit.disabled = false;

  const $modal = fomanticQuery(modal);
  $modal.modal('setting', 'closable', true);
  // the dialog is reused: the confirmation of the last opening must not run again
  removeListener?.abort();
  removeListener = new AbortController();
  submit.addEventListener('click', async () => {
    submit.disabled = true;
    submit.textContent = attr('data-removing') ?? '';
    for (const el of modal.querySelectorAll<HTMLButtonElement>('.actions .cancel, [data-dialog-close]')) el.disabled = true;
    $modal.modal('setting', 'closable', false);
    hideElem(banner);

    const data = new FormData(form);
    data.delete('uid');
    for (const row of rows) {
      const uid = row.querySelector<HTMLInputElement>('[data-article-collaborator-select]')?.value;
      if (uid) data.append('uid', uid);
    }
    let message = failed;
    try {
      const response = await POST(form.action, {data, headers: {Accept: 'application/json'}});
      let json: {redirect?: string, errorMessage?: string} = {};
      try {
        json = await response.json();
      } catch {
        // a page that is not JSON (a server error) keeps the generic message
      }
      if (response.ok && json.redirect) {
        window.location.href = json.redirect;
        return;
      }
      // (HTML, Tr escapes it: the banner shows its text)
      if (json.errorMessage) message = new DOMParser().parseFromString(json.errorMessage, 'text/html').body.textContent;
    } catch {
      // the generic message says to try again
    }
    banner.querySelector('[data-role="dialog-banner-text"]').textContent = message;
    showElem(banner);
    submit.disabled = false;
    submit.textContent = confirm;
    for (const el of modal.querySelectorAll<HTMLButtonElement>('.actions .cancel, [data-dialog-close]')) el.disabled = false;
    $modal.modal('setting', 'closable', true);
    // the disabled button lost the focus to the page: it goes back to the dialog
    submit.focus();
  }, {signal: removeListener.signal});
  $modal.modal('show');
}

// The collaborators list: search, role filter, selection with bulk removal and the role
// dropdown of each row. The list is short, so filtering happens on the rendered rows.
function initArticleCollaborators(): void {
  const root = document.querySelector<HTMLElement>('#article-settings-collaborators');
  if (!root) return;

  const rows = Array.from(root.querySelectorAll<HTMLElement>('[data-article-collaborator]'));
  const elSelectAll = root.querySelector<HTMLInputElement>('#article-collaborators-select-all');
  const elSelectLabel = root.querySelector<HTMLElement>('[data-role="select-all-label"]');
  const elRemoveSelected = root.querySelector<HTMLButtonElement>('#article-collaborators-remove-selected');
  const elRoleFilter = root.querySelector<HTMLElement>('#article-collaborators-role-filter');
  const elSearch = root.querySelector<HTMLInputElement>('#article-collaborators-search');
  const elNoMatch = root.querySelector<HTMLElement>('#article-collaborators-no-match');
  let roleFilter = '';

  // The search field is in the removal form, which has no submit button: Enter in it
  // would submit the form, removing whoever is checked without asking. The form is only
  // ever sent by the confirmation dialog.
  root.querySelector<HTMLFormElement>('#article-collaborators-remove-form')?.addEventListener('submit', (e) => e.preventDefault());

  const rowCheckbox = (row: HTMLElement) => row.querySelector<HTMLInputElement>('[data-article-collaborator-select]');
  const visibleRows = () => rows.filter((row) => !row.classList.contains('tw-hidden'));
  const visibleCheckboxes = () => visibleRows().map(rowCheckbox).filter(Boolean);

  // "1 selected" and "Remove selected" in place of "Select all" and the Role filter
  // (figma "Collaborators – 1 selected" 7618:67929)
  const syncSelection = () => {
    const checkboxes = visibleCheckboxes();
    const checked = checkboxes.filter((checkbox) => checkbox.checked).length;
    if (elSelectAll) {
      elSelectAll.checked = checkboxes.length > 0 && checked === checkboxes.length;
      elSelectAll.indeterminate = checked > 0 && checked < checkboxes.length;
      elSelectAll.disabled = checkboxes.length === 0;
    }
    if (elSelectLabel) {
      elSelectLabel.textContent = checked > 0 ?
        fillMessage(elSelectLabel.getAttribute('data-label-selected'), {count: checked}) :
        (elSelectLabel.getAttribute('data-label-all') ?? '');
    }
    if (elRemoveSelected) toggleElem(elRemoveSelected, checked > 0);
    if (elRoleFilter) toggleElem(elRoleFilter, checked === 0);
  };

  const applyFilter = () => {
    const query = elSearch?.value.trim().toLowerCase() ?? '';
    let shown = 0;
    for (const row of rows) {
      const matches = (!query || row.getAttribute('data-search').toLowerCase().includes(query)) &&
        (!roleFilter || row.getAttribute('data-collaborator-role') === roleFilter);
      toggleElem(row, matches);
      // a collaborator filtered out of sight must not be removed along with the visible ones
      const checkbox = rowCheckbox(row);
      if (!matches && checkbox) checkbox.checked = false;
      if (matches) shown++;
    }
    if (elNoMatch) toggleElem(elNoMatch, rows.length > 0 && shown === 0);
    syncSelection();
  };

  elSelectAll?.addEventListener('change', () => {
    for (const checkbox of visibleCheckboxes()) checkbox.checked = elSelectAll.checked;
    syncSelection();
  });
  for (const row of rows) rowCheckbox(row)?.addEventListener('change', syncSelection);
  elSearch?.addEventListener('input', applyFilter);

  if (elRoleFilter) {
    // the filter keeps reading "Role", picking a role ticks it in the menu instead; the
    // button is in the brand colour while a role is filtered
    const $roleFilter = fomanticQuery(elRoleFilter);
    $roleFilter.dropdown({
      action(_text: string, value: string) {
        $roleFilter.dropdown('hide');
        roleFilter = value;
        markMenuItem(elRoleFilter, value);
        elRoleFilter.classList.toggle('tw-text-primary', Boolean(value));
        applyFilter();
      },
    });
  }

  elRemoveSelected?.addEventListener('click', () => {
    openRemoveCollaboratorsModal(visibleRows().filter((row) => rowCheckbox(row)?.checked));
  });
  // removing a single collaborator names them in the dialog, whatever else is selected
  for (const button of root.querySelectorAll<HTMLElement>('[data-article-collaborator-remove]')) {
    button.addEventListener('click', () => {
      const row = button.closest<HTMLElement>('[data-article-collaborator]');
      if (row) openRemoveCollaboratorsModal([row]);
    });
  }

  for (const dropdownEl of root.querySelectorAll<HTMLElement>('.ui.dropdown.article-collaborator-role')) {
    const textEl = dropdownEl.querySelector<HTMLElement>(':scope > .text');
    const row = dropdownEl.closest<HTMLElement>('[data-article-collaborator]');
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
          markMenuItem(dropdownEl, value);
          row?.setAttribute('data-collaborator-role', value);
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

// Opens the Article settings dialogs (invite/transfer/archive/delete) and switches the
// settings sections. Submission is handled by the forms inside the dialogs.
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

  // the × of a dialog's header (Fomantic only closes on its own direct "close" children)
  addDelegatedEventListener(document, 'click', '.forkana-dialog [data-dialog-close]', (el: HTMLButtonElement) => {
    if (el.disabled) return;
    const modal = el.closest<HTMLElement>('.ui.modal');
    if (modal) fomanticQuery(modal).modal('hide');
  });

  addDelegatedEventListener(document, 'click', '[data-article-settings-section]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    showArticleSettingsSection(el.getAttribute('data-article-settings-section'));
  });

  addDelegatedEventListener(document, 'input', '[data-article-confirm-value]', (el: HTMLInputElement) => {
    syncConfirmInput(el);
  });
}
