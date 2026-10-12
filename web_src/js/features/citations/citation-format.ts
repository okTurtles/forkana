// Citations of external sources (issue #399).
//
// A citation is a Markdown footnote: "[^1]" in the text, and its definition "[^1]: …" whose
// text is the citation in a readable, Wikipedia-like form that the Cite dialog writes:
//
//   website  Author, First (23 August 2023). ["Title"](https://…). *Website*. Retrieved 1 October 2026.
//   news     the same, with the publication
//   journal  Team (1970). "Article title". *Science*. 167 (3918): 449–458. doi:[10.1126/…](https://doi.org/10.1126/…).
//   book     Chaikin, Andrew (1994). *A Man on the Moon* (1st ed.). Viking. ISBN 978-0-670-81446-6.
//   other    the citation as written, and its link if any
//
// Nothing else is stored: the type of a source shown to readers is read back from that text
// (classifyCitation) with the same rules the Cite dialog uses to guess it from what is pasted
// (detectSourceType): a DOI means Journal, an ISBN means Book, a known news site means News,
// any other link means Website.

import newsHosts from '../../../../services/citation/news_hosts.json' with {type: 'json'};

export type SourceType = 'website' | 'news' | 'journal' | 'book' | 'other';
export const sourceTypes: SourceType[] = ['website', 'news', 'journal', 'book', 'other'];

const newsHostSet = new Set<string>(newsHosts);

// isNewsHost reports whether the host (or one of its parent domains) is a known news site.
export function isNewsHost(host: string): boolean {
  let h = host.toLowerCase();
  while (h) {
    if (newsHostSet.has(h)) return true;
    const dot = h.indexOf('.');
    if (dot < 0) break;
    h = h.slice(dot + 1);
  }
  return false;
}

