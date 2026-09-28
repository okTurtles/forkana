<script setup lang="ts">
/* Simple, reusable legend; keeps layout file cleaner. The figma legend
   (node 641-61421) always shows all three keys — Article, Point of
   contention, Deleted — whatever the graph contains. */
</script>

<template>
  <div class="tw-flex tw-items-center tw-justify-center tw-gap-10 tw-py-6">
    <div class="tw-flex tw-items-center tw-gap-2 tw-text-slate-600">
      <!-- #386 item 6: the swatch matches the bubbles it explains (theme vars,
           not a near-white hardcoded gradient that read as unselected). -->
      <span class="legend-swatch legend-swatch--article inline-block tw-w-4 tw-h-4 tw-rounded-full"/>
      <span class="tw-text-sm">Article</span>
    </div>
    <div class="tw-flex tw-items-center tw-gap-2 tw-text-slate-600">
      <!-- #386 item 7: the ring takes the joint dots' own stroke colour instead
           of the default (black) border. -->
      <span class="legend-swatch legend-swatch--contention inline-block tw-w-4 tw-h-4 tw-rounded-full"/>
      <span class="tw-text-sm">Point of contention</span>
    </div>
    <div class="tw-flex tw-items-center tw-gap-2 tw-text-slate-600">
      <!-- The figma's ⊘ mark: an outlined circle with a slash, in the same
           emphasis stroke as the joint rings. -->
      <svg class="legend-swatch--deleted" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
        <circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.5"/>
        <line x1="3.4" y1="12.6" x2="12.6" y2="3.4" stroke="currentColor" stroke-width="1.5"/>
      </svg>
      <span class="tw-text-sm">Deleted</span>
    </div>
  </div>
</template>

<style scoped>
.legend-swatch--article {
  /* The figma draws the swatch FLAT #D1D9E0 — the gradient's strong end, one
     step deeper than the bubbles themselves — which --bubble-stroke carries
     in both themes. A miniature of the bubble gradient read as dim (#386
     item 4). */
  background: var(--bubble-stroke, #d1d9e0);
  box-shadow: 0 1px 2px rgba(100, 116, 139, 0.25);
}

.legend-swatch--contention {
  background: var(--bubble-joint-fill, #fff);
  border: 2px solid var(--bubble-joint-stroke, #c7d2df);
}

.legend-swatch--deleted {
  color: var(--bubble-joint-stroke, #818b98);
}
</style>
