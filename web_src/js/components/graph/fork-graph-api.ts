/* The fork-graph response, as GET /api/v1/repos/{owner}/{repo}/fork-graph returns it and
   as the subject page embeds it (pageData.subjectForkGraph). Only the fields the bubble
   view reads are listed; every one is optional because the view tolerates their absence. */

export type ForkGraphRepository = {
  name?: string;
  repo_name?: string;
  full_name?: string;
  owner?: {name?: string; username?: string} | null;
  owner_name?: string;
  subject?: string;
  subject_slug?: string;
  subject_name?: string;
  description?: string;
  default_branch?: string;
  html_url?: string;
  empty?: boolean;
  archived?: boolean;
  updated_at?: string;
  updated?: string;
};

export type ForkGraphNode = {
  id?: string;
  repository?: ForkGraphRepository | null;
  /* absent when the server could not count the article's contributors */
  contributors?: {total_count?: number} | null;
  is_tombstoned?: boolean;
  children?: ForkGraphNode[] | null;
};

export type ForkGraphResponse = {
  root?: ForkGraphNode | null;
};
