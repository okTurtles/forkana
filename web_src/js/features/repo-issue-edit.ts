import {handleReply} from './repo-issue.ts';
import {getComboMarkdownEditor, initComboMarkdownEditor, ComboMarkdownEditor} from './comp/ComboMarkdownEditor.ts';
import {getToastCommentEditor, initToastCommentEditor, ToastCommentEditor} from './comp/ToastCommentEditor.ts';
import {POST} from '../modules/fetch.ts';
import {showErrorToast} from '../modules/toast.ts';
import {hideElem, querySingleVisibleElem, showElem, type DOMEvent} from '../utils/dom.ts';
import {attachRefIssueContextPopup} from './contextpopup.ts';
import {triggerUploadStateChanged} from './comp/EditorUpload.ts';
import {convertHtmlToMarkdown} from '../markup/html2markdown.ts';
import {applyAreYouSure, reinitializeAreYouSure} from '../vendor/jquery.are-you-sure.ts';

async function tryOnEditContent(e: DOMEvent<MouseEvent>) {
  const clickTarget = e.target.closest('.edit-content');
  if (!clickTarget) return;

  e.preventDefault();
  const commentContent = clickTarget.closest('.comment-header').nextElementSibling;
  const editContentZone = commentContent.querySelector('.edit-content-zone');
  let renderContent = commentContent.querySelector('.render-content');
  const rawContent = commentContent.querySelector('.raw-content');

  // Support both Toast and Combo editors
  type EditorType = ComboMarkdownEditor | ToastCommentEditor;
  let editor: EditorType;

  const cancelAndReset = (e: Event) => {
    e.preventDefault();
    showElem(renderContent);
    hideElem(editContentZone);
    editor.dropzoneReloadFiles();
  };

  const saveAndRefresh = async (e: Event) => {
    e.preventDefault();
    // we are already in a form, do not bubble up to the document otherwise there will be other "form submit handlers"
    // at the moment, the form submit event conflicts with initRepoDiffConversationForm (global '.conversation-holder form' event handler)
    e.stopPropagation();
    renderContent.classList.add('is-loading');
    showElem(renderContent);
    hideElem(editContentZone);
    try {
      const params = new URLSearchParams({
        content: editor.value(),
        context: editContentZone.getAttribute('data-context'),
        content_version: editContentZone.getAttribute('data-content-version'),
      });
      for (const file of editor.dropzoneGetFiles() ?? []) {
        params.append('files[]', file);
      }

      const response = await POST(editContentZone.getAttribute('data-update-url'), {data: params});
      const data = await response.json();
      if (!response.ok) {
        showErrorToast(data?.errorMessage ?? window.config.i18n.error_occurred);
        return;
      }

      reinitializeAreYouSure(editContentZone.querySelector('form')); // the form is no longer dirty
      editContentZone.setAttribute('data-content-version', data.contentVersion);

      // replace the render content with new one, to trigger re-initialization of all features
      const newRenderContent = renderContent.cloneNode(false) as HTMLElement;
      newRenderContent.innerHTML = data.content;
      renderContent.replaceWith(newRenderContent);
      renderContent = newRenderContent;

      rawContent.textContent = editor.value();
      const refIssues = renderContent.querySelectorAll<HTMLElement>('p .ref-issue');
      attachRefIssueContextPopup(refIssues);

      if (!commentContent.querySelector('.dropzone-attachments')) {
        if (data.attachments !== '') {
          commentContent.insertAdjacentHTML('beforeend', data.attachments);
        }
      } else if (data.attachments === '') {
        commentContent.querySelector('.dropzone-attachments').remove();
      } else {
        commentContent.querySelector('.dropzone-attachments').outerHTML = data.attachments;
      }
      editor.dropzoneSubmitReload();
    } catch (error) {
      showErrorToast(`Failed to save the content: ${error}`);
      console.error(error);
    } finally {
      renderContent.classList.remove('is-loading');
    }
  };

  // Show write/preview tab and copy raw content as needed
  showElem(editContentZone);
  hideElem(renderContent);

  // Try to get existing editor (Toast first, then Combo)
  const existingToastEditor = getToastCommentEditor(editContentZone.querySelector('.toast-comment-editor'));
  const existingComboEditor = getComboMarkdownEditor(editContentZone.querySelector('.combo-markdown-editor'));
  editor = existingToastEditor || existingComboEditor;

  if (!editor) {
    editContentZone.innerHTML = document.querySelector('#issue-comment-editor-template').innerHTML;
    const form = editContentZone.querySelector('form');
    applyAreYouSure(form);
    const saveButton = querySingleVisibleElem<HTMLButtonElement>(editContentZone, '.ui.primary.button');
    const cancelButton = querySingleVisibleElem<HTMLButtonElement>(editContentZone, '.ui.cancel.button');

    // Check if template uses Toast editor or Combo editor
    const toastEditorContainer = editContentZone.querySelector('.toast-comment-editor');
    const comboEditorContainer = editContentZone.querySelector('.combo-markdown-editor');

    if (toastEditorContainer) {
      editor = await initToastCommentEditor(toastEditorContainer as HTMLElement);
      const syncUiState = () => saveButton.disabled = editor.isUploading();
      editor.container.addEventListener(ToastCommentEditor.EventUploadStateChanged, syncUiState);
    } else if (comboEditorContainer) {
      editor = await initComboMarkdownEditor(comboEditorContainer as HTMLElement);
      const syncUiState = () => saveButton.disabled = editor.isUploading();
      editor.container.addEventListener(ComboMarkdownEditor.EventUploadStateChanged, syncUiState);
    } else {
      throw new Error('No editor container found in template');
    }

    cancelButton.addEventListener('click', cancelAndReset);
    form.addEventListener('submit', saveAndRefresh);
  }

  // FIXME: ideally here should reload content and attachment list from backend for existing editor, to avoid losing data
  if (!editor.value()) {
    editor.value(rawContent.textContent);
  }
  editor.switchTabToEditor();
  editor.focus();
  triggerUploadStateChanged(editor.container);
}

