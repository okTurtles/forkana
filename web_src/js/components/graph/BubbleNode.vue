<script setup lang="ts">
/* BubbleNode.vue
   This component is responsible for rendering ONE bubble (circle + labels).
   It does NOT know about the graph; it only gets coordinates, radius, and a
   zoom factor (k). When k or size changes, it re-evaluates what text fits.
   This keeps label logic independent from layout and D3. */

import { computed, watch, reactive } from "vue";
import { formatDateYMD } from '../../utils/time.ts';
import type { BubbleLabelDetail } from './bubble-size.ts';

/* ──────────────────────────────────────────────────────────────────────────────
   LABEL LAYOUT CONSTANTS (all values explained to avoid "magic numbers")
   ─────────────────────────────────────────────────────────────────────────── */

/* === FONT SIZING === */
const FONT_SIZE_COUNT_MIN = 10;      // Minimum font size for contributor count
const FONT_SCALE_MIN = 0.62;         // Type may shrink to this to honour the tier's detail
const FONT_SIZE_FLOOR = 8;           // ...but never below this, in screen px: it stops being legible
const FONT_SIZE_COUNT_MAX = 34;      // Maximum font size for contributor count
const FONT_SIZE_COUNT_SCALE = 0.95;  // Scale factor: count font size = on-screen radius * scale
const FONT_SIZE_LABEL = 12;          // Fixed font size for "Contributor(s)" label
const FONT_SIZE_SMALL = 11;          // Font size for "Last updated" lines
const FONT_SIZE_COMBINED = 22;       // Font size for combined count + label (1.375rem)

/* === LABEL SPACING === */
/* Breathing room between the arc and the text, in SCREEN px. Proportional to
   the bubble rather than constant: 12px is right on a large circle but eats a
   quarter of a small one's diameter, which is what used to stop mid-sized
   bubbles from showing their "Contributors" line at all. */
const LABEL_PADDING_RATIO = 0.12;    // fraction of the on-screen radius
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
const CHAR_WIDTH_RATIO_LABEL = 0.62; // width of a label char as a ratio of font size
const CHAR_WIDTH_RATIO_SMALL = 0.62; // ...and of small text, which is mostly digits

const props = defineProps<{
  id: string;
  x: number; y: number;          // world coordinates (graph space)
  r: number;                      // bubble radius (graph units)
  k: number;                      // current zoom scale (world→screen)
  contributors: number;           // primary number (always shown)
  updatedAt?: string;             // secondary line if visible
  /* What this bubble's SIZE TIER is meant to say (bubble-size.ts). A floor,
     not a ceiling: a bubble drawn large enough shows more than its tier asks
     for, and one drawn small shrinks its type to honour it. */
  detail?: BubbleLabelDetail;
  isActive?: boolean;
  isCompareMode?: boolean;        // whether compare mode is active
  compareState?: 'none' | 'first' | 'second';  // compare selection state
}>();

/* Emits so the parent can wire up interactions without D3 binding. Opening the
   article is no longer a per-bubble affordance: clicking a bubble opens the
   detail view (ArticleDetailView.vue), and "Read full article" lives there. */
const emit = defineEmits<{
  (e: "click", id: string, ev: MouseEvent): void;
}>();

/* Label fit model in *screen pixels* so it looks consistent across zoom.
   We inverse-scale the label group by 1/k. */
const fit = reactive({
  showLabel: false,
  showUpdated: false,
  showCombined: false,  // True if enough space for count + label on same line
  // vertical offset to keep the whole label block visually centered
  shiftPx: 0,
  // font sizes in px (on screen)
  fsCount: FONT_SIZE_LABEL,
  fsLabel: FONT_SIZE_LABEL,
  fsSmall: FONT_SIZE_SMALL,
  fsCombined: FONT_SIZE_COMBINED,
  stackPx: 0,
});

/* Label text: simple helper function for pluralization (not computed to avoid unnecessary reactivity) */
const getLabelText = (count: number) => count === 1 ? "Contributor" : "Contributors";

/* Format date to yyyy-mm-dd using shared utility */
const formattedDate = computed(() => formatDateYMD(props.updatedAt));

