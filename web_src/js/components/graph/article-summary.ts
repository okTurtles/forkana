import {GET} from '../../modules/fetch.ts';

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

/** The raw README.md of an article (an article IS its repository's README, see
   AGENTS.md) on `branch`; each segment of a branch such as "feature/x" is escaped on
   its own, so its slash stays a path separator. */
export function articleReadmeUrl(appSubUrl: string, owner: string, repo: string, branch: string): string {
  const ref = branch.split('/').map(encodeURIComponent).join('/');
  return `${appSubUrl}/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/raw/branch/${ref}/README.md`;
}

/** The summary of the README at `url` (articleReadmeUrl), shared by the Bubble and Table
   views: `summary` is empty when there is none, and `retry` says whether asking again
   later may give one (a server or network error) or not (no README: a 404). */
export async function fetchReadmeSummary(url: string): Promise<{summary: string, retry: boolean}> {
  try {
    const res = await GET(url);
    if (!res.ok) return {summary: '', retry: res.status >= 500};
    return {summary: extractArticleSummary(await res.text()), retry: false};
  } catch {
    return {summary: '', retry: true};
  }
}
