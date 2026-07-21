// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import '@toast-ui/editor/dist/toastui-editor.css';
import {createBase64WidgetRule, installBase64WidgetPatch} from './comp/base64ImageWidget.ts';
import {showErrorToast} from '../modules/toast.ts';
import {ensureFilesWithinLimit, getMaxAttachmentSize, showFileTooLargeError} from './comp/editorFileLimit.ts';
import {POST} from '../modules/fetch.ts';

export type ToastEditorOptions = {
  height?: string;
  initialEditType?: 'markdown' | 'wysiwyg';
  previewStyle?: 'tab' | 'vertical';
  usageStatistics?: boolean;
  hideModeSwitch?: boolean;
  toolbarItems?: string[][];
};

// resolveRelativeSrc resolves a relative image path (e.g. "./img/a.png", "../b.png")
// against the raw URL of the file being edited. The base URL is provided by the server
// (data-raw-file-url), so it already accounts for appSubUrl and branch names containing
// slashes; native URL resolution handles "./" and "../" segments correctly.
function resolveRelativeSrc(src: string, baseFileUrl: string): string {
  if (!baseFileUrl) return src;
  try {
    const base = new URL(baseFileUrl, window.location.origin);
    return new URL(src, base).href;
  } catch {
    return src;
  }
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

  // Server-provided raw URL of the file being edited, used to resolve relative image paths.
  const rawFileUrl = textarea.getAttribute('data-raw-file-url') || '';
  // Endpoint that stores a pasted/dropped image as a repo attachment and returns its uuid.
  const attachmentUploadUrl = textarea.getAttribute('data-attachment-upload-url') || '';

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
      addImageBlobHook: async (blob: Blob, callback: (url: string, text?: string) => void) => {
        const max = getMaxAttachmentSize();
        if (max && blob.size > max) {
          showFileTooLargeError((blob as File).name || 'image');
          return;
        }
        let name = (blob as File).name || '';
        if (!name.includes('.')) {
          // Clipboard screenshots often arrive without a filename/extension. The server's
          // attachment type check is extension-based, so derive one from the MIME type;
          // otherwise the upload is rejected (e.g. "image/svg+xml" -> "svg").
          const ext = (blob.type.split('/')[1] || 'png').split('+')[0];
          name = name ? `${name}.${ext}` : `image.${ext}`;
        }
        // Upload the image as a repo attachment and reference it by URL. Storing it inline as
        // base64 bloats the Markdown and makes a single line exceed MAX_GIT_DIFF_LINE_CHARACTERS,
        // which suppresses the whole file diff (issue #233).
        if (attachmentUploadUrl) {
          try {
            const form = new FormData();
            form.append('file', blob, name);
            const resp = await POST(attachmentUploadUrl, {data: form});
            if (!resp.ok) throw new Error(`attachment upload failed: ${resp.status}`);
            const data = await resp.json();
            callback(data.url, name);
          } catch (err) {
            console.error(err);
            showErrorToast(window.config.i18n.editor_image_upload_failed || 'Failed to upload the image file.');
          }
          return;
        }
        // Fallback when no upload endpoint is configured: embed as base64 so the image is not lost.
        const reader = new FileReader();
        reader.addEventListener('load', () => {
          callback(reader.result as string, name);
        });
        reader.addEventListener('error', () => {
          showErrorToast(window.config.i18n.editor_image_read_failed || 'Failed to read the image file.');
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
          src = resolveRelativeSrc(src, rawFileUrl);
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

  // Intercept drop and paste events in the capture phase to block oversized files
  container.addEventListener('drop', (e: DragEvent) => {
    if (!ensureFilesWithinLimit(e.dataTransfer?.files)) {
      e.preventDefault();
      e.stopPropagation();
    }
  }, true);

  // Some engines (notably Safari) ignore `clipboardData` passed to the ClipboardEvent
  // constructor, which would make the re-dispatch below carry no data and drop the paste.
  // Detect support once; if unsupported, skip the strip and let the paste proceed normally
  // (the gitdiff base64 placeholder is the backend safety net).
  let canConstructClipboardData = false;
  try {
    canConstructClipboardData = new ClipboardEvent('paste', {clipboardData: new DataTransfer()}).clipboardData !== null;
  } catch {
    canConstructClipboardData = false;
  }

  container.addEventListener('paste', (e: ClipboardEvent) => {
    if (!ensureFilesWithinLimit(e.clipboardData?.files)) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    if (!canConstructClipboardData) return;

    // When the clipboard contains HTML (e.g. content copied from YouTube or other media
    // sites), it may include external thumbnail <img> elements. If left in, Toast UI
    // converts them to base64 blobs via addImageBlobHook, creating very long lines in the
    // saved markdown file and triggering the git diff suppression (issue #233).
    // Rewrite the clipboard as HTML with external images removed, keeping only data: URIs
    // (locally pasted/dropped images that the user deliberately embedded).
    const html = e.clipboardData?.getData('text/html');
    if (html && e.clipboardData?.types.includes('text/html')) {
      const parser = new DOMParser();
      const doc = parser.parseFromString(html, 'text/html');
      let strippedAny = false;
      for (const img of doc.querySelectorAll('img')) {
        const src = img.getAttribute('src') || '';
        if (!src.startsWith('data:')) {
          // Replace external image with its alt text as a plain text node, or remove entirely
          const alt = img.getAttribute('alt');
          if (alt) {
            img.replaceWith(doc.createTextNode(alt));
          } else {
            img.remove();
          }
          strippedAny = true;
        }
      }
      if (strippedAny) {
        e.preventDefault();
        e.stopPropagation();
        const cleanHtml = doc.body.innerHTML;
        const text = e.clipboardData.getData('text/plain');
        const dt = new DataTransfer();
        dt.setData('text/html', cleanHtml);
        dt.setData('text/plain', text);
        // Re-dispatch on the element the editor actually listens on (the pseudo-clipboard
        // textarea in markdown mode, or the ProseMirror contenteditable in WYSIWYG) — NOT
        // `container`, which is an ancestor the editor's paste handler never receives. The
        // synthetic event re-enters this capture listener, but with no external <img> left
        // it falls through and reaches the editor. Rebuilding the DataTransfer without the
        // image item is what keeps the pasted text while dropping the incidental thumbnail.
        const target = (e.target as HTMLElement) ?? container;
        target.dispatchEvent(new ClipboardEvent('paste', {bubbles: true, cancelable: true, clipboardData: dt}));
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
