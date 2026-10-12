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
  /* The row for the original article, the oldest ancestor (the last row). */
  isOrigin?: boolean;
};
</script>

<script setup lang="ts">
/* ArticleHistoryPopup.vue
   The fork lineage of one article, as a list of expandable rows.

   Desktop: a card beside the opened bubble (FishboneGraph places it, never
   over the bubble — #423).
   Narrow viewports: the same card as a bottom sheet, sliding up over the
   dimmed bubble. That switch is a CSS media query rather than a JS viewport
   check — it cannot get out of step with an actual resize, and it needs no
   listener. The breakpoint is the one the rest of the app uses, 768px.

   Every value below is from figma "Bubble view - Selected" (1996:36527), the
   card "ActionMenu/History" 641:62665 and its rows 641:62676-62679, unless a
   comment says otherwise. */

import { ref, watch } from 'vue';

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
/* Track the prop, do not snapshot it: today the popup is remounted on every open
   (v-if="historyOpen"), so the initial read is correct — but nothing in here says
   a caller may not keep it mounted across a change of article. */
watch(() => props.expandedId, (id) => { expanded.value = id ?? null; });

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
      <!-- x-24 (641:62668): the 24px octicon, #59636e -->
      <button class="history-close" aria-label="Close history" @click="emit('close')">
        <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor" aria-hidden="true">
          <path d="M5.72 5.72a.75.75 0 0 1 1.06 0L12 10.94l5.22-5.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L13.06 12l5.22 5.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L12 13.06l-5.22 5.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L10.94 12 5.72 6.78a.75.75 0 0 1 0-1.06Z"/>
        </svg>
      </button>
    </header>

    <hr class="history-divider">

    <ul class="history-list">
      <li
        v-for="entry in entries" :key="entry.id"
        class="history-row"
        :class="{ 'is-current': entry.isCurrent, 'is-origin': entry.isOrigin, 'is-open': expanded === entry.id }"
      >
        <button class="history-row-head" :aria-expanded="expanded === entry.id" @click="toggle(entry.id)">
          <!-- repo-forked-24 at 20px, #1f2328 on every row -->
          <svg class="history-fork-icon" viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true">
            <path d="M8.75 19.25a3.25 3.25 0 1 1 6.5 0 3.25 3.25 0 0 1-6.5 0ZM15 4.75a3.25 3.25 0 1 1 6.5 0 3.25 3.25 0 0 1-6.5 0Zm-12.5 0a3.25 3.25 0 1 1 6.5 0 3.25 3.25 0 0 1-6.5 0ZM5.75 6.5a1.75 1.75 0 1 0-.001-3.501A1.75 1.75 0 0 0 5.75 6.5ZM12 21a1.75 1.75 0 1 0-.001-3.501A1.75 1.75 0 0 0 12 21Zm6.25-14.5a1.75 1.75 0 1 0-.001-3.501A1.75 1.75 0 0 0 18.25 6.5Z"/>
            <path d="M6.5 7.75v1A2.25 2.25 0 0 0 8.75 11h6.5a2.25 2.25 0 0 0 2.25-2.25v-1H19v1a3.75 3.75 0 0 1-3.75 3.75h-6.5A3.75 3.75 0 0 1 5 8.75v-1Z"/>
            <path d="M11.25 16.25v-5h1.5v5h-1.5Z"/>
          </svg>
          <span class="history-row-title">
            <template v-if="entry.isFork">Fork of: </template>{{ entry.title }}
          </span>
          <!-- chevron-down-24 at 16px, #1f2328; flipped up when open -->
          <svg class="history-chevron" viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
            <path d="M5.22 8.22a.749.749 0 0 0 0 1.06l6.25 6.25a.749.749 0 0 0 1.06 0l6.25-6.25a.749.749 0 1 0-1.06-1.06L12 13.939 6.28 8.22a.749.749 0 0 0-1.06 0Z"/>
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

    <hr class="history-divider">

    <footer class="history-footer">
      <button class="history-full" @click="emit('view-full-history')">View full history</button>
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

  /* Beside the OPENED bubble, 43px clear of its right edge as the design
     draws it, never over it (#423). `--history-left` is the card's left edge
     in canvas-box px, already clamped to the window, and `--history-anchor-y`
     the bubble's centre (FishboneGraph.updateHistoryAnchor). The card may hang
     past the box into the page margin, as it does in the design.

     Vertically the anchor is clamped to the middle 30% of the box by the
     component, and this card is at most 70% of the box tall, so
     `anchor ± 35%` is inside the box for any bubble position. */
  --history-width: 383px;
  --history-edge-gap: 12px;

  left: var(--history-left, 50%);
  top: var(--history-anchor-y, 50%);
  transform: translateY(-50%);
  width: var(--history-width);
  /* Last-resort guard for a window narrower than the card itself. */
  max-width: calc(100vw - var(--history-edge-gap) * 2);
  max-height: 70%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  padding: 8px;
  background: var(--color-body, #fff);
  border-radius: 12px;
  /* Primer's overlay shadow; the last one is the card's 1px #d1d9e0 @50% ring */
  box-shadow:
    0 6px 18px rgb(37 41 46 / 12%),
    0 6px 12px -3px rgb(37 41 46 / 4%),
    0 0 0 1px rgb(209 217 224 / 50%);
  overflow: hidden;
}

