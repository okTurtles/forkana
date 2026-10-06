<script setup lang="ts">
/* ArticleComparePopup.vue
   The Compare box shown once two articles are picked in Compare mode: the two
   articles and a button to the comparison page.

   It follows the figma frame "2/2 selected – ready to compare" (641:61930):
   the box is ActionMenu/Compare (641:62019) and its caret is Caret
   (641:62048). It is a popover BESIDE the picked bubbles, not a modal: no
   backdrop, the graph stays visible. Where it goes is decided by the parent
   (FishboneGraph, see ./compare-popover.ts), which tells it its `placement`
   and where its caret sits; this component only draws the box.

   Figma, top to bottom (383px wide, radius 12, 8px padding, three shadows):
     header 641:62020 — "2 articles selected", Inter 600 16/24, and a close x;
     a full-width divider (ActionList.Divider 641:62023, #d1d9e0 @70%);
     per article (641:62030, 641:62034): a 20px fork icon and the owner /
       subject link in the brand indigo, Inter 600 14/20; under it the
       details (always shown) in muted Inter 600 12/20 and 400 italic;
       then a divider;
     footer 641:62040 — "Compare articles", full width, 40px. */

import { formatDateYMD } from '../../utils/time.ts';
import { COMPARE_CARET_HEIGHT, COMPARE_CARET_WIDTH, COMPARE_POPOVER_WIDTH } from './compare-popover.ts';

const props = withDefaults(defineProps<{
  articles: Array<{
    id: string;
    repoOwner?: string;
    repoName?: string;
    repoSubject?: string;
    fullName?: string;
    articleLink?: string;
    contributors: number;
    children: string[];
    updatedAt?: string;
  }>;
  subject: string;
  /** Beside the picked bubbles ('right' / 'left'), or the bottom sheet of a phone ('sheet'). */
  placement?: 'right' | 'left' | 'sheet';
  /** Centre of the caret, from the top of the box (side placements only). */
  caretY?: number;
}>(), {
  placement: 'sheet',
  caretY: 0,
});

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'compare'): void;
}>();

function getOwner(article: { repoOwner?: string; fullName?: string }): string {
  return article.repoOwner || article.fullName?.split('/')[0] || 'Unknown';
}

function getSubjectName(article: { repoSubject?: string; fullName?: string }, subject: string): string {
  return article.repoSubject || article.fullName?.split('/')[1] || subject || 'Unknown';
}

/* The article url the server built carries the article index, which only the server
   knows. Without it, the permanent repository url still names this exact article,
   whereas "/{owner}/{subject}" names a repository only when it is named like its subject. */
function getArticleHref(article: { repoOwner?: string; repoName?: string; fullName?: string; articleLink?: string }): string {
  if (article.articleLink) return article.articleLink;
  const owner = article.repoOwner || article.fullName?.split('/')[0] || '';
  const repo = article.repoName || article.fullName?.split('/')[1] || '';
  return `${window.config.appSubUrl}/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`;
}

const caretStyle = () => ({top: `${props.caretY - COMPARE_CARET_HEIGHT / 2}px`});
/* The box's geometry comes from compare-popover.ts, which places it: the CSS reads it
   from these properties, so the painted box and the placement math cannot drift. */
const geometryStyle = {
  '--compare-popover-width': `${COMPARE_POPOVER_WIDTH}px`,
  '--compare-caret-width': `${COMPARE_CARET_WIDTH}px`,
};
</script>

