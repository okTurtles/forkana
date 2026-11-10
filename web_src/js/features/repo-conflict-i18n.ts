/* The conflict resolution page's strings (repo-conflict-review.ts), translated by the
   server: custom/templates/repo/diff/conflicts_box.tmpl puts them as JSON in
   #diff-file-boxes's data-i18n (modules/page-i18n.ts). The English is the fallback;
   page-i18n.test.ts checks it against custom en-US and the template. */

import {pageMessages} from '../modules/page-i18n.ts';

export const conflictMessages = {
  'repo.pulls.conflicts_counter': '%[1]d of %[2]d conflicts',
  'repo.pulls.conflicts_prev': '‹ Prev',
  'repo.pulls.conflicts_next': 'Next ›',
  'repo.pulls.conflicts_keep': 'Keep this',
  'repo.pulls.conflicts_use': 'Use this',
  'repo.pulls.conflicts_resolve': 'Resolve',
  'repo.pulls.conflicts_resolved_button': '✓ Resolved',
  'repo.pulls.conflicts_resolved': 'Resolved',
  'repo.pulls.conflicts_resolved_more_1': 'Resolved · %d more conflict',
  'repo.pulls.conflicts_resolved_more_n': 'Resolved · %d more conflicts',
  'repo.pulls.conflicts_edit': 'Edit',
  'repo.pulls.conflicts_show_context': 'Show context lines',
  'repo.pulls.conflicts_hide_context': 'Hide context lines',
  'repo.pulls.conflicts_submitting': 'Submitting…',
  'repo.pulls.conflicts_submit_failed': 'Submit failed: %s. Your resolutions have not been saved — you will need to redo them after reloading.',
  'repo.pulls.conflicts_submit_network_error': 'Submit failed: network error',
  'repo.pulls.conflicts_init_failed': 'Failed to initialize the conflict editor. Please try reloading the page.',
  'repo.pulls.conflicts_draft_title': 'Restore conflict resolution draft?',
  'repo.pulls.conflicts_draft_text': 'We found a saved draft of your resolutions for this conflict. Would you like to restore it?',
  'repo.pulls.conflicts_draft_restore': 'Restore Draft',
  'repo.pulls.conflicts_draft_dismiss': 'Dismiss',
} as const;

export type ConflictMessageKey = keyof typeof conflictMessages;

export const {t, tn, reset: resetConflictMessages} = pageMessages<ConflictMessageKey>('#diff-file-boxes', conflictMessages);
