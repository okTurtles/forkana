<script setup lang="ts">
/* ArticleDetailView.vue
   The state the graph enters when a bubble is clicked: that one article, drawn
   large, with its excerpt and its actions inside the circle.

   This is an OVERLAY over the canvas, not a zoom of the real bubble. The
   bubble's own labels live in a <foreignObject> that is 2r WORLD units wide and
   inverse-scaled by 1/k, so its on-screen width is fixed at 2r px however far
   the graph is zoomed — a paragraph and two buttons would have had to be laid
   out in a box narrower than the circle around them, and the pan clamp and
   view-fitting would have had to be special-cased for one node. As an overlay
   the sizes are plain screen pixels, the typography is ordinary CSS, and every
   zoom/pan guarantee behind it is untouched. */

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { formatDateYMD } from '../../utils/time.ts';

/* Screen-space circle the detail view grows out of / shrinks back into: the
   bubble that was clicked, in viewport px. Null when there is nothing to grow
   from (a single-article subject, which opens straight into this view). */
export type DetailOrigin = { cx: number; cy: number; r: number };

const props = defineProps<{
  contributors: number;
  description?: string;
  updatedAt?: string;
  /* Diameter of the circle in screen px. */
  size?: number;
  /* The bubble this view came from, for the open/close zoom. */
  origin?: DetailOrigin | null;
  /* False on a single-article subject: there is no graph state worth going
     back to, so the control is not rendered at all. */
  showBack?: boolean;
  /* Flipped by the parent to ask for the closing animation; the parent unmounts
     us when we emit "closed". */
  closing?: boolean;
}>();

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'read'): void;
  (e: 'history'): void;
  (e: 'closed'): void;
}>();

const diameter = computed(() => props.size ?? 430);
const label = computed(() => (props.contributors === 1 ? 'Contributor' : 'Contributors'));
const formattedDate = computed(() => formatDateYMD(props.updatedAt));
const showBack = computed(() => props.showBack !== false);

/* ──────────────────────────────────────────────────────────────────────────────
   OPEN/CLOSE ZOOM

   The circle travels between the clicked bubble and its resting place with a
   TRANSFORM ONLY (translate + scale) — never width/height/top/left — so the
   whole move is composited and the layout inside the circle is computed once,
   at its final size. The text is laid out for the big circle and merely scaled
   down while it travels, which is why the content only fades in once the
   circle is already on its way (and fades out first on the way back): a
   scaled-down paragraph mid-flight reads as a smudge.

   The timings themselves live in the stylesheet below (350ms travel, 120ms
   content fade-out leading an 80ms shrink delay); the only numbers JS needs are
   when the element may be unmounted — the CSS total plus a frame of slack. */
const CLOSE_TOTAL_MS = 480;   // 120 lead + 80 delay + 350 travel, rounded up
const FADE_ONLY_MS = 220;     // no origin to fly from: plain cross-fade

const bubbleRef = ref<HTMLElement | null>(null);
const entered = ref(false);   // backdrop + content visible
const leaving = ref(false);   // guard: the close runs once (distinct from the `closing` prop)
let timer: number | null = null;

function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/* The transform that maps this circle, at its resting size and position, onto
   the bubble it came from. Measured rather than derived from `size` because the
   circle is also clamped by `min(--detail-size, 84vw)`. */
function originTransform(): string | null {
  const el = bubbleRef.value;
  const o = props.origin;
  if (!el || !o || !(o.r > 0)) return null;
  const b = el.getBoundingClientRect();
  if (!b.width) return null;
  const scale = (o.r * 2) / b.width;
  const dx = o.cx - (b.left + b.width / 2);
  const dy = o.cy - (b.top + b.height / 2);
  return `translate(${dx.toFixed(2)}px, ${dy.toFixed(2)}px) scale(${scale.toFixed(4)})`;
}

function clearTimer() {
  if (timer !== null) window.clearTimeout(timer);
  timer = null;
}

onMounted(() => {
  if (prefersReducedMotion()) {
    entered.value = true;    // instant swap, no travel, no fade
    return;
  }
  const el = bubbleRef.value;
  const from = originTransform();
  if (!el || !from) {
    /* Opened without a source bubble (single-article subject): subtle fade. */
    requestAnimationFrame(() => { entered.value = true; });
    return;
  }
  /* Commit the starting transform with transitions OFF, force the style to be
     computed, then hand the element back to the stylesheet's transition and
     clear the transform — the classic FLIP. Setting the start transform with
     the transition live would animate INTO the bubble instead of out of it,
     which is exactly the no-op the first attempt produced. Written straight to
     the element rather than through a reactive binding so the two steps land in
     one task, with the forced reflow between them, instead of at Vue's
     convenience. */
  el.style.transition = 'none';
  el.style.transform = from;
  void el.offsetWidth;               // commit "small, at the bubble"
  el.style.transition = '';          // back to 350ms from the stylesheet
  el.style.transform = '';           // -> travels to its resting place
  entered.value = true;
});

