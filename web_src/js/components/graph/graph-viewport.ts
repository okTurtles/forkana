/* graph-viewport.ts — how the bubble graph turns a MEASURED container into the
   numbers the layout runs at (#348).

   These four lines used to live inside FishboneGraph.vue, mixed into the
   ResizeObserver callback. They are pure functions of a measurement, so they
   live here where a test can pin them down: the whole point of #348 is that the
   canvas must be derived from the container EVERY time the container changes,
   and a re-measure that mis-reports "nothing changed" is exactly how the graph
   kept its mount-time height. */

/** Never collapse the canvas below this, however short the container is.
 *
 * The page-layout floor in web_src/css/features/bubble-graph.css
 * (".history-bubble-root { min-height }") is the same number, so the box the canvas is
 * measured from is never smaller than the canvas itself -- change the two together. */
export const MIN_SVG_HEIGHT = 320;
/** Container width the layout is capped at: the graph is centred in wider
   containers rather than spread across them. */
export const MAX_LAYOUT_WIDTH = 1100;
/** Width used before anything has been measured (a container that is not
   rendered yet reports 0, and a layout at width 0 draws nothing). */
export const DEFAULT_CONTAINER_WIDTH = MAX_LAYOUT_WIDTH;
/** ONE default for "container height we have not measured yet". There used to
   be two (DEFAULT_SVG_HEIGHT 1000 and DEFAULT_CONTAINER_HEIGHT 800) that meant
   the same thing and disagreed, so the first layout was fitted against one
   value and drawn at the other. */
export const DEFAULT_CONTAINER_HEIGHT = 800;
/** Sub-pixel jitter (zoom levels, fractional layout) must not count as a
   resize: a re-layout per frame would fight the scrollbar it can create.
   1px, not the 2px the old inline observer compared with: at 2px a genuine
   2px change is silently skipped, and everything downstream of a spurious
   pass is now cheap (one rect read, then the bail-out below). The only churn
   this has to absorb is sub-pixel, and a scrollbar — the one thing a re-layout
   can itself add to the container — is ~15px wide, far above either value. */
export const SIZE_EPSILON = 1;

export type ContainerSize = {width: number; height: number};

/** Height available to the SVG canvas: the measured container minus anything
   else inside its scroll box (the legend sits under the graph). Without this
   the canvas was given the FULL container height, the legend was pushed past
   the fold and the container grew a scrollbar. */
export function canvasHeightFor(containerHeight: number, legendHeight: number): number {
  return Math.max(MIN_SVG_HEIGHT, (containerHeight || DEFAULT_CONTAINER_HEIGHT) - legendHeight);
}

/** The width a layout runs at, from a raw measurement. Clamped — and the clamp
   is why the raw measurement has to be kept separately: comparing a clamped
   value against the next raw one reported "changed" on every single delivery
   for any container wider than the cap, which re-ran the layout (and threw
   away the user's pan/zoom) for resizes that never happened.

   The `|| DEFAULT_CONTAINER_WIDTH` fallback is for callers that do NOT
   pre-filter with isMeasurable() below — FishboneGraph does, so a 0 never
   reaches here from the component; it is the answer for anyone asking "what
   width would a layout run at" before anything has been measured. */
export function layoutWidthFor(rawWidth: number): number {
  return Math.min(rawWidth || DEFAULT_CONTAINER_WIDTH, MAX_LAYOUT_WIDTH);
}

/** Is this a real, drawable measurement? A container that is `display:none`
   (the other view is showing) measures 0×0; sizing the canvas from that would
   bake the placeholder in, which is symptom two of #348. */
export function isMeasurable(size: ContainerSize): boolean {
  return size.width > 0 && size.height > 0;
}

/** Bail-out test for the re-measure path: compares the numbers a layout
   actually RUNS AT — the CLAMPED width and the raw height — measurement to
   measurement. Both sides go through the same clamp, so an unchanged container
   never triggers a re-render (the observer cannot feed itself) and neither
   does a width-only change above the cap, which cannot alter a single layout
   input yet used to cost a full re-layout plus the user's pan/zoom.

   The old bug was the asymmetry, not the clamp: the CLAMPED width was stored
   and the next RAW one compared against it, so above the cap every delivery
   read as a change. Clamping both sides keeps that fixed. */
export function sizeChanged(prev: ContainerSize, next: ContainerSize, epsilon = SIZE_EPSILON): boolean {
  return Math.abs(layoutWidthFor(next.width) - layoutWidthFor(prev.width)) > epsilon ||
    Math.abs(next.height - prev.height) > epsilon;
}

/** The name of the "the bubble section is on screen now" event, dispatched by
   repo-history.ts after the table→bubble switch. One constant so the dispatch
   and the listener cannot drift apart. */
export const BUBBLE_VISIBLE_EVENT = 'repo:bubble-visible';

/** Register every re-measure trigger the ResizeObserver cannot serve, and
   return the cleanup.

   None of these depend on the container element, which is the point: the
   caller registers them BEFORE it awaits anything, so an event dispatched
   right after `app.mount()` (the `repo:bubble-visible` handoff) cannot arrive
   before the listener exists. That handoff used to work only because the
   component's first `await nextTick()` happened to resolve ahead of
   `ensureBubbleView`'s — true today, and silently false the moment an `await`
   is added earlier in onMounted.

   A throttled or background tab is why `visibilitychange` is here: rendering
   is throttled there, so a resize is delivered late or coalesced away and the
   graph paints at the size it last rendered at. Every trigger runs the same
   bail-out, so a spurious pass costs one rect read. */
export function registerRemeasureTriggers(
  remeasure: () => void,
  win: Window = window,
  doc: Document = document,
): () => void {
  const onTrigger = () => remeasure();
  const onVisibility = () => { if (!doc.hidden) remeasure(); };
  /* github/prefer-observers is right in general and wrong here: the observer
     IS the primary trigger (observeContainerResize below). This listener only
     covers the deliveries a ResizeObserver cannot make — a tab whose rendering
     is throttled coalesces or delays them, and there is no observer for
     "the tab came back". `resize`/`orientationchange` are not cancelable, so
     {passive: true} would be a no-op on them and is left off. */
  // eslint-disable-next-line github/prefer-observers
  win.addEventListener('resize', onTrigger);
  win.addEventListener('orientationchange', onTrigger);
  win.addEventListener(BUBBLE_VISIBLE_EVENT, onTrigger);
  doc.addEventListener('visibilitychange', onVisibility);
  return () => {
    win.removeEventListener('resize', onTrigger);
    win.removeEventListener('orientationchange', onTrigger);
    win.removeEventListener(BUBBLE_VISIBLE_EVENT, onTrigger);
    doc.removeEventListener('visibilitychange', onVisibility);
  };
}

/** Observe the CONTAINER — never the <svg> whose height the graph sets, which
   would be a feedback loop — and return the disconnect. Takes the same
   callback as the triggers above: one behaviour, one funnel, one place that
   can be tested. */
export function observeContainerResize(el: Element, remeasure: () => void): () => void {
  if (typeof ResizeObserver === 'undefined') return () => {};
  const ro = new ResizeObserver(() => remeasure());
  ro.observe(el);
  return () => ro.disconnect();
}
