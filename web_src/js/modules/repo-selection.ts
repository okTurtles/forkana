// The selected article of a subject is shared between the history view and the fishbone
// graph through localStorage, so both sides have to agree on the keys and on how a
// partially stored selection is read back. This module is that single agreement.

export type RepoSelection = {
  owner: string;
  repo: string;
  subject?: string | null;
  archived?: boolean;
};

const LS_OWNER_KEY = 'selectedArticleOwner';
const LS_SUBJECT_KEY = 'selectedArticleSubject';
const LS_REPO_KEY = 'selectedArticleRepo';
const LS_ARCHIVED_KEY = 'selectedArticleArchived';

export function readStoredSelection(): RepoSelection | null {
  if (typeof window === 'undefined') return null;
  try {
    const owner = window.localStorage.getItem(LS_OWNER_KEY);
    const repo = window.localStorage.getItem(LS_REPO_KEY);
    const subject = window.localStorage.getItem(LS_SUBJECT_KEY);
    const archived = window.localStorage.getItem(LS_ARCHIVED_KEY) === 'true';
    if (!owner) return null;
    if (repo) {
      return {owner, repo, subject: subject || null, archived};
    }
    // selections written before the repository name was stored only carry the subject,
    // which is the vanity url of the subject's active article
    if (!subject) return null;
    return {owner, repo: subject, subject, archived};
  } catch {
    return null;
  }
}

export function writeStoredSelection(selection: RepoSelection | null) {
  if (typeof window === 'undefined') return;
  try {
    if (!selection) {
      window.localStorage.removeItem(LS_OWNER_KEY);
      window.localStorage.removeItem(LS_SUBJECT_KEY);
      window.localStorage.removeItem(LS_REPO_KEY);
      window.localStorage.removeItem(LS_ARCHIVED_KEY);
      return;
    }
    window.localStorage.setItem(LS_OWNER_KEY, selection.owner);
    if (selection.subject) {
      window.localStorage.setItem(LS_SUBJECT_KEY, selection.subject);
    } else {
      window.localStorage.removeItem(LS_SUBJECT_KEY);
    }
    window.localStorage.setItem(LS_REPO_KEY, selection.repo);
    if (selection.archived) {
      window.localStorage.setItem(LS_ARCHIVED_KEY, 'true');
    } else {
      window.localStorage.removeItem(LS_ARCHIVED_KEY);
    }
  } catch {
    // ignore storage errors, e.g. a full or unavailable quota
  }
}
