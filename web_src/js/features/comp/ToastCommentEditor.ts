/**
 * ToastCommentEditor - A wrapper around Toast UI Editor for comment/issue forms
 * Provides the same API as ComboMarkdownEditor for compatibility with existing code
 */

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import '@toast-ui/editor/dist/toastui-editor.css';
import {hideElem, generateElemId} from '../../utils/dom.ts';
import {
  EventUploadStateChanged,
  triggerUploadStateChanged,
} from './EditorUpload.ts';
import {DropzoneCustomEventReloadFiles, initDropzone} from '../dropzone.ts';

// Event dispatched when editor content changes
export const EventEditorContentChanged = 'ce-editor-content-changed';

export function triggerEditorContentChanged(target: HTMLElement) {
  target.dispatchEvent(new CustomEvent(EventEditorContentChanged));
}

export type ToastCommentEditorOptions = {
  height?: string;
  initialEditType?: 'markdown' | 'wysiwyg';
  previewStyle?: 'tab' | 'vertical';
  toolbarItems?: string[][];
};

type ToastCommentEditorContainer = HTMLElement & {_giteaToastCommentEditor?: ToastCommentEditor};

export class ToastCommentEditor {
  static EventEditorContentChanged = EventEditorContentChanged;
  static EventUploadStateChanged = EventUploadStateChanged;

  public container: ToastCommentEditorContainer;
  // eslint-disable-next-line @typescript-eslint/no-redundant-type-constituents -- Editor type has issues
  public editor: Editor | null = null;

  options: ToastCommentEditorOptions;
  textarea: HTMLTextAreaElement;
  editorWrapper: HTMLElement;

  dropzone: HTMLElement | null = null;
  attachedDropzoneInst: any = null;

  constructor(container: ToastCommentEditorContainer, options: ToastCommentEditorOptions = {}) {
    if (container._giteaToastCommentEditor) throw new Error('ToastCommentEditor already initialized');
    container._giteaToastCommentEditor = this;
    this.options = options;
    this.container = container;
  }

  async init() {
    await this.setupDropzone();
    await this.setupEditor();
  }

  async setupEditor() {
    this.textarea = this.container.querySelector('.toast-editor-textarea');
    this.editorWrapper = this.container.querySelector('.toast-editor-wrapper');

    if (!this.textarea || !this.editorWrapper) {
      throw new Error('ToastCommentEditor: textarea or wrapper element not found');
    }

    // Generate unique ID for this editor instance
    const editorId = generateElemId('toast-editor-');
    this.editorWrapper.id = editorId;

    const {
      height = '200px',
      initialEditType = 'wysiwyg',
      previewStyle = 'vertical',
      toolbarItems = [
        ['heading', 'bold', 'italic'],
        ['indent', 'outdent', 'code', 'link'],
        ['ul', 'ol', 'task'],
        ['image', 'table'],
      ],
    } = this.options;

    this.editorWrapper.style.minHeight = height;

    // Initialize Toast UI Editor
    this.editor = new Editor({
      el: this.editorWrapper,
      height: 'auto',
      minHeight: '0',
      initialEditType,
      previewStyle,
      usageStatistics: false,
      hideModeSwitch: false,
      toolbarItems,
      events: {
        change: () => {
          if (this.editor) {
            const content = this.editor.getMarkdown();
            this.textarea.value = content;
            this.textarea.dispatchEvent(new Event('change'));
            triggerEditorContentChanged(this.container);
          }
        },
      },
    });

    // Set initial content from textarea
    if (this.textarea.value) {
      this.editor.setMarkdown(this.textarea.value);
    }

    // Rename mode switch labels
    const switchEl = this.editorWrapper.querySelector('.toastui-editor-mode-switch');
    if (switchEl) {
      for (const el of switchEl.querySelectorAll('.tab-item')) {
        if (el.textContent?.trim() === 'WYSIWYG') el.textContent = 'Visual editor';
        else if (el.textContent?.trim() === 'Markdown') el.textContent = 'Source editor';
      }
    }

    // Hide the original textarea
    hideElem(this.textarea);

    // Remove loading indicator if present
    const loading = this.container.querySelector('.editor-loading');
    if (loading) loading.remove();

    // Move the dropzone hint text inside the editor so it behaves like a flex item
    // The hint text is expected to be a sibling of .toast-comment-editor within the parent .field container
    const defaultUI = this.editorWrapper.querySelector('.toastui-editor-defaultUI');
    const fieldContainer = this.container.closest('.field');
    const hintText = fieldContainer?.querySelector('.dropzone-hint-text');
    if (defaultUI && hintText) {
      defaultUI.append(hintText);
    }
  }

