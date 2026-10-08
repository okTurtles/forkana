// The selected article of a subject page. ONE selection is shared by the three views of
// the page (Bubble, Table and Article view) and by the browser history, so every view
// shows the same article whatever order they are visited in (#405, #406):
//
//   * repo-history.ts owns it. Every change goes through it and is broadcast with the
//     SELECTION_UPDATED_EVENT; FishboneGraph reads the current value with
//     getCurrentSelection() when its graph has loaded and follows the event afterwards.
//   * it is written into the current history entry (history.state, and the url as the
//     "selected={owner}/{repo}" parameter), so Back/Forward and a reload restore the
//     selection of that entry, and a link that carries the parameter (the view tabs of
//     the compare page, "Back to bubble view") opens the page with it.
//   * it is NOT kept anywhere else. It used to live in localStorage, which made a subject
//     opened again from Explore come back with the selection of the last visit (#402):
//     a fresh visit has no parameter and no history state, so it starts with nothing
//     selected.

export type RepoSelection = {
  owner: string;
  repo: string;
  subject?: string | null;
  archived?: boolean;
  // The article url the server built for this repository. An owner can hold several
  // articles for one subject, and only the server knows which index each one carries
  // in "/subject/{subject}/{owner}/{n}", so the link is carried along instead of being
  // rebuilt from the owner and subject alone. It is only valid for the page it was
  // rendered on, since the indexes renumber whenever the owner archives, deletes or adds
  // an article for the subject, so it is never put in a url.
  link?: string | null;
};

/** The url parameter carrying the selection: "selected={owner}/{repo}". */
export const SELECTION_PARAM = 'selected';

/** Broadcast whenever the selection changes; the detail is the new selection or null. */
export const SELECTION_UPDATED_EVENT = 'repo:selection-updated';

/** Sent by the bubble view when a bubble is selected (detail: the selection) or cleared (null). */
export const BUBBLE_SELECTED_EVENT = 'repo:bubble-selected';

/** Sent by the bubble view when a bubble asks for its article to be opened; the detail is its selection. */
export const BUBBLE_OPEN_ARTICLE_EVENT = 'repo:bubble-open-article';

// keys of the localStorage selection used by earlier versions, only ever removed now
const LEGACY_STORAGE_KEYS = [
  'selectedArticleOwner',
  'selectedArticleSubject',
  'selectedArticleRepo',
  'selectedArticleArchived',
  'selectedArticleLink',
];

let currentSelection: RepoSelection | null = null;

export function getCurrentSelection(): RepoSelection | null {
  return currentSelection;
}

export function setCurrentSelection(selection: RepoSelection | null) {
  currentSelection = selection;
}

/** Fills in the optional fields, or returns null when the selection names no article. */
export function normalizeSelection(selection: RepoSelection | null | undefined): RepoSelection | null {
  if (!selection) return null;
  const repo = selection.repo || selection.subject || '';
  if (!selection.owner || !repo) return null;
  return {
    owner: selection.owner,
    repo,
    subject: selection.subject ?? selection.repo ?? null,
    archived: selection.archived === true,
    link: selection.link ?? null,
  };
}

/** Do both name the same article? Owner and repository names are case-insensitive. */
export function matchesSelection(a: RepoSelection | null | undefined, b: RepoSelection | null | undefined): boolean {
  if (!a || !b) return false;
  if (a.owner.toLowerCase() !== b.owner.toLowerCase()) return false;
  if (a.repo && b.repo) return a.repo.toLowerCase() === b.repo.toLowerCase();
  return (a.subject ?? '').toLowerCase() === (b.subject ?? '').toLowerCase();
}

/** The value of the url parameter for a selection. */
export function selectionToParam(selection: RepoSelection): string {
  return `${selection.owner}/${selection.repo}`;
}

/** Parses the url parameter. Only owner and repo are known from it. */
export function selectionFromParam(value: string | null | undefined): RepoSelection | null {
  if (!value) return null;
  const idx = value.indexOf('/');
  if (idx <= 0 || idx === value.length - 1) return null;
  const owner = value.slice(0, idx);
  const repo = value.slice(idx + 1);
  if (repo.includes('/')) return null;
  return {owner, repo, subject: null, archived: false, link: null};
}

/** Returns `url` (a path with an optional query) with the selection parameter set to
   `selection`, or removed when it is null. */
export function withSelectionParam(url: string, selection: RepoSelection | null): string {
  const parsed = new URL(url, 'http://localhost');
  if (selection) {
    parsed.searchParams.set(SELECTION_PARAM, selectionToParam(selection));
  } else {
    parsed.searchParams.delete(SELECTION_PARAM);
  }
  return parsed.pathname + parsed.search + parsed.hash;
}

/** Completes a selection known only by owner and repo (from the url) with what the page
   knows about that article: the table lists every article of the subject. Returns null
   when the article is not one of them, so a stale or foreign parameter selects nothing. */
export function resolveSelection(selection: RepoSelection | null, candidates: RepoSelection[]): RepoSelection | null {
  if (!selection) return null;
  const match = candidates.find((c) => matchesSelection(c, selection));
  return match ? normalizeSelection(match) : null;
}

/** Removes the selection earlier versions kept in localStorage. */
export function clearLegacyStoredSelection() {
  try {
    for (const key of LEGACY_STORAGE_KEYS) window.localStorage.removeItem(key);
  } catch {
    // storage can be unavailable (private mode, blocked site data); nothing to clear then
  }
}