/* The parent asks for the close and waits for "closed" before unmounting, so
   the animation is never cut off by the element disappearing. Because the
   travel is a CSS transition, a close that interrupts a half-finished open
   starts from wherever the circle currently is — no jump. */
watch(() => props.closing, (want) => {
  if (!want || leaving.value) return;
  leaving.value = true;
  clearTimer();
  if (prefersReducedMotion()) {
    emit('closed');
    return;
  }
  const back = originTransform();
  entered.value = false;                 // content + backdrop fade out first
  /* No priming here: the transition is wanted, and because it interpolates
     from the element's CURRENT computed transform, a close that lands
     mid-open picks the circle up wherever it is. */
  if (bubbleRef.value && back) bubbleRef.value.style.transform = back;
  timer = window.setTimeout(() => emit('closed'), back ? CLOSE_TOTAL_MS : FADE_ONLY_MS);
});

onBeforeUnmount(clearTimer);
</script>

<template>
  <div
    class="detail-layer" :class="{'is-open': entered, 'is-closing': leaving}"
    :style="{'--detail-size': diameter + 'px'}"
  >
    <button v-if="showBack" class="detail-back" @click="emit('back')">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5">
        <path d="M15 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      Back
    </button>

    <!-- `transform` on this element is owned by the open/close animation above
         and written straight to the node, so it must not appear in this
         binding — Vue would patch it away on the next render. -->
    <div ref="bubbleRef" class="detail-bubble" :style="{'--detail-size': diameter + 'px'}">
      <div class="detail-content">
        <!-- Count and word on ONE line here, as in the design. -->
        <p class="detail-count">{{ contributors }} {{ label }}</p>

        <!-- The article excerpt. This one WRAPS: it is a paragraph, not a label. -->
        <p v-if="description" class="detail-description">{{ description }}</p>

        <button class="btn-neutral detail-read" @click="emit('read')">Read full article</button>
        <button class="detail-history" @click="emit('history')">View history</button>

        <p v-if="updatedAt" class="detail-updated">
          <span>Last updated</span>
          <span>{{ formattedDate }}</span>
        </p>
      </div>
    </div>

    <slot/>
  </div>
</template>

<style scoped>
/* The layer fills its container — which is .graph-container, the canvas box —
   and nothing beyond it. It is deliberately position:absolute (never fixed,
   never teleported): everything outside the box (navbar, subject title, view
   tabs, Compare button, and the legend below the box) must stay visible and
   untouched while the detail view is open. */
.detail-layer {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  /* Centred in the canvas box, horizontally and vertically. The mockup drew the
     circle left of centre with the history card in the space it left on the
     right, but a bubble that sits off-centre in an otherwise empty box reads as
     a layout bug — so the circle owns the centre and the history card overlaps
     its right edge instead (see ArticleHistoryPopup, which positions itself
     from the same centre and clamps to this box's right edge). */
  justify-content: center;
  /* Follow the canvas box's own rounded frame instead of squaring it off, and
     keep the circle and the history card clipped to it. */
  border-radius: inherit;
  overflow: hidden;

  /* Single source of truth for the open/close motion. The JS above mirrors
     these numbers only to know when it may unmount. */
  --detail-zoom-ms: 350ms;
  --detail-zoom-ease: cubic-bezier(0.25, 0.1, 0.25, 1);
}

/* The opaque backing that hides the graph is a pseudo-element, not the layer's
   own background: it has to fade IN while the circle is already flying, so the
   graph is still visible behind the travelling bubble instead of being replaced
   by a white box the instant the click lands. */
.detail-layer::before {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--color-body, #fff);
  opacity: 0;
  transition: opacity var(--detail-zoom-ms) linear;
  pointer-events: none;
}

.detail-layer.is-open::before {
  opacity: 1;
}

.detail-back {
  position: absolute;
  top: 12px;
  left: 16px;
  display: inline-flex;
  gap: 4px;
  align-items: center;
  padding: 6px 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-primary, #6d28d9);
  cursor: pointer;
}

