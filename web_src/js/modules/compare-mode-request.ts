// The header's Compare button can be pressed on the Table or Article view, before the
// bubble view (which owns compare mode) has ever been mounted. repo-history.ts then
// switches to the bubble view and leaves the request here; FishboneGraph takes it once
// it listens for the button.
let pending = false;

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
