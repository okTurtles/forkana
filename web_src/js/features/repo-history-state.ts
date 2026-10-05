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
};

/** The selection a history entry recorded: null when it recorded none, undefined when
   the state is not one of ours (no entry was recorded by this page yet). */
export function selectionFromHistoryState(state: HistoryState | null | undefined): RepoSelection | null | undefined {
  if (!state || typeof state !== 'object' || !state.view) return undefined;
  if (!state.owner || !(state.repo || state.subject)) return null;
  return normalizeSelection({
    owner: state.owner,
    repo: state.repo || state.subject,
    subject: state.subject ?? state.repo ?? null,
    archived: state.archived === true,
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
    return resolveSelection(input.historySelection, input.candidates) ?? input.historySelection;
  }
  const fromUrl = resolveSelection(input.urlSelection, input.candidates);
  if (fromUrl) return fromUrl;
  if (input.candidates.length === 1) return normalizeSelection(input.candidates[0]);
  return null;
}

// The parameters that describe the subject page's state, which each url sets for itself:
// the view, its article mode, the selection, and a pending Compare press.
const STATE_PARAMS = new Set(['view', 'mode', 'selected', 'compare']);

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
