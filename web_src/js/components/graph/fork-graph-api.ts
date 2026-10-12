/* The fork-graph response, as GET /api/v1/repos/{owner}/{repo}/forks/graph returns it and
   as the subject page embeds it (pageData.subjectForkGraph). Only the fields the bubble
   view reads are listed, as the API sends them (api.Repository, api.User); every one is
   optional because the view tolerates their absence. */

export type ForkGraphRepository = {
  name?: string;
  full_name?: string;
  owner?: {login?: string, username?: string} | null;
  subject?: string;
  description?: string;
  default_branch?: string;
  html_url?: string;
  empty?: boolean;
  archived?: boolean;
  updated_at?: string;
};

export type ForkGraphNode = {
  id?: string;
  /* absent for a subject root the reader may not see (see FishboneGraph's isHidden) */
  repository?: ForkGraphRepository | null;
  /* absent when the server could not count the article's contributors */
  contributors?: {total_count?: number} | null;
  is_tombstoned?: boolean;
  children?: ForkGraphNode[] | null;
};

export type ForkGraphResponse = {
  root?: ForkGraphNode | null;
};