/* Frame 714 (641:62666): 6px above and below a 24px row, the title 8px in */
.history-header {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  padding: 6px 3px 6px 8px;
}

/* "History" label: Inter 600 16/24 #1f2328 */
.history-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  color: var(--color-text-primary, #1f2328);
}

.history-close {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-muted-text, #59636e);
  cursor: pointer;
}

.history-close:hover {
  background: var(--color-button-hover, #eff2f5);
}

/* ActionList.Divider (641:62669, 641:62682): 16px tall, a 1px #d1d9e0 @70%
   line across the whole card */
.history-divider {
  flex: none;
  height: 0;
  margin: 7.5px -8px;
  border: none;
  border-top: 1px solid rgb(209 217 224 / 70%);
}

.history-list {
  flex: 1;
  min-height: 0;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.history-row {
  position: relative;
}

/* _Item divider: rows after the first are separated by a #d1d9e0 @70% line
   that starts at the label (8px padding + 20px icon + 8px gap) and ends at the
   row's padding */
.history-row + .history-row::before {
  content: '';
  position: absolute;
  top: 0;
  left: 36px;
  right: 8px;
  border-top: 1px solid rgb(209 217 224 / 70%);
}

/* ActionList.Item (641:62676-62678): 36px tall, 8px padding, 8px gaps, 6px
   radius; the label Inter 400 14/20 #1f2328 */
.history-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px;
  border: none;
  border-radius: 6px;
  background: transparent;
  font-size: 14px;
  font-weight: 400;
  line-height: 20px;
  color: var(--color-text-primary, #1f2328);
  text-align: left;
  cursor: pointer;
}

.history-row-head:hover {
  background: var(--color-button-hover, #eff2f5);
}

/* The current article ("Mike T / Moon Landing", 641:62676): bold black */
.history-row.is-current .history-row-head {
  font-weight: 600;
}

/* The original article ("Fork of: PeterG / Moon Landing", 641:62678): bold
   blue-violet, "Fork of:" included. After .is-current, so a lineage of one —
   the original IS the current article — reads as the original. */
.history-row.is-origin .history-row-head {
  font-weight: 600;
  color: var(--color-primary, #4e40fa);
}

/* The icons stay #1f2328 on every row, the blue one included (#423) */
.history-fork-icon,
.history-chevron {
  flex: none;
  color: var(--color-text-primary, #1f2328);
}

.history-row-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-chevron {
  transition: transform 0.15s ease;
}

.history-row.is-open .history-chevron {
  transform: rotate(180deg);
}

/* The expanded row's text (641:62679): under the label, 37px from the right,
   8px below; Inter 12/20 #59636e, "Original point of contention:" 600, the
   @mention in the brand colour */
.history-row-body {
  padding: 0 37px 8px 36px;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-muted-text, #59636e);
}

.history-body-label {
  font-weight: 600;
}

.history-mention {
  color: var(--color-primary, #4e40fa);
}

/* Frame 713 (641:62683): 12px above and below the button, 8px at the sides */
.history-footer {
  flex: none;
  padding: 12px 8px;
}

/* "View full history" (641:62684): Primer's default button, 40px tall —
   #f6f8fa, a 1px #d1d9e0 border, 6px radius, Inter 600 14/20 #25292e — gray
   (#eff2f5) on hover (#423) */
.history-full {
  display: block;
  width: 100%;
  padding: 9px 16px;
  border: 1px solid var(--color-border-light, #d1d9e0);
  border-radius: 6px;
  background: var(--color-surface-muted, #f6f8fa);
  box-shadow: 0 1px 0 rgb(31 35 40 / 4%);
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--color-button-text, #25292e);
  cursor: pointer;
}

.history-full:hover {
  background: var(--color-button-hover, #eff2f5);
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
    /* the desktop card clamps its width to the window; the sheet spans the
       whole viewport edge to edge and must drop that clamp */
    max-width: none;
    max-height: 55vh;
    border-radius: 16px 16px 0 0;
    transform: none;
    animation: sheet-up 0.22s ease-out;
  }

  .history-header {
    padding-top: 12px;
  }
}

@keyframes sheet-up {
  from { transform: translateY(100%); }
  to { transform: translateY(0); }
}
</style>
