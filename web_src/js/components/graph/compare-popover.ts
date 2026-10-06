/* compare-popover.ts
   Where the Compare box (ArticleComparePopup.vue) goes once two bubbles are
   picked. Pure geometry, so it can be unit-tested.

   From the figma frame "2/2 selected – ready to compare" (641:61930): the box
   (ActionMenu/Compare, 641:62019) is 383px wide and sits BESIDE the picked
   bubbles, with a 7×14 caret (641:62048) on its edge pointing back at them;
   the caret's tip is 24px from the nearest picked bubble (the rightmost picked
   bubble ends at x=978, the caret's tip is at x=1002). The box is not modal:
   the graph stays visible and usable around it.

   Placement, in order of preference:
     right  — beside the picked bubbles, on their right (as in figma);
     left   — the mirror, when the right side has no room for the box;
     below  — neither side has room (a graph as wide as the screen): the
              box is shown as the bottom sheet, as on a phone (figma "."
              641:63496, see compareBoxMode below), so it never covers the
              bubbles it describes.
   Vertically the caret points at the midpoint of the picked bubbles, and the
   box is centred on the caret as figma draws it, then kept inside the graph's
   box; when that clamp moves the box, the caret slides along its edge to keep
   pointing at the same place. */

export const COMPARE_POPOVER_WIDTH = 383;
/** The box's height as figma draws it (641:62019), until the rendered box can be measured. */
export const COMPARE_POPOVER_HEIGHT = 372;
/** The caret's size (figma Caret 641:62048): it points sideways. */
export const COMPARE_CARET_WIDTH = 7;
export const COMPARE_CARET_HEIGHT = 14;
/** From the nearest picked bubble's edge to the caret's tip. */
export const COMPARE_POPOVER_GAP = 24;
/** Breathing room the box keeps from the edges of the window. */
export const COMPARE_POPOVER_MARGIN = 16;
/** The caret never comes closer than this to the box's rounded corners. */
export const COMPARE_CARET_INSET = 16;

export type CompareCircle = {cx: number, cy: number, r: number};
export type ComparePlacement = 'right' | 'left' | 'below';
export type ComparePopoverLayout = {
  placement: ComparePlacement;
  /** Box position inside the graph's box (unused for 'below'). */
  left: number;
  top: number;
  /** Centre of the caret, from the top of the box (unused for 'below'). */
  caretY: number;
};

export type ComparePopoverInput = {
  /** The picked bubbles, in the graph box's coordinates. */
  bubbles: CompareCircle[];
  /** Height of the graph's box: the box is kept inside it. */
  containerHeight: number;
  /** The window's horizontal extent, in the graph box's coordinates. */
  viewportLeft: number;
  viewportRight: number;
  /** The rendered box's height. */
  boxHeight: number;
};

function clamp(v: number, lo: number, hi: number) {
  return hi < lo ? lo : Math.min(hi, Math.max(lo, v));
}

export function placeComparePopover(input: ComparePopoverInput): ComparePopoverLayout {
  const width = COMPARE_POPOVER_WIDTH;
  const height = input.boxHeight;
  if (!input.bubbles.length) return {placement: 'below', left: 0, top: 0, caretY: 0};

  const reach = COMPARE_POPOVER_GAP + COMPARE_CARET_WIDTH;
  const rightmost = Math.max(...input.bubbles.map((b) => b.cx + b.r));
  const leftmost = Math.min(...input.bubbles.map((b) => b.cx - b.r));

  let placement: ComparePlacement;
  let left: number;
  if (rightmost + reach + width <= input.viewportRight - COMPARE_POPOVER_MARGIN) {
    placement = 'right';
    left = rightmost + reach;
  } else if (leftmost - reach - width >= input.viewportLeft + COMPARE_POPOVER_MARGIN) {
    placement = 'left';
    left = leftmost - reach - width;
  } else {
    return {placement: 'below', left: 0, top: 0, caretY: 0};
  }

  const anchorY = input.bubbles.reduce((sum, b) => sum + b.cy, 0) / input.bubbles.length;
  const top = clamp(anchorY - height / 2, 0, input.containerHeight - height);
  const caretInset = COMPARE_CARET_INSET + COMPARE_CARET_HEIGHT / 2;
  const caretY = clamp(anchorY - top, caretInset, height - caretInset);
  return {placement, left, top, caretY};
}

/** The breakpoint below which the Compare box is a bottom sheet (the app's
   mobile breakpoint, see web_src/css/repo/header.css). */
export const COMPARE_SHEET_QUERY = '(max-width: 767.98px)';

export type CompareBoxMode = 'popover' | 'sheet' | 'none';

/** How the Compare box is shown: beside the bubbles on a desktop, as a bottom
   sheet on a phone (figma "." 641:63496) or when neither side of the bubbles
   has room for it, and not at all while it is closed. Pure, so the switch
   across the breakpoint can be tested. */
export function compareBoxMode(input: {open: boolean, narrow: boolean, placement: ComparePlacement}): CompareBoxMode {
  if (!input.open) return 'none';
  if (input.narrow || input.placement === 'below') return 'sheet';
  return 'popover';
}
