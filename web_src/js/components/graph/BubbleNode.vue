<script setup lang="ts">
/* BubbleNode.vue
   This component is responsible for rendering ONE bubble (circle + labels).
   It does NOT know about the graph; it only gets coordinates, radius and
   whether it is the EXPANDED bubble (hovered/opened). When any of those
   change it re-evaluates what text fits. This keeps label logic independent
   from layout, D3 and the zoom (labels scale with the world, #386 item 5).

   TWO RENDERINGS
   --------------
   * RESTING — one of the five ladder sizes (22/34/58/90/126, see
     bubble-size.ts). EVERY rung shows its contributor count, at the fixed size
     its rung carries (9/12/14/22px — never a size derived from the radius, so
     nothing re-sizes while a bubble grows). The count is never dropped and
     never shrunk: if it is too wide for the circle it arrives here already
     abbreviated (bubbleCountTextFor -> "1k", "12k"). The SECONDARY lines are
     what give way — they shrink, and then drop, to keep the block inside the
     arc.
   * EXPANDED — 202px, the article's card: count, excerpt and last-updated.
     It is laid out with ordinary CSS inside the <foreignObject> rather than
     through the fit model — at 202px everything it carries has room — and it
     carries NO actions: hover says what the article IS, and clicking it opens
     the article on its own (ArticleDetailView, 425px, centred), which is where
     "Read full article" and "View history" live. */

import { computed, watch, reactive } from "vue";
import { formatDateYMD } from '../../utils/time.ts';
import {
  COMPARE_OUTLINE, COMPARE_RING_WIDTH, compareBadgeCenter, compareOrderFor,
  type BubbleLabelDetail, type CompareBadgeStyle, type ComparePickState,
} from './bubble-size.ts';

/* ──────────────────────────────────────────────────────────────────────────────
   LABEL LAYOUT CONSTANTS (all values explained to avoid "magic numbers")
   ─────────────────────────────────────────────────────────────────────────── */

/* === FONT SIZING ===
   The COUNT's size is not here: it comes from the rung (bubble-size.ts) and is
   passed in, fixed. Only the secondary lines are sized here, and only they are
   allowed to shrink — by a single uniform scale, so the block keeps its
   proportions — to keep a line that would otherwise not fit. */
const SECONDARY_SCALE_MIN = 0.5;     // Secondary lines may shrink to half...
const FONT_SIZE_FLOOR = 8;           // ...but never below this, in WORLD px (8px on screen at zoom 1)
const FONT_SIZE_LABEL = 12;          // Base font size for the "Contributor(s)" label
const FONT_SIZE_SMALL = 11;          // Base font size for the "Last updated" lines

/* === LABEL SPACING === */
/* Breathing room between the arc and the text, in WORLD px (see the fit-model
   note above: on-screen size = this × zoom). Proportional to the bubble
   rather than constant: 12px is right on a large circle but eats a quarter of
   a small one's diameter, which is what used to stop mid-sized bubbles from
   showing their "Contributors" line at all. */
const LABEL_PADDING_RATIO = 0.12;    // fraction of the bubble's radius
const LABEL_PADDING_MAX = 12;        // ...capped, so big bubbles are not hollow
const LABEL_PADDING_MIN = 4;         // ...and floored, so small ones still breathe
const LABEL_GAP_PRIMARY = 6;         // Gap between count and contributor label
const LABEL_GAP_SECONDARY = 6;       // Gap between contributor label and updated block
const LABEL_GAP_UPDATED_INNER = 6;   // Gap between two lines of updated text

/* === TEXT WIDTH ESTIMATION (for fit calculations) ===
   Deliberately generous. These decide whether a line is shown at all, and the
   font they will be rendered in is not the font this code can measure: production
   loads Inter, a fallback stack is used elsewhere, and digits (in a date) are
   wider than lowercase in most faces. Over-estimating means a line is dropped a
   little earlier than strictly necessary; under-estimating means it is drawn
   overflowing the circle, which is the failure we are avoiding. */
