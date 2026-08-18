<script setup lang="ts">
/* BubbleNode.vue
   This component is responsible for rendering ONE bubble (circle + labels).
   It does NOT know about the graph; it only gets coordinates, radius, a zoom
   factor (k) and whether it is the EXPANDED bubble (hovered/opened). When any
   of those change it re-evaluates what text fits. This keeps label logic
   independent from layout and D3.

   TWO RENDERINGS
   --------------
   * RESTING — one of the five ladder sizes (22/34/58/90/126, see
     bubble-size.ts). What it may say is decided by its rung and measured
     against the arc, so a line is dropped rather than drawn outside the
     circle. 22 and 34 say nothing at all: there is no legible type at that
     size, which is exactly why hover exists.
   * EXPANDED — 202px, the whole article card: count, excerpt, last-updated,
     and (once the bubble has been CLICKED, `active`) its two actions. This is
     laid out with ordinary CSS inside the <foreignObject> rather than through
     the fit model: at 202px everything the card carries has room, and the two
     buttons need to be real, focusable, clickable elements. */

import { computed, watch, reactive } from "vue";
import { formatDateYMD } from '../../utils/time.ts';
import type { BubbleLabelDetail } from './bubble-size.ts';

/* ──────────────────────────────────────────────────────────────────────────────
   LABEL LAYOUT CONSTANTS (all values explained to avoid "magic numbers")
   ─────────────────────────────────────────────────────────────────────────── */

/* === FONT SIZING === */
const FONT_SIZE_COUNT_MIN = 10;      // Minimum font size for contributor count
const FONT_SCALE_MIN = 0.5;          // Type may shrink to this to keep a line
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
  description?: string;           // article excerpt, expanded state only
  /* What this bubble's RUNG is meant to say (bubble-size.ts). A CEILING: a
     bubble never says more than its rung asks for, and says less (or shrinks
     its type) rather than spill a line outside the circle. */
  detail?: BubbleLabelDetail;
  /* True for the hovered/focused/opened bubble: 202px, whole card. */
  expanded?: boolean;
  /* True once the expanded bubble has been CLICKED: adds the two actions. */
  active?: boolean;
  isActive?: boolean;             // selected article (persisted selection)
  isCompareMode?: boolean;        // whether compare mode is active
  compareState?: 'none' | 'first' | 'second';  // compare selection state
}>();

/* Emits so the parent can wire up interactions without D3 binding. The parent
   owns the hover/active state because growing one bubble re-runs the whole
   layout — see FishboneGraph.setHovered(). */
const emit = defineEmits<{
  (e: "click", id: string, ev: MouseEvent): void;
  (e: "hover", id: string, on: boolean, pointerType: string): void;
  (e: "read", id: string): void;
  (e: "history", id: string): void;
}>();

/* Label fit model in *screen pixels* so it looks consistent across zoom.
   We inverse-scale the label group by 1/k. */
const fit = reactive({
  showCount: false,
  showLabel: false,
  showUpdated: false,
  showCombined: false,  // True if enough space for count + label on same line
  // font sizes in px (on screen)
  fsCount: FONT_SIZE_LABEL,
  fsLabel: FONT_SIZE_LABEL,
  fsSmall: FONT_SIZE_SMALL,
  fsCombined: FONT_SIZE_COMBINED,
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
    return {w, h, fsCount, fsLabel, fsSmall, fsCombined,
      smallest: level >= 2 ? fsSmall : (level >= 1 && !combined ? fsLabel : fsCount)};
  };

  /* The rung's CEILING, capped by what there is anything to show at all.
     'none' is level -1: the 22px and 34px rungs carry no text. */
  const detail = props.detail ?? 'none';
  const ceiling = Math.min(
    detail === 'full' ? 2 : detail === 'label' ? 1 : detail === 'count' ? 0 : -1,
    hasUpd ? 2 : 1,
  );

  /* The most detailed rendering the rung allows THAT FITS. Type may shrink
     (down to FONT_SCALE_MIN, never past FONT_SIZE_FLOOR) to keep a level; if
     even the count will not fit at its smallest, the bubble shows nothing
     rather than spill outside the arc. Both directions matter: at 58px a
     three-digit count has to shrink, and at 22px nothing is shown at all. */
  let chosen: ReturnType<typeof measure> | null = null;
  let level = -1, combined = false;
  outer:
  for (let lv = ceiling; lv >= 0; lv--) {
    for (let s = 1; s >= FONT_SCALE_MIN - 1e-9; s -= 0.02) {
      const asCombined = lv >= 1 ? measure(lv, s, true) : null;
      const asStacked = measure(lv, s, false);
      if (asCombined && asCombined.smallest >= FONT_SIZE_FLOOR && fitsInCircle(asCombined.w, asCombined.h)) {
        chosen = asCombined; level = lv; combined = true; break outer;
      }
      if (asStacked.smallest >= FONT_SIZE_FLOOR && fitsInCircle(asStacked.w, asStacked.h)) {
        chosen = asStacked; level = lv; combined = false; break outer;
      }
    }
  }

  fit.showCount = level >= 0;
  fit.showCombined = level >= 1 && combined;
  fit.showLabel = level >= 1 && !combined;
  fit.showUpdated = level >= 2;
  fit.fsCount = chosen?.fsCount ?? FONT_SIZE_COUNT_MIN;
  fit.fsLabel = chosen?.fsLabel ?? FONT_SIZE_LABEL;
  fit.fsSmall = chosen?.fsSmall ?? FONT_SIZE_SMALL;
  fit.fsCombined = chosen?.fsCombined ?? FONT_SIZE_COMBINED;
}

