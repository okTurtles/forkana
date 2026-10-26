// Keeps raw HTML that Toast UI cannot map from breaking the Visual editor (issue #446).
//
// Loading: Toast UI's markdown→WYSIWYG converters for `htmlBlock` and `htmlInline` assume the
// HTML starts with a tag (`<div>`, `</p>`) or, for blocks, a well-formed comment. They read
// `[1]` of a regex match without checking it, so any other HTML CommonMark accepts throws
// "Cannot read properties of null (reading '1')" in setMarkdown and the editor never opens:
//   - processing instructions `<?php … ?>`, declarations `<!DOCTYPE html>`, `<![CDATA[…]]>`;
//   - comments Toast's own comment pattern rejects: `<!---->`, `<!-- a -- b -->`, an
//     unclosed `<!--`;
//   - an HTML block indented by 1–3 spaces (`  <div>`): the spaces are part of the block's
//     text, so it no longer starts with `<`;
//   - an open tag left unfinished on its line (`<div`).
// Such a block is now kept as a raw HTML block (Toast's `htmlComment` node, shown as source
// text and written back byte for byte), and such inline HTML as plain text. Everything the
// converters do understand still goes through them.
//
// Saving: the WYSIWYG serializer's escaper misses the same constructs, so text typed in the
// Visual editor such as `<!DOCTYPE html>`, `<?php` or `<div` was saved unescaped, became raw
// HTML in the article, and the next Edit crashed on it — the editor wrote markup it could not
// read back. Text typed in the Visual editor is literal text, so every `<` the serializer
// leaves unescaped before a letter, `/`, `!` or `?` is now escaped (`\<`), which renders the
// same `<` and can never start HTML. (unescapeTypedMarkdown deliberately keeps `\<`.)

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import type Editor from '@toast-ui/editor';

// Toast UI's own patterns (editor 3.2.2: HTML_TAG / reHTMLTag and reHTMLComment), written
// out case-insensitively. The converters only cope with HTML these match at its very start.
const TAG_NAME = '[a-z][a-z0-9-]*';
const ATTRIBUTE_NAME = '[a-z_:][\\w:.-]*';
const ATTRIBUTE_VALUE = `(?:[^"'=<>\`\\x00-\\x20]+|'[^']*'|"[^"]*")`;
const ATTRIBUTE = `\\s+${ATTRIBUTE_NAME}(?:\\s*=\\s*${ATTRIBUTE_VALUE})?`;
const OPEN_TAG = `<(${TAG_NAME})(${ATTRIBUTE})*\\s*/?>`;
const CLOSE_TAG = `</(${TAG_NAME})\\s*>`;
const TOAST_HTML_TAG_RE = new RegExp(`^(?:${OPEN_TAG}|${CLOSE_TAG})`, 'i');
const TOAST_HTML_COMMENT_RE = /<! ---->|<!---?[^>-](?:-?[^-])*-->/;

// Can Toast UI's htmlBlock converter handle this block's HTML?
export function isConvertibleHtmlBlock(html: string): boolean {
  return TOAST_HTML_COMMENT_RE.test(html) || TOAST_HTML_TAG_RE.test(html);
}

// Can Toast UI's htmlInline converter handle this inline HTML?
export function isConvertibleHtmlInline(html: string): boolean {
  return TOAST_HTML_TAG_RE.test(html);
}

// A `<` that could start HTML: followed by a letter, `/`, `!` or `?`.
const HTML_START_RE = /<(?=[A-Za-z/!?])/g;

// Escapes every `<` in `text` that could start HTML and is not already escaped. `before` is
// the markdown already written in front of `text`, for a backslash that ends it.
export function escapeHtmlStarts(text: string, before = ''): string {
  return text.replace(HTML_START_RE, (lt: string, offset: number) => {
    let backslashes = 0;
    for (let i = offset - 1; i >= 0 && text[i] === '\\'; i--) backslashes++;
    if (backslashes === offset) {
      for (let i = before.length - 1; i >= 0 && before[i] === '\\'; i--) backslashes++;
    }
    return backslashes % 2 === 1 ? lt : `\\${lt}`;
  });
}

type ToWwState = {
  schema: {nodes: Record<string, unknown>},
  openNode: (type: unknown) => void,
  closeNode: () => void,
  addText: (text: string) => void,
};
type MdNode = {literal: string | null};
type ToWwConvertor = (state: ToWwState, node: MdNode, context: unknown) => void;
type ToMdState = {result: string};
type ToMdConvertor = (state: ToMdState, nodeInfo: unknown) => void;

// The internals this module patches, per editor instance (Toast UI 3.2.2's Convertor).
type ToastConvertor = {
  toWwConvertors: Record<string, ToWwConvertor>,
  toMdConvertors: {nodeTypeConvertors: Record<string, ToMdConvertor>},
};

// Installs both guards on one editor. Call it right after `new Editor(...)`, before any
// setMarkdown. The patch is per instance: Toast UI builds the converter tables per editor.
export function installRawHtmlGuard(editor: Editor): void {
  const convertor = (editor as {convertor?: ToastConvertor}).convertor;
  const toWw = convertor?.toWwConvertors;
  const textToMd = convertor?.toMdConvertors?.nodeTypeConvertors?.text;
  if (!toWw?.htmlBlock || !toWw.htmlInline || !textToMd) {
    // A Toast UI upgrade moved the internals: fail loudly in tests rather than silently.
    console.error('installRawHtmlGuard: Toast UI converter internals not found');
    return;
  }

  const htmlBlock = toWw.htmlBlock;
  toWw.htmlBlock = (state, node, context) => {
    const html = node.literal ?? '';
    if (isConvertibleHtmlBlock(html)) {
      htmlBlock(state, node, context);
      return;
    }
    state.openNode(state.schema.nodes.htmlComment);
    state.addText(html.replace(/\n$/, ''));
    state.closeNode();
  };

  const htmlInline = toWw.htmlInline;
  toWw.htmlInline = (state, node, context) => {
    const html = node.literal ?? '';
    if (isConvertibleHtmlInline(html)) {
      htmlInline(state, node, context);
      return;
    }
    state.addText(html);
  };

  convertor.toMdConvertors.nodeTypeConvertors.text = (state, nodeInfo) => {
    const start = state.result.length;
    textToMd(state, nodeInfo);
    const before = state.result.slice(0, start);
    state.result = before + escapeHtmlStarts(state.result.slice(start), before);
  };
}