const CHAR_WIDTH_RATIO_LABEL = 0.62; // width of a label char as a ratio of font size (measured 0.594)
const CHAR_WIDTH_RATIO_SMALL = 0.62; // ...and of small text, which is mostly digits
const CHAR_WIDTH_RATIO_COUNT = 0.7;  // digits are wider: measured 0.696 (see bubble-size.ts)

const props = defineProps<{
  id: string;
  x: number; y: number;          // world coordinates (graph space)
  r: number;                      // bubble radius (graph units)
  contributors: number;           // primary number (always shown)
  /* The count as it is to be WRITTEN — already abbreviated if this rung is too
     small to spell it out (bubble-size.ts), so this component never has to
     decide between shrinking the count and hiding it. */
  countText: string;
  /* ...and the size to write it at: this rung's rung-fixed 9/12/14/22px. */
  countFontSize: number;
  updatedAt?: string;             // secondary line if visible
  description?: string;           // article excerpt, expanded state only
  /* What this bubble's RUNG is meant to say (bubble-size.ts). A CEILING: a
     bubble never says more than its rung asks for, and says less (or shrinks
     its type) rather than spill a line outside the circle. */
  detail?: BubbleLabelDetail;
  /* True for the hovered/focused/first-tapped bubble: 202px, whole card. */
  expanded?: boolean;
  /* True while this bubble's RADIUS is being animated. The label is hidden for
     the duration and not recomputed, so no text is ever caught mid-scale: it
     fades out, the circle moves, and the final text fades back in. */
  frozen?: boolean;
  isActive?: boolean;             // selected article (persisted selection)
  isCompareMode?: boolean;        // whether compare mode is active
  compareState?: ComparePickState;  // compare selection state
  /* The order badge of this bubble's rung in Compare mode (bubble-size.ts). */
  compareBadgeStyle?: CompareBadgeStyle;
  /* The author deleted this article. The bubble is kept — the forks below it
     need the ancestry — and stays interactive; it is drawn muted and dashed
     and says so in the expanded card. */
  isTombstoned?: boolean;
  /* the subject root the reader may not see: drawn like a tombstone for its forks'
     sake, but no article, so neither a control nor anything a screen reader names */
  isHidden?: boolean;
}>();

/* Emits so the parent can wire up interactions without D3 binding. The parent
   owns the hover/active state because growing one bubble re-runs the whole
   layout — see FishboneGraph.setHovered(). */
const emit = defineEmits<{
  (e: "click", id: string, ev: MouseEvent): void;
  (e: "hover", id: string, on: boolean, pointerType: string): void;
}>();

/* Label fit model in WORLD pixels (== screen px at zoom 1). The label group
   used to be inverse-scaled by 1/k so its type held a constant screen size
   across zoom, but #386 item 5 wants the content to grow and shrink WITH the
   bubble — so the group lives in world units like the circle, and the zoom
   scales both together. The fit is computed once, against the bubble's own
   radius, and is therefore zoom-independent. */
const fit = reactive({
  showLabel: false,
  showUpdated: false,
  // secondary font sizes in WORLD px; the count's is props.countFontSize
  fsLabel: FONT_SIZE_LABEL,
  fsSmall: FONT_SIZE_SMALL,
});

/* Label text: simple helper function for pluralization (not computed to avoid unnecessary reactivity) */
const getLabelText = (count: number) => count === 1 ? "Contributor" : "Contributors";

/* Format date to yyyy-mm-dd using shared utility */
const formattedDate = computed(() => formatDateYMD(props.updatedAt));

/* Recompute which SECONDARY lines a bubble can carry, and at what size.
   The count is not part of this decision: it is always drawn, at its rung's
   size, in the string it was given. */
