<script setup lang="ts">
/* ArticleComparePopup.vue
   Modal popup for comparing two selected articles.
   Shows article details and provides a button to navigate to comparison page. */

import { formatDateYMD } from '../../utils/time.ts';

defineProps<{
  articles: Array<{
    id: string;
    repoOwner?: string;
    repoSubject?: string;
    fullName?: string;
    contributors: number;
    children: string[];
    updatedAt?: string;
  }>;
  subject: string;
}>();

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
</script>

<template>
  <div class="compare-popup-overlay" @click.self="emit('close')">
    <div class="compare-popup" role="dialog" aria-labelledby="compare-popup-title" aria-modal="true">
      <!-- Header -->
      <header class="compare-popup-header">
        <h2 id="compare-popup-title" class="compare-popup-title">{{ articles.length }} articles selected</h2>
        <button class="compare-popup-close" @click="emit('close')" aria-label="Close comparison popup">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </button>
      </header>

      <!-- Article Cards -->
      <div class="compare-popup-articles">
        <article v-for="article in articles" :key="article.id" class="compare-article-card">
          <div class="compare-article-icon">
            <!-- Fork icon -->
            <svg viewBox="0 0 16 16" fill="currentColor">
              <path
                d="M5 3.25a.75.75 0 11-1.5 0 .75.75 0 011.5 0zm0 2.122a2.25 2.25 0 10-1.5 0v.878A2.25 2.25 0 005.75 8.5h1.5v2.128a2.251 2.251 0 101.5 0V8.5h1.5a2.25 2.25 0 002.25-2.25v-.878a2.25 2.25 0 10-1.5 0v.878a.75.75 0 01-.75.75h-4.5A.75.75 0 015 6.25v-.878zm3.75 7.378a.75.75 0 11-1.5 0 .75.75 0 011.5 0zm3-8.75a.75.75 0 100-1.5.75.75 0 000 1.5z" />
            </svg>
          </div>
          <div class="compare-article-content">
            <a class="compare-article-name" :href="`/${getOwner(article)}/${getSubjectName(article, subject)}`">
              {{ getOwner(article) }} / {{ getSubjectName(article, subject) }}
            </a>
            <div class="compare-article-meta">
              {{ article.contributors }} Contributor{{ article.contributors === 1 ? '' : 's' }}
            </div>
            <div class="compare-article-meta">
              {{ article.children?.length || 0 }} Fork{{ (article.children?.length || 0) === 1 ? '' : 's' }}
            </div>
            <div class="compare-article-meta">
              Last updated: {{ formatDateYMD(article.updatedAt, 'Unknown') }}
            </div>
          </div>
        </article>
      </div>

      <!-- Compare Button -->
      <footer class="compare-popup-footer">
        <button class="compare-popup-button" @click="emit('compare')">
          Compare articles
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.compare-popup-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.compare-popup {
  background: var(--color-body);
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12), 0 2px 8px rgba(0, 0, 0, 0.08);
  width: 100%;
  max-width: 420px;
  margin: 1rem;
  overflow: hidden;
}

.compare-popup-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--color-secondary);
}

.compare-popup-title {
  font-size: 1.125rem;
  font-weight: 600;
  color: var(--color-text);
  margin: 0;
}

.compare-popup-close {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  border-radius: 6px;
  color: var(--color-text-light-2);
  cursor: pointer;
  transition: background-color 0.15s, color 0.15s;
}

.compare-popup-close:hover {
  background: var(--color-hover);
  color: var(--color-text);
}

.compare-popup-close svg {
  width: 18px;
  height: 18px;
}

.compare-popup-articles {
  padding: 0.5rem 0;
}

.compare-article-card {
  display: flex;
  gap: 0.875rem;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--color-secondary);
}

.compare-article-card:last-child {
  border-bottom: none;
}

.compare-article-icon {
  width: 24px;
  height: 24px;
  color: var(--color-text-light-2);
  flex-shrink: 0;
  margin-top: 2px;
}

.compare-article-icon svg {
  width: 100%;
  height: 100%;
}

.compare-article-content {
  flex: 1;
  min-width: 0;
}

.compare-article-name {
  font-size: 1rem;
  font-weight: 500;
  color: var(--color-primary, #4f46e5);
  text-decoration: none;
  display: block;
  margin-bottom: 0.375rem;
}

.compare-article-name:hover {
  text-decoration: underline;
}

.compare-article-meta {
  font-size: 0.875rem;
  color: var(--color-text-light-2);
  line-height: 1.5;
}

.compare-popup-footer {
  padding: 1rem 1.5rem 1.25rem;
}

.compare-popup-button {
  width: 100%;
  padding: 0.75rem 1.5rem;
  background: var(--color-hover);
  border: 1px solid var(--color-secondary);
  border-radius: 8px;
  font-size: 0.9375rem;
  font-weight: 500;
  color: var(--color-text);
  cursor: pointer;
  transition: background-color 0.15s, border-color 0.15s, transform 0.1s;
}

.compare-popup-button:hover {
  background: var(--color-active);
  border-color: var(--color-secondary);
}

.compare-popup-button:active {
  transform: scale(0.99);
}

.compare-popup-button:focus {
  outline: 2px solid var(--color-primary, #4f46e5);
  outline-offset: 2px;
}

/* Mobile: sticky bottom, full width */
@media (max-width: 640px) {
  .compare-popup-overlay {
    align-items: flex-end;
  }

  .compare-popup {
    max-width: 100%;
    margin: 0;
    border-radius: 16px 16px 0 0;
    box-shadow: 0 -4px 24px rgba(0, 0, 0, 0.15);
  }

  .compare-popup-header {
    padding: 1rem 1.25rem;
  }

  .compare-article-card {
    padding: 0.875rem 1.25rem;
  }

  .compare-popup-footer {
    padding: 0.875rem 1.25rem 1.5rem;
    /* Extra bottom padding for safe area on iOS */
    padding-bottom: max(1.5rem, env(safe-area-inset-bottom));
  }
}
</style>
