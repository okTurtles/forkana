/* zoom-gestures.ts — keep every zoom gesture made over the bubble diagram on the
   diagram, never on the page (#422).

   The page used to zoom whenever:
     - d3-zoom ignored a ctrl+wheel (trackpad pinch in Chrome/Firefox, or
       ctrl+mouse wheel) because the graph was already at its scale limit:
       d3's `wheeled` returns BEFORE preventDefault when the clamped scale
       does not change, so the browser zoomed the page instead;
     - the gesture landed on something d3 does not handle at all: the opened
       article (ArticleDetailView) that covers the canvas, which is the whole
       diagram on a one-article subject, or the History card beside it;
     - Safari's trackpad pinch, which arrives as `gesture*` events rather than
       ctrl+wheel, and which d3-zoom does not listen to;
     - a two-finger pinch on a touch screen over the opened article.

   attachZoomGestures() funnels all four into one `onZoom(factor, x, y)` and
   cancels the browser's own zoom for each. It runs on the canvas box (the
   `.graph-container`), so the gestures over the legend, the tabs and the rest
   of the page keep their usual meaning. */

/** d3-zoom's own wheel step (defaultWheelDelta), so a pinch over the opened
   article zooms at the same rate as one over the graph. */
export function wheelZoomFactor(ev: Pick<WheelEvent, 'deltaY' | 'deltaMode' | 'ctrlKey'>): number {
  const perUnit = ev.deltaMode === 1 ? 0.05 : ev.deltaMode ? 1 : 0.002;
  return 2 ** (-ev.deltaY * perUnit * (ev.ctrlKey ? 10 : 1));
}

/** Clamp a zoom level to its range. */
export function clampZoom(k: number, min: number, max: number): number {
  if (!Number.isFinite(k)) return min;
  return Math.min(max, Math.max(min, k));
}

export type ZoomGestureHandlers = {
  /** Zoom by `factor` (>1 in, <1 out) around the client point (x, y). The
     target is the element the gesture started on. Return nothing; the
     browser's own zoom is cancelled whether or not anything zoomed. */
  onZoom: (factor: number, x: number, y: number, target: EventTarget | null) => void;
  /** Whether a gesture on this target is already zoomed by someone else (d3
     on the <svg>): its wheel and touch events are then only kept off the page. */
  handledElsewhere?: (target: EventTarget | null) => boolean;
};

/* Safari's GestureEvent is not in the TS DOM lib. */
type SafariGestureEvent = UIEvent & {scale: number; clientX: number; clientY: number};

/** A ctrl+wheel this recent means the browser reports the pinch as wheel
   events too, and the gesture events must not zoom a second time. */
const WHEEL_GESTURE_OVERLAP_MS = 250;

export function attachZoomGestures(el: HTMLElement, handlers: ZoomGestureHandlers): () => void {
  let lastCtrlWheelAt = -Infinity;
  const elsewhere = (t: EventTarget | null) => handlers.handledElsewhere?.(t) === true;

  /* Capture phase: every ctrl+wheel is seen here, including those d3 zooms.
     d3-zoom's `noevent` calls stopImmediatePropagation, so a wheel it zoomed
     never reaches the bubble-phase listener below, and the gesture path's
     "already served by a wheel" check would miss it. */
  const noteCtrlWheel = (ev: WheelEvent) => {
    if (ev.ctrlKey) lastCtrlWheelAt = ev.timeStamp;
  };

  /* Bubble phase, after d3's listener on the <svg>: a wheel d3 zoomed has been
     stopped and never gets here. One it ignored (at the scale limit) arrives,
     and is cancelled here so the page does not zoom in its place. */
  const onWheel = (ev: WheelEvent) => {
    if (!ev.ctrlKey) return;   // a plain wheel scrolls the page, as everywhere else
    if (ev.defaultPrevented) return;   // someone else already took it
    ev.preventDefault();
    if (elsewhere(ev.target)) return;
    handlers.onZoom(wheelZoomFactor(ev), ev.clientX, ev.clientY, ev.target);
  };

  /* Safari: gesturestart/change/end carry the cumulative pinch scale. A
     browser that ALSO reports the pinch as ctrl+wheel has already been served
     by the wheel path, so the gesture only cancels the page zoom there. */
  let lastGestureScale = 1;
  const onGestureStart = (ev: Event) => {
    ev.preventDefault();
    lastGestureScale = 1;
  };
  const onGestureChange = (ev: Event) => {
    ev.preventDefault();
    const g = ev as SafariGestureEvent;
    if (!Number.isFinite(g.scale) || g.scale <= 0) return;
    const factor = g.scale / lastGestureScale;
    lastGestureScale = g.scale;
    if (ev.timeStamp - lastCtrlWheelAt < WHEEL_GESTURE_OVERLAP_MS) return;
    handlers.onZoom(factor, g.clientX, g.clientY, ev.target);
  };
  const onGestureEnd = (ev: Event) => ev.preventDefault();

  /* Touch pinch on whatever d3 does not own (the opened article). Its CSS
     says `touch-action: pan-y`, so the browser leaves the pinch to us and a
     one-finger vertical drag still scrolls the page. */
  const touches = new Map<number, {x: number; y: number}>();
  let pinchDistance = 0;
  const distance = () => {
    const [a, b] = Array.from(touches.values());
    return Math.hypot(a.x - b.x, a.y - b.y);
  };
  const onPointerDown = (ev: PointerEvent) => {
    if (ev.pointerType !== 'touch' || elsewhere(ev.target)) return;
    touches.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
    if (touches.size === 2) pinchDistance = distance();
  };
  const onPointerMove = (ev: PointerEvent) => {
    if (!touches.has(ev.pointerId)) return;
    touches.set(ev.pointerId, {x: ev.clientX, y: ev.clientY});
    if (touches.size !== 2 || pinchDistance <= 0) return;
    const d = distance();
    if (d <= 0) return;
    const [a, b] = Array.from(touches.values());
    handlers.onZoom(d / pinchDistance, (a.x + b.x) / 2, (a.y + b.y) / 2, ev.target);
    pinchDistance = d;
  };
  const onPointerEnd = (ev: PointerEvent) => {
    touches.delete(ev.pointerId);
    pinchDistance = touches.size === 2 ? distance() : 0;
  };

  // eslint-disable-next-line github/require-passive-events -- it must preventDefault the page zoom
  el.addEventListener('wheel', onWheel, {passive: false});
  el.addEventListener('wheel', noteCtrlWheel, {capture: true, passive: true});
  el.addEventListener('gesturestart', onGestureStart);
  el.addEventListener('gesturechange', onGestureChange);
  el.addEventListener('gestureend', onGestureEnd);
  el.addEventListener('pointerdown', onPointerDown);
  el.addEventListener('pointermove', onPointerMove);
  el.addEventListener('pointerup', onPointerEnd);
  el.addEventListener('pointercancel', onPointerEnd);
  return () => {
    el.removeEventListener('wheel', onWheel);
    el.removeEventListener('wheel', noteCtrlWheel, {capture: true});
    el.removeEventListener('gesturestart', onGestureStart);
    el.removeEventListener('gesturechange', onGestureChange);
    el.removeEventListener('gestureend', onGestureEnd);
    el.removeEventListener('pointerdown', onPointerDown);
    el.removeEventListener('pointermove', onPointerMove);
    el.removeEventListener('pointerup', onPointerEnd);
    el.removeEventListener('pointercancel', onPointerEnd);
  };
}
