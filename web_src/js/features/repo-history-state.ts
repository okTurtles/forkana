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
  /** The article an article url (/subject/{subject}/{owner}[/{n}]) names, if it is one. */
  urlArticle: RepoSelection | null;
  /** What the current history entry recorded (Back/Forward, reload); undefined if none. */
  historySelection: RepoSelection | null | undefined;
  /** The "selected" url parameter, owner and repo only. */
  urlSelection: RepoSelection | null;
  /** Every article of the subject. */
  candidates: RepoSelection[];
};

/** The selection a subject page opens with, in order of precedence:
   1. the article an article url names;
   2. what the history entry recorded, which is exactly what the page showed when the
      entry was left, including "nothing selected";
   3. the "selected" url parameter, when it names one of the subject's articles;
   4. the subject's only article, when it has a single one;
   5. nothing. A subject opened afresh (from Explore, say) has no history entry and no
      parameter, so it starts with nothing selected (#402). */
export function pickInitialSelection(input: InitialSelectionInput): RepoSelection | null {
  if (input.urlArticle) return normalizeSelection(input.urlArticle);
  if (input.historySelection !== undefined) {
    return resolveSelection(input.historySelection, input.candidates) ?? input.historySelection;
  }
  const fromUrl = resolveSelection(input.urlSelection, input.candidates);
  if (fromUrl) return fromUrl;
  if (input.candidates.length === 1) return normalizeSelection(input.candidates[0]);
  return null;
}
