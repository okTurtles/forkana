// The header's Compare button can be pressed on the Table or Article view, before the
// bubble view (which owns compare mode) has ever been mounted. repo-history.ts then
// switches to the bubble view and leaves the request here; FishboneGraph takes it once
// it listens for the button.
let pending = false;

/** Sent by the header's Compare button: compare mode is to be turned on or off. */
export const COMPARE_MODE_TOGGLE_EVENT = 'repo:compare-mode-toggle';

/** Sent by the bubble view whenever compare mode changes; the button follows it. */
export const COMPARE_MODE_STATE_EVENT = 'repo:compare-mode-state';

/** The detail of COMPARE_MODE_STATE_EVENT. */
export type CompareModeState = {on: boolean, available: boolean};

/** Can compare mode be offered? While the graph loads it is assumed so (a press then is
   checked once it is in), and a graph that failed to load says nothing about the
   subject; otherwise it takes two live articles (a deleted one cannot be compared). */
export function compareAvailableFor(graph: {loading: boolean, failed: boolean, liveArticles: number}): boolean {
  return graph.loading || graph.failed || graph.liveArticles >= 2;
}

/** Forgets a pending request (tests). */
export function resetCompareModeRequest() {
  pending = false;
}

export function requestCompareMode() {
  pending = true;
}

export function takeCompareModeRequest(): boolean {
  const was = pending;
  pending = false;
  return was;
}

/** Loads the graph (`load`), then replays a pending Compare press (`press`) on it. The
   request is taken before the load, so a press made meanwhile is not replayed twice. */
export async function replayCompareModeRequest(load: () => Promise<void>, press: () => void): Promise<void> {
  const requested = takeCompareModeRequest();
  await load();
  if (requested) press();
}

/** The url parameter carrying a Compare press to a page that has the bubble view:
   pressed where there is no graph (the compare page, a repository page), the button
   opens the subject's Bubble view with it, and repo-history.ts turns it into a request. */
export const COMPARE_REQUEST_PARAM = 'compare';

export function withCompareRequest(url: string): string {
  const parsed = new URL(url, window.location.origin);
  parsed.searchParams.set(COMPARE_REQUEST_PARAM, '1');
  return parsed.pathname + parsed.search + parsed.hash;
}

/** Takes a Compare press carried by the current url: removes the parameter from the
   address bar (a reload or a shared link must not press it again) and records it as a
   request. Returns whether there was one. */
export function takeCompareRequestFromUrl(): boolean {
  const url = new URL(window.location.href);
  if (!url.searchParams.has(COMPARE_REQUEST_PARAM)) return false;
  url.searchParams.delete(COMPARE_REQUEST_PARAM);
  window.history.replaceState(window.history.state, '', url.pathname + url.search + url.hash);
  requestCompareMode();
  return true;
}
