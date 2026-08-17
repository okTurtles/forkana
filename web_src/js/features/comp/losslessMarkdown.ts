// Lossless markdown tracking for Toast UI Editor (issue #262).
//
// Toast UI's WYSIWYG→markdown serializer normalizes and escapes the source (headers
// rewritten, `\[` `\_` `\#` escapes added), and the core overwrites the markdown document
// with that lossy serialization on every Visual→Source mode switch, even when the user
// edited nothing. Committing `getMarkdown()` therefore rewrote articles wholesale and the
// escaped output no longer rendered as markdown.
//
// This tracker keeps the authoritative source text alongside the editor and guarantees:
//   - an untouched document round-trips byte-identical, regardless of tab switching;
//   - edits made purely in Source (markdown) mode keep the rest of the text verbatim;
//   - a genuine edit in the Visual editor adopts the serialized form only for the lines it
//     actually touched: the serialization is three-way merged back onto the pristine source
//     (see markdownThreeWayMerge.ts), so untouched paragraphs keep their original bytes and
//     their reference links keep rendering. When that merge cannot be done confidently the
//     whole serialization is used, which is the pre-merge behavior.
//
// It installs itself by overriding `editor.getMarkdown`/`editor.setMarkdown` (the same
// pattern as installBase64WidgetPatch, which must be installed first so widget-placeholder
// stripping is applied uniformly to every comparison).

import {mergeVisualEdit} from './markdownThreeWayMerge.ts';

// Minimal structural surface of Toast UI Editor used by the tracker, so the fast unit tests
// in losslessMarkdown.test.ts can drive it with a fake editor. The real editor is exercised
// separately in losslessMarkdown.editor.test.ts, which pins the Toast UI behaviors the fake
// imitates (event order, lossy serialization) so the two cannot drift apart.
export type LosslessEditor = {
  isMarkdownMode(): boolean;
  getMarkdown(): string;
  setMarkdown(markdown: string, cursorToEnd?: boolean): void;
  on(event: string, handler: (...args: unknown[]) => void): void;
  // Optional: used only to put the caret back after the Source-mode writeback below. Toast UI
  // reports markdown-mode positions as 1-based [line, ch] pairs. The unit-test fakes omit
  // these, and the caret restore is skipped when they are absent.
  getSelection?(): unknown;
  setSelection?(start: unknown, end: unknown): void;
};

// A 1-based [line, ch] position in the markdown (Source) editor.
type MarkdownPos = [number, number];

// Reads the markdown-mode caret/selection, or null if it is unavailable or not in the
// markdown-mode shape (in WYSIWYG mode Toast UI returns two flat numbers instead).
function readMarkdownSelection(editor: LosslessEditor): [MarkdownPos, MarkdownPos] | null {
  if (!editor.getSelection || !editor.setSelection) return null;
  let selection: unknown;
  try {
    selection = editor.getSelection();
  } catch {
    return null; // no selection to preserve
  }
  if (!Array.isArray(selection) || selection.length !== 2) return null;
  const positions: MarkdownPos[] = [];
  for (const end of selection) {
    if (!Array.isArray(end) || typeof end[0] !== 'number' || typeof end[1] !== 'number') return null;
    positions.push([end[0], end[1]]);
  }
  return [positions[0], positions[1]];
}

// Puts the caret back after the document was replaced, clamped into the new text so the
// position is always structurally valid (an out-of-range position makes Toast UI throw).
function restoreMarkdownSelection(editor: LosslessEditor, selection: [MarkdownPos, MarkdownPos] | null, text: string): void {
  if (!selection || !editor.setSelection) return;
  const lines = text.split('\n');
  const clamp = ([line, ch]: MarkdownPos): MarkdownPos => {
    const clampedLine = Math.min(Math.max(line, 1), lines.length);
    return [clampedLine, Math.min(Math.max(ch, 1), lines[clampedLine - 1].length + 1)];
  };
  try {
    editor.setSelection(clamp(selection[0]), clamp(selection[1]));
  } catch {
    // Leave the caret where setMarkdown(cursorToEnd) put it rather than break the mode switch.
  }
}

// Resolves the markdown to commit for a WYSIWYG serialization, merging the user's Visual
// edit back onto the pristine source. `null` from mergeVisualEdit means "not confident",
// and the serialization is used wholesale (the behavior before the merge existed).
function resolveSerialization(pristine: string, baseline: string, serialized: string): string {
  return mergeVisualEdit(pristine, baseline, serialized) ?? serialized;
}

