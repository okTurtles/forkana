// Internal links in the Visual editor (issue #399).
//
// Articles write a link to a subject with the subject's spaces: [Solar System](subject/Solar
// System). The server renders it (modules/markup/markdown/forkana_link.go), but CommonMark —
// and so Toast UI's parser — does not allow spaces in a link destination, and the Visual
// editor would show that text as it is. So whenever the Visual editor is filled from
// Markdown, such text becomes a real link again. Toast UI serializes a link as
// "[label](url)" with the url as it is, so the Markdown round-trips unchanged.
//
// installInternalLinkView must be installed before the lossless tracker, so that the
// tracker's snapshot of the Visual editor ("no effective edit" baseline) is taken after the
// conversion: it wraps the setMarkdown the tracker uses, and its changeMode handler (and
// microtask) runs before the tracker's.

import type {ToolsEditor} from './editor-ops.ts';

// The roots of the Forkana pages, like markup.ForkanaInternalLinkRoots.
const roots = ['subject', 'explore', 'about', 'terms', 'privacy', 'security', 'community-notes', 'contact'];
const internalLinkTextRe = new RegExp(
  String.raw`\[([^[\]\n]+)\]\((\/?(?:${roots.join('|')})/[^()<>\n]*(?:\([^()<>\n]*\)[^()<>\n]*)*)\)`, 'g',
);

// a destination followed by a quoted title is a standard link (the server leaves it too)
const quotedTitleRe = /\s+("[^"]*"|'[^']*')$/;

// convertInternalLinkText turns the internal link text with spaces of the Visual editor's
// document into links; source is the Markdown the document was built from. It returns
// whether it changed the document.
export function convertInternalLinkText(view: any, source = ''): boolean {
  const {state} = view;
  const linkMark = state.schema.marks.link;
  if (!linkMark) return false;
  const matches: Array<{from: number, to: number, label: string, url: string}> = [];
  state.doc.descendants((node: any, pos: number, parent: any) => {
    if (!node.isText) return true;
    if (parent?.type.spec.code || node.marks.some((m: any) => m.type === linkMark || m.type.spec.code || m.type.name === 'code')) return false;
    for (const m of node.text.matchAll(internalLinkTextRe)) {
      if (!m[2].includes(' ') || quotedTitleRe.test(m[2])) continue; // Toast UI parsed it already
      matches.push({from: pos + m.index, to: pos + m.index + m[0].length, label: m[1].replace(/\\(.)/g, '$1'), url: m[2].trim()});
    }
    return false;
  });
  // "[Mercury](subject/Mercury (planet))" has no space problem but a CommonMark title:
  // Toast UI reads a link to "subject/Mercury" titled "planet". On Forkana a parenthesis in an
  // internal link is part of the subject name (the server reads it so), so put it back; a
  // quoted title ([Moon](subject/Moon "the satellite")) is a title, and stays one.
  const titled: Array<{from: number, to: number, mark: any}> = [];
  state.doc.descendants((node: any, pos: number) => {
    if (!node.isText) return true;
    const mark = node.marks.find((m: any) => m.type === linkMark);
    if (mark?.attrs.title && roots.some((r) => mark.attrs.linkUrl.replace(/^\//, '').startsWith(`${r}/`)) &&
      source.includes(`](${mark.attrs.linkUrl} (${mark.attrs.title}))`)) {
      titled.push({from: pos, to: pos + node.nodeSize, mark});
    }
    return false;
  });
  if (!matches.length && !titled.length) return false;
  const tr = state.tr;
  for (const t of titled) {
    tr.removeMark(t.from, t.to, linkMark);
    tr.addMark(t.from, t.to, linkMark.create({...t.mark.attrs, linkUrl: `${t.mark.attrs.linkUrl} (${t.mark.attrs.title})`, title: null}));
  }
  for (const m of matches.reverse()) {
    const marks = state.doc.resolve(m.from).marks().filter((mark: any) => mark.type !== linkMark);
    tr.replaceWith(m.from, m.to, state.schema.text(m.label, [...marks, linkMark.create({linkUrl: m.url})]));
  }
  tr.setMeta('addToHistory', false);
  view.dispatch(tr);
  return true;
}

export function installInternalLinkView(editor: ToolsEditor): void {
  const baseSetMarkdown = editor.setMarkdown.bind(editor);
  editor.setMarkdown = (markdown: string, cursorToEnd?: boolean) => {
    baseSetMarkdown(markdown, cursorToEnd);
    if (!editor.isMarkdownMode()) convertInternalLinkText(editor.wwEditor.view, markdown);
  };
  editor.on('changeMode', (mode: unknown) => {
    if (mode !== 'wysiwyg') return;
    // after the core has restored the caret, like the tracker's snapshot (which runs next)
    queueMicrotask(() => {
      if (!editor.isMarkdownMode()) convertInternalLinkText(editor.wwEditor.view, editor.mdEditor.getMarkdown());
    });
  });
}
