// The pure part of the subject page's state handling (repo-history.ts): which article is
// selected when the page is opened, and how a history entry describes it. Kept apart from
// the DOM wiring so the rules can be tested on their own.
import {normalizeSelection, resolveSelection, type RepoSelection} from '../modules/repo-selection.ts';

export type ViewKey = 'bubble' | 'table' | 'article';

/** What the subject page stores in each of its history entries. */
export type HistoryState = {
  view: ViewKey;
  mode?: string;
  owner?: string | null;
  subject?: string | null;
  repo?: string | null;
  archived?: boolean;
  link?: string | null;
  /** The version (commit) of the article an entry shows, for "?version=" pages. */
  version?: string | null;
};

/** The selection a history entry recorded: null when it recorded none, undefined when
   the state is not one of ours (no entry was recorded by this page yet). */
export function selectionFromHistoryState(state: HistoryState | null | undefined): RepoSelection | null | undefined {
  if (!state || typeof state !== 'object' || !state.view) return undefined;
  // normalizeSelection makes every other check (an owner, and a repository or subject)
  return normalizeSelection({
    owner: state.owner ?? '',
    repo: state.repo ?? '',
    subject: state.subject ?? null,
    archived: state.archived,
    link: state.link ?? null,
  });
}

export type InitialSelectionInput = {
  /** The Article view's article as the server chose it: the article an article url names,
     the one the "selected" parameter names, or a subject's only article; null when the
     server rendered the Article view with no article chosen. Undefined when the page was
     opened on another view, where the server chooses nothing. */
  serverArticle: RepoSelection | null | undefined;
  /** What the current history entry recorded (Back/Forward, reload); undefined if none. */
  historySelection: RepoSelection | null | undefined;
  /** The "selected" url parameter, owner and repo only. */
  urlSelection: RepoSelection | null;
  /** Every article of the subject. */
  candidates: RepoSelection[];
};

/** The selection a subject page opens with, in order of precedence:
   1. on the Article view, the article the server rendered, or nothing if it rendered
      none: the page shows what the server chose, without fetching anything (#405);
   2. what the history entry recorded, which is exactly what the page showed when the
      entry was left, including "nothing selected";
   3. the "selected" url parameter, when it names one of the subject's articles;
   4. the subject's only article, when it has a single one;
   5. nothing. A subject opened afresh (from Explore, say) has no history entry and no
      parameter, so it starts with nothing selected (#402). */
export function pickInitialSelection(input: InitialSelectionInput): RepoSelection | null {
  if (input.serverArticle !== undefined) return normalizeSelection(input.serverArticle);
  if (input.historySelection !== undefined) {
    /* Unlike the url parameter, a recorded selection is kept even when no candidate
       matches it: an entry made on an article url (an archived article, or one
       outside the candidate list) recorded what the page really showed, link
       included, and dropping it would forget that article on Back/Forward. The url
       parameter is typed or shared by people, so an unknown one is ignored. */
    return resolveSelection(input.historySelection, input.candidates) ?? normalizeSelection(input.historySelection);
  }
  const fromUrl = resolveSelection(input.urlSelection, input.candidates);
  if (fromUrl) return fromUrl;
  if (input.candidates.length === 1) return normalizeSelection(input.candidates[0]);
  return null;
}

// The parameters that describe the subject page's state, which each url sets for itself:
// the view, its article mode, the selection, and a pending Compare press.
// (version is an article page's, never a subject view's: it must not stick to them)
const STATE_PARAMS = new Set(['view', 'mode', 'selected', 'compare', 'version']);

/** The Bubble or Table view url `url`, carrying the other parameters of the page's
   current query (`currentSearch`), such as the Table view's sort, so recording a
   selection or following a view tab does not drop them. `url`'s own parameters win. */
export function withCarriedQuery(url: string, currentSearch: string): string {
  const parsed = new URL(url, 'http://localhost');
  for (const [key, value] of new URLSearchParams(currentSearch)) {
    if (STATE_PARAMS.has(key) || parsed.searchParams.has(key)) continue;
    parsed.searchParams.append(key, value);
  }
  return parsed.pathname + parsed.search + parsed.hash;
}

/** What a url of the subject page says about the view and the article it shows. */
export type SubjectLocation = {
  view: ViewKey;
  mode: string;
  /** The article's owner, when the path names an article. */
  owner: string | null;
  /** The subject, for a vanity article url "/subject/{subject}/{owner}[/{n}]". */
  subject: string | null;
  /** The repository, for the permanent article url "/{owner}/{repo}". */
  repo: string | null;
};

/** Reads a url the subject page can be on:
   - "/subject/{subject}": the view of its "view" parameter (Bubble by default);
   - "/subject/{subject}/{owner}[/{n}]": an article, on the Article view;
   - "/{owner}/{repo}" (the permanent article url, an archived article's for one):
     that article, on the view of its "view" parameter (Article by default). */
export function parseSubjectLocation(href: string, appSubUrl: string | undefined): SubjectLocation {
  const url = new URL(href, 'http://localhost');
  const params = url.searchParams;
  const mode = params.get('mode') || 'read';
  const basePrefix = (appSubUrl || '').replace(/\/+$/, '');
  const pathname = url.pathname;
  const trimmedPath = basePrefix && pathname.startsWith(basePrefix) ? pathname.slice(basePrefix.length) : pathname;
  const segments = trimmedPath.replace(/^\/+/, '').split('/').filter(Boolean).map((s) => decodeURIComponent(s));

  if (segments[0] === 'subject') {
    if (segments.length >= 3) return {view: 'article', mode, owner: segments[2], subject: segments[1], repo: null};
    return {view: (params.get('view') as ViewKey) || 'bubble', mode, owner: null, subject: null, repo: null};
  }
  if (segments[0] === 'explore' && segments[1] === 'articles' && segments[2] === 'history' && segments.length >= 5) {
    // the legacy /explore/articles/history/{owner}/{repo}: the subject page of that
    // article, which it shows (and so selects) on the Article view only
    const view = (params.get('view') as ViewKey) || 'bubble';
    if (view !== 'article') return {view, mode, owner: null, subject: null, repo: null};
    return {view, mode, owner: segments[3], subject: null, repo: segments[4]};
  }
  if (segments.length >= 2) {
    return {view: (params.get('view') as ViewKey) || 'article', mode, owner: segments[0], subject: null, repo: segments[1]};
  }
  return {view: (params.get('view') as ViewKey) || 'bubble', mode, owner: null, subject: null, repo: null};
}
