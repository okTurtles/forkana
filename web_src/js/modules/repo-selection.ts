// The selected article of a subject is shared between the history view and the fishbone
// graph through localStorage, so both sides have to agree on the keys and on how a
// partially stored selection is read back. This module is that single agreement.

export type RepoSelection = {
  owner: string;
  repo: string;
  subject?: string | null;
  archived?: boolean;
  // The article url the server built for this repository. An owner can hold several
  // articles for one subject, and only the server knows which index each one carries
  // in "/subject/{subject}/{owner}/{n}", so the link is carried along instead of being
  // rebuilt from the owner and subject alone.
  link?: string | null;
};

const LS_OWNER_KEY = 'selectedArticleOwner';
const LS_SUBJECT_KEY = 'selectedArticleSubject';
const LS_REPO_KEY = 'selectedArticleRepo';
const LS_ARCHIVED_KEY = 'selectedArticleArchived';
const LS_LINK_KEY = 'selectedArticleLink';

export function readStoredSelection(): RepoSelection | null {
  if (typeof window === 'undefined') return null;
  try {
    const owner = window.localStorage.getItem(LS_OWNER_KEY);
    const repo = window.localStorage.getItem(LS_REPO_KEY);
    const subject = window.localStorage.getItem(LS_SUBJECT_KEY);
    const archived = window.localStorage.getItem(LS_ARCHIVED_KEY) === 'true';
    const link = window.localStorage.getItem(LS_LINK_KEY);
    if (!owner) return null;
    if (repo) {
      return {owner, repo, subject: subject || null, archived, link: link || null};
    }
    // selections written before the repository name was stored only carry the subject,
    // which is the vanity url of the subject's active article
    if (!subject) return null;
    return {owner, repo: subject, subject, archived, link: link || null};
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
      window.localStorage.removeItem(LS_LINK_KEY);
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
    if (selection.link) {
      window.localStorage.setItem(LS_LINK_KEY, selection.link);
    } else {
      window.localStorage.removeItem(LS_LINK_KEY);
    }
  } catch {
    // ignore storage errors, e.g. a full or unavailable quota
  }
}
