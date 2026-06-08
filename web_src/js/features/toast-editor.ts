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

function getRawUrl(src: string): string {
  const parts = window.location.pathname.split('/');
  if (parts.length < 5) return src;
  const owner = parts[1];
  const repo = parts[2];
  const branch = parts[4];

  const dirPathParts = parts.slice(5, -1);

  let relativePath = src;
  if (relativePath.startsWith('./')) {
    relativePath = relativePath.substring(2);
  }

  let dirParts = dirPathParts;
  while (relativePath.startsWith('../')) {
    relativePath = relativePath.substring(3);
    if (dirParts.length > 0) {
      dirParts = dirParts.slice(0, -1);
    }
  }
  const cleanDirPath = dirParts.length > 0 ? `${dirParts.join('/')}/` : '';

  const subUrl = window.config.appSubUrl || '';
  return `${subUrl}/${owner}/${repo}/raw/branch/${branch}/${cleanDirPath}${relativePath}`;
}

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
    customHTMLRenderer: {
      image(node: any, context: any) {
        const {skipChildren, getChildrenText} = context;
        skipChildren();
        const altText = getChildrenText(node);
        let src = node.destination;
        if (src && !src.startsWith('data:') && !src.startsWith('http:') && !src.startsWith('https:') && !src.startsWith('/') && !src.startsWith('#')) {
          src = getRawUrl(src);
        }
        return [
          {type: 'openTag', tagName: 'img', attributes: {src, alt: altText}},
          {type: 'closeTag', tagName: 'img'},
        ];
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
