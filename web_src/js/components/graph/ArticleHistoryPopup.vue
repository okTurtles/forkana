<script lang="ts">
export type HistoryEntry = {
  id: string;
  /* "Mike T / Moon Landing" — owner and article title. */
  title: string;
  /* True for every row but the first: the design prefixes them "Fork of:". */
  isFork: boolean;
  /* Whatever text we have about why this fork exists. */
  contention?: string;
  /* Owner handle, rendered as the @mention inside the contention line. */
  owner?: string;
  /* The row for the article the user actually clicked. */
  isCurrent: boolean;
};
</script>

<script setup lang="ts">
/* ArticleHistoryPopup.vue
   The fork lineage of one article, as a list of expandable rows.

   Desktop: a card beside the opened (202px) bubble in the graph.
   Narrow viewports: the same card as a bottom sheet, sliding up over the
   dimmed bubble. That switch is a CSS media query rather than a JS viewport
   check — it cannot get out of step with an actual resize, and it needs no
   listener. The breakpoint is the one the rest of the app uses, 768px. */

import { ref } from 'vue';

const props = defineProps<{
  entries: HistoryEntry[];
  /* Which row starts expanded — the design expands the oldest ancestor. */
  expandedId?: string | null;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'view-full-history'): void;
}>();

const expanded = ref<string | null>(props.expandedId ?? null);

function toggle(id: string) {
  expanded.value = expanded.value === id ? null : id;
}
</script>

<template>
  <!-- Backdrop: closes on click, and dims the bubble behind the bottom sheet. -->
  <div class="history-backdrop" @click="emit('close')"/>

  <aside class="history-popup" role="dialog" aria-label="Article history" @click.stop>
    <header class="history-header">
      <h3 class="history-title">History</h3>
      <button class="history-close" aria-label="Close history" @click="emit('close')">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 6l12 12M18 6L6 18" stroke-linecap="round"/>
        </svg>
      </button>
    </header>

    <ul class="history-list">
      <li
        v-for="entry in entries" :key="entry.id"
        class="history-row" :class="{ 'is-current': entry.isCurrent, 'is-open': expanded === entry.id }"
      >
        <button class="history-row-head" :aria-expanded="expanded === entry.id" @click="toggle(entry.id)">
          <svg class="history-fork-icon" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
            <path
              fill="currentColor"
              d="M5 3.25a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0Zm0 2.122a2.25 2.25 0 1 0-1.5 0v.878A2.25 2.25 0 0 0 5.75 8.5h1.5v2.128a2.251 2.251 0 1 0 1.5 0V8.5h1.5a2.25 2.25 0 0 0 2.25-2.25v-.878a2.25 2.25 0 1 0-1.5 0v.878a.75.75 0 0 1-.75.75h-4.5A.75.75 0 0 1 5 6.25v-.878ZM11.25 3.25a.75.75 0 1 1 1.5 0 .75.75 0 0 1-1.5 0Zm-3.5 9.5a.75.75 0 1 1 1.5 0 .75.75 0 0 1-1.5 0Z"
            />
          </svg>
          <span class="history-row-title">
            <span v-if="entry.isFork" class="history-fork-of">Fork of: </span>{{ entry.title }}
          </span>
          <svg
            class="history-chevron" viewBox="0 0 24 24" width="14" height="14"
            fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"
          >
            <path d="M6 9l6 6 6-6" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>

        <div v-if="expanded === entry.id" class="history-row-body">
          <template v-if="entry.contention">
            <span class="history-body-label">Original point of contention: </span>
            <span v-if="entry.owner" class="history-mention">@{{ entry.owner }}</span>
            {{ entry.contention }}
          </template>
          <span v-else class="history-body-empty">No description recorded for this fork.</span>
        </div>
      </li>
    </ul>

    <footer class="history-footer">
      <button class="btn-neutral history-full" @click="emit('view-full-history')">View full history</button>
    </footer>
  </aside>
</template>

<style scoped>
.history-backdrop {
  position: absolute;
  inset: 0;
  z-index: 20;
  background: transparent;
}

