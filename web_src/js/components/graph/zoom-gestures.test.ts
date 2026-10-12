import {attachZoomGestures, clampZoom, wheelZoomFactor} from './zoom-gestures.ts';

/* The test DOM's WheelEvent drops the modifier and pointer fields, so they are
   set on the instance. */
function wheel(init: WheelEventInit): WheelEvent {
  const ev = new WheelEvent('wheel', {bubbles: true, cancelable: true, ...init});
  for (const key of ['ctrlKey', 'clientX', 'clientY', 'deltaY', 'deltaMode'] as const) {
    Object.defineProperty(ev, key, {value: init[key] ?? (key === 'ctrlKey' ? false : 0)});
  }
  return ev;
}

test('the wheel step is d3-zoom\'s, ten times faster for a pinch (ctrl)', () => {
  expect(wheelZoomFactor({deltaY: -100, deltaMode: 0, ctrlKey: false})).toBeCloseTo(2 ** 0.2);
  expect(wheelZoomFactor({deltaY: -10, deltaMode: 0, ctrlKey: true})).toBeCloseTo(2 ** 0.2);
  expect(wheelZoomFactor({deltaY: 10, deltaMode: 0, ctrlKey: true})).toBeLessThan(1);
  expect(wheelZoomFactor({deltaY: -1, deltaMode: 1, ctrlKey: false})).toBeCloseTo(2 ** 0.05);
});

test('a zoom level is kept in its range', () => {
  expect(clampZoom(3, 0.5, 2)).toBe(2);
  expect(clampZoom(0.1, 0.5, 2)).toBe(0.5);
  expect(clampZoom(1.2, 0.5, 2)).toBe(1.2);
  expect(clampZoom(Number.NaN, 0.5, 2)).toBe(0.5);
});

test('a pinch (ctrl+wheel) over the canvas zooms it and never the page', () => {
  const el = document.createElement('div');
  document.body.append(el);
  const onZoom = vi.fn();
  const detach = attachZoomGestures(el, {onZoom});
  const ev = wheel({ctrlKey: true, deltaY: -10, clientX: 5, clientY: 6});
  el.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(true);
  expect(onZoom).toHaveBeenCalledWith(expect.closeTo(2 ** 0.2), 5, 6, el);
  detach();
  el.remove();
});

test('a plain wheel is left to the page, to scroll it', () => {
  const el = document.createElement('div');
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom});
  const ev = wheel({deltaY: 40});
  el.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(false);
  expect(onZoom).not.toHaveBeenCalled();
});

/* The #422 bug: at its scale limit d3-zoom returns before preventDefault, and
   the browser zoomed the page instead. */
test('a pinch d3 ignored (at its scale limit) is still kept off the page', () => {
  const el = document.createElement('div');
  const svg = document.createElement('span');
  el.append(svg);
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom, handledElsewhere: (t) => t === svg});
  const ev = wheel({ctrlKey: true, deltaY: -10});
  svg.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(true);
  expect(onZoom).not.toHaveBeenCalled();
});

/* d3-zoom's `noevent`: preventDefault AND stopImmediatePropagation, on the <svg>. */
function d3LikeChild(parent: HTMLElement): HTMLElement {
  const svg = document.createElement('span');
  parent.append(svg);
  // eslint-disable-next-line github/require-passive-events -- models d3, which cancels the wheel
  svg.addEventListener('wheel', (e) => {
    e.preventDefault();
    e.stopImmediatePropagation();
  }, {passive: false});
  return svg;
}

test('a pinch d3 already zoomed is not zoomed twice', () => {
  const el = document.createElement('div');
  const svg = d3LikeChild(el);
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom});
  const ev = wheel({ctrlKey: true, deltaY: -10});
  svg.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(true);
  expect(onZoom).not.toHaveBeenCalled();
});

/* A Safari that reports one trackpad pinch both as ctrl+wheel (zoomed and
   stopped by d3) and as gesture events must not zoom the graph a second time. */
test('a pinch d3 zoomed as ctrl+wheel is not zoomed again by its gesture events', () => {
  const el = document.createElement('div');
  const svg = d3LikeChild(el);
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom});
  const w = wheel({ctrlKey: true, deltaY: -10});
  svg.dispatchEvent(w);
  const g = new Event('gesturechange', {bubbles: true, cancelable: true});
  Object.assign(g, {scale: 1.5, clientX: 0, clientY: 0});
  Object.defineProperty(g, 'timeStamp', {value: w.timeStamp + 10});
  svg.dispatchEvent(g);
  expect(g.defaultPrevented).toBe(true);
  expect(onZoom).not.toHaveBeenCalled();
});

test('Safari\'s trackpad pinch (gesture events) zooms by the scale step and not the page', () => {
  const el = document.createElement('div');
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom});
  const gesture = (type: string, scale: number) => {
    const ev = new Event(type, {bubbles: true, cancelable: true});
    Object.assign(ev, {scale, clientX: 1, clientY: 2});
    el.dispatchEvent(ev);
    return ev;
  };
  expect(gesture('gesturestart', 1).defaultPrevented).toBe(true);
  expect(gesture('gesturechange', 1.5).defaultPrevented).toBe(true);
  expect(gesture('gesturechange', 3).defaultPrevented).toBe(true);
  expect(onZoom.mock.calls.map((c) => c[0])).toEqual([1.5, 2]);
});

test('detaching removes every listener', () => {
  const el = document.createElement('div');
  const onZoom = vi.fn();
  attachZoomGestures(el, {onZoom})();
  const ev = wheel({ctrlKey: true, deltaY: -10});
  el.dispatchEvent(ev);
  expect(ev.defaultPrevented).toBe(false);
  expect(onZoom).not.toHaveBeenCalled();
});
