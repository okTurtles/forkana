// Interprets markdown the user typed into the Visual (WYSIWYG) editor (issues #322, #367).
//
// Toast UI's WYSIWYG→markdown serializer backslash-escapes markdown punctuation in every
// text node: typing `# Heading`, `**bold**` or a ```` ```mermaid ```` fence in Visual mode
// is saved as `\# Heading`, `\*\*bold\*\*`, `` \`\`\`mermaid `` — which then renders as
// literal text instead of markdown. That is standard WYSIWYG semantics ("what you see is
// what you get"), but Forkana's product decision is the opposite: markdown-looking input
// typed or pasted into the Visual editor is markdown and must render as such.
//
// unescapeTypedMarkdown() removes those serializer escapes from the lines of a Visual edit,
// so the committed source contains real markdown. It is careful about the places where a
// backslash is user content rather than a serializer artifact — code:
//   - real fenced code blocks (a raw ``` fence in the input, i.e. a WYSIWYG code block,
//     whose content the serializer emits verbatim): content lines are left untouched, so a
//     `\d` there is the user's regex. Fences are tracked as they form, and the block's
//     content is protected until a closing fence ends it;
//   - fences typed as plain text (the #322/#367 case): the opener arrives escaped
//     (`` \`\`\`mermaid ``) because at serialization time there was no fence — every line,
//     the body included, was serialized as an escaped *paragraph*. So when the opener itself
//     needed unescaping to become a fence, the adopted body lines are serializer-escaped
//     text too and their escapes are removed as well; a closing fence — itself possibly
//     still escaped — ends the block;
//   - inline code spans: `` `a\*b` `` keeps its backslash, escapes around it are removed.
//
// Inherent limitation of the typed-fence path: the serializer has already interleaved the
// body's paragraphs with blank lines and collapsed multi-space runs before this module ever
// sees the text, and that loss cannot be undone here. The code-block toolbar button remains
// the only fully lossless way to author a code block in Visual mode.
//
// `<` is deliberately excluded from unescaping: stripping the serializer's `\<` would
// promote literal angle-bracket text typed in Visual mode (`<br>`, `<script>`) to raw HTML,
// which no #322/#367 construct needs and which would silently change the text's meaning.
//
// The caller says which lines were actually produced by the user's Visual edit (see
// `adoptedLines` in markdownThreeWayMerge.ts); all other lines are pristine source bytes and
// are never modified — only scanned, so that fences already present in the article still
// protect their content.

// The escaped-punctuation set is shared with the three-way merge (single source of truth;
// markdownThreeWayMerge.ts does not import this module, so there is no cycle).
import {ASCII_PUNCTUATION_RE, stripEscapes} from './markdownThreeWayMerge.ts';

// Is `ch` a character whose serializer escape this module removes? See the module comment
// for why `<` is excluded.
function isUnescapable(ch: string): boolean {
  return ch !== '<' && ASCII_PUNCTUATION_RE.test(ch);
}

// An opening or closing code fence: up to 3 spaces of indentation, then a run of 3+
// backticks or tildes. m[2] is the fence run, m[3] the rest (info string, if opening).
const FENCE_LINE_RE = /^( {0,3})(`{3,}|~{3,})(.*)$/;

type FenceState = {
  char: string,
  len: number,
  // True when the opener was an escaped text line: the fence was typed as paragraphs, so
  // its adopted body lines carry serializer escapes too (see the module comment).
  bodyIsTypedText: boolean,
};

// Parses a line as the opening fence of a code block, or null if it is not one.
function parseOpeningFence(line: string): Omit<FenceState, 'bodyIsTypedText'> | null {
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

// Removes backslash escapes outside inline code spans; code-span content is verbatim.
export function unescapeLine(line: string): string {
  let out = '';
  let i = 0;
  while (i < line.length) {
    const ch = line[i];
    if (ch === '\\' && i + 1 < line.length && isUnescapable(line[i + 1])) {
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
      // Inside a fenced code block. A raw closing fence always ends it.
      if (closesFence(line, fence)) {
        out[i] = line;
        fence = null;
        continue;
      }
      if (adopted && fence.bodyIsTypedText) {
        // The opener was an escaped text line, so this fence was typed as paragraphs and
        // this adopted body line was serialized as an escaped paragraph, not as verbatim
        // code: its escapes are serializer artifacts and are removed (stripEscapes, not
        // unescapeLine — there are no inline code spans inside a code block, and every
        // escape here, `\<` included, is a paragraph-serialization artifact).
        const candidate = stripEscapes(line);
        if (closesFence(candidate, fence)) {
          out[i] = candidate;
          fence = null;
          continue;
        }
        out[i] = candidate;
        continue;
      }
      if (adopted) {
        // Real fence: content is verbatim. The only transformation allowed is recognizing
        // an adopted line as the (still escaped) closing fence.
        const candidate = stripEscapes(line);
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
    const opener = parseOpeningFence(result);
    fence = opener ? {...opener, bodyIsTypedText: adopted && result !== line} : null;
  }
  return out.join('\n');
}
