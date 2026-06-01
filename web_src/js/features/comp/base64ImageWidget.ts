// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import type Editor from '@toast-ui/editor';

export const imageIconSvg = `
<svg class="base64-image-icon" width="12" height="12" viewBox="0 0 16 16" fill="currentColor" style="vertical-align: middle;">
  <path d="M1.75 2.5a.25.25 0 0 0-.25.25v10.5c0 .138.112.25.25.25h12.5a.25.25 0 0 0 .25-.25V2.75a.25.25 0 0 0-.25-.25H1.75zM0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0 1 14.25 15H1.75A1.75 1.75 0 0 1 0 13.25V2.75zm9.5 4.75a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0zm-7.25 5h11.5a.25.25 0 0 0 .222-.365l-3.5-7a.25.25 0 0 0-.43 0L7.5 10.386l-2.066-3.1a.25.25 0 0 0-.416 0l-3 4.5A.25.25 0 0 0 2.25 12.5z"/>
</svg>
`;

export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = Math.max(0, decimals);
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / (k ** i)).toFixed(dm))} ${sizes[i]}`;
}

export function createBase64WidgetRule(
  // eslint-disable-next-line @typescript-eslint/no-redundant-type-constituents -- Editor type has issues
  getEditor: () => Editor | null,
) {
  return {
    rule: /!\[([^\]]*)\]\((data:image\/[a-zA-Z+.-]+;base64,[A-Za-z0-9+/=\s]{50,})\)/,
    toDOM(text: string) {
      let isWysiwyg = false;
      const editor = getEditor();
      try {
        if (editor && typeof editor.isWysiwygMode === 'function') {
          isWysiwyg = editor.isWysiwygMode();
        }
      } catch {}

      const match = /!\[([^\]]*)\]\((data:image\/[a-zA-Z+.-]+;base64,[A-Za-z0-9+/=\s]+)\)/.exec(text);
      const altText = match ? match[1] : '';
      const base64Url = match ? match[2] : '';

      if (isWysiwyg) {
        const img = document.createElement('img');
        img.src = base64Url;
        img.alt = altText || 'Image';
        img.className = 'base64-wysiwyg-preview';
        img.style.maxWidth = '100%';
        return img;
      }

      const mimeMatch = /^data:(image\/[a-zA-Z+.-]+);base64,/.exec(base64Url);
      const mimeType = mimeMatch ? mimeMatch[1] : 'image';
      const extension = mimeType.replace('image/', '').toUpperCase();
      const sizeBytes = Math.round((base64Url.length - (base64Url.indexOf(',') + 1)) * 0.75);
      const formattedSize = formatBytes(sizeBytes);

      const el = document.createElement('span');
      el.className = 'base64-image-widget';
      el.title = `Alt: ${altText} | Base64 Encoded Image - Click to copy original data URI`;
      el.innerHTML = imageIconSvg.trim();

      const label = document.createElement('span');
      label.textContent = `Base64 Image (${extension}, ${formattedSize}${altText ? ` - ${altText}` : ''})`;
      el.append(label);

      el.addEventListener('click', async (e) => {
        e.preventDefault();
        e.stopPropagation();
        try {
          await navigator.clipboard.writeText(base64Url);
          const label = el.querySelector('span');
          if (label) {
            const originalText = label.textContent;
            label.textContent = 'Copied to clipboard!';
            setTimeout(() => {
              label.textContent = originalText;
            }, 2000);
          }
        } catch (err) {
          console.error('Failed to copy to clipboard', err);
        }
      });

      return el;
    },
  };
}
