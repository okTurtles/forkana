<script setup lang="ts">
/* Simple, reusable legend; keeps layout file cleaner. */

defineProps<{
  /* Only graphs that actually contain a deleted article explain the muted,
     dashed bubble — the key is noise everywhere else. */
  hasTombstones?: boolean;
}>();
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
    <div v-if="hasTombstones" class="tw-flex tw-items-center tw-gap-2 tw-text-slate-600">
      <span
        class="inline-block tw-w-4 tw-h-4 tw-rounded-full"
        style="background: radial-gradient(circle at 35% 30%, #FAFBFC 0%, #EEF2F7 60%, #E6EBF2 100%);
                   border:1px dashed #9CA3AF; opacity:.55"
      />
      <span class="tw-text-sm">Deleted article</span>
    </div>
  </div>
</template>

<style scoped>
.legend-swatch--article {
  background: radial-gradient(circle at 35% 30%,
    var(--bubble-grad-start, #fafbfc) 0%,
    var(--bubble-grad-mid, #e3e9f1) 60%,
    var(--bubble-grad-end, #d5dde8) 100%);
  box-shadow: 0 1px 2px rgba(100, 116, 139, 0.25);
}

.legend-swatch--contention {
  background: var(--bubble-joint-fill, #fff);
  border: 2px solid var(--bubble-joint-stroke, #c7d2df);
}
</style>