/* Recompute label visibility whenever r or k or updatedAt change. */
function recomputeFit() {
  const k = props.k, r = props.r;
  /* Everything here is in SCREEN px: the label group is inverse-scaled by 1/k,
     so its type is drawn at a constant size whatever the zoom. */
  const screenR = r * k;
  const pad = Math.max(LABEL_PADDING_MIN, Math.min(LABEL_PADDING_MAX, screenR * LABEL_PADDING_RATIO));
  const inner = Math.max(0, screenR - pad);             // usable radius for text

  const hasUpd = !!props.updatedAt;
  const labelTextStr = getLabelText(props.contributors);
  const countStr = String(props.contributors);
  const updLine1 = "Last updated";
  const updLine2 = formattedDate.value;

  /* Does a block of text of this size fit INSIDE the circle? Corners, not a
     bounding square: that is what actually has to clear the arc, and it is
     what the visual regression harness measures. */
  const fitsInCircle = (w: number, h: number) => (w * w + h * h) <= 4 * inner * inner;

  /* Geometry of one candidate rendering, at a uniform type scale `s`.
     level 0 = count, 1 = count + label, 2 = count + label + updated block. */
  const measure = (level: number, s: number, combined: boolean) => {
    const fsCount = Math.min(FONT_SIZE_COUNT_MAX, Math.max(FONT_SIZE_COUNT_MIN, screenR * FONT_SIZE_COUNT_SCALE)) * s;
    const fsLabel = FONT_SIZE_LABEL * s;
    const fsSmall = FONT_SIZE_SMALL * s;
    const fsCombined = FONT_SIZE_COMBINED * s;

    let w: number, h: number;
    if (level >= 1 && combined) {
      w = (countStr.length + 1 + labelTextStr.length) * fsCombined * CHAR_WIDTH_RATIO_LABEL;
      h = fsCombined;
    } else {
      w = countStr.length * fsCount * CHAR_WIDTH_RATIO_LABEL;
      h = fsCount;
      if (level >= 1) {
        w = Math.max(w, labelTextStr.length * fsLabel * CHAR_WIDTH_RATIO_LABEL);
        h += LABEL_GAP_PRIMARY + fsLabel;
      }
    }
    if (level >= 2) {
      w = Math.max(w,
        updLine1.length * fsSmall * CHAR_WIDTH_RATIO_SMALL,
        updLine2.length * fsSmall * CHAR_WIDTH_RATIO_SMALL);
      h += (level >= 1 ? LABEL_GAP_SECONDARY : LABEL_GAP_PRIMARY) + (2 * fsSmall + LABEL_GAP_UPDATED_INNER);
    }
    return {w, h, fsCount, fsLabel, fsSmall, fsCombined, smallest: level >= 2 ? fsSmall : (level >= 1 && !combined ? fsLabel : fsCount)};
  };

  /* The tier's floor, capped by what there is anything to show at all. */
  const detail = props.detail ?? 'count';
  const floor = Math.min(detail === 'full' ? 2 : detail === 'label' ? 1 : 0, hasUpd ? 2 : 1);

  /* Best rendering: the most detailed level that fits at full size, never less
     than the tier floor. To honour the floor the type is allowed to shrink,
     down to FONT_SCALE_MIN and never past FONT_SIZE_FLOOR — if even that will
     not fit, detail is dropped rather than spilled outside the circle. */
  let chosen = measure(0, 1, false);
  let level = 0, combined = false;
  for (let lv = hasUpd ? 2 : 1; lv >= 0; lv--) {
    const asCombined = lv >= 1 ? measure(lv, 1, true) : null;
    const asStacked = measure(lv, 1, false);
    if (asCombined && fitsInCircle(asCombined.w, asCombined.h)) { chosen = asCombined; level = lv; combined = true; break; }
    if (fitsInCircle(asStacked.w, asStacked.h)) { chosen = asStacked; level = lv; combined = false; break; }
  }
  if (level < floor) {
    for (let s = 1; s >= FONT_SCALE_MIN - 1e-9; s -= 0.02) {
      const cand = measure(floor, s, false);
      if (cand.smallest < FONT_SIZE_FLOOR) break;
      if (fitsInCircle(cand.w, cand.h)) { chosen = cand; level = floor; combined = false; break; }
    }
  }

  fit.showCombined = level >= 1 && combined;
  fit.showLabel = level >= 1 && !combined;
  fit.showUpdated = level >= 2;
  fit.fsCount = chosen.fsCount;
  fit.fsLabel = chosen.fsLabel;
  fit.fsSmall = chosen.fsSmall;
  fit.fsCombined = chosen.fsCombined;
  fit.stackPx = chosen.h;
  fit.shiftPx = 0;
}

/* Run once and whenever driving props change. */
watch(() => [props.k, props.r, props.updatedAt, props.contributors, props.detail], recomputeFit, { immediate: true });

/* Convenience computed transform strings */
const gTransform = computed(() => `translate(${props.x},${props.y})`);

/* Pointer handlers relay events upward (so parent can focus). */
function onClick(ev: MouseEvent) { emit("click", props.id, ev); }
/* Keyboard navigation support */
function onKeyDown(ev: KeyboardEvent) {
  if (ev.key === 'Enter' || ev.key === ' ') {
    ev.preventDefault();
    emit("click", props.id, ev as any);
  }
}
</script>

