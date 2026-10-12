// Editing operations of the link and cite tools (issue #399) on Toast UI Editor, in both of its
// modes. Both the Source (markdown) and the Visual (WYSIWYG) editors are ProseMirror views,
// so the tools read the text around the caret and edit through ProseMirror transactions; only
// the Visual editor needs a link mark where the Source editor gets Markdown text.

import {appendFootnoteDefinition} from './footnote-markdown.ts';
import {stripWidgetPlaceholders} from '../comp/base64ImageWidget.ts';

// The part of Toast UI Editor the tools use.
export type ToolsEditor = {
  isMarkdownMode(): boolean,
  getMarkdown(): string,
  setMarkdown(markdown: string, cursorToEnd?: boolean): void,
  focus(): void,
  on(event: string, handler: (...args: any[]) => void): void,
  mdEditor: {view: any, getMarkdown(): string},
  wwEditor: {view: any},
};

// The view of the editor's current mode.
export function currentView(editor: ToolsEditor): any {
  return editor.isMarkdownMode() ? editor.mdEditor.view : editor.wwEditor.view;
}

// textBeforeCaretInBlock is the text of the caret's paragraph (line, in the Source editor)
// before the caret; "" when the caret is in a code block of the Visual editor.
export function textBeforeCaretInBlock(view: any): string {
  const {$from} = view.state.selection;
  if ($from.parent.type.spec.code) return '';
  return $from.parent.textBetween(0, $from.parentOffset, undefined, '\ufffc');
}

// textBeforeCaret is the document's text before the caret, one block per line. In the
// Source editor it is the Markdown before the caret, which the reference numbering reads.
export function textBeforeCaret(view: any): string {
  return textBefore(view, view.state.selection.from);
}

// textBefore is the document's text before pos, one block per line.
export function textBefore(view: any, pos: number): string {
  return view.state.doc.textBetween(0, pos, '\n', '\ufffc');
}

// focusEditor puts the focus back in the document of the editor's current mode.
export function focusEditor(editor: ToolsEditor): void {
  const view = currentView(editor);
  if (!view.hasFocus()) view.focus();
}

export function caretPosition(view: any): number {
  return view.state.selection.from;
}

// caretRect returns the screen rectangle of the caret, where the popovers are anchored.
export function caretRect(view: any): DOMRect {
  try {
    const c = view.coordsAtPos(view.state.selection.from);
    return new DOMRect(c.left, c.top, Math.max(1, c.right - c.left), c.bottom - c.top);
  } catch {
    return view.dom.getBoundingClientRect();
  }
}

function setCaret(view: any, pos: number): void {
  const {state} = view;
  const SelectionCtor: any = state.selection.constructor; // TextSelection: Toast UI bundles its own prosemirror-state
  const clamped = Math.max(0, Math.min(pos, state.doc.content.size));
  try {
    view.dispatch(state.tr.setSelection(SelectionCtor.create(state.doc, clamped)));
  } catch {
    // leave the caret where it is
  }
}

// insertLink replaces the document range [from, to) with a link: Markdown text in the Source
// editor ("[label](url)", the url written as it is, spaces included), a link mark in the
// Visual editor. The caret ends after the link.
export function insertLink(editor: ToolsEditor, from: number, to: number, label: string, url: string): void {
  const view = currentView(editor);
  const {state} = view;
  if (editor.isMarkdownMode()) {
    const text = `[${label.replace(/([[\]\\])/g, '\\$1')}](${url})`;
    view.dispatch(state.tr.insertText(text, from, to).scrollIntoView());
  } else {
    const mark = state.schema.marks.link.create({linkUrl: url});
    const tr = state.tr.replaceWith(from, to, state.schema.text(label, [mark]));
    // typing after the link must not extend it
    tr.removeStoredMark(state.schema.marks.link);
    view.dispatch(tr.scrollIntoView());
  }
  focusEditor(editor);
}

// deleteRange removes [from, to) of the current view.
export function deleteRange(editor: ToolsEditor, from: number, to: number): void {
  const view = currentView(editor);
  if (to > from) view.dispatch(view.state.tr.delete(from, to));
}

// insertText inserts plain text at [from, to) of the current view.
export function insertText(editor: ToolsEditor, from: number, to: number, text: string): void {
  const view = currentView(editor);
  view.dispatch(view.state.tr.insertText(text, from, to).scrollIntoView());
  focusEditor(editor);
}

// A character that cannot be in an article, marking where the caret was while the Visual
// editor's document goes through Markdown.
const CARET_MARK = '\uE399';

// insertFootnote puts the reference "[^label]" at [from, to) of the current view and, when
// definition is given, appends "[^label]: definition" to the article. The caret ends after
// the reference.
//
// In the Source editor both are plain text edits. In the Visual editor the definition has to
// be Markdown to render, so the edit goes through the article's Markdown: the caret is marked
// in the document, the (lossless) Markdown is edited, and the document is rebuilt from it.
export function insertFootnote(editor: ToolsEditor, from: number, to: number, label: string, definition?: string): void {
  const reference = `[^${label}]`;
  const view = currentView(editor);
  if (editor.isMarkdownMode()) {
    view.dispatch(view.state.tr.insertText(reference, from, to));
    const caret = from + reference.length;
    if (definition !== undefined) {
      // The Source editor's text is the article's Markdown, as it is; appending the
      // definition leaves everything before the caret, so its position, unchanged.
      const markdown = stripWidgetPlaceholders(editor.mdEditor.getMarkdown());
      // cursorToEnd: a structurally valid caret (see losslessMarkdown.ts), moved back below
      editor.setMarkdown(appendFootnoteDefinition(markdown, label, definition), true);
    }
    setCaret(currentView(editor), caret);
    focusEditor(editor);
    return;
  }

  view.dispatch(view.state.tr.insertText(CARET_MARK, from, to));
  let markdown = editor.getMarkdown();
  if (!markdown.includes(CARET_MARK)) {
    // the serialization lost the mark: fall back to a plain text reference
    const v = currentView(editor);
    const at = findText(v, CARET_MARK);
    if (at >= 0) v.dispatch(v.state.tr.insertText(reference, at, at + CARET_MARK.length));
    return;
  }
  // the reference may already be used before: the caret goes after the occurrence just added
  const occurrence = markdown.slice(0, markdown.indexOf(CARET_MARK)).split(reference).length - 1;
  markdown = markdown.replace(CARET_MARK, reference);
  if (definition !== undefined) markdown = appendFootnoteDefinition(markdown, label, definition);
  editor.setMarkdown(markdown, true);
  const v = currentView(editor);
  const at = findText(v, reference, occurrence);
  if (at >= 0) setCaret(v, at + reference.length);
  focusEditor(editor);
}

// findText returns the document position of the (skip+1)-th occurrence of text in a text
// node, or -1.
export function findText(view: any, text: string, skip = 0): number {
  let found = -1;
  let left = skip;
  view.state.doc.descendants((node: any, pos: number) => {
    if (found >= 0) return false;
    if (node.isText) {
      let i = node.text.indexOf(text);
      while (i >= 0 && left > 0) {
        left--;
        i = node.text.indexOf(text, i + text.length);
      }
      if (i >= 0) found = pos + i;
    }
    return found < 0;
  });
  return found;
}
