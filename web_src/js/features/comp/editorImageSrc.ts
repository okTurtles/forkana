// Relative image paths in the article editor (issue #167).
//
// An article can show an image stored in its repository by a relative path, written as
// markdown (`![a](pic.png)`) or as HTML (`<img src="pic.png">`). The page of the editor is
// not the file, so the browser would resolve such a path against the page url and miss it:
// the editor resolves it against the media url of the file being edited, which the server
// provides (data-media-file-url). That is the "/media/" route the Read view uses too, so an
// image stored in LFS displays in both.
// Only what the Visual editor shows changes: the markdown keeps the path as written.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import type Editor from '@toast-ui/editor';

// Is `src` a path relative to the file? Absolute urls, data: uris, root paths ("/x",
// "//host/x") and fragments are left as they are, as in the Read view.
export function isRelativeImageSrc(src: string): boolean {
  return Boolean(src) && !/^[a-z][a-z0-9+.-]*:/i.test(src) && !src.startsWith('/') && !src.startsWith('#');
}

// resolveRelativeSrc resolves a relative image path (e.g. "./img/a.png", "../b.png")
// against the media URL of the file being edited. The base URL is provided by the server
// (data-media-file-url), so it already accounts for appSubUrl and branch names containing
// slashes; native URL resolution handles "./" and "../" segments correctly.
export function resolveRelativeSrc(src: string, baseFileUrl: string): string {
  if (!baseFileUrl || !isRelativeImageSrc(src)) return src;
  try {
    const base = new URL(baseFileUrl, window.location.origin);
    return new URL(src, base).href;
  } catch {
    return src;
  }
}

type ImageNodeView = {dom: HTMLElement};
type NodeViewFactory = (...args: unknown[]) => ImageNodeView;
type WwView = {props: {nodeViews?: Record<string, NodeViewFactory>}, setProps: (props: Record<string, unknown>) => void};

// Makes the Visual editor show every image with its relative src resolved, including an
// `<img>` written as HTML, inline or in an HTML block (a markdown image is already resolved
// by the editor's image renderer, and is left as it is). Only the displayed `<img>` changes,
// never the document: the image keeps its path as written, so editing the paragraph around
// it cannot save the resolved url into the article.
export function installImageSrcResolver(editor: Editor, baseFileUrl: string): void {
  if (!baseFileUrl) return;
  const view = (editor as {wwEditor?: {view?: WwView}}).wwEditor?.view;
  const nodeViews = view?.props.nodeViews;
  const imageView = nodeViews?.image;
  if (!imageView) {
    // A Toast UI upgrade moved the internals: fail loudly in tests rather than silently.
    console.error('installImageSrcResolver: Toast UI image node view not found');
    return;
  }
  view.setProps({
    nodeViews: {
      ...nodeViews,
      image: (...args: unknown[]) => {
        const nodeView = imageView(...args);
        const img = nodeView.dom instanceof HTMLImageElement ? nodeView.dom : nodeView.dom.querySelector('img');
        const src = img?.getAttribute('src') ?? '';
        const resolved = resolveRelativeSrc(src, baseFileUrl);
        if (resolved !== src) img.setAttribute('src', resolved);
        return nodeView;
      },
    },
  });
}
