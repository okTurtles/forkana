// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import '@toast-ui/editor/dist/toastui-editor.css';
import {createBase64WidgetRule, installBase64WidgetPatch} from './comp/base64ImageWidget.ts';
import {showErrorToast} from '../modules/toast.ts';

const MAX_FILE_SIZE = 20 * 1024 * 1024; // 20MB

export type ToastEditorOptions = {
  height?: string;
  initialEditType?: 'markdown' | 'wysiwyg';
  previewStyle?: 'tab' | 'vertical';
  usageStatistics?: boolean;
  hideModeSwitch?: boolean;
  toolbarItems?: string[][];
};

export async function createToastEditor(
  textarea: HTMLTextAreaElement,
  options: ToastEditorOptions = {},
): Promise<Editor> {
  const {
    height = '500px',
    initialEditType = 'wysiwyg',
    previewStyle = 'vertical',
    usageStatistics = false,
    hideModeSwitch = false,   // must be false to show the tabs
    toolbarItems = [
      ['heading', 'bold', 'italic'],
      ['indent', 'outdent', 'code', 'link'],
      ['ul', 'ol', 'task'],
      ['image', 'table'],
    ],
  } = options;

  // Use the existing container from the template
  let container = document.querySelector<HTMLElement>('#toast-editor-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-editor-container';
    container.className = 'toast-editor-container';
    container.style.height = height;
    if (!textarea.parentNode) throw new Error('Parent node absent');
    textarea.parentNode.append(container);
  } else {
    container.style.height = height;
  }

  // Initialize Toast UI Editor
  // eslint-disable-next-line @typescript-eslint/no-redundant-type-constituents -- Editor type has issues
  const editorRef = {current: null as Editor | null};
  const widgetRules = [
    createBase64WidgetRule((): Editor => editorRef.current!),
  ];
  const editor: Editor = new Editor({
    el: container,
    height,
    initialEditType,
    previewStyle,
    usageStatistics,
    hideModeSwitch,
    toolbarItems,
    events: {
      change: () => {
        const content = editorRef.current!.getMarkdown();
        textarea.value = content;
        textarea.dispatchEvent(new Event('change'));
      },
    },
    hooks: {
      addImageBlobHook: (blob: Blob, callback: (url: string, text?: string) => void) => {
        if (blob.size > MAX_FILE_SIZE) {
          showErrorToast(`File exceeds the limit of 20MB and cannot be saved.`);
          return;
        }
        const reader = new FileReader();
        reader.addEventListener('load', () => {
          callback(reader.result as string, (blob as File).name || 'image');
        });
        reader.readAsDataURL(blob);
      },
    },
    widgetRules,
  });
  editorRef.current = editor;

  // Intercept drop and paste events in the capture phase to block files > 20MB
  container.addEventListener('drop', (e: DragEvent) => {
    const files = e.dataTransfer?.files;
    if (files && files.length > 0) {
      for (const file of files) {
        if (file.size > MAX_FILE_SIZE) {
          e.preventDefault();
          e.stopPropagation();
          showErrorToast(`File "${file.name}" exceeds the limit of 20MB and cannot be saved.`);
          return;
        }
      }
    }
  }, true);

  container.addEventListener('paste', (e: ClipboardEvent) => {
    const files = e.clipboardData?.files;
    if (files && files.length > 0) {
      for (const file of files) {
        if (file.size > MAX_FILE_SIZE) {
          e.preventDefault();
          e.stopPropagation();
          showErrorToast(`File "${file.name}" exceeds the limit of 20MB and cannot be saved.`);
          return;
        }
      }
    }
  }, true);

  // Override getMarkdown to strip internal $$widget placeholders
  installBase64WidgetPatch(editor);

  // Set initial content
  if (textarea.value) {
    editor.setMarkdown(textarea.value);
  }

  // Rename mode switch labels
  const switchEl = container.querySelector('.toastui-editor-mode-switch');
  if (switchEl) {
    for (const el of switchEl.querySelectorAll('.tab-item')) {
      if (el.textContent?.trim() === 'WYSIWYG') el.textContent = 'Visual editor';
      else if (el.textContent?.trim() === 'Markdown') el.textContent = 'Source editor';
    }
  }

  // Hide the original textarea
  textarea.style.display = 'none';

  // Remove loading indicator if present
  const loading = document.querySelector('.editor-loading');
  if (loading) loading.remove();

  return editor;
}

export function destroyToastEditor(editor: Editor): void {
  editor.destroy();
}
