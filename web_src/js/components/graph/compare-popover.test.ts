import {
  COMPARE_CARET_HEIGHT, COMPARE_CARET_INSET, COMPARE_CARET_WIDTH, COMPARE_POPOVER_GAP,
  COMPARE_POPOVER_WIDTH, placeComparePopover,
} from './compare-popover.ts';

/* The figma frame 641:61930, in the frame's own coordinates: the picked bubbles
   are 100px at (824,655) and (878,764); the box is 383×372. */
const figmaBubbles = [{cx: 874, cy: 705, r: 50}, {cx: 928, cy: 814, r: 50}];
const desktop = {bubbles: figmaBubbles, containerHeight: 960, viewportLeft: 0, viewportRight: 1440, boxHeight: 372};

test('beside the picked bubbles on the right, with the caret tip 24px from them, as in figma', () => {
  const layout = placeComparePopover(desktop);
  expect(layout.placement).toBe('right');
  // figma: the rightmost picked bubble ends at 978, the caret tip is at 1002 and the box starts at 1009
  expect(layout.left - COMPARE_CARET_WIDTH - COMPARE_POPOVER_GAP).toBe(978);
  expect(layout.left).toBe(1009);
});

test('the caret points at the midpoint of the picked bubbles and the box is centred on it', () => {
  const layout = placeComparePopover(desktop);
  expect(layout.top + layout.caretY).toBe((705 + 814) / 2);
  expect(layout.caretY).toBe(372 / 2);
});

test('the box stays inside the graph; the caret slides to keep pointing at the bubbles', () => {
  const layout = placeComparePopover({...desktop, bubbles: [{cx: 400, cy: 40, r: 30}, {cx: 450, cy: 60, r: 30}], containerHeight: 600});
  expect(layout.top).toBe(0);
  expect(layout.caretY).toBe(50);
  const low = placeComparePopover({...desktop, bubbles: [{cx: 400, cy: 590, r: 30}], containerHeight: 600});
  expect(low.top).toBe(600 - 372);
  // never into the rounded corner
  expect(low.caretY).toBe(372 - COMPARE_CARET_INSET - COMPARE_CARET_HEIGHT / 2);
});

test('mirrors to the left when the right side has no room', () => {
  const layout = placeComparePopover({...desktop, bubbles: [{cx: 1300, cy: 400, r: 50}]});
  expect(layout.placement).toBe('left');
  expect(layout.left + COMPARE_POPOVER_WIDTH + COMPARE_CARET_WIDTH + COMPARE_POPOVER_GAP).toBe(1250);
});

test('goes under the graph when neither side has room (a phone)', () => {
  const phone = {bubbles: [{cx: 150, cy: 300, r: 45}, {cx: 220, cy: 420, r: 17}], containerHeight: 700, viewportLeft: 0, viewportRight: 375, boxHeight: 372};
  expect(placeComparePopover(phone).placement).toBe('below');
});
