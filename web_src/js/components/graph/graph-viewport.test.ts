import {
  BUBBLE_VISIBLE_EVENT,
  DEFAULT_CONTAINER_HEIGHT,
  MAX_LAYOUT_WIDTH,
  MIN_SVG_HEIGHT,
  canvasHeightFor,
  isMeasurable,
  layoutWidthFor,
  observeContainerResize,
  registerRemeasureTriggers,
  sizeChanged,
} from './graph-viewport.ts';

test('the canvas is the measured container minus the legend', () => {
  expect(canvasHeightFor(720, 62)).toBe(658);
  expect(canvasHeightFor(546, 0)).toBe(546);
});

test('the canvas never collapses below the minimum, however short the container', () => {
  expect(canvasHeightFor(246, 62)).toBe(MIN_SVG_HEIGHT);
  expect(canvasHeightFor(1, 0)).toBe(MIN_SVG_HEIGHT);
});

test('an unmeasured container (0) falls back to the one default, not to zero', () => {
  expect(canvasHeightFor(0, 0)).toBe(DEFAULT_CONTAINER_HEIGHT);
  expect(canvasHeightFor(0, 62)).toBe(DEFAULT_CONTAINER_HEIGHT - 62);
});

test('the layout width is the measurement, capped', () => {
  expect(layoutWidthFor(880)).toBe(880);
  expect(layoutWidthFor(1440)).toBe(MAX_LAYOUT_WIDTH);
  expect(layoutWidthFor(0)).toBe(MAX_LAYOUT_WIDTH);
});

test('a container that is not rendered is not a measurement', () => {
  expect(isMeasurable({width: 0, height: 0})).toBe(false);
  expect(isMeasurable({width: 1229, height: 0})).toBe(false);
  expect(isMeasurable({width: 1229, height: 486})).toBe(true);
});

test('a real resize is a change, in either dimension', () => {
  expect(sizeChanged({width: 1229, height: 486}, {width: 1229, height: 720})).toBe(true);
  expect(sizeChanged({width: 1229, height: 486}, {width: 980, height: 486})).toBe(true);
});

/* The bug this file exists for: the previous code kept the CLAMPED width as
   the "last seen" value, so every redelivery for a container wider than the
   cap compared 1440 against 1100 and re-ran the layout (throwing away the
   user's pan/zoom) for a resize that never happened. */
test('a redelivery of the same box is not a change, above the width cap too', () => {
  expect(sizeChanged({width: 486, height: 486}, {width: 486, height: 486})).toBe(false);
  const wide = {width: 1440, height: 486};
  expect(sizeChanged(wide, {...wide})).toBe(false);
  expect(layoutWidthFor(wide.width)).toBe(MAX_LAYOUT_WIDTH);
});

test('sub-pixel jitter is not a resize', () => {
  expect(sizeChanged({width: 1229.4, height: 486.2}, {width: 1229.9, height: 486.4})).toBe(false);
  expect(sizeChanged({width: 1229, height: 486}, {width: 1229, height: 487.5})).toBe(true);
});

/* The narrower form of the same bug: the box really did change, but not in a
   way any layout input can see, so re-running the layout would only cost the
   user their pan/zoom. */
test('a width-only change above the cap is not a change the layout can see', () => {
  expect(sizeChanged({width: 1200, height: 486}, {width: 1300, height: 486})).toBe(false);
  expect(sizeChanged({width: 1000, height: 486}, {width: 1080, height: 486})).toBe(true);
  // A height change at the same widths is still a resize.
  expect(sizeChanged({width: 1200, height: 486}, {width: 1300, height: 720})).toBe(true);
});

/* The mount path used to adopt the RAW width while the observer adopted the
   clamped one, so the first frame on a wide screen was laid out differently
   from every frame after it. */
test('the first measurement is clamped exactly like every later one', () => {
  expect(layoutWidthFor(1440)).toBe(layoutWidthFor(1441));
  expect(sizeChanged({width: 0, height: 0}, {width: 1440, height: 486})).toBe(true);
  expect(sizeChanged({width: 1440, height: 486}, {width: 1441, height: 486})).toBe(false);
});

/* The wiring is where the bug lived, so it gets tests of its own: these are
   the deliveries the ResizeObserver cannot make. */
test('every re-measure trigger reaches the funnel, and cleanup removes them all', () => {
  let calls = 0;
  const doc = document;
  const stop = registerRemeasureTriggers(() => { calls++ }, window, doc);

  window.dispatchEvent(new Event('resize'));
  expect(calls).toBe(1);
  window.dispatchEvent(new Event('orientationchange'));
  expect(calls).toBe(2);
  window.dispatchEvent(new CustomEvent(BUBBLE_VISIBLE_EVENT));
  expect(calls).toBe(3);
  doc.dispatchEvent(new Event('visibilitychange'));
  expect(calls).toBe(4);

  stop();
  window.dispatchEvent(new Event('resize'));
  window.dispatchEvent(new Event('orientationchange'));
  window.dispatchEvent(new CustomEvent(BUBBLE_VISIBLE_EVENT));
  doc.dispatchEvent(new Event('visibilitychange'));
  expect(calls).toBe(4);
});

test('a tab going to the background does not re-measure; coming back does', () => {
  let calls = 0;
  const doc = {
    hidden: true,
    listeners: {} as Record<string, EventListener>,
    addEventListener(type: string, fn: EventListener) { this.listeners[type] = fn },
    removeEventListener(type: string) { delete this.listeners[type] },
  };
  const stop = registerRemeasureTriggers(() => { calls++ }, window, doc as unknown as Document);

  doc.listeners.visibilitychange(new Event('visibilitychange'));
  expect(calls).toBe(0);   // measuring a hidden tab would adopt a box nobody sees
  doc.hidden = false;
  doc.listeners.visibilitychange(new Event('visibilitychange'));
  expect(calls).toBe(1);
  stop();
});

test('the container observer is connected on registration and disconnected by its cleanup', () => {
  const observed: Element[] = [];
  let disconnects = 0;
  let deliver: (() => void) | null = null;
  const realRO = globalThis.ResizeObserver;
  const fakeRO = function (cb: () => void) {
    deliver = cb;
    return {
      observe: (el: Element) => { observed.push(el) },
      disconnect: () => { disconnects++ },
      unobserve: () => {},
    };
  };
  globalThis.ResizeObserver = fakeRO as unknown as typeof ResizeObserver;
  try {
    let calls = 0;
    const el = document.createElement('div');
    const stop = observeContainerResize(el, () => { calls++ });
    expect(observed).toEqual([el]);
    deliver?.();
    expect(calls).toBe(1);
    stop();
    expect(disconnects).toBe(1);
  } finally {
    globalThis.ResizeObserver = realRO;
  }
});