<template>
  <section
    class="compare-popover" :class="`is-${props.placement}`" :style="geometryStyle"
    role="dialog" :aria-modal="props.placement === 'sheet' ? 'true' : 'false'" aria-labelledby="compare-popover-title"
  >
    <!-- Caret 641:62048: a bordered triangle on the box's edge, pointing at the picked bubbles. -->
    <svg
      v-if="props.placement !== 'sheet'" class="compare-popover-caret" :style="caretStyle()"
      :width="COMPARE_CARET_WIDTH" :height="COMPARE_CARET_HEIGHT" viewBox="0 0 7 14" aria-hidden="true"
    >
      <path class="caret-border" d="M7 0L0 7l7 7z"/>
      <path class="caret-fill" d="M7 1.5L1.5 7 7 12.5z"/>
    </svg>

    <header class="compare-popover-header">
      <h2 id="compare-popover-title" class="compare-popover-title">{{ articles.length }} articles selected</h2>
      <button type="button" class="compare-popover-icon-button" aria-label="Close comparison" @click="emit('close')">
        <!-- octicon x (x-24 in figma) -->
        <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
          <path fill="currentColor" d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/>
        </svg>
      </button>
    </header>
    <div class="compare-popover-divider" role="separator"/>

    <template v-for="article in articles" :key="article.id">
      <article class="compare-popover-article">
        <div class="compare-popover-row">
          <!-- octicon repo-forked, 20px (repo-forked-24 in figma) -->
          <svg class="compare-popover-fork" viewBox="0 0 16 16" width="20" height="20" aria-hidden="true">
            <path fill="currentColor" d="M5 5.372v.878c0 .414.336.75.75.75h4.5a.75.75 0 0 0 .75-.75v-.878a2.25 2.25 0 1 1 1.5 0v.878a2.25 2.25 0 0 1-2.25 2.25h-1.5v2.128a2.251 2.251 0 1 1-1.5 0V8.5h-1.5A2.25 2.25 0 0 1 3.5 6.25v-.878a2.25 2.25 0 1 1 1.5 0ZM5 3.25a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Zm6.75.75a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm-3 8.75a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Z"/>
          </svg>
          <a class="compare-article-name" :href="getArticleHref(article)">{{ getOwner(article) }} / {{ getSubjectName(article, subject) }}</a>
        </div>
        <!-- Always shown: the details do not fold. One text node in figma
             (I641:62036;15096:48946;15039:46266, sheet I641:63515;…): the
             first two lines Inter 600 12/20, the date line Inter 400 italic
             12/20, all #59636e. -->
        <div class="compare-popover-details">
          <div class="compare-article-count">{{ article.contributors }} Contributor{{ article.contributors === 1 ? '' : 's' }}</div>
          <div class="compare-article-count">{{ article.children?.length || 0 }} Fork{{ (article.children?.length || 0) === 1 ? '' : 's' }}</div>
          <div class="compare-article-date">Last updated: {{ formatDateYMD(article.updatedAt, 'Unknown') }}</div>
        </div>
      </article>
      <div class="compare-popover-divider" role="separator"/>
    </template>

    <footer class="compare-popover-footer">
      <button type="button" class="compare-popover-button" @click="emit('compare')">Compare articles</button>
    </footer>
  </section>
</template>

<style scoped>
/* ActionMenu/Compare 641:62019 */
.compare-popover {
  position: relative;
  box-sizing: border-box;
  width: var(--compare-popover-width);
  padding: 8px;
  border-radius: 12px;
  background: var(--color-surface);
  /* figma's three shadows: two #25292e drop shadows and a 1px #d1d9e0 @50% ring */
  box-shadow:
    0 6px 18px 0 rgba(37, 41, 46, 0.12),
    0 6px 12px -3px rgba(37, 41, 46, 0.04),
    0 0 0 1px color-mix(in srgb, var(--color-border-light) 50%, transparent);
  color: var(--color-text-primary);
  text-align: left;
}

/* On the sheet the close x sits 13px from the right edge (641:63499), not 11. */
.compare-popover.is-sheet .compare-popover-header {
  padding-right: 5px;
}

/* The bottom sheet of a phone (figma "." 641:63496: 375 wide, white, 8px
   padding, no radius and no shadow; the backdrop behind it is the parent's).
   Scrolls on its own if a short screen cannot hold it. */
.compare-popover.is-sheet {
  width: 100%;
  max-height: 85vh;
  overflow-y: auto;
  padding-bottom: calc(8px + env(safe-area-inset-bottom, 0px));
  border-radius: 0;
  box-shadow: none;
}

