<script setup lang="ts">
/* Simple, reusable legend; keeps layout file cleaner. The figma legend always
   shows all three keys — Article, Point of contention, Deleted — whatever the
   graph contains.

   Measurements from the legend in the figma "Bubble view" (Frame 432,
   6399:44054; the same in 641:61930 and the Comparing frames): the three keys
   24px apart, each key a mark 4px from its label, the labels Inter 400 12/20
   in #1f2328, the marks 12px (the Deleted one 14×11), centred on the label. */
</script>

<template>
  <div class="legend tw-py-6">
    <div class="legend-key">
      <!-- Ellipse 5 (6399:44057): a flat #d1d9e0 disc, the bubbles' strong end -->
      <span class="legend-swatch legend-swatch--article"/>
      <span>Article</span>
    </div>
    <div class="legend-key">
      <!-- Ellipse 23 (6399:44061): white, 1px #818b98 ring, as the joint dots -->
      <span class="legend-swatch legend-swatch--contention"/>
      <span>Point of contention</span>
    </div>
    <div class="legend-key">
      <!-- #421 item 3: the tombstone of figma's Group 214 (6399:44065): a 14×11
           outline with a flat base wider than the stone and two 2px eyes in
           #59636e. Figma ships it as an unexported vector, so it is redrawn
           here at the same size and placement. -->
      <svg class="legend-swatch--deleted" viewBox="0 0 14 11.13" width="14" height="11.13" aria-hidden="true">
        <path class="tombstone-outline" d="M2.4 10.63V3.6L4.9.5h4.2l2.5 3.1v7.03"/>
        <path class="tombstone-outline" d="M0 10.63h14"/>
        <rect class="tombstone-eye" x="3.86" y="4.19" width="2.1" height="2.1" rx=".3"/>
        <rect class="tombstone-eye" x="8.05" y="4.19" width="2.1" height="2.1" rx=".3"/>
      </svg>
      <span>Deleted</span>
    </div>
  </div>
</template>

<style scoped>
/* Frame 432: the keys 24px apart, centred */
.legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 8px 24px;
}

/* Frame 434: a mark, 4px, the label (Inter 400 12/20, #1f2328) */
.legend-key {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 400;
  line-height: 20px;
  color: var(--color-text-primary);
}

.legend-swatch {
  display: inline-block;
  flex-shrink: 0;
  box-sizing: border-box;
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.legend-swatch--article {
  /* The figma draws the swatch FLAT #D1D9E0 — the gradient's strong end, one
     step deeper than the bubbles themselves — which --bubble-stroke carries
     in both themes. */
  background: var(--bubble-stroke, #d1d9e0);
}

.legend-swatch--contention {
  background: var(--bubble-joint-fill, #fff);
  border: 1px solid var(--bubble-joint-stroke, #818b98);
}

.legend-swatch--deleted {
  flex-shrink: 0;
  overflow: visible;
}

/* The outline is the label's colour (#1f2328 / black in figma), so it follows
   the theme; the eyes are the muted grey. */
.tombstone-outline {
  fill: none;
  stroke: var(--color-text-primary);
  stroke-width: 1;
  stroke-linejoin: round;
}

.tombstone-eye {
  fill: var(--color-muted-text);
}
</style>
