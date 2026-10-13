// Reading and writing the Markdown footnotes that hold an article's citations (issue #399):
// "[^label]" in the text, "[^label]: citation" on a line of its own. The renderer numbers the
// references by the order of their first use in the text, as Wikipedia does, so these helpers
// do the same to show writers the number a reference gets.

export type FootnoteSource = {
  label: string,
  number: number, // as rendered: by first use in the text
  text: string, // the definition's first line, Markdown
};

const FENCE_RE = /^ {0,3}(`{3,}|~{3,})/;
const DEFINITION_RE = /^ {0,3}\[\^([^\]\s]+)\]:[ \t]?(.*)$/;
// a "[^1]:" in running text is a reference too (definition lines are skipped)
const REFERENCE_RE = /\[\^([^\]\s]+)\]/g;

type Line = {text: string, inCode: boolean};

// markdownLines splits the Markdown in lines, telling those in fenced code; openFence is the
// fence of a code block left open at the end, if any.
function markdownLines(markdown: string): {lines: Line[], openFence: string} {
  const out: Line[] = [];
  let fence = '';
  for (const text of markdown.split('\n')) {
    const m = FENCE_RE.exec(text);
    if (fence) {
      out.push({text, inCode: true});
      if (m && m[1][0] === fence[0] && m[1].length >= fence.length && text.trim() === m[1]) fence = '';
      continue;
    }
    if (m) {
      fence = m[1];
      out.push({text, inCode: true});
      continue;
    }
    out.push({text, inCode: false});
  }
  return {lines: out, openFence: fence};
}

// isInFencedCode reports whether the end of the Markdown is inside a fenced code block.
export function isInFencedCode(markdown: string): boolean {
  return markdownLines(markdown).lines.at(-1)?.inCode ?? false;
}

// stripCodeSpans blanks the inline code of a line, where "[^1]" is not a reference.
function stripCodeSpans(line: string): string {
  return line.replace(/(`+)[\s\S]*?\1/g, (s) => ' '.repeat(s.length));
}

// footnoteDefinitions returns the footnote definitions of the Markdown, by label.
export function footnoteDefinitions(markdown: string): Map<string, string> {
  const defs = new Map<string, string>();
  for (const line of markdownLines(markdown).lines) {
    if (line.inCode) continue;
    const m = DEFINITION_RE.exec(line.text);
    if (m && !defs.has(m[1])) defs.set(m[1], m[2].trim());
  }
  return defs;
}

// referencedLabels returns the labels of the footnotes used in the text, in order of first
// use, whether they are defined or not.
export function referencedLabels(markdown: string): string[] {
  const seen = new Set<string>();
  const labels: string[] = [];
  for (const line of markdownLines(markdown).lines) {
    if (line.inCode || DEFINITION_RE.test(line.text)) continue;
    for (const m of stripCodeSpans(line.text).matchAll(REFERENCE_RE)) {
      if (!seen.has(m[1])) {
        seen.add(m[1]);
        labels.push(m[1]);
      }
    }
  }
  return labels;
}

// footnoteSources returns the sources the article cites, numbered as they render.
export function footnoteSources(markdown: string): FootnoteSource[] {
  const defs = footnoteDefinitions(markdown);
  const sources: FootnoteSource[] = [];
  for (const label of referencedLabels(markdown)) {
    const text = defs.get(label);
    if (text !== undefined) sources.push({label, number: sources.length + 1, text});
  }
  return sources;
}

// nextFootnoteLabel returns an unused label for a new footnote: the next number.
export function nextFootnoteLabel(markdown: string): string {
  let max = 0;
  const labels = [...footnoteDefinitions(markdown).keys(), ...referencedLabels(markdown)];
  for (const label of labels) {
    if (/^\d+$/.test(label)) max = Math.max(max, Number(label));
  }
  return String(max + 1);
}

// referenceNumberAt returns the number a reference to label gets when it is inserted at the
// end of textBefore (the Markdown before the caret): an existing source keeps its number if
// it is already used before; otherwise it comes after the sources used before the caret.
// Like the renderer, it only numbers the references that have a definition: defined holds
// the article's labels (label itself counts as defined).
export function referenceNumberAt(textBefore: string, label: string, defined: ReadonlySet<string>): number {
  const before = referencedLabels(textBefore).filter((l) => l === label || defined.has(l));
  const i = before.indexOf(label);
  return i >= 0 ? i + 1 : before.length + 1;
}

// appendFootnoteDefinition adds "[^label]: text" at the end of the Markdown, after the other
// definitions when the document ends with them, else after a blank line. A code block left
// open at the end is closed first, or the definition would be code.
export function appendFootnoteDefinition(markdown: string, label: string, text: string): string {
  const definition = `[^${label}]: ${text}`;
  let body = markdown.replace(/\s+$/, '');
  const fence = markdownLines(body).openFence;
  if (fence) body += `\n${fence}`;
  if (!body) return `${definition}\n`;
  const lastLine = body.slice(body.lastIndexOf('\n') + 1);
  const separator = DEFINITION_RE.test(lastLine) ? '\n' : '\n\n';
  return `${body}${separator}${definition}\n`;
}

// plainCitationSummary turns a citation's Markdown into the short text the reuse lists show:
// its title and publisher, e.g. '"Luna 2", NASA–NSSDC'.
export function plainCitationSummary(markdown: string): {title: string, publisher: string} {
  const unescape = (s: string) => s.replace(/\\(.)/g, '$1');
  // the title is quoted, and the quote ends before its link or the sentence's period
  const quoted = /"(.+?)"(?=\]\(|\.)/.exec(markdown);
  const italic = /(?<![\\*])\*([^*]+)\*(?!\*)/.exec(markdown);
  let title = quoted ? `"${unescape(quoted[1])}"` : '';
  let publisher = italic ? unescape(italic[1]) : '';
  if (!title && italic) {
    // a book: the italic text is its title
    title = unescape(italic[1]);
    publisher = '';
  }
  if (!title) {
    // drop the unescaped emphasis and autolink markers, then the escapes
    title = unescape(markdown.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/(?<!\\)[*_<>]/g, '')).trim();
    if (title.length > 80) title = `${title.slice(0, 79)}…`;
  }
  return {title, publisher};
}
