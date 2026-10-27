// The header's Compare button can be pressed on the Article view, before the bubble view
// (which owns compare mode there) has ever been mounted. repo-history.ts then switches to
// the bubble view and leaves the request here; FishboneGraph takes it once it listens for
// the button. (The Table view has a compare mode of its own: repo-history-table.ts.)
let pending = false;

/** Sent by the header's Compare button: compare mode is to be turned on or off. */
export const COMPARE_MODE_TOGGLE_EVENT = 'repo:compare-mode-toggle';

/** Sent by the view whose compare mode changed (the Bubble view's FishboneGraph, or the
   Table view's repo-history-table.ts); the header's button follows it. */
export const COMPARE_MODE_STATE_EVENT = 'repo:compare-mode-state';

/** The detail of COMPARE_MODE_STATE_EVENT. */
export type CompareModeState = {on: boolean, available: boolean};

/** Can compare mode be offered? While the graph loads it is assumed so (a press then is
   checked once it is in), and a graph that failed to load says nothing about the
   subject; otherwise it takes two live articles (a deleted one cannot be compared). */
export function compareAvailableFor(graph: {loading: boolean, failed: boolean, liveArticles: number}): boolean {
  return graph.loading || graph.failed || graph.liveArticles >= 2;
}

/** Can this bubble be picked for a comparison? A deleted article has nothing to
   compare (the compare page is a 404 for it), and a hidden root is no article. */
export function canPickForCompare(node: {isTombstoned?: boolean, isHidden?: boolean}): boolean {
  return node.isTombstoned !== true && node.isHidden !== true;
}

/** What a screen reader hears after a compare action: the banner's (localized) message,
   led by the article the action was about, if any. */
export function compareAnnouncementFor(article: string | null, message: string): string {
  return article ? `${article}: ${message}` : message;
}

/** The Compare banner's states (CompareAnnouncement.vue), one per figma frame. */
export type CompareAnnouncementState = 'select' | 'one' | 'ready' | 'unavailable';

/** The Compare banner's messages (CompareAnnouncement.vue). */
export type CompareAnnouncementMessages = {
  select: string;
  one: string;
  ready: string;
  unavailable: string;
  compareNow: string;
  /** The x's label while compare mode is on: it exits compare mode. */
  dismiss: string;
  /** The x's label on the `unavailable` notice, which it only hides. */
  dismissNotice: string;
  /** The `unavailable` message split around its "Fork this article" link (#451), or null
     when the page gives no fork url: the message is then plain text. */
  unavailableLink?: CompareMessageLink | null;
};

/** A message with one link in it: `before`, the link's `text` to `href`, then `after`. */
export type CompareMessageLink = {before: string; text: string; href: string; after: string};

/** The banner's localized messages, from the `data-msg-*` attributes the subject page puts
   on `root` (#compare-announcement-root), with English fallbacks. `overrides` replaces
   some of them (the Table view's own one-picked message); empty ones are ignored. */
export function readCompareMessages(root: Element | null, overrides: Partial<CompareAnnouncementMessages> = {}): CompareAnnouncementMessages {
  const msg = (name: string) => root?.getAttribute(`data-msg-${name}`);
  /* "No forks yet" comes with a %s where its "Fork this article" link goes (#451): the
     link's text fills it for the plain message (what a screen reader hears), and the
     banner draws it as a link to the page's fork url. */
  const noForks = msg('no-forks') || 'No forks yet. Compare needs at least 2 articles. %s to start comparing.';
  const forkText = msg('fork-link') || 'Fork this article';
  const forkUrl = root?.getAttribute('data-fork-url');
  // exactly one %s makes a link; a translation with none or several is plain text
  const parts = noForks.split('%s');
  const unavailableLink = forkUrl && parts.length === 2 ?
    {before: parts[0], text: forkText, href: forkUrl, after: parts[1]} :
    null;
  return {
    select: overrides.select || msg('select') || 'Select 2 articles to compare (0/2 selected).',
    one: overrides.one || msg('one') || '1/2 selected – select one more to compare.',
    ready: overrides.ready || msg('ready') || '2/2 selected – ready to compare.',
    unavailable: overrides.unavailable || parts.join(forkText),
    unavailableLink: overrides.unavailable ? null : unavailableLink,
    compareNow: overrides.compareNow || msg('compare-now') || 'Compare now',
    dismiss: overrides.dismiss || msg('dismiss') || 'Exit compare mode',
    dismissNotice: overrides.dismissNotice || msg('dismiss-notice') || 'Dismiss',
  };
}

/** The compare page of two owners' articles of a subject. */
export function subjectCompareUrl(appSubUrl: string, subject: string, ownerA: string, ownerB: string): string {
  return `${appSubUrl}/subject/${encodeURIComponent(subject)}/compare/${encodeURIComponent(ownerA)}...${encodeURIComponent(ownerB)}`;
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