  async setupDropzone() {
    const dropzoneParentContainer = this.container.getAttribute('data-dropzone-parent-container');
    if (!dropzoneParentContainer) return;

    this.dropzone = this.container.closest(dropzoneParentContainer)?.querySelector('.dropzone');
    if (!this.dropzone) return;

    this.attachedDropzoneInst = await initDropzone(this.dropzone);

    // Dropzone events for upload state tracking
    this.attachedDropzoneInst.on('processing', () => triggerUploadStateChanged(this.container));
    this.attachedDropzoneInst.on('queuecomplete', () => triggerUploadStateChanged(this.container));
  }

  dropzoneGetFiles(): string[] | null {
    if (!this.dropzone) return null;
    return Array.from(this.dropzone.querySelectorAll<HTMLInputElement>('.files [name=files]'), (el) => el.value);
  }

  dropzoneReloadFiles(): void {
    if (!this.dropzone || !this.attachedDropzoneInst) return;
    this.attachedDropzoneInst.emit(DropzoneCustomEventReloadFiles);
  }

  dropzoneSubmitReload(): void {
    if (!this.dropzone || !this.attachedDropzoneInst) return;
    this.attachedDropzoneInst.emit('submit');
    this.attachedDropzoneInst.emit(DropzoneCustomEventReloadFiles);
  }

  isUploading(): boolean {
    if (!this.dropzone || !this.attachedDropzoneInst) return false;
    return this.attachedDropzoneInst.getQueuedFiles().length || this.attachedDropzoneInst.getUploadingFiles().length;
  }

  value(v?: string): string {
    if (v === undefined) {
      if (this.editor) {
        return this.editor.getMarkdown();
      }
      return this.textarea?.value ?? '';
    }

    if (this.editor) {
      this.editor.setMarkdown(v);
    }
    if (this.textarea) {
      this.textarea.value = v;
    }
    return v;
  }

  focus(): void {
    if (this.editor) {
      this.editor.focus();
    }
  }

  moveCursorToEnd(): void {
    if (this.editor) {
      this.editor.focus();
      // Move to the end of the document
      this.editor.moveCursorToEnd();
    }
  }

  async switchToUserPreference(): Promise<void> {
    // Toast UI Editor manages its own mode state, no action needed
  }

  switchTabToEditor(): void {
    // Toast UI Editor doesn't have the same tab concept, but we can focus
    this.focus();
  }

  destroy(): void {
    if (this.editor) {
      this.editor.destroy();
      this.editor = null;
    }
    delete this.container._giteaToastCommentEditor;
  }
}

export function getToastCommentEditor(el: HTMLElement | null): ToastCommentEditor | null {
  if (!el) return null;
  return (el as ToastCommentEditorContainer)._giteaToastCommentEditor ?? null;
}

export async function initToastCommentEditor(
  container: HTMLElement,
  options: ToastCommentEditorOptions = {},
): Promise<ToastCommentEditor> {
  if (!container) {
    throw new Error('initToastCommentEditor: container is null');
  }
  const editor = new ToastCommentEditor(container, options);
  await editor.init();
  return editor;
}