function recomputeFit() {
  const r = props.r;
  /* Everything here is in WORLD px; the zoom scales the rendered result, so
     what fits at zoom 1 fits at every zoom. */
  const screenR = r;
  const pad = Math.max(LABEL_PADDING_MIN, Math.min(LABEL_PADDING_MAX, screenR * LABEL_PADDING_RATIO));
  const inner = Math.max(0, screenR - pad);             // usable radius for the block

  const hasUpd = !!props.updatedAt;
  const labelTextStr = getLabelText(props.contributors);
  const fsCount = props.countFontSize;
  const countW = props.countText.length * fsCount * CHAR_WIDTH_RATIO_COUNT;
  const updLine1 = "Last updated";
  const updLine2 = formattedDate.value;

  /* Does a block of text of this size fit INSIDE the circle? Corners, not a
     bounding square: that is what actually has to clear the arc, and it is
     what the visual regression harness measures. */
  const fitsInCircle = (w: number, h: number) => (w * w + h * h) <= 4 * inner * inner;

  /* Geometry of one candidate rendering, with the secondary lines at scale `s`.
     level 1 = count + label, 2 = count + label + updated block. The count line
     is a constant in all of them — that is the point. */
  const measure = (level: number, s: number) => {
    const fsLabel = FONT_SIZE_LABEL * s;
    const fsSmall = FONT_SIZE_SMALL * s;
    let w = Math.max(countW, labelTextStr.length * fsLabel * CHAR_WIDTH_RATIO_LABEL);
    let h = fsCount + LABEL_GAP_PRIMARY + fsLabel;
    let smallest = fsLabel;
    if (level >= 2) {
      w = Math.max(w,
        updLine1.length * fsSmall * CHAR_WIDTH_RATIO_SMALL,
        updLine2.length * fsSmall * CHAR_WIDTH_RATIO_SMALL);
      h += LABEL_GAP_SECONDARY + (2 * fsSmall + LABEL_GAP_UPDATED_INNER);
      smallest = fsSmall;
    }
    return {w, h, fsLabel, fsSmall, smallest};
  };

  /* The rung's CEILING for the secondary lines, capped by what there is to
     show at all. */
  const detail = props.detail ?? 'count';
  const ceiling = Math.min(detail === 'full' ? 2 : detail === 'label' ? 1 : 0, hasUpd ? 2 : 1);

  /* The most detailed rendering the rung allows THAT FITS. The secondary lines
     may shrink (to SECONDARY_SCALE_MIN, never past FONT_SIZE_FLOOR) to keep a
     level; failing that the level is dropped. Dropping every level leaves the
     count alone, which is always drawn. */
  let chosen: ReturnType<typeof measure> | null = null;
  let level = 0;
  outer:
  for (let lv = ceiling; lv >= 1; lv--) {
    for (let sc = 1; sc >= SECONDARY_SCALE_MIN - 1e-9; sc -= 0.02) {
      const cand = measure(lv, sc);
      if (cand.smallest >= FONT_SIZE_FLOOR && fitsInCircle(cand.w, cand.h)) {
        chosen = cand; level = lv; break outer;
      }
    }
  }

  fit.showLabel = level >= 1;
  fit.showUpdated = level >= 2;
  fit.fsLabel = chosen?.fsLabel ?? FONT_SIZE_LABEL;
  fit.fsSmall = chosen?.fsSmall ?? FONT_SIZE_SMALL;
}

/* Run once and whenever driving props change — EXCEPT while `frozen`, when the
   bubble's radius is mid-animation. Skipping the recompute is what stops the
   type being interpolated: the label keeps the size it had (hidden, so nobody
   sees it), and the first recompute after the freeze lifts uses the settled
   radius, at which point it fades back in. */
watch(
  () => [props.r, props.updatedAt, props.contributors, props.countText,
    props.countFontSize, props.detail, props.expanded, props.frozen],
  () => { if (!props.frozen) recomputeFit(); },
  {immediate: true},
);

/* Convenience computed transform strings */
const gTransform = computed(() => `translate(${props.x},${props.y})`);

