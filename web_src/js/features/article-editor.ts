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

    // Handle Submit Changes button
    const submitButton = document.querySelector<HTMLButtonElement>('#submit-changes-button');
    if (submitButton && !submitButton.classList.contains('disabled')) {
      submitButton.addEventListener('click', async () => {
        // Check if this is a fork-and-edit action that needs confirmation
        const isForkAndEdit = submitButton.getAttribute('data-fork-and-edit') === 'true';

        if (isForkAndEdit) {
          // Get confirmation modal content from data attributes
          const title = submitButton.getAttribute('data-fork-confirm-title') || 'Confirm Fork';
          const body = submitButton.getAttribute('data-fork-confirm-body') || 'Are you sure you want to fork this article?';
          const confirmText = submitButton.getAttribute('data-fork-confirm-yes') || 'Yes, Fork';
          const cancelText = submitButton.getAttribute('data-fork-confirm-cancel') || 'Cancel';

          // Show confirmation modal
          const confirmed = await showForkConfirmModal(title, body, confirmText, cancelText);
          if (!confirmed) {
            return; // User cancelled, do nothing
          }
        }

        // Update textarea with editor content before submission
        textarea.value = editor.getMarkdown();

        // Submit the form using fetch action to handle JSON redirect response
        await submitFormFetchAction(editForm);
      });
    }
  })();
}
