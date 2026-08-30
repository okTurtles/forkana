import {
  DEFAULT_CONTAINER_HEIGHT,
  MAX_LAYOUT_WIDTH,
  MIN_SVG_HEIGHT,
  canvasHeightFor,
  isMeasurable,
  layoutWidthFor,
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