const doiRe = /\b(10\.\d{4,9}\/[^\s"<>]+)/;

// normalizeDOI returns the DOI of a bare DOI, "doi:…" or a doi.org URL, or "".
export function normalizeDOI(s: string): string {
  let v = s.trim();
  const m = /^https?:\/\/(?:dx\.)?doi\.org\/(.+)$/i.exec(v);
  if (m) {
    try {
      v = decodeURIComponent(m[1]);
    } catch {
      v = m[1];
    }
  }
  v = v.replace(/^doi:\s*/i, '');
  const d = doiRe.exec(v);
  if (!d || d.index !== 0) return '';
  return d[1].replace(/[.,;]+$/, '');
}

// normalizeISBN returns the ISBN-10 or ISBN-13 of s without separators if its check digit is
// valid, or "" (the same rule as services/citation.NormalizeISBN).
export function normalizeISBN(s: string): string {
  const m = /^(?:isbn(?:-1[03])?:?\s*)?([0-9][0-9\- ]{8,15}[0-9X])$/i.exec(s.trim());
  if (!m) return '';
  const isbn = m[1].replace(/[- ]/g, '').toUpperCase();
  if (isbn.length === 10) {
    let sum = 0;
    for (let i = 0; i < 10; i++) {
      const c = isbn[i];
      if (c === 'X' && i !== 9) return '';
      sum += (10 - i) * (c === 'X' ? 10 : Number(c));
    }
    return sum % 11 === 0 ? isbn : '';
  }
  if (isbn.length === 13 && !isbn.includes('X')) {
    let sum = 0;
    for (let i = 0; i < 13; i++) sum += (i % 2 ? 3 : 1) * Number(isbn[i]);
    return sum % 10 === 0 ? isbn : '';
  }
  return '';
}

export function parseHttpURL(s: string): URL | null {
  try {
    const u = new URL(s.trim());
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.host ? u : null;
  } catch {
    return null;
  }
}

// detectSourceType guesses the type of the source a writer pasted: a DOI means Journal, an
// ISBN means Book, a known news site means News, and any other URL is Website. It is null
// for anything else.
export function detectSourceType(query: string): SourceType | null {
  if (normalizeDOI(query)) return 'journal';
  if (normalizeISBN(query)) return 'book';
  const u = parseHttpURL(query);
  if (!u) return null;
  return isNewsHost(u.hostname) ? 'news' : 'website';
}

// The fields of the Add citation dialog; all are plain text.
export type CitationFields = {
  url?: string,
  title?: string,
  site?: string, // the website, or the news publication
  authors?: string, // "Last, First; Last, First"
  published?: string, // a date: "2023-08-23", "23 August 2023", "1970"…
  journal?: string,
  volumeIssue?: string, // "167 / 3918"
  pages?: string,
  doi?: string,
  isbn?: string,
  publisher?: string,
  edition?: string,
  link?: string,
  citation?: string, // "other": the citation as written
  location?: string, // where in the source: "p. 42"
};

// requiredFields are the fields marked "*" in the dialog: only what a reader needs to find
// the source.
export const requiredFields: Record<SourceType, Array<keyof CitationFields>> = {
  website: ['url'],
  news: ['url', 'site'],
  journal: ['doi'],
  book: ['title', 'authors'],
  other: ['citation'],
};

export function missingRequiredFields(type: SourceType, fields: CitationFields): Array<keyof CitationFields> {
  return requiredFields[type].filter((f) => !fields[f]?.trim());
}

const months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

// formatCitationDate writes an ISO date the way a citation does ("23 August 2023",
// "August 2023", "2023"); any other text is kept as written.
export function formatCitationDate(s: string): string {
  const v = s.trim();
  let m = /^(\d{4})-(\d{2})-(\d{2})(?:T.*)?$/.exec(v);
  if (m) return `${Number(m[3])} ${months[Number(m[2]) - 1] ?? m[2]} ${m[1]}`;
  m = /^(\d{4})-(\d{2})$/.exec(v);
  if (m) return `${months[Number(m[2]) - 1] ?? m[2]} ${m[1]}`;
  return v;
}

export function todayCitationDate(now: Date = new Date()): string {
  return `${now.getDate()} ${months[now.getMonth()]} ${now.getFullYear()}`;
}

// yearOf returns the year of a date written in any of the forms above.
export function yearOf(s: string): string {
  return /\b(1[5-9]\d\d|2\d\d\d)\b/.exec(s)?.[1] ?? s.trim();
}

// escapeMarkdownText escapes the characters that would turn plain text into Markdown, and
// keeps the citation on one line (a footnote definition is one line).
export function escapeMarkdownText(s: string): string {
  return s.replace(/\s+/g, ' ').trim().replace(/([\\`*_[\]<>|])/g, '\\$1');
}

// markdownDestination writes a URL as a link destination.
export function markdownDestination(url: string): string {
  const u = url.trim();
  return /[\s()<>]/.test(u) ? `<${u.replace(/[<>]/g, encodeURIComponent)}>` : u;
}

function authorsText(authors: string | undefined): string {
  return (authors ?? '').split(';').map((a) => a.trim()).filter(Boolean).join('; ');
}

function sentence(s: string): string {
  const t = s.trim();
  return !t || /[.!?]$/.test(t) ? t : `${t}.`;
}

// formatCitation returns the Markdown text of a citation, the text of its footnote definition.
export function formatCitation(type: SourceType, f: CitationFields, today: string = todayCitationDate()): string {
  const authors = escapeMarkdownText(authorsText(f.authors));
  const parts: string[] = [];
  switch (type) {
    case 'website':
    case 'news': {
      const url = (f.url ?? '').trim();
      const titleText = escapeMarkdownText(f.title?.trim() || url);
      const title = url ? `["${titleText}"](${markdownDestination(url)})` : `"${titleText}"`;
      const date = f.published?.trim() ? escapeMarkdownText(formatCitationDate(f.published)) : '';
      if (authors) parts.push(date ? `${authors} (${date}).` : sentence(authors));
      parts.push(`${title}.`);
      if (f.site?.trim()) parts.push(`*${escapeMarkdownText(f.site)}*.`);
      if (!authors && date) parts.push(`${date}.`);
      parts.push(`Retrieved ${today}.`);
      break;
    }
    case 'journal': {
      const doi = normalizeDOI(f.doi ?? '') || '';
      const url = (f.url ?? '').trim() || (doi ? '' : parseHttpURL(f.doi ?? '')?.href ?? '');
      const year = f.published?.trim() ? escapeMarkdownText(yearOf(f.published)) : '';
      if (authors) parts.push(year ? `${authors} (${year}).` : sentence(authors));
      const titleText = escapeMarkdownText(f.title ?? '');
      if (titleText) parts.push(url ? `["${titleText}"](${markdownDestination(url)}).` : `"${titleText}".`);
      if (f.journal?.trim()) parts.push(`*${escapeMarkdownText(f.journal)}*.`);
      const [volume, issue] = (f.volumeIssue ?? '').split('/').map((s) => escapeMarkdownText(s));
      let where = volume ?? '';
      if (issue) where += ` (${issue})`;
      if (f.pages?.trim()) where += `${where ? ': ' : ''}${escapeMarkdownText(f.pages).replace(/-/g, '–')}`;
      if (where.trim()) parts.push(`${where.trim()}.`);
      if (!authors && year) parts.push(`${year}.`);
      if (doi) parts.push(`doi:[${escapeMarkdownText(doi)}](https://doi.org/${encodeURI(doi)}).`);
      break;
    }
    case 'book': {
      const year = f.published?.trim() ? escapeMarkdownText(yearOf(f.published)) : '';
      if (authors) parts.push(year ? `${authors} (${year}).` : sentence(authors));
      const link = (f.link ?? '').trim();
      const titleText = `*${escapeMarkdownText(f.title ?? '')}*`;
      let title = link ? `[${titleText}](${markdownDestination(link)})` : titleText;
      const edition = escapeMarkdownText(f.edition ?? '').replace(/\s*ed\.?$/i, '');
      if (edition) title += ` (${edition} ed.)`;
      parts.push(`${title}.`);
      if (f.publisher?.trim()) parts.push(sentence(escapeMarkdownText(f.publisher)));
      if (!authors && year) parts.push(`${year}.`);
      const isbn = (f.isbn ?? '').trim();
      if (isbn) parts.push(`ISBN ${escapeMarkdownText(isbn)}.`);
      break;
    }
    default: {
      parts.push(sentence(escapeMarkdownText(f.citation ?? '')));
      const link = (f.link ?? '').trim();
      if (parseHttpURL(link)) parts.push(`<${link.replace(/[<>\s]/g, encodeURIComponent)}>`);
    }
  }
  if (f.location?.trim()) parts.push(sentence(escapeMarkdownText(f.location)));
  return parts.filter(Boolean).join(' ');
}

// A citation as readers see it, read from its rendered HTML.
export type CitationInfo = {
  type: SourceType,
  publisher: string, // "" when it cannot be told
  sourceURL: string, // the first link leaving Forkana, "" if none
  archived: boolean,
};

function firstExternalLink(el: Element): HTMLAnchorElement | null {
  for (const a of el.querySelectorAll<HTMLAnchorElement>('a[href]')) {
    if (a.classList.contains('footnote-backref')) continue;
    const u = parseHttpURL(a.href);
    if (u && u.origin !== window.location.origin) return a;
  }
  return null;
}

// classifyCitation reads the type, publisher and source link of a rendered citation (the
// content of a reference, without its back-links).
export function classifyCitation(el: Element): CitationInfo {
  const text = el.textContent ?? '';
  const link = firstExternalLink(el);
  const linkURL = link ? parseHttpURL(link.href) : null;
  let type: SourceType = 'other';
  const doiLink = Array.from(el.querySelectorAll<HTMLAnchorElement>('a[href]')).some((a) => /^https?:\/\/(dx\.)?doi\.org\//i.test(a.href));
  if (doiLink || /\bdoi:\s*10\./i.test(text)) type = 'journal';
  else if (/\bISBN[\s:]/.test(text)) type = 'book';
  else if (linkURL && isNewsHost(linkURL.hostname)) type = 'news';
  else if (linkURL) type = 'website';

  let publisher = '';
  if (type === 'book') {
    // "Author (1994). *Title* (1st ed.). Publisher. ISBN …": the sentence before the ISBN
    const before = text.split(/\bISBN\b/)[0].trim().replace(/\.$/, '');
    const sentences = before.split(/\.\s+/);
    if (sentences.length > 2) publisher = sentences[sentences.length - 1].trim();
  } else {
    for (const em of el.querySelectorAll('em')) {
      if (em.closest('a')) continue; // a linked title, not the publisher
      publisher = em.textContent?.trim() ?? '';
      if (publisher) break;
    }
  }
  if (!publisher && linkURL && type !== 'other') publisher = linkURL.hostname.replace(/^www\./, '');

  return {
    type,
    publisher,
    sourceURL: link?.href ?? '',
    archived: /\barchived\b/i.test(text),
  };
}