/* Caret 641:62048 on the edge facing the picked bubbles. It covers the box's
   1px ring where it meets it, so the two read as one outline. */
.compare-popover-caret {
  position: absolute;
  left: calc(-1 * var(--compare-caret-width));
}

.compare-popover.is-left .compare-popover-caret {
  left: auto;
  right: calc(-1 * var(--compare-caret-width));
  transform: scaleX(-1);
}

.caret-border {
  fill: var(--color-border-light);
}

.caret-fill {
  fill: var(--color-surface);
}

/* Frame 714 641:62020: 6px above and below a 24px row with 8px side padding */
.compare-popover-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 3px 6px 8px;
}

.compare-popover-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  color: var(--color-text-primary);
}

/* x-24 641:62022: a 24px target around a muted 16px icon */
.compare-popover-icon-button {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--color-muted-text);
  cursor: pointer;
}

.compare-popover-icon-button:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

.compare-popover-icon-button:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 0;
}

/* ActionList.Divider 641:62023: 16px tall, a line in the middle that spans the
   whole box, through its 8px padding */
.compare-popover-divider {
  height: 0;
  margin: 8px -8px 7px;   /* + the 1px line: 16px, as figma */
  border-top: 1px solid color-mix(in srgb, var(--color-border-light) 70%, transparent);
}

/* The article's title row, ActionList.Item 641:62035 (desktop) / 641:63508
   (sheet): 36px tall with 8px all round, the 20px fork icon at the start
   (16px from the box's edge), 8px to the link (44px). */
.compare-popover-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px;
}

.compare-popover-fork {
  flex-shrink: 0;
  color: var(--color-text-primary);
}

/* The link: Inter 600 14/20, #4e40fa */
.compare-article-name {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  color: var(--color-primary);
  text-decoration: none;
  overflow-wrap: anywhere;
}

.compare-article-name:hover {
  text-decoration: underline;
}

/* Display-only capitalization, as the home search suggestions do (#410): the
   first letter of the owner is uppercased ("bubble_a5 / …" reads
   "Bubble_a5 / …"); the subject keeps its own casing, and neither the stored
   name nor the url changes. The link is a flex item, so it is a block box
   and ::first-letter applies to it. */
.compare-article-name::first-letter {
  text-transform: uppercase;
}

/* The details, always shown. On desktop the lines start right under the link and
   line up with its text (38px in, past the fork icon), with 6px below the last
   line (Pierre's layout check of the popover, #425). */
.compare-popover-details {
  padding: 0 38px 6px;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-muted-text);
}

/* ...and on the sheet, 641:63509: the lines from the top of the row, 45px
   from the edge (aligned with the link), ending where figma's 261px text node
   ends, 306px (69px from the right edge). */
.compare-popover.is-sheet .compare-popover-details {
  padding: 0 61px 8px 37px;
}

/* "335 Contributors", "1 Fork": Inter 600 12/20, #59636e */
.compare-article-count {
  font-weight: 600;
}

/* "Last updated: 2025-06-10": Inter 400 italic 12/20, #59636e */
.compare-article-date {
  font-weight: 400;
  font-style: italic;
}

/* Frame 713 641:62040: 12px/8px around a full-width 40px button (Action
   641:62041: #f6f8fa, 1px #d1d9e0, radius 6, Inter 600 14/20) */
.compare-popover-footer {
  padding: 12px 8px;
}

.compare-popover-button {
  width: 100%;
  height: 40px;
  padding: 10px 16px;
  border: 1px solid var(--color-border-light);
  border-radius: 6px;
  background: var(--color-surface-muted);
  box-shadow: 0 1px 0 0 rgba(31, 35, 40, 0.04);
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
  /* "Compare articles" I641:62041;30258:5600: #25292e, the button label colour */
  color: var(--color-button-text);
  cursor: pointer;
}

.compare-popover-button:hover {
  background: var(--color-hover);
}

.compare-popover-button:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
</style>