.history-popup {
  position: absolute;
  z-index: 21;

  /* Beside the OPENED bubble, overlapping its right edge as the design shows.
     `--history-anchor-x` is that bubble's right edge in canvas-box px and
     `--history-anchor-y` its centre (FishboneGraph.updateHistoryAnchor), so
     the card follows the article it belongs to instead of sitting in the
     middle of the box — the bubble no longer moves to the centre when it is
     opened, it grows where it is.

     Both axes are clamped to the box: the card never hangs out of it (the
     bubble does not move to make room, the card overlaps it further instead).
     Vertically the anchor is clamped to the middle 30% of the box by the
     component, and this card is at most 70% of the box tall, so
     `anchor ± 35%` is inside the box for any bubble position. */
  --history-width: 320px;
  --history-edge-gap: 12px;
  --history-overlap: 48px;

  left: clamp(
    var(--history-edge-gap),
    calc(var(--history-anchor-x, 50%) - var(--history-overlap)),
    calc(100% - var(--history-width) - var(--history-edge-gap))
  );
  top: var(--history-anchor-y, 50%);
  transform: translateY(-50%);
  width: var(--history-width);
  /* Last-resort guard for a box narrower than the card itself. */
  max-width: calc(100% - var(--history-edge-gap) * 2);
  max-height: 70%;
  display: flex;
  flex-direction: column;
  background: var(--color-body, #fff);
  border: 1px solid var(--color-secondary, #e5e7eb);
  border-radius: 12px;
  box-shadow: 0 12px 32px rgb(15 23 42 / 18%);
  overflow: hidden;
}

.history-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 10px;
}

.history-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--color-text, #111827);
}

.history-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-light-2, #6b7280);
  cursor: pointer;
}

.history-close:hover {
  background: var(--color-hover, #f3f4f6);
}

.history-list {
  flex: 1;
  min-height: 0;
  margin: 0;
  padding: 0 8px 8px;
  overflow-y: auto;
  list-style: none;
}

.history-row + .history-row {
  margin-top: 2px;
}

.history-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  font-size: 13px;
  color: var(--color-text, #111827);
  text-align: left;
  cursor: pointer;
}

.history-row-head:hover {
  background: var(--color-hover, #f3f4f6);
}

.history-row.is-current .history-row-head {
  color: var(--color-primary, #6d28d9);
  font-weight: 600;
}

.history-fork-icon {
  flex: none;
  opacity: 0.75;
}

.history-row-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-fork-of {
  color: var(--color-text-light-2, #6b7280);
}

.history-row.is-current .history-fork-of {
  color: inherit;
}

.history-chevron {
  flex: none;
  transition: transform 0.15s ease;
}

.history-row.is-open .history-chevron {
  transform: rotate(180deg);
}

.history-row-body {
  padding: 2px 12px 10px 32px;
  font-size: 12px;
  line-height: 1.45;
  color: var(--color-text-light-2, #6b7280);
}

.history-body-label {
  font-weight: 600;
  color: var(--color-text-light-1, #4b5563);
}

.history-mention {
  color: var(--color-primary, #6d28d9);
  font-weight: 600;
}

.history-footer {
  padding: 10px 12px 12px;
  border-top: 1px solid var(--color-secondary, #e5e7eb);
}

.history-full {
  width: 100%;
}

/* Shared neutral button, as in the design: light surface, subtle border. */
.btn-neutral {
  padding: 8px 14px;
  border: 1px solid var(--color-secondary, #d1d5db);
  border-radius: 8px;
  background: var(--color-body, #fff);
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text, #111827);
  cursor: pointer;
}

.btn-neutral:hover {
  background: var(--color-hover, #f9fafb);
}

/* ── Narrow viewports: the same card as a bottom sheet ─────────────────── */
@media (width <= 767px) {
  .history-backdrop {
    position: fixed;
    z-index: 30;
    background: rgb(15 23 42 / 35%);
  }

  .history-popup {
    position: fixed;
    z-index: 31;
    inset: auto 0 0;
    top: auto;
    left: 0;
    width: 100%;
    /* the desktop card clamps its width to the canvas box; the sheet spans the
       whole viewport edge to edge and must drop that clamp */
    max-width: none;
    max-height: 55vh;
    border: none;
    border-radius: 16px 16px 0 0;
    transform: none;
    animation: sheet-up 0.22s ease-out;
  }

  .history-header {
    padding-top: 18px;
  }
}

@keyframes sheet-up {
  from { transform: translateY(100%); }
  to { transform: translateY(0); }
}
</style>