/* Picked for either slot of a comparison — the states that thicken and colour
   the ring. */
const compareSelected = computed(() => props.compareState === 'first' || props.compareState === 'second');
/* #405 item 6: in compare mode every bubble not (yet) picked is outlined with a
   thin dark dashed line, and a picked one gets a solid indigo ring and an
   order badge instead — as the figma draws them (values and node ids in
   bubble-size.ts). Both are unfilled in figma, over the white page; here the
   disc takes the page colour, which reads the same and still hides the
   connectors behind it, and drops the resting gradient and shadow. */
/* A deleted article keeps its own look (faded, grey 4/4 dashes) in compare mode
   until it is picked: the figma's compare frames only draw live articles, and the
   compare dashes would make it indistinguishable from them. */
const compareOutlined = computed(() => props.isCompareMode === true && (compareSelected.value || props.isTombstoned !== true));
const compareOrder = computed(() => props.isCompareMode === true ? compareOrderFor(props.compareState) : null);
const compareBadge = computed(() => compareBadgeCenter(props.r));

/* What a screen reader hears. In compare mode the order badge (aria-hidden: it is
   drawn) is said in words, and aria-pressed follows the pick, not the page's
   selection. */
const ariaLabel = computed(() => {
  const count = `${props.contributors} contributor${props.contributors === 1 ? '' : 's'}`;
  const base = props.isTombstoned ?
    `Repository node with ${count}, deleted by its author` :
    `Repository node with ${count}${props.updatedAt ? `, last updated ${props.updatedAt}` : ''}`;
  const pick = compareOrder.value !== null ? `, selected for comparison (${compareOrder.value} of 2)` : '';
  return `${base}${pick}. Press Enter to select.`;
});

/* The compare stroke's width, or 0 where there is no compare outline. */
const compareStrokeWidth = computed(() => {
  if (!compareOutlined.value) return 0;
  return compareSelected.value ? COMPARE_RING_WIDTH : COMPARE_OUTLINE.width;
});

/* Figma draws both strokes INSIDE the bubble, so the circle is inset by half
   a stroke: the stroke's outer edge is then the bubble's edge, where the
   connectors end. */
const circleRadius = computed(() => Math.max(0, props.r - compareStrokeWidth.value / 2));

/* The circle's paint in compare mode. An inline style rather than the
   presentation attributes below, so the focus/hover rules in the stylesheet
   (a 1px primary ring) cannot thin the compare ring of the bubble that was
   just clicked and therefore holds the focus. Null outside compare mode,
   where nothing changes. The compare paint intentionally replaces the selection
   stroke too: figma's compare frames show only the picked rings, and the mini
   circle (.joint-parent.is-selected) still marks the selected article. */
const compareCircleStyle = computed(() => {
  if (!compareOutlined.value) return null;
  if (compareSelected.value) {
    return {
      fill: 'var(--bubble-compare-fill)',
      stroke: 'var(--bubble-compare-selected)',
      strokeWidth: `${COMPARE_RING_WIDTH}px`,
      strokeDasharray: 'none',
    };
  }
  return {
    fill: 'var(--bubble-compare-fill)',
    stroke: 'var(--bubble-compare-outline)',
    strokeWidth: `${COMPARE_OUTLINE.width}px`,
    strokeDasharray: `${COMPARE_OUTLINE.dash} ${COMPARE_OUTLINE.gap}`,
  };
});

/* Pointer handlers relay events upward (so the parent can grow this bubble and
   reflow the graph around it). `pointerType` travels with the event because
   touch has no hover: the parent turns the FIRST tap into a hover and the
   second into a click. */
function onClick(ev: MouseEvent) { emit("click", props.id, ev); }
/* pointerdown is what tells a tap from a click: a `click` event carries no
   pointerType, and on a touch device the enter/leave pair below may not fire
   at all. */