export function installLosslessMarkdownTracker(editor: LosslessEditor, textarea: HTMLTextAreaElement): void {
  // Captured after installBase64WidgetPatch: strips $$widget$$ placeholders, so baseline
  // comparisons can never differ on placeholder syntax alone.
  const baseGetMarkdown = editor.getMarkdown.bind(editor);
  const baseSetMarkdown = editor.setMarkdown.bind(editor);

  // Authoritative lossless markdown. In markdown mode the editor text is raw and kept in
  // sync by the change handler; in WYSIWYG mode this holds the last lossless text.
  let sourceText = textarea.value;
  // Serialization snapshot taken on every entry into WYSIWYG mode (including initial
  // load). Equality with the current serialization ⇔ "no effective Visual edit".
  let wysiwygBaseline = '';
  // The pristine source at the moment WYSIWYG mode was entered; restored when the user
  // returns to Source mode without having made an effective Visual edit.
  let mdSnapshot = sourceText;
  // Suppresses the change handler during programmatic setMarkdown (initial load, restore).
  let suppressChange = false;

  const getLosslessMarkdown = (): string => {
    if (editor.isMarkdownMode()) return sourceText;
    const serialized = baseGetMarkdown();
    if (serialized === wysiwygBaseline) return sourceText;
    // The user edited in Visual mode: keep their edit, but only let it overwrite the lines
    // it actually touched. `sourceText` is still the pristine source here — the change
    // handler below only reassigns it in markdown mode.
    return resolveSerialization(sourceText, wysiwygBaseline, serialized);
  };

  const syncTextarea = () => {
    textarea.value = getLosslessMarkdown();
    textarea.dispatchEvent(new Event('change'));
  };

  // Programmatic content replacement: the given text becomes the new pristine source.
  const applyMarkdown = (markdown: string, cursorToEnd?: boolean) => {
    suppressChange = true;
    try {
      baseSetMarkdown(markdown, cursorToEnd);
    } finally {
      suppressChange = false;
    }
    sourceText = markdown;
    mdSnapshot = markdown;
    if (!editor.isMarkdownMode()) wysiwygBaseline = baseGetMarkdown();
    syncTextarea();
  };

  editor.on('change', () => {
    if (suppressChange) return;
    // Markdown mode is lossless (the source is stored verbatim), so the editor content is authoritative.
    // WYSIWYG changes are only adopted lazily via getLosslessMarkdown's baseline check.
    if (editor.isMarkdownMode()) sourceText = baseGetMarkdown();
    syncTextarea();
  });

  editor.on('changeMode', (mode: unknown) => {
    if (mode !== 'markdown' && mode !== 'wysiwyg') return;
    if (mode === 'wysiwyg') {
      mdSnapshot = sourceText;
      // Deferred for the same reason as the markdown-mode branch below: the core still
      // reads its own tracked cursor/selection mapping and applies it to the WYSIWYG model
      // *after* emitting `changeMode`. Calling baseGetMarkdown() synchronously here forces
      // Toast UI's internal markdown<->WYSIWYG convertor to run a serialization pass right
      // in the middle of that, which can leave it with stale position-mapping state; the
      // core's own subsequent setSelection(pos) then throws a ProseMirror range error.
      queueMicrotask(() => {
        // The user may have switched modes again before the microtask ran.
        if (editor.isMarkdownMode()) return;
        wysiwygBaseline = baseGetMarkdown();
        syncTextarea();
      });
      return;
    }
    // mode === 'markdown': the core just overwrote the markdown document with the WYSIWYG
    // serialization (lossy), even if nothing was edited, and that write already ran
    // through the change handler above, clobbering sourceText.
    const serialized = baseGetMarkdown();
    // Either nothing was edited in Visual mode (restore the pristine source verbatim) or
    // something was (merge it back onto the pristine source, keeping untouched lines). Note
    // `mdSnapshot`, not `sourceText`: the change handler above already clobbered sourceText
    // with the lossy serialization the core just wrote into the markdown document.
    const resolved = serialized === wysiwygBaseline ?
      mdSnapshot :
      resolveSerialization(mdSnapshot, wysiwygBaseline, serialized);
    sourceText = resolved;
    if (resolved !== serialized) {
      // The editor's markdown document currently holds the lossy serialization, so the
      // Source editor would *display* text the user never wrote — the other half of #262.
      // Replace it with the resolved text.
      //
      // Deferred, because the core still restores focus/selection (with positions mapped
      // against the serialized doc) after emitting `changeMode`; replacing the document
      // synchronously would race that.
      // Must pass cursorToEnd=true: with false, Toast UI's markdown editor does a wholesale
      // ProseMirror document replace and lets the transaction auto-map the old (now-stale)
      // selection through it. That mapped position is not just imprecise, it can point
      // outside the new document's node structure entirely (different text, different
      // length), and Toast UI's next mode switch reuses that broken position to restore the
      // WYSIWYG selection, throwing "Index N out of range" from inside ProseMirror. Passing
      // true makes it call moveCursorToEnd instead, which is always structurally valid.
      //
      // On its own that would drop the user at the bottom of the article after every Visual
      // edit, so the caret is captured and re-applied explicitly. By the time this microtask
      // runs the core has already restored the caret itself (it does so synchronously, while
      // emitting `changeMode`), mapped against the serialized document. The merge emits
      // exactly one line per serialized line, in order (the safety invariant in
      // markdownThreeWayMerge.ts), so the line number carries over unchanged; only the column
      // can shift, because the merged line may be the shorter unescaped spelling. Both are
      // clamped into the new text, which is what makes the restored position structurally
      // valid where the auto-mapped one was not.
      queueMicrotask(() => {
        // The user may have switched modes again before the microtask ran.
        if (!editor.isMarkdownMode()) return;
        const selection = readMarkdownSelection(editor);
        applyMarkdown(sourceText, true);
        restoreMarkdownSelection(editor, selection, sourceText);
      });
    }
    syncTextarea();
  });

  // Initial content load, guarded so the setMarkdown-triggered change event cannot
  // overwrite the textarea (and thus later submits) with WYSIWYG-normalized output.
  if (textarea.value) {
    applyMarkdown(textarea.value);
  } else if (!editor.isMarkdownMode()) {
    wysiwygBaseline = baseGetMarkdown();
  }

  // Every consumer (submit handlers reading getMarkdown(), external code replacing the
  // content via setMarkdown()) now goes through the lossless layer.
  editor.getMarkdown = getLosslessMarkdown;
  editor.setMarkdown = applyMarkdown;
}
