// What the Bubble view keeps in a history entry of the subject page, next to the
// selection repo-history.ts records there (#431). It lives under the `graph` key of
// history.state, which repo-history.ts carries over whenever it rewrites the entry, so a
// selection change or a reload does not drop it:
//
//   * `poc`: the point of contention (joint id "{sourceId}-{targetId}") that was clicked
//     last. A point of contention and an article are selected separately (#431 item 5),
//     so it only counts on an entry that records no selected article.
//   * `scrollY`: the page scroll when the entry was left for the compare page, so coming
//     Back puts the graph where it was, even when the page had to be reloaded (a page
//     rendered for a signed-in user is never kept in the back/forward cache).
//   * `detail`: the entry was pushed when an article was opened in the graph, so the
//     browser's Back closes that article instead of leaving the page (#431 item 4).

export type GraphHistoryState = {
  poc?: string | null;
  scrollY?: number | null;
  detail?: boolean;
};

type StateWithGraph = Record<string, unknown> & {graph?: GraphHistoryState};

/** The Bubble view's part of a history state (an event's, or the current entry's). */
export function graphStateOf(state: unknown): GraphHistoryState {
  if (!state || typeof state !== 'object') return {};
  const graph = (state as StateWithGraph).graph;
  return graph && typeof graph === 'object' ? graph : {};
}

export function currentGraphState(): GraphHistoryState {
  return graphStateOf(window.history.state);
}

/** Merges `patch` into the current entry's graph state, url unchanged. */
export function replaceGraphState(patch: GraphHistoryState) {
  const state = (window.history.state ?? {}) as StateWithGraph;
  window.history.replaceState({...state, graph: {...graphStateOf(state), ...patch}}, '', window.location.href);
}

/** Pushes a copy of the current entry (same url, same page state) with `graph` as its
   graph state. */
export function pushGraphState(graph: GraphHistoryState) {
  const state = (window.history.state ?? {}) as StateWithGraph;
  window.history.pushState({...state, graph}, '', window.location.href);
}

// The compare page's "Back" goes back in the history (rather than loading the subject
// page afresh) when the page before it is the graph that opened it. The graph leaves this
// note when it opens a point of contention; the Navigation API, where a browser has it,
// answers the question directly.
const COMPARE_RETURN_KEY = 'forkana:poc-return';

export function rememberCompareReturn(compareUrl: string, subjectPath: string) {
  try {
    window.sessionStorage.setItem(COMPARE_RETURN_KEY, JSON.stringify({compareUrl, subjectPath}));
  } catch {
    // storage unavailable: the Back link then loads the subject page instead
  }
}

export function readCompareReturn(): {compareUrl: string, subjectPath: string} | null {
  try {
    const raw = window.sessionStorage.getItem(COMPARE_RETURN_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    return typeof parsed?.compareUrl === 'string' && typeof parsed?.subjectPath === 'string' ? parsed : null;
  } catch {
    return null;
  }
}