function onPointerDown(ev: PointerEvent) { emit("hover", props.id, true, ev.pointerType || 'mouse'); }
function onPointerEnter(ev: PointerEvent) { emit("hover", props.id, true, ev.pointerType || 'mouse'); }
function onPointerLeave(ev: PointerEvent) { emit("hover", props.id, false, ev.pointerType || 'mouse'); }
/* Keyboard: focus behaves like hover, blur dismisses it.
   focusIN/focusOUT rather than focus/blur — the bubbling pair. It is the one
   that reaches this handler for a real (trusted) focus on an SVG <g>, and it
   is also the right semantics once the bubble is open: moving focus onto one
   of its two buttons is still "inside this bubble", and only leaving the group
   entirely dismisses it. */
function onFocusIn() { emit("hover", props.id, true, 'keyboard'); }
function onFocusOut(ev: FocusEvent) {
  const next = ev.relatedTarget as Node | null;
  if (next && (ev.currentTarget as Element).contains(next)) return;   // still inside this bubble
  emit("hover", props.id, false, 'keyboard');
}
function onKeyDown(ev: KeyboardEvent) {
  if (ev.key === 'Enter' || ev.key === ' ') {
    ev.preventDefault();
    emit("click", props.id, ev as unknown as MouseEvent);
  }
}
</script>

<template>
  <!-- One node group at (x,y); we let the parent group receive the world transform -->
  <g
    class="node cursor-pointer select-none"
    :class="{ 'is-expanded': expanded, 'is-frozen': frozen, 'is-tombstoned': isTombstoned }"
    :transform="gTransform" :data-node-id="id" :role="isHidden ? undefined : 'button'"
    :aria-label="isHidden ? undefined : ariaLabel" :aria-hidden="isHidden ? 'true' : undefined"
    :aria-pressed="isHidden ? undefined : (isCompareMode === true ? compareSelected : isActive) ? 'true' : 'false'"
    :tabindex="isHidden ? -1 : 0" @click="onClick" @keydown="onKeyDown"
    @pointerdown="onPointerDown" @pointerenter="onPointerEnter" @pointerleave="onPointerLeave"
    @focusin="onFocusIn" @focusout="onFocusOut"
  >
    <!-- Bubble circle with soft gradient & subtle stroke/shadow -->
    <circle
      class="node-circle" :r="circleRadius" fill="url(#bubbleGrad)"
      :stroke="isActive || expanded ? 'var(--color-primary)' : 'none'"
      stroke-width="1"
      :stroke-dasharray="props.isTombstoned ? '4,4' : 'none'"
      :filter="compareCircleStyle ? undefined : 'url(#softShadow)'"
      :style="compareCircleStyle ?? undefined"
    />

    <!-- HTML Labels: using foreignObject for efficient text rendering -->
    <!-- Calculate the size needed for the foreignObject container -->
    <foreignObject
      :x="-r" :y="-r" :width="r * 2" :height="r * 2"
      style="overflow: visible; pointer-events: none;"
    >
      <!-- EXPANDED (202px): the whole card, laid out by CSS. -->
      <div v-if="expanded" xmlns="http://www.w3.org/1999/xhtml" class="html-label-wrapper expanded-wrapper">
        <!-- #386 item 10: the same stack as the resting bubble — count above
             its label — grown with the bubble, so nothing shifts onto another
             line and no text gets SMALLER as the bubble gets bigger. -->
        <div class="expanded-count-number">{{ contributors }}</div>
        <div class="expanded-count-label">{{ getLabelText(contributors) }}</div>
        <!-- The excerpt is the article's content and a tombstone has none left
             to show; the card says what happened to it instead. -->
        <div v-if="isTombstoned" class="expanded-deleted">Deleted by its author</div>
        <div v-else-if="description" class="expanded-description">{{ description }}</div>
        <div v-if="updatedAt" class="expanded-updated">
          <div>Last updated</div>
          <div>{{ formattedDate }}</div>
        </div>
      </div>

      <!-- RESTING: the count, always, at its rung's size; then whatever
           secondary lines this rung asks for AND the arc has room for. -->
      <div v-else xmlns="http://www.w3.org/1999/xhtml" class="html-label-wrapper">
        <div class="count" :style="`font-size: ${countFontSize}px;`">{{ countText }}</div>

        <!-- "Contributors/Contributor": only if it fits -->
        <div
          v-if="fit.showLabel" class="label"
          :style="`font-size: ${fit.fsLabel}px; margin-top: ${LABEL_GAP_PRIMARY}px;`"
        >
          {{ getLabelText(contributors) }}
        </div>

        <!-- "Last updated …": only if it fits -->
        <div
          v-if="fit.showUpdated" class="updated"
          :style="`font-size: ${fit.fsSmall}px; margin-top: ${LABEL_GAP_SECONDARY}px;`"
        >
          <div>Last updated</div>
          <div :style="`margin-top: ${LABEL_GAP_UPDATED_INNER}px;`">{{ formattedDate }}</div>
        </div>
      </div>
    </foreignObject>

    <!-- Compare mode: the picked bubble's place in the comparison, on its ring
         and over everything else in the bubble. -->
    <g
      v-if="compareOrder !== null && compareBadgeStyle" class="compare-badge" aria-hidden="true"
      :transform="`translate(${compareBadge.x},${compareBadge.y})`"
    >
      <circle :r="compareBadgeStyle.diameter / 2" fill="var(--bubble-compare-selected)"/>
      <text
        text-anchor="middle" dominant-baseline="central" fill="var(--bubble-compare-badge-text)"
        :font-size="compareBadgeStyle.fontSize" font-weight="600"
      >
        {{ compareOrder }}
      </text>
    </g>
  </g>
