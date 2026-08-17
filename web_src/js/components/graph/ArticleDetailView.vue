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

import { computed } from 'vue';
import { formatDateYMD } from '../../utils/time.ts';

const props = defineProps<{
  contributors: number;
  description?: string;
  updatedAt?: string;
  /* Diameter of the circle in screen px. */
  size?: number;
}>();

const emit = defineEmits<{
  (e: 'back'): void;
  (e: 'read'): void;
  (e: 'history'): void;
}>();

const diameter = computed(() => props.size ?? 430);
const label = computed(() => (props.contributors === 1 ? 'Contributor' : 'Contributors'));
const formattedDate = computed(() => formatDateYMD(props.updatedAt));
</script>

<template>
  <div class="detail-layer" :style="{'--detail-size': diameter + 'px'}">
    <button class="detail-back" @click="emit('back')">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5">
        <path d="M15 5l-7 7 7 7" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      Back
    </button>

    <div class="detail-bubble" :style="{'--detail-size': diameter + 'px'}">
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
.detail-layer {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  /* Slightly left of centre, as in the design: the history card sits in the
     space this leaves on the right. */
  justify-content: flex-start;
  padding-left: 12%;
  background: var(--color-body, #fff);
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
}

/* Kept inside the inscribed square of the circle (d / sqrt(2) ≈ 0.7d) so no
   line can touch the arc, then trimmed a little further for breathing room. */
.detail-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  width: 64%;
  text-align: center;
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
  padding: 8px 14px;
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
  margin: 2px 0 0;
  font-size: 11px;
  font-style: italic;
  line-height: 1.3;
  color: var(--color-text-light-2, #6b7280);
  white-space: nowrap;
}

@media (width <= 767px) {
  .detail-layer {
    justify-content: center;
    padding-left: 0;
  }

  .detail-content {
    width: 68%;
  }
}
</style>
