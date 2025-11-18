import {hideElem, showElem, toggleElem} from '../utils/dom.ts';
import {htmlEscape} from '../utils/html.ts';
import {fomanticQuery} from '../modules/fomantic/base.ts';
import {sanitizeRepoName, generateRepoNameFromSubject} from './repo-common.ts';

const {appSubUrl} = window.config;

function initRepoNewTemplateSearch(form: HTMLFormElement, updateSubmitButtonState: () => void) {
  const elCreateRepoErrorMessage = form.querySelector('#create-repo-error-message');
  const elRepoOwnerDropdown = form.querySelector('#repo_owner_dropdown');
  const elRepoTemplateDropdown = form.querySelector<HTMLInputElement>('#repo_template_search');
  const inputRepoTemplate = form.querySelector<HTMLInputElement>('#repo_template');
  const elTemplateUnits = form.querySelector('#template_units');
  const elNonTemplate = form.querySelector('#non_template');
  const checkTemplate = function () {
    const hasSelectedTemplate = inputRepoTemplate.value !== '' && inputRepoTemplate.value !== '0';
    toggleElem(elTemplateUnits, hasSelectedTemplate);
    toggleElem(elNonTemplate, !hasSelectedTemplate);
  };
  inputRepoTemplate.addEventListener('change', checkTemplate);
  checkTemplate();

  const $repoOwnerDropdown = fomanticQuery(elRepoOwnerDropdown);
  const $repoTemplateDropdown = fomanticQuery(elRepoTemplateDropdown);
  const onChangeOwner = function () {
    const ownerId = $repoOwnerDropdown.dropdown('get value');
    const $ownerItem = $repoOwnerDropdown.dropdown('get item', ownerId);
    hideElem(elCreateRepoErrorMessage);
    if ($ownerItem?.length) {
      const elOwnerItem = $ownerItem[0];
      elCreateRepoErrorMessage.textContent = elOwnerItem.getAttribute('data-create-repo-disallowed-prompt') ?? '';
      const hasError = Boolean(elCreateRepoErrorMessage.textContent);
      toggleElem(elCreateRepoErrorMessage, hasError);
    }
    updateSubmitButtonState();
    $repoTemplateDropdown.dropdown('setting', {
      apiSettings: {
        url: `${appSubUrl}/repo/search?q={query}&template=true&priority_owner_id=${ownerId}`,
        onResponse(response: any) {
          const results = [];
          results.push({name: '', value: ''}); // empty item means not using template
          for (const tmplRepo of response.data) {
            results.push({
              name: htmlEscape(tmplRepo.repository.full_name),
              value: String(tmplRepo.repository.id),
            });
          }
          $repoTemplateDropdown.fomanticExt.onResponseKeepSelectedItem($repoTemplateDropdown, inputRepoTemplate.value);
          return {results};
        },
        cache: false,
      },
    });
  };
  $repoOwnerDropdown.dropdown('setting', 'onChange', onChangeOwner);
  onChangeOwner();
}

export function initRepoNew() {
  const pageContent = document.querySelector('.page-content.repository.new-repo');
  if (!pageContent) return;

  const form = document.querySelector<HTMLFormElement>('.new-repo-form');
  const inputGitIgnores = form.querySelector<HTMLInputElement>('input[name="gitignores"]');
  const inputLicense = form.querySelector<HTMLInputElement>('input[name="license"]');
  const inputAutoInit = form.querySelector<HTMLInputElement>('input[name="auto_init"]');
  const updateUiAutoInit = () => {
    inputAutoInit.checked = Boolean(inputGitIgnores.value || inputLicense.value);
  };
  inputGitIgnores.addEventListener('change', updateUiAutoInit);
  inputLicense.addEventListener('change', updateUiAutoInit);
  updateUiAutoInit();

  const inputRepoName = form.querySelector<HTMLInputElement>('input[name="repo_name"]');
  const inputSubject = form.querySelector<HTMLInputElement>('input[name="subject"]');
  const inputPrivate = form.querySelector<HTMLInputElement>('input[name="private"]');

  // Track whether the user has manually edited the repo name
  let userHasEditedRepoName = false;

  const updateUiRepoName = () => {
    const helps = form.querySelectorAll(`.help[data-help-for-repo-name]`);
    hideElem(helps);
    let help = form.querySelector(`.help[data-help-for-repo-name="${CSS.escape(inputRepoName.value)}"]`);
    if (!help) help = form.querySelector(`.help[data-help-for-repo-name=""]`);
    showElem(help);
    const repoNamePreferPrivate: Record<string, boolean> = {'.profile': false, '.profile-private': true};
    const preferPrivate = repoNamePreferPrivate[inputRepoName.value];
    // inputPrivate might be disabled because site admin "force private"
    if (preferPrivate !== undefined && !inputPrivate.closest('.disabled, [disabled]')) {
      inputPrivate.checked = preferPrivate;
    }
  };

  // Auto-generate repo name from subject
  const updateRepoNameFromSubject = () => {
    if (!userHasEditedRepoName && inputSubject.value) {
      const generatedName = generateRepoNameFromSubject(inputSubject.value);
      inputRepoName.value = generatedName;
      updateUiRepoName();
    }
  };

  // Handle subject input changes
  inputSubject.addEventListener('input', updateRepoNameFromSubject);

  // Handle manual repo name changes
  inputRepoName.addEventListener('input', () => {
    userHasEditedRepoName = Boolean(inputRepoName.value.trim());
    if (!userHasEditedRepoName) {
      updateRepoNameFromSubject();
    }
    updateUiRepoName();
  });

  inputRepoName.addEventListener('change', () => {
    inputRepoName.value = sanitizeRepoName(inputRepoName.value);
    updateUiRepoName();
  });

  // Ensure repo name is populated on initial load (e.g., when subject is prefilled)
  updateRepoNameFromSubject();
  updateUiRepoName();

  // Handle template_requirements checkbox validation
  const inputTemplateRequirements = form.querySelector<HTMLInputElement>('#template_requirements');
  const elSubmitButton = form.querySelector<HTMLButtonElement>('#create-repo-submit-button');
  
  const updateSubmitButtonState = () => {
    if (!elSubmitButton || !inputTemplateRequirements) return;
    
    const isTemplateRequirementsChecked = inputTemplateRequirements.checked;
    const elCreateRepoErrorMessage = form.querySelector('#create-repo-error-message');
    const hasOwnerError = elCreateRepoErrorMessage && elCreateRepoErrorMessage.textContent?.trim() !== '';
    
    // Disable submit button if template_requirements is not checked or if there's an owner error
    elSubmitButton.disabled = !isTemplateRequirementsChecked || hasOwnerError;
  };

  // Listen for checkbox changes
  if (inputTemplateRequirements) {
    inputTemplateRequirements.addEventListener('change', updateSubmitButtonState);
    // Initial state check
    updateSubmitButtonState();
  }

  initRepoNewTemplateSearch(form, updateSubmitButtonState);
}
