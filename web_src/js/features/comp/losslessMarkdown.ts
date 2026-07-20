// Lossless markdown tracking for Toast UI Editor (issue #262).
//
// Toast UI's WYSIWYG→markdown serializer normalizes and escapes the source (headers
// rewritten, `\[` `\_` `\#` escapes added), and the core overwrites the markdown document
// with that lossy serialization on every Visual→Source mode switch — even when the user
// edited nothing. Committing `getMarkdown()` therefore rewrote articles wholesale and the
// escaped output no longer rendered as markdown.
//
// This tracker keeps the authoritative source text alongside the editor and guarantees:
//   - an untouched document round-trips byte-identical, regardless of tab switching;
//   - edits made purely in Source (markdown) mode keep the rest of the text verbatim;
//   - only a genuine edit in the Visual editor adopts the serialized form (unavoidable:
//     Toast UI's serializer is not configurable, and the visual edit rewrote the doc anyway).
//
// It installs itself by overriding `editor.getMarkdown`/`editor.setMarkdown` (the same
// pattern as installBase64WidgetPatch, which must be installed first so widget-placeholder
// stripping is applied uniformly to every comparison).

// Minimal structural surface of Toast UI Editor used by the tracker, so it can be
// unit-tested against a fake editor (the real editor cannot run under happy-dom).
export type LosslessEditor = {
  isMarkdownMode(): boolean;
  getMarkdown(): string;
  setMarkdown(markdown: string, cursorToEnd?: boolean): void;
  on(event: string, handler: (...args: unknown[]) => void): void;
};

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
  let wwBaseline = '';
  // The pristine source at the moment WYSIWYG mode was entered; restored when the user
  // returns to Source mode without having made an effective Visual edit.
  let mdSnapshot = sourceText;
  // Suppresses the change handler during programmatic setMarkdown (initial load, restore).
  let suppressChange = false;

  const getLosslessMarkdown = (): string => {
    if (editor.isMarkdownMode()) return sourceText;
    const serialized = baseGetMarkdown();
    return serialized === wwBaseline ? sourceText : serialized;
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
    if (!editor.isMarkdownMode()) wwBaseline = baseGetMarkdown();
    syncTextarea();
  };

  editor.on('change', () => {
    if (suppressChange) return;
    // Markdown mode is lossless (raw line texts), so the editor content is authoritative.
    // WYSIWYG changes are only adopted lazily via getLosslessMarkdown's baseline check.
    if (editor.isMarkdownMode()) sourceText = baseGetMarkdown();
    syncTextarea();
  });

  editor.on('changeMode', (...args: unknown[]) => {
    const mode = args[0] as string;
    if (mode === 'wysiwyg') {
      mdSnapshot = sourceText;
      wwBaseline = baseGetMarkdown();
      syncTextarea();
      return;
    }
    // mode === 'markdown': the core just overwrote the markdown document with the WYSIWYG
    // serialization (lossy) — even if nothing was edited — and that write already ran
    // through the change handler above, clobbering sourceText.
    const serialized = baseGetMarkdown();
    if (serialized === wwBaseline) {
      // No effective Visual edit: restore the pristine source. Deferred, because the core
      // still restores focus/selection (with positions mapped against the serialized doc)
      // after emitting `changeMode`; replacing the document synchronously would race that.
      sourceText = mdSnapshot;
      queueMicrotask(() => {
        // The user may have switched modes again before the microtask ran.
        if (!editor.isMarkdownMode()) return;
        applyMarkdown(sourceText, false);
      });
    } else {
      // Genuine Visual edits: the serialization is now the authoritative source.
      sourceText = serialized;
    }
    syncTextarea();
  });

  // Initial content load, guarded so the setMarkdown-triggered change event cannot
  // overwrite the textarea (and thus later submits) with WYSIWYG-normalized output.
  if (textarea.value) {
    applyMarkdown(textarea.value);
  } else if (!editor.isMarkdownMode()) {
    wwBaseline = baseGetMarkdown();
  }

  // Every consumer — submit handlers reading getMarkdown(), external code replacing the
  // content via setMarkdown() — now goes through the lossless layer.
  editor.getMarkdown = getLosslessMarkdown;
  editor.setMarkdown = applyMarkdown;
}