</template>

<style scoped>
.node-circle {
  transition: stroke 0.2s ease, stroke-width 0.2s ease;
}

.node:focus {
  outline: none;
}

/* #386 item 8: resting bubbles carry NO border (the figma draws bare circles);
   only keyboard focus outlines one, as its visible focus indicator. */
.node:focus .node-circle {
  stroke: var(--color-primary);
  stroke-width: 1;
}

/* The expanded bubble paints over its neighbours' connectors, and its two
   buttons have to be clickable even though the label layer is otherwise
   inert. */
.node.is-expanded {
  cursor: default;
}

/* ── TOMBSTONE ───────────────────────────────────────────────────────────
   A deleted article keeps its place in the graph so its forks keep their
   ancestry, and it stays selectable like any other bubble; it is only drawn
   faded with a dashed outline (the dash pattern itself is on the circle). In
   compare mode it keeps this look until it is picked (compareOutlined). The
   stroke is set here rather than in the binding so it also wins over the
   hover/focus rules below. */
.node.is-tombstoned {
  opacity: 0.55;
}

.node.is-tombstoned .node-circle,
.node.is-tombstoned .node-circle:hover,
.node.is-tombstoned:focus .node-circle {
  stroke: var(--color-text-light-3, #9ca3af);
  stroke-width: 1;
}

/* ── LABEL OPACITY IS DECOUPLED FROM THE GEOMETRY ────────────────────────
   A bubble's text used to be re-laid-out on every frame of the hover reflow,
   so it grew and shrank with the circle — "a bit junky". It no longer moves at
   all: while the radius is animating the group is `is-frozen`, the label is
   hidden (fast, 80ms) and NOT recomputed, and the settled text fades back in
   (120ms) once the circle has stopped. So at any frame the type is either
   invisible or at a final size — never at an interpolated one. The count's
   size is a rung constant (bubble-size.ts), so it never interpolates even
   when it is visible. */
.html-label-wrapper {
  opacity: 1;
  transition: opacity 120ms linear;
}

.node.is-frozen .html-label-wrapper {
  opacity: 0;
  transition: opacity 80ms linear;
}

@media (prefers-reduced-motion: reduce) {
  .html-label-wrapper,
  .node.is-frozen .html-label-wrapper {
    transition: none;
  }
}

/* HTML Label Wrapper - efficient text rendering */
.html-label-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  pointer-events: none;
  height: 100%;
  width: 100%;
  /* Never wrap a label. The <foreignObject> this sits in is 2r WORLD units
     wide and scales with the zoom like the circle, but a label can still be
     legitimately wider than the box (recomputeFit measures against the
     circle's CORNERS, not its bounding square). The lines are centred, so a
     nowrap line simply overflows the box symmetrically; whether there is room
     for it in the CIRCLE is decided by recomputeFit(), which measures
     single-line widths. Inherited by every label line. */
  white-space: nowrap;
}