function extractSelectedMarkdown(container: HTMLElement) {
  const selection = window.getSelection();
  if (!selection.rangeCount) return '';
  const range = selection.getRangeAt(0);
  if (!container.contains(range.commonAncestorContainer)) return '';

  // todo: if commonAncestorContainer parent has "[data-markdown-original-content]" attribute, use the parent's markdown content
  // otherwise, use the selected HTML content and respect all "[data-markdown-original-content]/[data-markdown-generated-content]" attributes
  const contents = selection.getRangeAt(0).cloneContents();
  const el = document.createElement('div');
  el.append(contents);
  return convertHtmlToMarkdown(el);
}

async function tryOnQuoteReply(e: Event) {
  const clickTarget = (e.target as HTMLElement).closest('.quote-reply');
  if (!clickTarget) return;

  e.preventDefault();
  const contentToQuoteId = clickTarget.getAttribute('data-target');
  const targetRawToQuote = document.querySelector<HTMLElement>(`#${contentToQuoteId}.raw-content`);
  const targetMarkupToQuote = targetRawToQuote.parentElement.querySelector<HTMLElement>('.render-content.markup');
  let contentToQuote = extractSelectedMarkdown(targetMarkupToQuote);
  if (!contentToQuote) contentToQuote = targetRawToQuote.textContent;
  const quotedContent = `${contentToQuote.replace(/^/mg, '> ')}\n\n`;

  let editor;
  if (clickTarget.classList.contains('quote-reply-diff')) {
    const replyBtn = clickTarget.closest('.comment-code-cloud').querySelector<HTMLElement>('button.comment-form-reply');
    editor = await handleReply(replyBtn);
  } else {
    // for normal issue/comment page - try Toast editor first, then Combo
    editor = getToastCommentEditor(document.querySelector('#comment-form .toast-comment-editor')) ||
      getComboMarkdownEditor(document.querySelector('#comment-form .combo-markdown-editor'));
  }

  if (editor.value()) {
    editor.value(`${editor.value()}\n\n${quotedContent}`);
  } else {
    editor.value(quotedContent);
  }
  editor.focus();
  editor.moveCursorToEnd();
}

export function initRepoIssueCommentEdit() {
  document.addEventListener('click', (e) => {
    tryOnEditContent(e); // Edit issue or comment content
    tryOnQuoteReply(e); // Quote reply to the comment editor
  });
}
