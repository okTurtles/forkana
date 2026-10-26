// The "Back" of the compare page (Point of contention) goes BACK in the history when the
// page before it is the subject page whose graph opened it, instead of loading that page
// afresh as a new entry (#431 item 1). Loading it again lost the reader's place: the page
// opened at the top, the graph was rebuilt, and the browser's Back then led to the compare
// page again. Going back returns to the very entry that was left, which the bubble view
// restores (modules/graph-history.ts): the point of contention selected and the page at
// the same scroll position, or the page itself when the browser kept it in its
// back/forward cache.
import {readCompareReturn} from '../modules/graph-history.ts';
import {isPlainClick} from '../utils/dom.ts';

function decodedPath(path: string): string {
  try {
    return decodeURIComponent(path).replace(/\/+$/, '');
  } catch {
    return path.replace(/\/+$/, '');
  }
}

export type CompareBackInput = {
  /** The compare page's own path and query. */
  currentUrl: string;
  /** The subject page's path. */
  subjectPath: string;
  /** The url of the history entry before this one, when the browser can tell (Navigation
     API); null or undefined when it cannot. */
  previousEntryUrl: string | null | undefined;
  /** The note the graph left when it opened a point of contention. */
  note: {compareUrl: string, subjectPath: string} | null;
  /** Is there an entry to go back to at all? */
  canGoBack: boolean;
};

/** Should Back go back in the history rather than follow its link? */
export function shouldGoBackInHistory(input: CompareBackInput): boolean {
  if (!input.canGoBack) return false;
  const subject = decodedPath(input.subjectPath);
  if (input.previousEntryUrl) {
    // the browser says what the previous entry is: trust it, and only it. (WebKit lists
    // the entry but gives no url for it: the note decides then.)
    try {
      return decodedPath(new URL(input.previousEntryUrl, 'http://localhost').pathname) === subject;
    } catch {
      return false;
    }
  }
  return Boolean(input.note) &&
    input.note.compareUrl === input.currentUrl &&
    decodedPath(input.note.subjectPath) === subject;
}

type NavigationLike = {currentEntry?: {index: number} | null, entries?: () => {url: string | null}[]};

function previousEntryUrl(): string | null | undefined {
  const nav = (window as unknown as {navigation?: NavigationLike}).navigation;
  if (!nav?.currentEntry || typeof nav.entries !== 'function') return undefined;
  const index = nav.currentEntry.index;
  if (index < 1) return null;
  return nav.entries()[index - 1]?.url ?? null;
}

function canGoBack(): boolean {
  const nav = (window as unknown as {navigation?: NavigationLike}).navigation;
  if (nav?.currentEntry) return nav.currentEntry.index > 0;
  return window.history.length > 1;
}

export function initSubjectCompareBack() {
  const link = document.querySelector<HTMLAnchorElement>('a[data-subject-back]');
  if (!link) return;
  link.addEventListener('click', (event: MouseEvent) => {
    if (!isPlainClick(event)) return;
    const goBack = shouldGoBackInHistory({
      currentUrl: window.location.pathname + window.location.search,
      subjectPath: link.getAttribute('data-subject-path') ?? '',
      previousEntryUrl: previousEntryUrl(),
      note: readCompareReturn(),
      canGoBack: canGoBack(),
    });
    if (!goBack) return;
    event.preventDefault();
    window.history.back();
  });
}