/* Run once and whenever driving props change. */
watch(
  () => [props.k, props.r, props.updatedAt, props.contributors, props.detail, props.expanded],
  recomputeFit,
  {immediate: true},
);

/* Convenience computed transform strings */
const gTransform = computed(() => `translate(${props.x},${props.y})`);
const countLabel = computed(() => `${props.contributors} ${getLabelText(props.contributors)}`);

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
    class="node cursor-pointer select-none" :class="{ 'is-expanded': expanded }" :transform="gTransform" role="button"
    :aria-label="`Repository node with ${contributors} contributor${contributors === 1 ? '' : 's'}${updatedAt ? ', last updated ' + updatedAt : ''}. Press Enter to select.`"
    :aria-pressed="isActive ? 'true' : 'false'" tabindex="0" @click="onClick" @keydown="onKeyDown"
    @pointerdown="onPointerDown" @pointerenter="onPointerEnter" @pointerleave="onPointerLeave"
    @focusin="onFocusIn" @focusout="onFocusOut"
  >
    <!-- Bubble circle with soft gradient & subtle stroke/shadow -->
    <circle
      class="node-circle" :class="{
        'compare-dashed': props.isCompareMode && props.compareState === 'none',
        'compare-selected-first': props.compareState === 'first',
        'compare-selected-second': props.compareState === 'second'
      }" :r="r" fill="url(#bubbleGrad)"
      :stroke="props.compareState === 'first' || props.compareState === 'second' ? 'var(--color-primary)' : isActive || expanded ? 'var(--color-primary)' : 'var(--bubble-stroke)'"
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
      <!-- EXPANDED (202px): the whole card, laid out by CSS. -->
      <div
        v-if="expanded" xmlns="http://www.w3.org/1999/xhtml"
        class="html-label-wrapper expanded-wrapper" :class="{ 'is-active': active }"
      >
        <div class="combined expanded-count">{{ countLabel }}</div>
        <div v-if="description" class="expanded-description">{{ description }}</div>
        <template v-if="active">
          <button class="expanded-read" type="button" @click.stop="emit('read', id)">Read full article</button>
          <button class="expanded-history" type="button" @click.stop="emit('history', id)">View history</button>
        </template>
        <div v-if="updatedAt" class="expanded-updated">
          <div>Last updated</div>
          <div>{{ formattedDate }}</div>
        </div>
      </div>

      <!-- RESTING: whatever this rung says, measured against the arc. -->
      <div v-else xmlns="http://www.w3.org/1999/xhtml" class="html-label-wrapper">
        <!-- Combined layout: count and label on same line with larger font -->
        <div v-if="fit.showCombined" class="combined" :style="`font-size: ${fit.fsCombined}px;`">
          {{ contributors }} {{ getLabelText(contributors) }}
        </div>

        <!-- Stacked layout: count and label on separate lines (fallback) -->
        <template v-else>
          <div v-if="fit.showCount" class="count" :style="`font-size: ${fit.fsCount}px;`">
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

/* The expanded bubble paints over its neighbours' connectors, and its two
   buttons have to be clickable even though the label layer is otherwise
   inert. */
.node.is-expanded {
  cursor: default;
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

.expanded-wrapper .expanded-count {
  font-size: 14px;
  font-weight: 700;
  line-height: 1.2;
  color: var(--color-text-primary);
  white-space: nowrap;
}

.expanded-description {
  font-size: 10px;
  line-height: 1.35;
  color: var(--color-text-secondary);
  /* Three lines when the card is only being read (hover), two once the two
     actions are in it as well — the circle's height budget is fixed. */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
}

.expanded-wrapper.is-active .expanded-description {
  -webkit-line-clamp: 2;
}

.expanded-read,
.expanded-history {
  /* The label layer is inert so the graph underneath keeps its own pointer
     behaviour; the buttons opt back in. */
  pointer-events: auto;
  cursor: pointer;
  white-space: nowrap;
}

.expanded-read {
  padding: 4px 10px;
  border: 1px solid var(--color-secondary, #d1d5db);
  border-radius: 8px;
  background: var(--color-body, #fff);
  font-size: 11px;
  font-weight: 600;
  color: var(--color-text, #111827);
}

.expanded-read:hover {
  background: var(--color-hover, #f9fafb);
}

/* A button that reads as bold text: no border, no background. */
.expanded-history {
  padding: 0;
  border: none;
  background: transparent;
  font-size: 11px;
  font-weight: 700;
  color: var(--color-text, #111827);
}

.expanded-history:hover {
  text-decoration: underline;
}

.expanded-updated {
  font-size: 9px;
  font-style: italic;
  line-height: 1.3;
  color: var(--color-text-light-2, #6b7280);
  white-space: nowrap;
}
</style>
