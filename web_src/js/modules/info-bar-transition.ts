// Info bars that come and go on a subject page without a reload (the Compare banners, the
// archived notice, the Edit tab's sign-in banner) slide instead of making the page jump
// (#469): one that appears grows from 0 to its height, revealing it from the top; one that
// goes shrinks back to 0, and what is below follows it. Under `prefers-reduced-motion:
// reduce` they appear and go at once.

export const INFO_BAR_DURATION = 200;

export function prefersReducedMotion(): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
}

const running = new WeakMap<HTMLElement, Animation>();

// The box of the bar at full size: its height and everything else that takes room
// vertically, which all go to 0 so the bar takes none at the end (its padding and borders
// would otherwise keep it a few pixels tall).
function fullKeyframe(el: HTMLElement): Keyframe {
  const style = getComputedStyle(el);
  return {
    height: `${el.getBoundingClientRect().height}px`,
    minHeight: '0px',
    paddingTop: style.paddingTop,
    paddingBottom: style.paddingBottom,
    borderTopWidth: style.borderTopWidth,
    borderBottomWidth: style.borderBottomWidth,
    marginTop: style.marginTop,
    marginBottom: style.marginBottom,
  };
}

const emptyKeyframe: Keyframe = {
  height: '0px',
  minHeight: '0px',
  paddingTop: '0px',
  paddingBottom: '0px',
  borderTopWidth: '0px',
  borderBottomWidth: '0px',
  marginTop: '0px',
  marginBottom: '0px',
};

function slide(el: HTMLElement, keyframes: Keyframe[], done?: () => void) {
  running.get(el)?.cancel();
  // A bar above the part of the page on screen changes at once: nothing on screen moves
  // then, as the browser keeps what is on screen in place (scroll anchoring) in one step,
  // which it does not do reliably while the bar's height is animated (WebKit).
  const offscreen = el.getBoundingClientRect().bottom <= 0;
  if (offscreen || prefersReducedMotion() || typeof el.animate !== 'function') {
    done?.();
    return;
  }
  // (box-sizing so that the height animated is the whole box; overflow so that the bar's
  // content is revealed from the top rather than drawn over what is below)
  const animation = el.animate(keyframes, {duration: INFO_BAR_DURATION, easing: 'ease-out', fill: 'forwards'});
  el.style.overflow = 'hidden';
  el.style.boxSizing = 'border-box';
  running.set(el, animation);
  /* The slide is driven frame by frame rather than by the clock. A bar often appears
     with a tab whose content is heavy to set up (the Edit tab's editor), which keeps the
     main thread busy for the first frames: on a clock, a slow machine (WebKit on CI) had
     spent the 200ms before the first frame was drawn, and the bar appeared at once. Here
     the bar is drawn at its first keyframe, then each frame advances the slide by the
     time since the last one, but never by more than 34ms (two frames at 60Hz): it always
     takes at least six frames, and 200ms when nothing holds the page up. */
  const resetStyles = () => {
    el.style.overflow = '';
    el.style.boxSizing = '';
  };
  const finish = () => {
    if (running.get(el) !== animation) return;
    running.delete(el);
    done?.();
  };
  animation.pause();
  animation.currentTime = 0;
  let elapsed = 0;
  let last: number | null = null;
  const step = (now: number) => {
    if (running.get(el) !== animation) return;
    if (last !== null) elapsed += Math.min(now - last, 34);
    last = now;
    if (elapsed >= INFO_BAR_DURATION) {
      // The end is handled here, in this frame, not on the animation's `finish` event,
      // which comes after the frame is drawn. The last keyframe stays applied (fill) for
      // one more frame, while whoever asked hides or removes the bar: a closed bar was
      // otherwise drawn at full height for a frame, between its animation's end and its
      // removal. (The last keyframe of an opening is the bar's own height.)
      animation.currentTime = INFO_BAR_DURATION;
      finish();
      requestAnimationFrame(() => {
        animation.cancel();
        // (the styles stay if another slide of this bar has started meanwhile)
        if (!running.has(el)) resetStyles();
      });
      return;
    }
    animation.currentTime = elapsed;
    requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
  // cancelled while it runs: another slide of this bar takes over, or the slide is undone
  animation.addEventListener('cancel', () => {
    if (running.get(el) !== animation) return;
    running.delete(el);
    resetStyles();
  });
}

/** Slides a bar that has just been shown open from 0 to its height. */
export function expandInfoBar(el: HTMLElement, done?: () => void) {
  slide(el, [emptyKeyframe, fullKeyframe(el)], done);
}

/** Slides a bar, still shown, closed to 0; `done` then hides it. */
export function collapseInfoBar(el: HTMLElement, done?: () => void) {
  slide(el, [fullKeyframe(el), emptyKeyframe], done);
}

// While a bar is closing it carries this class, which keeps it displayed (see
// web_src/css/features/subject-views.css) until it has slid to 0.
const LEAVING = 'is-info-bar-leaving';

/**
 * Bars shown and hidden by CSS or by a class (the sign-in banner follows the Article
 * view's tab through `:has()`, the archived notice `tw-hidden`): `watchInfoBars` notices
 * when one becomes shown or hidden after a change under `root`, and slides it. What they
 * show on the first load does not slide.
 */
export function watchInfoBars(bars: HTMLElement[], root: HTMLElement) {
  const isShown = (el: HTMLElement) => !el.classList.contains(LEAVING) && el.getClientRects().length > 0;
  const shown = new Map(bars.map((el) => [el, isShown(el)]));

  let observer: MutationObserver;
  const update = () => {
    for (const el of bars) {
      if (!el.isConnected) continue;
      const leaving = el.classList.contains(LEAVING);
      // a bar closing is shown again if what hides it went away meanwhile
      if (leaving) el.classList.remove(LEAVING);
      const now = isShown(el);
      const before = shown.get(el);
      if (now === before) {
        if (leaving) el.classList.add(LEAVING);
        continue;
      }
      shown.set(el, now);
      if (now) {
        expandInfoBar(el);
      } else {
        el.classList.add(LEAVING);
        collapseInfoBar(el, () => el.classList.remove(LEAVING));
      }
    }
    // (the class changes made here are not changes to react to)
    observer.takeRecords();
  };

  observer = new MutationObserver(update);
  observer.observe(root, {subtree: true, childList: true, attributes: true, attributeFilter: ['hidden', 'data-article-mode']});
  for (const el of bars) observer.observe(el, {attributes: true, attributeFilter: ['class']});
  return observer;
}
