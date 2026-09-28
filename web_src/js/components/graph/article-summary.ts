/** First real paragraph of a Markdown article: front matter, headings, images,
   tables, HTML, thematic breaks and code fences skipped; inline markup
   stripped; clamped so a pathological first paragraph cannot flood the circle
   (the CSS line-clamps at 4 lines anyway). */
export function extractArticleSummary(markdown: string): string {
  let text = markdown.replace(/^\ufeff/, '');
  const frontMatter = /^---[^\S\n]*\r?\n[\s\S]*?\r?\n---[^\S\n]*(?:\r?\n|$)/.exec(text);
  if (frontMatter) text = text.slice(frontMatter[0].length);
  const paragraph: string[] = [];
  let inFence = false;
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (/^(```|~~~)/.test(line)) {
      if (paragraph.length) break;
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    if (!line) {
      if (paragraph.length) break;
      continue;
    }
    /* Thematic breaks (---, ***, ___, spaces allowed) are skipped like
       headings; without this a README opening with a bare "---" (or unclosed
       front matter) would surface the rule itself as the "summary". */
    if (/^(#{1,6}\s|!\[|\||<)/.test(line) || /^(?:[-*_][ \t]*){3,}$/.test(line)) {
      if (paragraph.length) break;
      continue;
    }
    paragraph.push(line);
  }
  const summary = paragraph.join(' ')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')  // links and images -> their text
    .replace(/[*_`]/g, '')                      // emphasis and code markers
    .replace(/\s+/g, ' ')
    .trim();
  const maxLength = 400;
  return summary.length > maxLength ? `${summary.slice(0, maxLength - 1).trimEnd()}…` : summary;
}