/* #421 item 4: the bubble's text as figma's Bubble view draws it (641:61415,
   e.g. the 538 bubble 641:61440-61442), in every mode: the count and
   "Contributors" in the primary text colour (#1f2328) at weight 600, the
   date in the same colour at 400 italic. The compare frames use the very same
   styles (6484:44242-44243), so compare mode needs no override. */

/* Count: always visible, bold and prominent */
.html-label-wrapper .count {
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1;
  pointer-events: none;
}

/* Label text: "Contributor(s)" */
.html-label-wrapper .label {
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1;
  pointer-events: none;
}

/* Updated date information */
.html-label-wrapper .updated {
  color: var(--color-text-primary);
  font-weight: 400;
  font-style: italic;
  line-height: 1;
  pointer-events: none;
}

/* The order badge sits over the bubble's edge; a click on it is a click on the
   bubble (to deselect it), so it must not catch the pointer itself. */
.compare-badge {
  pointer-events: none;
}

/* ── EXPANDED CARD (202px bubble) ────────────────────────────────────────
   Everything lives inside the circle's INSCRIBED SQUARE (202 / sqrt(2) ≈
   143px), so no line can reach the arc: the stack is 60% of the diameter
   wide (121px) and its lines are short enough that the worst corner is
   sqrt(60.5² + 72²) ≈ 94px from the centre, inside r = 101. The sizes below
   are what keeps that true with the buttons present — the harness measures
   every line's far corner against the radius (matrix.py labelOverflow). */
.expanded-wrapper {
  width: 60%;
  margin: 0 auto;
  gap: 6px;
  /* The excerpt is a paragraph and MUST wrap; the resting rule above must not
     leak into it. */
  white-space: normal;
}

/* #386 item 10: the count keeps the resting stack (number over label) and
   GROWS with the bubble instead of collapsing onto one 14px line. */
.expanded-wrapper .expanded-count-number {
  font-size: 22px;
  font-weight: 600;
  line-height: 1.1;
  color: var(--color-text-primary);
  white-space: nowrap;
}

/* The hovered bubble's text, as figma's "Bubble view - click + zoom" 335
   bubble (641:61744-61746): count 22px, "Contributors" 12px, both 600; the
   date 400 italic; all in the primary text colour, like the resting bubble. */
.expanded-wrapper .expanded-count-label {
  font-size: 12px;
  font-weight: 600;
  line-height: 1.1;
  color: var(--color-text-primary);
  white-space: nowrap;
}

.expanded-description {
  font-size: 10px;
  line-height: 1.35;
  color: var(--color-text-primary);
  /* Three lines: what the circle's height budget allows next to the count and
     the date. The whole excerpt is in the opened (425px) view. */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
}

.expanded-deleted {
  font-size: 10px;
  font-weight: 600;
  line-height: 1.35;
  color: var(--color-text-light-2, #6b7280);
}

.expanded-updated {
  /* #386 item 10: never SMALLER than the resting bubble's 11px "Last updated"
     lines — text must not shrink while the bubble it sits in grows. */
  font-size: 11px;
  font-style: italic;
  line-height: 1.3;
  color: var(--color-text-primary);
  white-space: nowrap;
}
</style>