<template>
  <!-- One node group at (x,y); we let the parent group receive the world transform -->
  <g
    class="node cursor-pointer select-none" :transform="gTransform" role="button"
    :aria-label="`Repository node with ${contributors} contributor${contributors === 1 ? '' : 's'}${updatedAt ? ', last updated ' + updatedAt : ''}. Press Enter to select.`"
    :aria-pressed="isActive ? 'true' : 'false'" tabindex="0" @click="onClick" @keydown="onKeyDown"
  >
    <!-- Bubble circle with soft gradient & subtle stroke/shadow -->
    <circle
      class="node-circle" :class="{
        'compare-dashed': props.isCompareMode && props.compareState === 'none',
        'compare-selected-first': props.compareState === 'first',
        'compare-selected-second': props.compareState === 'second'
      }" :r="r" fill="url(#bubbleGrad)"
      :stroke="props.compareState === 'first' || props.compareState === 'second' ? 'var(--color-primary)' : isActive ? 'var(--color-primary)' : 'var(--bubble-stroke)'"
      :stroke-width="props.compareState === 'first' || props.compareState === 'second' ? 3 : 1"
      :stroke-dasharray="props.isCompareMode && props.compareState === 'none' ? '8,4' : 'none'"
      filter="url(#softShadow)"
    />

    <!-- HTML Labels: using foreignObject for efficient text rendering -->
    <!-- Calculate the size needed for the foreignObject container -->
    <foreignObject
      :x="-r" :y="-r" :width="r * 2" :height="r * 2" :transform="`scale(${1 / k})`"
      style="overflow: visible; pointer-events: none;"
    >
      <div xmlns="http://www.w3.org/1999/xhtml" class="html-label-wrapper">
        <!-- Combined layout: count and label on same line with larger font -->
        <div v-if="fit.showCombined" class="combined" :style="`font-size: ${fit.fsCombined}px;`">
          {{ contributors }} {{ getLabelText(contributors) }}
        </div>

        <!-- Stacked layout: count and label on separate lines (fallback) -->
        <template v-else>
          <!-- Count is ALWAYS visible and centered -->
          <div class="count" :style="`font-size: ${fit.fsCount}px;`">
            {{ contributors }}
          </div>

          <!-- "Contributors/Contributor": only if fits -->
          <div
            v-if="fit.showLabel" class="label"
            :style="`font-size: ${fit.fsLabel}px; margin-top: ${LABEL_GAP_PRIMARY}px;`"
          >
            {{ getLabelText(contributors) }}
          </div>
        </template>

        <!-- "Last updated …": only if fits -->
        <div
          v-if="fit.showUpdated" class="updated"
          :style="`font-size: ${fit.fsSmall}px; margin-top: ${fit.showCombined || fit.showLabel ? LABEL_GAP_SECONDARY : LABEL_GAP_PRIMARY}px;`"
        >
          <div>Last updated</div>
          <div :style="`margin-top: ${LABEL_GAP_UPDATED_INNER}px;`">{{ formattedDate }}</div>
        </div>
      </div>
    </foreignObject>
  </g>
</template>

<style scoped>
.node-circle {
  transition: stroke 0.2s ease, stroke-width 0.2s ease;
}

.node:focus {
  outline: none;
}

.node-circle:hover,
.node:focus .node-circle {
  stroke: var(--color-primary);
  stroke-width: 1;
}

.node-circle:hover {
  cursor: pointer;
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
     wide, and it is inverse-scaled by 1/k, so its on-screen width is 2r px
     whatever the zoom — while the circle around it is 2rk px. Zoomed in, the
     box is therefore NARROWER than the circle it labels, and "Last updated"
     or the date would break onto two lines inside a bubble with room to
     spare. The lines are centred, so a nowrap line simply overflows the box
     symmetrically; whether there is room for it in the CIRCLE is decided by
     recomputeFit(), which measures single-line widths. Inherited by every
     label line. */
  white-space: nowrap;
}

/* Combined layout: count and label on same line with larger font */
.html-label-wrapper .combined {
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1;
  pointer-events: none;
}

/* Count: always visible, bold and prominent */
.html-label-wrapper .count {
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1;
  pointer-events: none;
}

/* Label text: "Contributor(s)" */
.html-label-wrapper .label {
  color: var(--color-text-secondary);
  font-weight: 700;
  line-height: 1;
  pointer-events: none;
}

/* Updated date information */
.html-label-wrapper .updated {
  color: var(--color-text-tertiary);
  line-height: 1;
  pointer-events: none;
}

</style>