.detail-back:hover {
  background: var(--color-hover, #f5f3ff);
}

.detail-bubble {
  /* Never wider than the viewport allows — on a phone the design's 430px
     circle would be clipped on both sides. */
  width: min(var(--detail-size, 430px), 84vw);
  height: min(var(--detail-size, 430px), 84vw);
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: radial-gradient(circle at 35% 30%,
    var(--bubble-grad-start, #fafbfc) 0%,
    var(--bubble-grad-mid, #eef2f7) 60%,
    var(--bubble-grad-end, #e6ebf2) 100%);
  border: 1px solid var(--bubble-stroke, #dbe2ea);
  box-shadow: 0 2px 6px rgb(100 116 139 / 18%);

  /* Transform only — the circle is never re-laid-out while it travels. */
  transition: transform var(--detail-zoom-ms) var(--detail-zoom-ease);
  will-change: transform;
}

/* On the way back the content clears out first, so the shrink starts a beat
   later than the fade. */
.detail-layer.is-closing .detail-bubble {
  transition-delay: 80ms;
}

/* Content follows the circle rather than travelling with it: it appears once
   the circle is under way (140ms into a 350ms move) and leaves before the
   circle starts back. */
.detail-content,
.detail-back {
  opacity: 0;
  transition: opacity 180ms linear 140ms;
}

.detail-layer.is-open .detail-content,
.detail-layer.is-open .detail-back {
  opacity: 1;
}

.detail-layer.is-closing .detail-content,
.detail-layer.is-closing .detail-back {
  opacity: 0;
  transition: opacity 120ms linear;
}

/* Reduced motion: the same states, swapped instantly. */
@media (prefers-reduced-motion: reduce) {
  .detail-layer::before,
  .detail-bubble,
  .detail-content,
  .detail-back {
    transition: none !important;
  }

  .detail-layer::before,
  .detail-content,
  .detail-back {
    opacity: 1;
  }
}

/* Kept inside the inscribed square of the circle (d / sqrt(2) ≈ 0.7d) so no
   line can touch the arc, then trimmed a little further for breathing room. */
.detail-content {
  /* Vertical rhythm from the design, on the 8px grid at the design size
     (430px circle): count → 16 → excerpt → 24 → "Read full article" → 16 →
     "View history" → 24 → "Last updated". The two steps are expressed as a
     fraction of the circle so a circle that had to shrink (narrow viewport,
     short canvas) keeps the same proportions and the stack still fits inside
     the arc: 0.0372 × 430 = 16, 0.0558 × 430 = 24. */
  --detail-gap-tight: clamp(8px, calc(var(--detail-size, 430px) * 0.0372), 16px);
  --detail-gap-loose: clamp(12px, calc(var(--detail-size, 430px) * 0.0558), 24px);

  display: flex;
  flex-direction: column;
  align-items: center;
  width: 64%;
  text-align: center;
}

/* Two classes deep on purpose: the elements below reset `margin`, and a
   single-class rule here would lose to them on source order. */
.detail-content > .detail-description,
.detail-content > .detail-history {
  margin-top: var(--detail-gap-tight);
}

/* The two wider steps in the rhythm. */
.detail-content > .detail-read,
.detail-content > .detail-updated {
  margin-top: var(--detail-gap-loose);
}

.detail-count {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
  color: var(--color-text, #111827);
  white-space: nowrap;
}

.detail-description {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--color-text-light-2, #6b7280);
  /* A paragraph, so it wraps — but never past four lines, which is what the
     design shows and what the circle has room for. */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 4;
  overflow: hidden;
}

.btn-neutral {
  padding: 8px 16px;
  border: 1px solid var(--color-secondary, #d1d5db);
  border-radius: 8px;
  background: var(--color-body, #fff);
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text, #111827);
  cursor: pointer;
  white-space: nowrap;
}

.btn-neutral:hover {
  background: var(--color-hover, #f9fafb);
}

/* A button that reads as bold text: no border, no background. */
.detail-history {
  padding: 2px 4px;
  border: none;
  background: transparent;
  font-size: 13px;
  font-weight: 700;
  color: var(--color-text, #111827);
  cursor: pointer;
  white-space: nowrap;
}

.detail-history:hover {
  text-decoration: underline;
}

.detail-updated {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;              /* the stack's rhythm owns the spacing above this */
  font-size: 11px;
  font-style: italic;
  line-height: 1.3;
  color: var(--color-text-light-2, #6b7280);
  white-space: nowrap;
}

@media (width <= 767px) {
  /* (the circle is centred at every width now, so there is nothing to undo
     here — only the contents of the smaller circle need adjusting) */
  .detail-content {
    width: 68%;
  }

  /* The circle is much smaller here while the rhythm above only scales down to
     a floor, so the excerpt gives up its fourth line rather than let the stack
     grow past the arc. */
  .detail-description {
    -webkit-line-clamp: 3;
  }
}
</style>
