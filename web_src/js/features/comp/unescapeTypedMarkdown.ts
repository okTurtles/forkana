// Interprets markdown the user typed into the Visual (WYSIWYG) editor (issues #322, #367).
//
// Toast UI's WYSIWYG→markdown serializer backslash-escapes markdown punctuation in every
// text node: typing `# Heading`, `**bold**` or a ```` ```mermaid ```` fence in Visual mode
// is saved as `\# Heading`, `\*\*bold\*\*`, `\```mermaid` — which then renders as literal
// text instead of markdown. That is standard WYSIWYG semantics ("what you typed is what you
// get"), but Forkana's product decision is the opposite: markdown-looking input typed or
// pasted into the Visual editor is markdown and must render as such.
//
// unescapeTypedMarkdown() removes those serializer escapes from the lines of a Visual edit,
// so the committed source contains real markdown. It is careful about the one place where a
// backslash is user content rather than a serializer artifact — code:
//   - fenced code blocks: content lines are verbatim (the serializer never escapes inside a
//     fence, so a `\d` there is the user's regex) and are left untouched. Fences are tracked
//     as they form: an escaped `\```mermaid` line unescapes into a real opening fence, and
//     from then on its content is protected until a closing fence — itself possibly still
//     escaped — ends the block;
//   - inline code spans: `` `a\*b` `` keeps its backslash, escapes around it are removed.
//
// The caller says which lines were actually produced by the user's Visual edit (see
// `adoptedLines` in markdownThreeWayMerge.ts); all other lines are pristine source bytes and
// are never modified — only scanned, so that fences already present in the article still
// protect their content.

// CommonMark honors a backslash escape only before ASCII punctuation, which is exactly the
// set Toast UI's serializer produces.
const ASCII_PUNCTUATION_RE = /[!"#$%&'()*+,\-./:;<=>?@[\]^_`{|}~\\]/;

// An opening or closing code fence: up to 3 spaces of indentation, then a run of 3+
// backticks or tildes. m[2] is the fence run, m[3] the rest (info string, if opening).
const FENCE_LINE_RE = /^( {0,3})(`{3,}|~{3,})(.*)$/;

type FenceState = {char: string, len: number};

// Does this line open a fenced code block?
function fenceOpen(line: string): FenceState | null {
  const m = FENCE_LINE_RE.exec(line);
  if (!m) return null;
  const char = m[2][0];
  // CommonMark: the info string of a backtick fence may not contain backticks
  // (that spelling is inline code, e.g. "``` `x` ```").
  if (char === '`' && m[3].includes('`')) return null;
  return {char, len: m[2].length};
}

// Does this line close the given open fence?
function closesFence(line: string, fence: FenceState): boolean {
  const m = FENCE_LINE_RE.exec(line);
  return m !== null && m[2][0] === fence.char && m[2].length >= fence.len && m[3].trim() === '';
}

// Removes every backslash escape, with no code-span awareness. Used only to test whether an
// escaped line would be a closing code fence, never to emit general content.
function unescapeAll(line: string): string {
  let out = '';
  for (let i = 0; i < line.length; i++) {
    if (line[i] === '\\' && i + 1 < line.length && ASCII_PUNCTUATION_RE.test(line[i + 1])) {
      out += line[i + 1];
      i++;
    } else {
      out += line[i];
    }
  }
  return out;
}

// Removes backslash escapes outside inline code spans; code-span content is verbatim.
export function unescapeLine(line: string): string {
  let out = '';
  let i = 0;
  while (i < line.length) {
    const ch = line[i];
    if (ch === '\\' && i + 1 < line.length && ASCII_PUNCTUATION_RE.test(line[i + 1])) {
      out += line[i + 1];
      i += 2;
      continue;
    }
    if (ch === '`') {
      // Start of a backtick run: a code span closes on a run of exactly the same length.
      let runEnd = i + 1;
      while (runEnd < line.length && line[runEnd] === '`') runEnd++;
      const runLen = runEnd - i;
      let close = -1;
      let j = runEnd;
      while (j < line.length) {
        if (line[j] === '`') {
          let k = j + 1;
          while (k < line.length && line[k] === '`') k++;
          if (k - j === runLen) {
            close = k;
            break;
          }
          j = k;
        } else {
          j++;
        }
      }
      if (close >= 0) {
        // Whole code span (delimiters and content) copied verbatim.
        out += line.slice(i, close);
        i = close;
      } else {
        // Unmatched run: literal backticks, keep unescaping after them.
        out += line.slice(i, runEnd);
        i = runEnd;
      }
      continue;
    }
    out += ch;
    i++;
  }
  return out;
}

/**
 * Removes the WYSIWYG serializer's backslash escapes from the lines of a Visual edit so
 * markdown typed in the Visual editor is committed as real markdown.
 *
 * @param text the resolved markdown document (merged, or the raw serialization)
 * @param adoptedLines which lines came from the user's Visual edit (parallel to the
 *   document's lines; see MergeStats.adoptedLines). Omitted means every line did — the
 *   wholesale-serialization fallback. Lines not adopted are pristine source and are only
 *   scanned for fence state, never modified.
 */
export function unescapeTypedMarkdown(text: string, adoptedLines?: boolean[]): string {
  const lines = text.split('\n');
  const out = new Array<string>(lines.length);
  let fence: FenceState | null = null;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const adopted = adoptedLines ? adoptedLines[i] === true : true;
    if (fence) {
      // Inside a fenced code block: content is verbatim. The only transformation allowed is
      // recognizing an adopted line as the (still escaped) closing fence.
      if (closesFence(line, fence)) {
        out[i] = line;
        fence = null;
        continue;
      }
      if (adopted) {
        const candidate = unescapeAll(line);
        if (closesFence(candidate, fence)) {
          out[i] = candidate;
          fence = null;
          continue;
        }
      }
      out[i] = line;
      continue;
    }
    const result = adopted ? unescapeLine(line) : line;
    out[i] = result;
    fence = fenceOpen(result);
  }
  return out.join('\n');
}
