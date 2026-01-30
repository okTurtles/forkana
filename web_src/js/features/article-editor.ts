import {createToastEditor} from './toast-editor.ts';
import {submitFormFetchAction} from './common-fetch-action.ts';
import {fomanticQuery} from '../modules/fomantic/base.ts';
import {createElementFromHTML} from '../utils/dom.ts';
import {svg} from '../svg.ts';
import {html, htmlRaw} from '../utils/html.ts';

// Create a confirmation modal with custom button text
function createForkConfirmModal(title: string, body: string, confirmText: string, cancelText: string): HTMLElement {
  return createElementFromHTML(html`
    <div class="ui g-modal-confirm modal">
      <div class="header">${title}</div>
      <div class="content"><p>${body}</p></div>
      <div class="actions">
        <button class="ui cancel button">${htmlRaw(svg('octicon-x'))} ${cancelText}</button>
        <button class="ui primary ok button">${htmlRaw(svg('octicon-check'))} ${confirmText}</button>
      </div>
    </div>
  `.trim());
}

// Show fork confirmation modal and return a promise that resolves to true if confirmed
function showForkConfirmModal(title: string, body: string, confirmText: string, cancelText: string): Promise<boolean> {
  const modal = createForkConfirmModal(title, body, confirmText, cancelText);
  return new Promise((resolve) => {
    let approved = false;
    const $modal = fomanticQuery(modal);
    $modal.modal({
      onApprove() {
        approved = true;
      },
      onHidden() {
        $modal.remove();
        resolve(approved);
      },
    }).modal('show');
  });
}

export function initArticleEditor() {
  const editForm = document.querySelector<HTMLFormElement>('#article-edit-form');
  if (!editForm) return;

  const textarea = document.querySelector<HTMLTextAreaElement>('#edit_area');
  if (!textarea) return;

  // Initialize Toast UI Editor
  (async () => {
    const editor = await createToastEditor(textarea, {
      height: '500px',
      initialEditType: 'wysiwyg',
      previewStyle: 'vertical',
      usageStatistics: false,
      hideModeSwitch: false,  // Allow mode switching
    });

    // Handle Fork Article button (fork and edit in user's own fork)
    const forkArticleButton = document.querySelector<HTMLButtonElement>('#fork-article-button');
    if (forkArticleButton && !forkArticleButton.classList.contains('disabled')) {
      forkArticleButton.addEventListener('click', async () => {
        // Get confirmation modal content from data attributes
        const title = forkArticleButton.getAttribute('data-fork-confirm-title') || 'Confirm Fork';
        const body = forkArticleButton.getAttribute('data-fork-confirm-body') || 'Are you sure you want to fork this article?';
        const confirmText = forkArticleButton.getAttribute('data-fork-confirm-yes') || 'Yes, Fork';
        const cancelText = forkArticleButton.getAttribute('data-fork-confirm-cancel') || 'Cancel';

        // Show confirmation modal
        const confirmed = await showForkConfirmModal(title, body, confirmText, cancelText);
        if (!confirmed) {
          return; // User cancelled, do nothing
        }

        // Set fork_and_edit to true, submit_change_request to false
        const forkAndEditField = document.querySelector<HTMLInputElement>('#fork_and_edit');
        const submitChangeRequestField = document.querySelector<HTMLInputElement>('#submit_change_request');
        if (forkAndEditField) forkAndEditField.value = 'true';
        if (submitChangeRequestField) submitChangeRequestField.value = 'false';

        // Update textarea with editor content before submission
        textarea.value = editor.getMarkdown();

        // Submit the form using fetch action to handle JSON redirect response
        await submitFormFetchAction(editForm);
      });
    }

    // Handle Pre-Submit Changes button (opens modal for title/description)
    const preSubmitChangesButton = document.querySelector<HTMLButtonElement>('#pre-submit-changes-button');
    const submitCRModal = document.querySelector<HTMLElement>('#submit-change-request-modal');

    if (preSubmitChangesButton && submitCRModal && !preSubmitChangesButton.classList.contains('disabled')) {
      const $modal = fomanticQuery(submitCRModal);
      const modalTitleInput = submitCRModal.querySelector<HTMLInputElement>('#modal-cr-title');
      const modalDescriptionInput = submitCRModal.querySelector<HTMLTextAreaElement>('#modal-cr-description');

      preSubmitChangesButton.addEventListener('click', () => {
        // Clear previous values when opening modal
        if (modalTitleInput) modalTitleInput.value = '';
        if (modalDescriptionInput) modalDescriptionInput.value = '';

        // Show the modal
        $modal.modal({
          closable: true,
          onApprove: () => {
            // Validate title is not empty
            const title = modalTitleInput?.value.trim() || '';
            if (!title) {
              // Show validation error - add error class to field
              modalTitleInput?.closest('.field')?.classList.add('error');
              return false; // Prevent modal from closing
            }

            // Get values from modal
            const description = modalDescriptionInput?.value.trim() || '';

            // Set form hidden fields
            const forkAndEditField = document.querySelector<HTMLInputElement>('#fork_and_edit');
            const submitChangeRequestField = document.querySelector<HTMLInputElement>('#submit_change_request');
            const changeRequestTitleField = document.querySelector<HTMLInputElement>('#change_request_title');
            const changeRequestDescriptionField = document.querySelector<HTMLInputElement>('#change_request_description');

            if (forkAndEditField) forkAndEditField.value = 'false';
            if (submitChangeRequestField) submitChangeRequestField.value = 'true';
            if (changeRequestTitleField) changeRequestTitleField.value = title;
            if (changeRequestDescriptionField) changeRequestDescriptionField.value = description;

            // Update textarea with editor content before submission
            textarea.value = editor.getMarkdown();

            // Submit the form using fetch action to handle JSON redirect response
            submitFormFetchAction(editForm);

            return true; // Allow modal to close
          },
          onHidden: () => {
            // Clear error state when modal is closed
            modalTitleInput?.closest('.field')?.classList.remove('error');
          },
        }).modal('show');
      });

      // Clear error state when user starts typing in title field
      modalTitleInput?.addEventListener('input', () => {
        modalTitleInput.closest('.field')?.classList.remove('error');
      });
    }

    // Handle direct Submit Changes button (for repo owners - no modal needed)
    const submitChangesButton = document.querySelector<HTMLButtonElement>('#submit-changes-button');
    if (submitChangesButton && !submitChangesButton.classList.contains('disabled')) {
      submitChangesButton.addEventListener('click', async () => {
        // Update textarea with editor content before submission
        textarea.value = editor.getMarkdown();

        // Submit the form using fetch action to handle JSON redirect response
        await submitFormFetchAction(editForm);
      });
    }
  })();
}
