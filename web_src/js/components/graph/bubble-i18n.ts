/* The Bubble view's strings, translated by the server: custom/templates/shared/repo/bubble.tmpl
   puts them, keyed by their locale key, as JSON in #bubble-view-root's data-i18n, the
   same way other Forkana scripts get theirs through data-msg-* attributes. The English
   below is the fallback (unit tests, a page without the root); bubble-i18n.test.ts checks
   that it is custom en-US's text and that the template sends every key. */

export const bubbleMessages = {
  'repo.bubble.contributor_1': 'Contributor',
  'repo.bubble.contributor_n': 'Contributors',
  'repo.contributor_count_title_1': '%s Contributor',
  'repo.contributor_count_title_n': '%s Contributors',
  'repo.contributor_count_lower_1': '%s contributor',
  'repo.contributor_count_lower_n': '%s contributors',
  'repo.bubble.fork_count_1': '%s Fork',
  'repo.bubble.fork_count_n': '%s Forks',
  'repo.bubble.last_updated': 'Last updated',
  'repo.article_last_updated': 'Last updated: %s',
  'repo.bubble.deleted_by_author': 'Deleted by its author',
  'repo.bubble.aria_node': 'Repository node with %s',
  'repo.bubble.aria_node_deleted': 'Repository node with %s, deleted by its author',
  'repo.bubble.aria_node_updated': 'Repository node with %[1]s, last updated %[2]s',
  'repo.bubble.aria_compare_pick': '%[1]s, selected for comparison (%[2]s of 2)',
  'repo.bubble.aria_press_enter': '%s. Press Enter to select.',
  'repo.bubble.articles_selected': '%s articles selected',
  'repo.bubble.close_comparison': 'Close comparison',
  'repo.bubble.compare_articles': 'Compare articles',
  'home.back': 'Back',
  'repo.bubble.read_full_article': 'Read full article',
  'repo.bubble.view_history': 'View history',
  'repo.file_history': 'History',
  'repo.bubble.article_history': 'Article history',
  'repo.bubble.close_history': 'Close history',
  'repo.bubble.fork_of': 'Fork of: %s',
  'repo.bubble.original_contention': 'Original point of contention:',
  'repo.bubble.no_fork_description': 'No description recorded for this fork.',
  'repo.bubble.view_full_history': 'View full history',
  'repo.bubble.create_first_article': 'Create the first article',
  'repo.bubble.legend_article': 'Article',
  'repo.bubble.legend_contention': 'Point of contention',
  'repo.tombstone.label': 'Deleted',
  'repo.bubble.graph_aria': 'Fork repository graph showing contributors and relationships',
  'repo.bubble.loading_aria': 'Loading fork graph',
  'loading': 'Loading…',
  'repo.bubble.load_failed_title': 'Failed to Load Fork Graph',
  'repo.bubble.load_failed': 'Failed to load fork graph',
  'repo.bubble.load_failed_status': 'Failed to load fork graph (%s)',
  'repo.subject_articles_retry': 'Try again',
  'repo.bubble.announce_loaded': 'Loaded fork graph with %s articles',
  'repo.bubble.announce_no_data': 'No fork data available',
  'repo.bubble.announce_selected': 'Selected %[1]s with %[2]s',
  'repo.bubble.announce_opened': 'Opened %s',
  'repo.bubble.aria_contention': 'Compare %[1]s with %[2]s',
  'repo.bubble.cannot_compare_subjects': 'Cannot compare articles from different subjects.',
  'unknown': 'Unknown',
} as const;

export type BubbleMessageKey = keyof typeof bubbleMessages;

let translated: Partial<Record<BubbleMessageKey, string>> | null = null;

function messages(): Partial<Record<BubbleMessageKey, string>> {
  if (translated) return translated;
  const json = document.querySelector('#bubble-view-root')?.getAttribute('data-i18n');
  try {
    translated = json ? JSON.parse(json) : {};
  } catch {
    translated = {};
  }
  return translated;
}

/* Fills Go-style verbs: %s / %d in order, %[n]s / %[n]d by position. */
export function formatMessage(format: string, args: Array<string | number>): string {
  let next = 0;
  return format.replace(/%(?:\[(\d+)\])?([sd%])/g, (_m, index: string | undefined, verb: string) => {
    if (verb === '%') return '%';
    const arg = index ? args[Number(index) - 1] : args[next++];
    return arg === undefined ? '' : String(arg);
  });
}

/* The translated string of a key, with its verbs filled. */
export function t(key: BubbleMessageKey, ...args: Array<string | number>): string {
  return formatMessage(messages()[key] ?? bubbleMessages[key], args);
}

/* The singular or plural key, by the page language's plural rules ("one" or not). */
export function tn(count: number, key1: BubbleMessageKey, keyN: BubbleMessageKey, ...args: Array<string | number>): string {
  let one = count === 1;
  try {
    one = new Intl.PluralRules(document.documentElement.lang || 'en').select(count) === 'one';
  } catch {
    // an unknown language tag: keep the English rule
  }
  return t(one ? key1 : keyN, ...args);
}

/* For tests: forget the messages read from the page. */
export function resetBubbleMessages() {
  translated = null;
}
