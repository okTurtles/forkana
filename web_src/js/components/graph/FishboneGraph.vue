<script setup lang="ts">
/* FishboneGraph.vue
   Interactive fork repository graph with fishbone layout.

   Key features:
   - Transforms the <g> group (ref="worldRef") that Vue renders into
   - Sets `touch-action: none` on the <svg> for pinch zoom on touch devices
   - All interactions (pan/zoom/click/background reset) wired through d3-zoom
   - Responsive auto-tuning based on container size and graph complexity
   - Accessibility support with ARIA labels and keyboard navigation

   SIZES ARE ON-SCREEN PIXELS (#284). Every bubble is one of the five diameters
   in ./bubble-size.ts, picked by its contributor count relative to the biggest
   article in this graph, and the view renders at zoom 1 so those numbers are
   what a ruler on the screen measures. Hovering (or focusing) a bubble grows
   it to 202px and RE-RUNS THE LAYOUT around it, tweening every node from where
   it is to where it now belongs; CLICKING that bubble opens the article on its
   own — a 425px circle centred in the canvas, with the graph behind it not
   drawn at all (ArticleDetailView.vue). Hover says what the article IS, click
   says what you can DO with it. See the HOVER / OPEN section. */

import { onMounted, reactive, ref, onBeforeUnmount, nextTick, computed, watch } from "vue";
// @ts-ignore - d3-selection types may not be available in CI environment
import { select } from "d3-selection";
// @ts-ignore - d3-selection types may not be available in CI environment
import type { Selection } from "d3-selection";
// @ts-ignore - d3-zoom types may not be available in CI environment
import { zoom, zoomIdentity, zoomTransform } from "d3-zoom";
// @ts-ignore - d3-zoom types may not be available in CI environment
import type { ZoomBehavior, ZoomTransform } from "d3-zoom";

import LegendFishbone from "./FishboneLegend.vue";
import BubbleNode from "./BubbleNode.vue";
import CreateFirstArticleBubble from "./CreateFirstArticleBubble.vue";
import ArticleComparePopup from "./ArticleComparePopup.vue";
import ArticleDetailView, { type DetailOrigin } from "./ArticleDetailView.vue";
import ArticleHistoryPopup, { type HistoryEntry } from "./ArticleHistoryPopup.vue";
import {
  BUBBLE_HOVER_RADIUS, BUBBLE_UNKNOWN_RUNG, bubbleRungFor, countTextForRung,
  maxContributors, type BubbleRung,
} from "./bubble-size.ts";

// Inline types replacing former seeds module
type Side = -1 | 1;
type NodeId = string;
type Node = {
  id: NodeId;
  contributors: number;
  parentId: NodeId | null;
  children: NodeId[];
  updatedAt?: string;
  x?: number;
  y?: number;
  depth?: number;
  repoOwner?: string;
  repoName?: string;
  repoSubject?: string;
  fullName?: string;
  /* Repository description — the article's excerpt, shown inside the expanded
     (202px) bubble. Already part of the fork-graph payload
     (api.Repository.Description), so no server-side change was needed. */
  description?: string;
  isEmpty?: boolean;
  /* The API answered 0 contributors for a repository that HAS content, which
     means the stats are still being generated server-side, not that nobody
     wrote it (see buildGraphFromApi). `contributors` carries a placeholder 1
     in that case, so this flag is the only way to tell a placeholder from a
     genuine count — and a placeholder must never define the graph maximum. */
  statsPending?: boolean;
};
type Graph = Record<string, Node>;

type RepoSelectionDetail = { owner: string; repo: string; subject?: string | null };

const LS_OWNER_KEY = 'selectedArticleOwner';
const LS_SUBJECT_KEY = 'selectedArticleSubject';
const LS_REPO_KEY = 'selectedArticleRepo';

/* ──────────────────────────────────────────────────────────────────────────────
   LAYOUT CONSTANTS (all values explained to avoid "magic numbers")
   ─────────────────────────────────────────────────────────────────────────── */

/* === BUBBLE SIZING ===
   Bubble radii are NOT computed here: every bubble takes one of the five
   diameters on the ladder in ./bubble-size.ts (22/34/58/90/126), chosen by its
   contributor count RELATIVE to the biggest article in this graph. That module
   is the single place where the sizes and their thresholds live.

   The ladder is in SCREEN pixels, which is why resetView() below renders at
   zoom 1 instead of zooming to fit: a 126px bubble has to measure 126px. */

/* === VERTICAL LAYOUT ===
   Generations are NOT a fixed distance apart. A constant LEVEL_GAP (240px) used
   to put the first child lane at `(depth + 1) * 240` whatever the parent looked
   like, which left a long stretch of bare trunk under a small/medium parent.
   The first lane is now derived per parent (see firstLaneY): the structural
   minimum is the parent's own radius + stem + elbow radius — the rib's corner
   arc has to clear the parent bubble — and FIRST_LANE_CLEARANCE is the only
   free breathing room on top of that. Children are then pushed further down
   only by the collision passes, never by a constant. */
const STEM_LEN_PARENT = 12;     // Short vertical stem extending from parent bubble
const STEM_LEN_CHILD = 18;      // Short vertical stem extending to child bubble
const FIRST_LANE_CLEARANCE = 24; // Breathing room under the parent, on the 8px grid (3 × 8)
/* Which side the FIRST child of a node goes to; the rest alternate from there.
   The design starts on the right (its first child is also the biggest one). */
const FIRST_CHILD_SIDE: Side = 1;

/* === LAYOUT DEFAULTS (used in manual mode or as auto-tuning hints) === */
const BRANCH_SPACING_DEFAULT = 28;   // Default vertical gap between branch joints on trunk
const LANE_PAD_DEFAULT = 12;   // Default padding between bubbles in same lane
const H_OFFSET_DEFAULT = 48;   // Default horizontal rib length (parent to child)
const ELBOW_R_DEFAULT = 28;   // Default elbow corner radius

/* === CLEARANCES ===
   These are CLEARANCES ONLY: every separation rule adds them on top of the two
   bubbles' ACTUAL radii (see bubbleSeparation / subtreeStackGap in the layout
   engine), so the layout stays correct if the tier table in ./bubble-size.ts
   changes. */
const BUBBLE_PAD_DEFAULT = 8;   // Minimum clearance between bubbles
const PATH_PAD_DEFAULT = 8;   // Minimum clearance between a bubble and a connector

/* === ZOOM/PAN CONSTRAINTS ===
   The canvas is FINITE (issue #104): the graph can be panned and zoomed, but
   never right out of the window. See constrainToViewport(). */
const ZOOM_MIN = 0.35;          // Absolute minimum zoom level (35% scale)
const ZOOM_MAX = 3.5;           // Maximum zoom level (350% scale)
const PAN_SLACK_PX = 80;        // How far past "useful" the graph may be dragged, per axis
const ZOOM_OUT_FIT_FRACTION = 0.5; // ...and it may not shrink below half of the zoom-to-fit scale

/* === VIEW RESET PARAMETERS ===
   "Reset view" means: put the graph back at ZOOM 1, centred in the canvas box.
   It used to mean "zoom to fit", which is incompatible with the ladder — the
   sizes in ./bubble-size.ts are on-screen pixels, and a fit scale of 0.62
   would draw the 126px bubble at 78px. A graph taller than the canvas is
   pinned to the top (RESET_TOP_MARGIN) and panned instead of being shrunk;
   the pan clamp keeps it reachable and stops it being flung away. */
const RESET_SCALE = 1;          // The graph renders at 1:1 — see ./bubble-size.ts
const RESET_TOP_MARGIN = 40;    // Minimum top margin when resetting the view
/* NOTE: a MAX_REF_DROP constant used to cap how far a child could be pushed
   down (baseY + 130). It was applied AFTER collision resolution and therefore
   silently threw the result away, which is what made bubbles overlap once the
   tiered radii from ./bubble-size.ts made them larger than the cap. Vertical
   room is now bounded by the zoom-fit in resetView(), not by a magic cap. */

/* === RESPONSIVE BREAKPOINTS & FACTORS === */
const WIDTH_BREAKPOINT_MIN = 480;    // Minimum container width for responsive calculations
const WIDTH_BREAKPOINT_MAX = 1200;   // Maximum container width for responsive calculations
const COMPLEXITY_THRESHOLD = 10;     // Number of forks to reach full complexity factor
const FANOUT_THRESHOLD = 6;          // Number of children to reach full fanout factor

/* === RESPONSIVE H_OFFSET (horizontal rib length) === */
const H_OFFSET_MIN = 36;        // Minimum horizontal offset for narrow/simple graphs
const H_OFFSET_MAX = 84;        // Maximum horizontal offset for wide/complex graphs
const H_OFFSET_WIDTH_WEIGHT = 0.35;   // Weight of container width in h_offset calculation
const H_OFFSET_COMPLEXITY_WEIGHT = 0.65;  // Weight of complexity/fanout in h_offset calculation

/* === RESPONSIVE ELBOW_R (corner radius) === */
const ELBOW_MIN = 20;           // Minimum elbow radius
const ELBOW_MAX = 36;           // Maximum elbow radius
const ELBOW_RATIO = 0.55;       // Elbow radius as ratio of h_offset

/* === RESPONSIVE BRANCH_SPACING (vertical joint gap) === */
const BRANCH_SPACING_MIN = 24;  // Minimum vertical spacing between branch joints
const BRANCH_SPACING_MAX = 36;  // Maximum vertical spacing between branch joints
const BRANCH_SPACING_BASE_WEIGHT = 0.25;  // Base weight before width/complexity factors
const BRANCH_SPACING_FACTOR_WEIGHT = 0.75;  // Weight of width/complexity factors

/* === RESPONSIVE LANE_PAD (bubble clearance in lanes) === */
const LANE_PAD_BASE = 8;        // Base lane padding
const LANE_PAD_EXTRA = 12;      // Extra lane padding at maximum responsiveness
const LANE_PAD_WIDTH_WEIGHT = 0.5;    // Weight of width factor in lane padding
const LANE_PAD_COMPLEXITY_WEIGHT = 0.3;  // Weight of complexity factor in lane padding

/* NOTE: a RADIUS_* block used to attenuate every radius by container height and
   fork count, and a FILL_FRACTION_* block decided how much of the viewport the
   zoom-to-fit should fill. Both are gone: bubble diameters are now exact
   on-screen pixels from the ladder, so neither a radius multiplier nor a fit
   scale may touch them. The zoom-to-fit scale is still COMPUTED (fitScale) —
   it is the floor for zooming out — but never applied to the resting view. */

/* === SVG LAYOUT === */
const MIN_SVG_HEIGHT = 320;          // Never collapse the canvas below this
const CONTENT_BOUNDS_EXTRA = 16;     // Extra horizontal padding for elbow overhang
const DEFAULT_CONTAINER_WIDTH = 1100;   // Default container width when not measured
/* ONE default for "container height we have not measured yet". There used to be
   two (DEFAULT_SVG_HEIGHT 1000 and DEFAULT_CONTAINER_HEIGHT 800) that meant the
   same thing and disagreed, so the first layout was fitted against one value
   and drawn at the other. */
const DEFAULT_CONTAINER_HEIGHT = 800;   // Default container height when not measured

/* === API PARAMETERS === */
const API_CONTRIBUTOR_DAYS = 90;     // Number of days to look back for contributor counts
const API_MAX_DEPTH = 10;            // Maximum fork depth to fetch from API
const API_LIMIT = 50;                // Maximum number of forks to fetch per request

/* === ANIMATION DURATIONS === */
const VIEW_TRANSITION_DURATION = 420;  // Duration of zoom/pan animations in milliseconds
const SCREEN_READER_ANNOUNCEMENT_DURATION = 1000;  // How long to show SR announcements
/* Hover reflow: the layout is re-run with the hovered node at 101px radius and
   every node TWEENED from where it is to where it lands. One tween, no physics
   — see animateTo(). 220ms is the middle of the 200-250ms band the design
   asks for; the whole move is interruptible (a new hover starts from the
   positions currently on screen, so there is never a jump). */
const HOVER_REFLOW_MS = 220;
/* Pointer debounce. Sweeping the mouse across a row of bubbles would otherwise
   re-run the layout on every pointerenter; instead the LAST bubble the pointer
   settled on wins, one reflow. Also swallows the enter/leave pair when the
   pointer crosses a gap between two bubbles. */
const HOVER_DEBOUNCE_MS = 60;

/* ──────────────────────────────────────────────────────────────────────────────
   STATE
   ─────────────────────────────────────────────────────────────────────────── */
// NodeId defined above

const state = reactive({
  graph: {} as Graph,

  /* Layout dials (manual when auto=false; hints when auto=true) */
  elbowR: ELBOW_R_DEFAULT,
  hOffset: H_OFFSET_DEFAULT,
  lanePad: LANE_PAD_DEFAULT,
  branchSpacing: BRANCH_SPACING_DEFAULT,
  bubblePad: BUBBLE_PAD_DEFAULT,
  pathPad: PATH_PAD_DEFAULT,

  auto: true,                             // responsive auto-tuning toggle
  /* Largest contributor count in the graph — the denominator every bubble's
     size ratio is taken against (./bubble-size.ts). Recomputed with the
     layout, so adding or removing a fork can resize the whole graph, which is
     the point of a relative scale. */
  maxContributors: 0,
  /* True when NOT ONE node has a real contributor count yet: there is no scale
     to compare against, so every bubble takes BUBBLE_UNKNOWN_RUNG instead of a
     ratio. Only reachable after the retries in loadForkGraph give up. */
  statsUnknown: false,
});

/* Derived arrays used for Vue rendering (instead of D3 joins).

   EVERY rendered coordinate comes from ONE snapshot — a Placement per node —
   rather than from the Node objects themselves. The layout engine still writes
   x/y onto the nodes, but that is the TARGET; what is on screen may be a tween
   frame between the resting layout and the hovered one. Deriving the ribs,
   trunks and joints from the same snapshot as the bubbles is what keeps a
   connector attached to its bubble mid-flight. */
type Placement = { x: number; y: number; r: number };
type Placements = Map<NodeId, Placement>;
type FrameNode = { node: Node; x: number; y: number; r: number };

type EdgeGeom = {
  source: FrameNode; target: FrameNode; side: Side;
  ex: number; ey: number; hx: number; hy: number; cx: number; cy: number; sx1: number; sy1: number; sx2: number; sy2: number;
};
const nodesList = ref<FrameNode[]>([]);
const edgesList = ref<EdgeGeom[]>([]);
const trunksList = ref<{ x: number; y1: number; y2: number; id: string }[]>([]);
const jointDots = ref<{ x: number; y: number; id: string; sourceOwner: string; targetOwner: string; subject: string }[]>([]);

/* SVG/zoom plumbing */
const svgHeight = ref(DEFAULT_CONTAINER_HEIGHT);
const svgRef = ref<SVGSVGElement | null>(null);
const legendRef = ref<HTMLDivElement | null>(null);
/* IMPORTANT: This is the single world group that Vue renders into AND
   that d3-zoom transforms. This fixes the "graph doesn't move" bug. */
const worldRef = ref<SVGGElement | null>(null);

let svgSel!: Selection<SVGSVGElement, unknown, null, undefined>;
let worldSel!: Selection<SVGGElement, unknown, null, undefined>;
let zoomBehavior!: ZoomBehavior<Element, unknown>;
const currentK = ref(1);
/* Bubble bounds in world units, cached at layout time: the pan constraint
   reads them on every zoom event and should not walk the graph. */
let contentBox = { minX: 0, maxX: 0, minY: 0, maxY: 0 };
/* Zoom-to-fit scale from the last resetView(), the floor for zooming out. */
let fitScale = 1;

/* ──────────────────────────────────────────────────────────────────────────────
   COMPARE MODE STATE
   ─────────────────────────────────────────────────────────────────────────── */
const isCompareMode = ref(false);
const compareSelection = ref<Node[]>([]);
const showComparePopup = ref(false);

/* Computed: get compare state for a node ('none' | 'first' | 'second') */
function getCompareState(nodeId: string): 'none' | 'first' | 'second' {
  if (!isCompareMode.value) return 'none';
  const idx = compareSelection.value.findIndex(n => n.id === nodeId);
  if (idx === 0) return 'first';
  if (idx === 1) return 'second';
  return 'none';
}

/* Accessibility: Screen reader announcements */
const srAnnouncement = ref("");
function announceToScreenReader(message: string) {
  srAnnouncement.value = message;
  setTimeout(() => { srAnnouncement.value = ""; }, SCREEN_READER_ANNOUNCEMENT_DURATION);
}

/* Component state management (loading, error, empty) */
const isLoading = ref(false);
const errorMessage = ref<string | null>(null);
const hasData = computed(() => {
  const nodes = Object.values(state.graph);
  if (nodes.length === 0) return false;

  // If we have multiple nodes (forks), always show the graph
  if (nodes.length > 1) {
    return true;
  }

  // For a single node (root repository), check if it has content
  if (nodes.length === 1) {
    const rootNode = nodes[0];

    // If repository has the isEmpty flag set, use that (most reliable)
    if (rootNode.isEmpty !== undefined) {
      // If isEmpty is false, the repo has content - show the bubble
      // If isEmpty is true, the repo is empty - don't show
      return !rootNode.isEmpty;
    }

    // Fallback: check for meaningful activity indicators
    const hasChildren = rootNode.children && rootNode.children.length > 0;
    const hasContributors = rootNode.contributors && rootNode.contributors > 0;

    // Show the bubble if there are children or at least 1 contributor
    return hasChildren || hasContributors;
  }

  return false;
});

/* Container width affects responsive dials; observe it. */
const containerRef = ref<HTMLDivElement | null>(null);
let ro: ResizeObserver | null = null;
let containerWidth = DEFAULT_CONTAINER_WIDTH;
let containerHeight = DEFAULT_CONTAINER_HEIGHT;
let pendingRaf: number | null = null;
let pointerCleanup: (() => void) | null = null;

/* ──────────────────────────────────────────────────────────────────────────────
   PROPS & API CONFIGURATION
   ─────────────────────────────────────────────────────────────────────────── */

interface FishboneGraphProps {
  // Core data source
  apiUrl?: string | null;
  owner?: string | null;
  repo?: string | null;
  subject?: string | null;
  defaultBranch?: string | null;

  // API query parameters (with sensible defaults from constants)
  includeContributors?: boolean;
  contributorDays?: number;
  maxDepth?: number;
  sortBy?: 'updated' | 'created' | 'pushed' | 'full_name';
  limit?: number;
}

const props = withDefaults(defineProps<FishboneGraphProps>(), {
  apiUrl: null,
  owner: null,
  repo: null,
  subject: null,
  defaultBranch: null,
  includeContributors: true,
  contributorDays: API_CONTRIBUTOR_DAYS,
  maxDepth: API_MAX_DEPTH,
  sortBy: 'updated',
  limit: API_LIMIT,
});

const selectedNodeId = ref<NodeId | null>(null);
let pendingExternalSelection: RepoSelectionDetail | null = null;

function normalize(value?: string | null) {
  return (value ?? '').toLowerCase();
}

function readStoredSelection(): RepoSelectionDetail | null {
  try {
    const owner = window.localStorage.getItem(LS_OWNER_KEY);
    const repo = window.localStorage.getItem(LS_REPO_KEY);
    const subject = window.localStorage.getItem(LS_SUBJECT_KEY);
    if (!owner) return null;
    if (repo) {
      return { owner, repo, subject: subject || null };
    }
    if (!subject) return null;
    return { owner, repo: subject, subject };
  } catch {
    return null;
  }
}

function getSelectionDetailFromNode(n: Node): RepoSelectionDetail | null {
  const ownerCandidates = [
    n.repoOwner,
    n.fullName?.split('/')?.[0],
    n.parentId === null ? (props.owner ?? null) : null,
  ].filter(Boolean) as string[];
  const repoCandidates = [
    n.repoName,
    n.fullName?.split('/')?.[1],
    n.repoSubject,
    n.parentId === null ? (props.repo ?? null) : null,
  ].filter(Boolean) as string[];
  const subjectCandidates = [
    n.repoSubject,
    n.repoName,
    n.fullName?.split('/')?.[1],
    n.parentId === null ? (props.subject ?? null) : null,
  ].filter(Boolean) as string[];

  const owner = ownerCandidates[0];
  const repo = repoCandidates[0] || subjectCandidates[0];
  if (!owner || !repo) return null;
  const subject = subjectCandidates[0] || null;
  return { owner, repo, subject };
}

function normalizeDetail(detail: RepoSelectionDetail | null): RepoSelectionDetail | null {
  if (!detail) return null;
  const repo = detail.repo || detail.subject || '';
  if (!detail.owner || !repo) return null;
  return {
    owner: detail.owner,
    repo,
    subject: detail.subject ?? detail.repo ?? null,
  };
}

function findNodeBySelection(detail: RepoSelectionDetail): Node | null {
  const desiredOwner = normalize(detail.owner);
  const desiredRepo = normalize(detail.repo || detail.subject || '');
  if (!desiredOwner || !desiredRepo) return null;
  for (const node of Object.values(state.graph)) {
    const ownerCandidates = [
      node.repoOwner,
      node.fullName?.split('/')?.[0],
      node.parentId === null ? (props.owner ?? null) : null,
    ].filter(Boolean) as string[];
    const repoCandidates = [
      node.repoName,
      node.fullName?.split('/')?.[1],
      node.repoSubject,
      node.parentId === null ? (props.repo ?? null) : null,
    ].filter(Boolean) as string[];
    if (
      ownerCandidates.some((c) => normalize(c) === desiredOwner) &&
      repoCandidates.some((c) => normalize(c) === desiredRepo)
    ) {
      return node;
    }
  }
  return null;
}

function applySelection(node: Node | null, _detail: RepoSelectionDetail | null) {
  selectedNodeId.value = node ? node.id : null;
}

function setSelectionFromDetail(detail: RepoSelectionDetail | null) {
  const normalized = normalizeDetail(detail);
  if (!normalized) {
    pendingExternalSelection = null;
    applySelection(null, null);
    return;
  }
  const node = findNodeBySelection(normalized);
  if (node) {
    pendingExternalSelection = null;
    applySelection(node, normalized);
  } else {
    pendingExternalSelection = normalized;
    applySelection(null, normalized);
  }
}

function restoreSelectionAfterGraphLoad() {
  const desired = pendingExternalSelection ?? readStoredSelection();
  if (desired) {
    pendingExternalSelection = null;
    setSelectionFromDetail(desired);
  } else {
    applySelection(null, null);
  }
}

function handleExternalSelection(event: Event) {
  const rawDetail = (event as CustomEvent<RepoSelectionDetail | null>).detail ?? null;
  const normalized = normalizeDetail(rawDetail);
  setSelectionFromDetail(normalized);
}

/* WAITING FOR THE CONTRIBUTOR STATS.

   The server computes contributor stats asynchronously and, until they are
   ready, reports every repository as 0 contributors. Sizes here are RATIOS
   against the biggest article, so a graph of all-zeros has no scale to draw:
   rendering it anyway paints every bubble at the top rung (126px) and — since
   the graph is fetched exactly once, on mount — leaves it that way until the
   user reloads the page.

   So a graph that is entirely placeholders is not drawn. The loading state is
   held and the fetch is repeated, backing off, for as long as it is worth
   waiting. The delays are bounded: stats generation can fail, and a spinner
   that never resolves is worse than a rough picture. When they run out the
   graph is drawn from the placeholders, with `statsUnknown` putting every
   bubble on the bottom rung rather than the top. */
const STATS_RETRY_DELAYS_MS = [1500, 2500, 4000, 6000] as const;
let statsRetry = 0;
let statsRetryTimer: number | null = null;

function cancelStatsRetry() {
  if (statsRetryTimer !== null) { window.clearTimeout(statsRetryTimer); statsRetryTimer = null; }
}

/** True when no article in the graph has a real contributor count yet. An empty
   graph is not "pending" — that is the no-article state, not a slow one. */
function graphIsAllPlaceholder(g: Graph): boolean {
  const nodes = Object.values(g);
  return nodes.length > 0 && nodes.every((n) => n.statsPending);
}

async function fetchForkGraphAndSet() {
  // Set loading state at the start
  isLoading.value = true;
  errorMessage.value = null;
  cancelStatsRetry();

  try {
    if (!props.apiUrl) {
      console.warn('FishboneGraph: apiUrl not provided');
      errorMessage.value = 'No API URL provided';
      isLoading.value = false;
      syncCanvasHeight();
      return;
    }
    const urlObj = new URL(props.apiUrl, window.location.origin);

    // Set API query parameters from props (only if not already in URL)
    // This allows the URL to override component props if needed
    if (!urlObj.searchParams.get('include_contributors')) {
      urlObj.searchParams.set('include_contributors', props.includeContributors.toString());
    }
    if (!urlObj.searchParams.get('contributor_days')) {
      urlObj.searchParams.set('contributor_days', props.contributorDays.toString());
    }
    if (!urlObj.searchParams.get('max_depth')) {
      urlObj.searchParams.set('max_depth', props.maxDepth.toString());
    }
    if (!urlObj.searchParams.get('sort')) {
      urlObj.searchParams.set('sort', props.sortBy);
    }
    if (!urlObj.searchParams.get('limit')) {
      urlObj.searchParams.set('limit', props.limit.toString());
    }

    const res = await fetch(urlObj.toString(), { credentials: 'same-origin' });
    if (!res.ok) {
      const errorText = `Failed to load fork graph (${res.status} ${res.statusText})`;
      console.error('FishboneGraph: API error', res.status);
      errorMessage.value = errorText;
      isLoading.value = false;
      syncCanvasHeight();
      announceToScreenReader(errorText);
      return;
    }
    const json = await res.json();
    const graph = buildGraphFromApi(json?.root);

    /* Nothing real to draw yet: stay on the loading state and come back for the
       numbers rather than rendering a graph of placeholders. state.graph is
       deliberately NOT set — a half-real graph must never reach the layout. */
    if (graphIsAllPlaceholder(graph) && statsRetry < STATS_RETRY_DELAYS_MS.length) {
      const wait = STATS_RETRY_DELAYS_MS[statsRetry];
      statsRetry++;
      statsRetryTimer = window.setTimeout(() => {
        statsRetryTimer = null;
        void fetchForkGraphAndSet();
      }, wait);
      return;                        // isLoading stays true
    }
    statsRetry = 0;
    state.graph = graph;

    // Clear loading state before layout/render
    isLoading.value = false;

    // Only layout and render if we have data
    if (Object.keys(graph).length > 0) {
      // Wait for Vue to update the DOM with the new graph data before calculating layout
      await nextTick();
      layoutAndRender();
      /* One more tick: layoutAndRender() is what makes `hasData` true, so the
         legend only exists in the DOM after Vue has flushed. resetView() needs
         its height to know how much canvas the graph actually gets. */
      await nextTick();
      resetView();
      restoreSelectionAfterGraphLoad();
      announceToScreenReader(`Loaded fork graph with ${Object.keys(graph).length} repositories`);
    } else {
      /* No article yet: the "Create the first article" bubble is centred in the
         canvas box by CSS, so the box has to be the size of the space it has. */
      await nextTick();
      syncCanvasHeight();
      announceToScreenReader('No fork data available');
    }
  } catch (err) {
    const errorText = err instanceof Error ? err.message : 'Failed to load fork graph';
    console.error('FishboneGraph: failed to fetch graph', err);
    errorMessage.value = errorText;
    isLoading.value = false;
    syncCanvasHeight();
    announceToScreenReader(errorText);
  }
}

function buildGraphFromApi(root: any): Graph {
  const g: Graph = {};
  if (!root) return g;

  // Store the root API data so we can check repository.empty flag
  let rootApiData = root;

  const visit = (n: any, parentId: string | null): string => {
    if (!n) return '';
    const id: string = n?.id ?? (n?.repository?.full_name ?? Math.random().toString(36).slice(2));
    const baseContrib: number = Number(n?.contributors?.total_count ?? n?.contributors?.recent_count ?? 0);
    let contributors: number = Number.isFinite(baseContrib) ? baseContrib : 0;
    const updatedAt: string | undefined = n?.repository?.updated_at ?? n?.repository?.updated ?? undefined;
    const repo = n?.repository ?? {};
    const ownerName: string | null =
      repo?.owner?.name ?? repo?.owner_name ?? repo?.owner?.username ?? null;
    const repoName: string | null = repo?.name ?? repo?.repo_name ?? null;
    const repoSubject: string | null =
      repo?.subject ?? repo?.subject_slug ?? repo?.subject_name ?? repoName ?? null;
    const fullName: string | null = repo?.full_name ?? (ownerName && repoName ? `${ownerName}/${repoName}` : null);
    const isEmpty: boolean = repo?.empty === true;
    const description: string = typeof repo?.description === 'string' ? repo.description : '';

    /* A repository with content has at least one commit and therefore at least
       one contributor, so 0 on a NON-EMPTY repo never means "nobody": it means
       the server has not finished computing the stats yet (it answers
       TotalCount 0 while generation is in flight — services/repository/
       fork_graph.go). Keep the placeholder 1 so a give-up render still draws
       something, but remember that it IS a placeholder: fed into a ratio as if
       it were real, it makes every bubble tie for biggest and paint at 126px. */
    const statsPending: boolean = !isEmpty && contributors === 0;
    if (statsPending) {
      contributors = 1;
    }

    const node: Node = {
      id,
      contributors,
      parentId,
      children: [],
      updatedAt,
      repoOwner: ownerName ?? undefined,
      repoName: repoName ?? undefined,
      repoSubject: repoSubject ?? undefined,
      fullName: fullName ?? undefined,
      description: description || undefined,
      isEmpty: isEmpty,
      statsPending,
    };
    if (!node.repoSubject && parentId === null && props.subject) {
      node.repoSubject = props.subject;
    }
    g[id] = node;
    for (const child of (n?.children ?? [])) {
      const childId = visit(child, id);
      if (childId) {
        node.children.push(childId);
      }
    }
    return id;
  };
  visit(rootApiData, null);
  return g;
}

/* ──────────────────────────────────────────────────────────────────────────────
   HELPERS (math + graph)
   ─────────────────────────────────────────────────────────────────────────── */

/* Radius of a bubble, in world units (== screen px, the view is at zoom 1).
   One of the five ladder values from ./bubble-size.ts, chosen by this node's
   contributors against the graph maximum — EXCEPT for the one node the layout
   is currently being run "expanded" for, which is 101 (202px across). That
   override is the entire hover mechanism: the same layout engine, one radius
   swapped, and every separation rule downstream reads it. */
let layoutExpandedId: NodeId | null = null;

/* THE one rung decision. Radius, label detail, count text and count size are
   all read off the SAME rung, so size and content can never disagree — and
   when no contributor count in the graph is real yet, that single decision is
   the only place the fallback has to be made. */
function rungFor(contributors: number): BubbleRung {
  if (state.statsUnknown) return BUBBLE_UNKNOWN_RUNG;
  return bubbleRungFor(contributors, state.maxContributors);
}

function rFor(n: Node) {
  if (layoutExpandedId !== null && n.id === layoutExpandedId) return BUBBLE_HOVER_RADIUS;
  return rungFor(n.contributors).diameter / 2;
}

/* What a bubble of this size is meant to say. Paired with rFor(): the same
   rung decides both, so size and content can never disagree. */
function detailFor(n: number) {
  return rungFor(n).labelDetail;
}

/* The count as it is written in the circle, and the size it is written at.
   Both come from the rung, so neither depends on the radius being animated —
   see ./bubble-size.ts. */
function countTextFor(n: number) {
  return countTextForRung(n, rungFor(n));
}

function countFontFor(n: number) {
  return rungFor(n).countFontSize;
}

function getRoot(g: Graph) { return Object.values(g).find(n => n.parentId === null) ?? null; }

function computeDepths(g: Graph) {
  /* BFS depth tagging so we can place parents top-down and sort render order. */
  const root = getRoot(g);
  if (!root) return; // Guard against empty graph
  (root as any).depth = 0;
  const q = [root];
  while (q.length) {
    const n: any = q.shift();
    for (const cid of n.children) {
      const c: any = g[cid];
      if (!c) continue; // Skip missing child nodes
      c.depth = (n.depth ?? 0) + 1;
      q.push(c);
    }
  }
}

function forkCount(g: Graph) { return Object.values(g).filter(n => n.parentId !== null).length; }
function parentMaxChildren(g: Graph) { return Math.max(0, ...Object.values(g).map(n => n.children.length)); }

/* ─────────────────────────────────────────────────────────────────────────────-
   RESPONSIVE AUTO-TUNING (adapts dials to width & complexity)
   ─────────────────────────────────────────────────────────────────────────── */
function applyResponsiveDials() {
  if (!state.auto) return;                 // manual mode: honor sliders
  const forks = forkCount(state.graph);
  const maxKids = parentMaxChildren(state.graph);
  const w = containerWidth;

  // Normalize width to 0..1 range based on breakpoints
  const widthFactor = Math.min(1, Math.max(0, (w - WIDTH_BREAKPOINT_MIN) / (WIDTH_BREAKPOINT_MAX - WIDTH_BREAKPOINT_MIN)));
  // Normalize complexity based on fork count (0..1 over COMPLEXITY_THRESHOLD forks)
  const complexity = Math.min(1, (forks / COMPLEXITY_THRESHOLD));
  // Normalize fanout based on children count (0..1 over FANOUT_THRESHOLD children)
  const fanout = Math.min(1, (maxKids / FANOUT_THRESHOLD));

  // Calculate horizontal offset (rib length) using weighted combination
  const mix = H_OFFSET_WIDTH_WEIGHT * widthFactor + H_OFFSET_COMPLEXITY_WEIGHT * Math.max(complexity, fanout);
  state.hOffset = Math.round(H_OFFSET_MIN + (H_OFFSET_MAX - H_OFFSET_MIN) * mix);

  // Calculate elbow radius as a ratio of h_offset, clamped to min/max
  state.elbowR = Math.min(ELBOW_MAX, Math.max(ELBOW_MIN, Math.round(ELBOW_RATIO * state.hOffset)));

  // Calculate branch spacing (vertical joint gap)
  const branchFactor = BRANCH_SPACING_BASE_WEIGHT + BRANCH_SPACING_FACTOR_WEIGHT * Math.max(widthFactor, complexity);
  state.branchSpacing = Math.round(BRANCH_SPACING_MIN + (BRANCH_SPACING_MAX - BRANCH_SPACING_MIN) * branchFactor);

  // Calculate lane padding (bubble clearance)
  state.lanePad = Math.round(LANE_PAD_BASE + LANE_PAD_EXTRA * Math.max(widthFactor * LANE_PAD_WIDTH_WEIGHT, complexity * LANE_PAD_COMPLEXITY_WEIGHT));

  /* NOTE: bubble radii used to be attenuated here by container height and fork
     count. They are exact on-screen pixels now (./bubble-size.ts), so nothing
     may scale them — a tall graph is panned, not shrunk. */
}

/* ─────────────────────────────────────────────────────────────────────────────-
   LAYOUT ENGINE — recursive fishbone (tidy-tree specialised to the design)

   Every node owns its OWN short vertical trunk, descending from the bottom of
   its bubble. Its children hang off that trunk with a short elbow + rib,
   alternating sides (the design puts the first, usually biggest, child on the
   right). A child is not placed as a lone bubble: its ENTIRE SUBTREE is laid
   out first and then positioned as one rigid block, which is what keeps the
   picture free of crossing connectors. Two invariants do all the work:

     (A) a child's whole subtree stays on ITS SIDE of the parent's trunk
         column (shifted outboard after layout if its own descendants reach
         back in), so the parent trunk never runs through it and two subtrees
         on opposite sides can never meet;
     (B) subtrees on the SAME side are stacked vertically, clear of each
         other's bounding boxes, so a rib leaving the trunk for the next one
         always passes below everything already placed there.

   Together they mean connectors can only meet at a junction they share, so no
   global obstacle lists, no collision pushing and no trunk-column bookkeeping
   are needed — the previous versions of this file carried all three.

   Everything below is derived from the ACTUAL tier radii in ./bubble-size.ts
   plus the clearance dials, so changing the tier table cannot invalidate it. */

/** Axis-aligned bounds of a laid-out subtree, in world units. */
type Bbox = { minX: number; maxX: number; minY: number; maxY: number };

/* ── SEPARATION RULES ──────────────────────────────────────────────────────
   The two clearance rules, both taking the two real radii and adding a dial. */

/** Minimum centre-to-centre distance so two bubbles do not touch. */
function bubbleSeparation(rA: number, rB: number) {
  return rA + rB + state.bubblePad;
}

/** Vertical gap between one subtree's bottom edge and the next bubble's edge
   on the same side of a trunk. */
function subtreeStackGap() {
  return Math.max(2 * state.lanePad, state.bubblePad);
}

/** Height available to the SVG canvas: the measured container minus anything
   else inside its scroll box (the legend sits under the graph). Without this
   the canvas was given the FULL container height, the legend was pushed past
   the fold and the container grew a scrollbar — the dead space this branch set
   out to remove, reintroduced from the other end. */
function graphViewportHeight() {
  const legendH = legendRef.value?.offsetHeight ?? 0;
  return Math.max(MIN_SVG_HEIGHT, (containerHeight || DEFAULT_CONTAINER_HEIGHT) - legendH);
}

/** Give the canvas box the height it actually has, in EVERY state.

   This used to happen only where a layout ran — setFrame() and resetView() —
   and both of those return early when there is no graph to draw. A subject with
   no article yet never reaches either, so `svgHeight` kept its placeholder
   (DEFAULT_CONTAINER_HEIGHT, 800px) and the canvas box was 800px tall inside a
   730px (desktop) or 494px (phone) container. The "Create the first article"
   bubble is centred in that box by CSS, so it was centred in a box taller than
   the one on screen — 35px low on a desktop, 153px low on a phone. That is the
   "not centered vertically" report, and it is a measurement bug, not an offset
   to nudge. Called from mount, resize, and every branch of the fetch. */
function syncCanvasHeight() {
  svgHeight.value = graphViewportHeight();
}

/** Y of the first child lane under a parent, derived from the PARENT'S OWN
   RADIUS. `stem + elbowR` is structural: the rib leaves the parent through a
   short stem and turns with a corner arc of radius `elbowR` whose top edge
   must clear the bubble, so a child centred any higher than this would have
   its rib cut through the parent. FIRST_LANE_CLEARANCE is the visual
   breathing room added on top. */
function firstLaneY(parentY: number, parentR: number, elbowR: number) {
  return parentY + parentR + STEM_LEN_PARENT + elbowR + FIRST_LANE_CLEARANCE;
}

function unionBbox(a: Bbox, b: Bbox): Bbox {
  return {
    minX: Math.min(a.minX, b.minX), maxX: Math.max(a.maxX, b.maxX),
    minY: Math.min(a.minY, b.minY), maxY: Math.max(a.maxY, b.maxY),
  };
}

/** Move a node and every descendant sideways. The shift is RIGID, so the
   subtree's internal geometry — and with it invariants (A) and (B) inside that
   subtree — is unchanged. */
function shiftSubtree(g: Graph, id: NodeId, dx: number) {
  const n = g[id];
  if (!n) return;
  n.x = (n.x ?? 0) + dx;
  for (const childId of n.children) shiftSubtree(g, childId, dx);
}

/** Lay out the subtree rooted at `n`, whose own x/y are already fixed, and
   return its bounding box (bubbles + trunk) in world units. */
function layoutSubtree(g: Graph, n: Node, ownSide: Side = FIRST_CHILD_SIDE, seen: Set<NodeId> = new Set()): Bbox {
  if (seen.has(n.id)) return { minX: n.x ?? 0, maxX: n.x ?? 0, minY: n.y ?? 0, maxY: n.y ?? 0 };
  seen.add(n.id);

  const nx = n.x ?? 0, ny = n.y ?? 0, nr = rFor(n);
  let box: Bbox = { minX: nx - nr, maxX: nx + nr, minY: ny - nr, maxY: ny + nr };

  const kids = n.children.map((id) => g[id]).filter((c): c is Node => c !== undefined);
  if (!kids.length) return box;

  const R = state.elbowR;
  /* How far out the child sits from the trunk. `hOffset` is the design's short
     rib; the other two terms keep the elbow itself drawable. */
  const minOffset = Math.max(state.hOffset, state.pathPad + 1, R + state.pathPad + 1);

  /* Start on the side this node itself hangs on, so a subtree grows AWAY from
     its parent's trunk instead of doubling back under it. Doubling back is
     legal — invariant (A) just slides the block outboard — but it costs a long
     rib, and the design's ribs are short. The root has no parent trunk to grow
     away from, so it starts on FIRST_CHILD_SIDE (right, as in the design). */
  let side: Side = ownSide;
  /* Bottom edge of the last subtree placed on each side — invariant (B). */
  let bottomLeft = -Infinity, bottomRight = -Infinity;
  /* Where the previous rib left the trunk, so junctions stay distinguishable. */
  let prevJoint = ny + nr + STEM_LEN_PARENT - state.branchSpacing;
  let lastJoint = ny + nr + STEM_LEN_PARENT;

  for (const c of kids) {
    const cr = rFor(c);
    const cx = nx + side * (cr + minOffset);

    let cy = Math.max(
      /* just below the parent bubble — the design's tight vertical rhythm */
      firstLaneY(ny, nr, R),
      /* far enough down the trunk for this rib's junction to be its own */
      prevJoint + state.branchSpacing + R,
      /* clear of everything already hanging on this side — invariant (B) */
      (side === -1 ? bottomLeft : bottomRight) + subtreeStackGap() + cr,
    );
    /* Exact (not axis-aligned) clearance from the parent bubble, so a child can
       sit as close as the design draws it without ever touching. */
    const sep = bubbleSeparation(cr, nr), dx = Math.abs(cx - nx);
    if (dx < sep) cy = Math.max(cy, ny + Math.sqrt(sep * sep - dx * dx));

    c.x = cx; c.y = cy;
    let sub = layoutSubtree(g, c, side, seen);

    /* Invariant (A): the subtree may have grown back towards the parent trunk
       (a grandchild placed on the inboard side). Slide the whole block
       outboard until it clears the trunk column by `pathPad`. */
    const intrusion = side === 1 ? (nx + state.pathPad) - sub.minX : sub.maxX - (nx - state.pathPad);
    if (intrusion > 0) {
      shiftSubtree(g, c.id, side * intrusion);
      sub = {...sub, minX: sub.minX + side * intrusion, maxX: sub.maxX + side * intrusion};
    }

    if (side === -1) bottomLeft = sub.maxY; else bottomRight = sub.maxY;
    box = unionBbox(box, sub);
    prevJoint = cy - R;
    lastJoint = cy - R;
    side = -side as Side;
  }

  /* The trunk itself: from the bottom of the bubble to the last junction. */
  return unionBbox(box, { minX: nx, maxX: nx, minY: ny + nr, maxY: lastJoint });
}

/* ─────────────────────────────────────────────────────────────────────────────-
   LAYOUT → PLACEMENTS → FRAME → TWEEN

   computeLayout()  runs the engine above for a given "expanded" node and
                    returns one Placement per node. Pure output: nothing is
                    rendered from it directly.
   setFrame()       derives the ribs, trunks, joints and bubble list from ONE
                    set of placements. Whatever it is handed is what is drawn,
                    so a half-finished tween is still a consistent picture.
   animateTo()      interpolates between the placements on screen and a new
                    set. This is the hover reflow: no simulation, no forces —
                    one lerp per node over HOVER_REFLOW_MS.
   ─────────────────────────────────────────────────────────────────────────── */

/** The placements currently ON SCREEN (mid-tween while one is running). */
let framePlacements: Placements = new Map();
/** The resting layout — the one with nothing expanded. Also the anchor the
   hovered layout is re-registered against, see computeLayout(). */
let restingPlacements: Placements = new Map();
let reflowRaf: number | null = null;
/** Bubbles whose RADIUS is being animated right now. Their labels are hidden
   and left uncomputed for the duration, so no text is ever drawn mid-scale;
   see BubbleNode's `frozen` prop. Only the bubble that is actually changing
   size is in here — the neighbours merely slide, and their type never moves,
   so blanking them too would just make the whole graph blink. */
const labelFrozen = ref<Set<NodeId>>(new Set());

/** Run the layout engine with `expandedId` (if any) blown up to the hover
   radius, and return where every node lands.

   The whole result is then TRANSLATED so the expanded node's centre is exactly
   where it sits in the resting layout. Without that the hovered bubble moves
   out from under the pointer as it grows (its own centre is derived from its
   radius), the pointer leaves it, the layout collapses, the pointer is back
   over it — a loop. Anchoring it makes the interaction what it says it is:
   THIS bubble grows in place, the others are pushed aside. A rigid translation
   cannot affect any separation or crossing property. */
function computeLayout(g: Graph, expandedId: NodeId | null): Placements {
  layoutExpandedId = expandedId;
  computeDepths(g);
  const root = getRoot(g);
  const out: Placements = new Map();
  if (!root) { layoutExpandedId = null; return out; }
  root.x = 0; root.y = 0;
  layoutSubtree(g, root);

  let dx = 0, dy = 0;
  if (expandedId !== null) {
    const anchor = restingPlacements.get(expandedId);
    const moved = g[expandedId];
    if (anchor && moved) { dx = anchor.x - (moved.x ?? 0); dy = anchor.y - (moved.y ?? 0); }
  }
  for (const n of Object.values(g)) {
    out.set(n.id, {x: (n.x ?? 0) + dx, y: (n.y ?? 0) + dy, r: rFor(n)});
  }
  layoutExpandedId = null;
  return out;
}

/** Derive everything Vue renders from one set of placements. */
function setFrame(g: Graph, placements: Placements) {
  framePlacements = placements;
  const at = (id: NodeId): Placement => placements.get(id) ?? {x: 0, y: 0, r: 0};

  const frame: FrameNode[] = [];
  for (const n of Object.values(g)) {
    const p = at(n.id);
    frame.push({node: n, x: p.x, y: p.y, r: p.r});
  }
  /* DOM ORDER IS STABLE, deliberately. An earlier version drew the expanded
     bubble last so it could not be painted over — but SVG has no z-index, so
     "last" means MOVING the element in the DOM, and moving a focused element
     makes Chrome drop the focus. With focus behaving like hover that turned
     into a loop: focus grows the bubble, the move blurs it, the blur collapses
     it. Nothing needs the reordering anyway: the layout guarantees no bubble
     overlaps another and no connector passes through one (matrix.py measures
     both at zero), so there is nothing that could paint over the expanded
     bubble in the first place. */
  nodesList.value = frame;

  const byId = new Map(frame.map((f) => [f.node.id, f]));
  const R = state.elbowR;
  const edges: EdgeGeom[] = [];
  for (const target of frame) {
    const pid = target.node.parentId;
    const source = pid ? byId.get(pid) : undefined;
    if (!source) continue;
    const side: Side = (target.x >= source.x) ? +1 : -1;
    const ex = source.x, ey = target.y - R, hx = ex + side * R, hy = target.y;
    const cx = target.x - side * (target.r + STEM_LEN_CHILD), cy = hy;
    const sx1 = target.x - side * target.r, sy1 = hy, sx2 = cx, sy2 = cy;
    edges.push({source, target, side, ex, ey, hx, hy, cx, cy, sx1, sy1, sx2, sy2});
  }
  edgesList.value = edges;

  trunksList.value = frame.filter((f) => f.node.children.length > 0).map((f) => {
    const yStart = f.y + f.r + STEM_LEN_PARENT;
    const ys = f.node.children.map((id) => byId.get(id)).filter((c): c is FrameNode => !!c).map((c) => c.y - R);
    return {x: f.x, y1: f.y + f.r, y2: Math.max(yStart, ...ys), id: f.node.id};
  });

  jointDots.value = edges.map(e => ({
    x: e.ex,
    y: e.ey,
    id: `${e.source.node.id}-${e.target.node.id}`,
    sourceOwner: e.source.node.repoOwner || e.source.node.fullName?.split('/')[0] || '',
    targetOwner: e.target.node.repoOwner || e.target.node.fullName?.split('/')[0] || '',
    subject: e.source.node.repoSubject || e.target.node.repoSubject || props.subject || '',
  }));

  /* The canvas is the VIEWPORT, not the content. It used to be sized from the
     lowest bubble in WORLD units (`maxY + 240`), which left the graph stranded
     at the top of an over-tall, scrolling canvas — the empty band under the
     first bubble. */
  syncCanvasHeight();
}

function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Standard ease (the "ease-in-out" of the design system), as a scalar. */
function easeInOut(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

function cancelReflow() {
  if (reflowRaf !== null) cancelAnimationFrame(reflowRaf);
  reflowRaf = null;
  if (labelFrozen.value.size) labelFrozen.value = new Set();
}

/** Tween every node from where it is drawn now to `target`.
   Interruptible by construction: a second call reads the CURRENT frame as its
   starting point, so a hover that lands mid-reflow continues from the picture
   on screen instead of snapping back. */
function animateTo(g: Graph, target: Placements, onSettle?: () => void) {
  cancelReflow();
  const from = framePlacements;
  if (!from.size || prefersReducedMotion()) {
    setFrame(g, target);
    onSettle?.();
    return;
  }
  /* Whose type must sit still: the bubbles whose radius actually changes. */
  const morphing = new Set<NodeId>();
  for (const [id, to] of target) {
    const f = from.get(id);
    if (f && Math.abs(f.r - to.r) > 0.5) morphing.add(id);
  }
  labelFrozen.value = morphing;

  const start = performance.now();
  const tick = (now: number) => {
    const t = Math.min(1, (now - start) / HOVER_REFLOW_MS);
    const e = easeInOut(t);
    const cur: Placements = new Map();
    for (const [id, to] of target) {
      const f = from.get(id) ?? to;
      cur.set(id, {
        x: f.x + (to.x - f.x) * e,
        y: f.y + (to.y - f.y) * e,
        r: f.r + (to.r - f.r) * e,
      });
    }
    setFrame(g, cur);
    if (t < 1) {
      reflowRaf = requestAnimationFrame(tick);
    } else {
      reflowRaf = null;
      /* Geometry has settled: let the labels recompute at the final radius and
         fade back in. This is the only place the freeze is lifted on a
         completed tween, so the fade can never start early. */
      if (labelFrozen.value.size) labelFrozen.value = new Set();
      onSettle?.();
    }
  };
  reflowRaf = requestAnimationFrame(tick);
}

/* ─────────────────────────────────────────────────────────────────────────────-
   VIEW FITTING (responsive reset + tiny-graph elegance)
   ─────────────────────────────────────────────────────────────────────────── */
function contentBounds() {
  if (nodesList.value.length === 0) {
    return { minX: 0, maxX: 100, minY: 0, maxY: 100 };
  }
  const minX = Math.min(...nodesList.value.map(n => n.x - n.r));
  const maxX = Math.max(...nodesList.value.map(n => n.x + n.r));
  const minY = Math.min(...nodesList.value.map(n => n.y - n.r));
  const maxY = Math.max(...nodesList.value.map(n => n.y + n.r));
  // Account for elbow overhang beyond rightmost/leftmost bubbles
  const extraX = state.hOffset + state.elbowR + STEM_LEN_CHILD + CONTENT_BOUNDS_EXTRA;
  return { minX: minX - extraX, maxX: maxX + extraX, minY, maxY };
}

/** Bounds of the BUBBLES alone, in world units. contentBounds() pads the x
   range for elbow overhang, which is right for the zoom-to-fit but wrong for a
   pan limit — the padding is empty space, and clamping to it just lets the
   bubbles themselves drift that much further out. */
function bubbleBounds() {
  const xs0: number[] = [], xs1: number[] = [], ys0: number[] = [], ys1: number[] = [];
  for (const n of nodesList.value) {
    xs0.push(n.x - n.r); xs1.push(n.x + n.r);
    ys0.push(n.y - n.r); ys1.push(n.y + n.r);
  }
  return { minX: Math.min(...xs0), maxX: Math.max(...xs1), minY: Math.min(...ys0), maxY: Math.max(...ys1) };
}

/** Pan clamp for ONE axis, in screen px. Returns how far the content has to
   move to get back inside the allowed range.

   Larger than the viewport: the content has to keep covering it, give or take
   `slack` of empty space at either edge — so you can scroll from one end of a
   big graph to the other and no further.

   Smaller than the viewport (the usual case after zoom-to-fit): only `slack`
   of play either side of the centred position — the graph can be nudged, not
   flung. */
function clampAxis(c0: number, c1: number, v0: number, v1: number, slack: number) {
  if (c1 - c0 > v1 - v0) {
    if (c0 > v0 + slack) return (v0 + slack) - c0;   // empty band at the start
    if (c1 < v1 - slack) return (v1 - slack) - c1;   // empty band at the end
    return 0;
  }
  const offset = (c0 + c1) / 2 - (v0 + v1) / 2;      // how far off-centre it sits
  if (offset > slack) return slack - offset;
  if (offset < -slack) return -slack - offset;
  return 0;
}

/** d3-zoom `constrain` hook: bound panning so the graph cannot be dragged away
   (issue #104 — the canvas used to be infinite in x and y, so the bubbles could
   be flung out of the window and be hard to find again).

   A conventional clamp on the content's bounding box, per axis, with
   PAN_SLACK_PX of give. An earlier version of this only stopped the LAST bubble
   from leaving, which technically bounded the canvas but still let the graph be
   dragged almost entirely off screen in every direction.

   It is deliberately not a `translateExtent`: that is derived for one zoom
   level and then fights the zoom-to-fit at every other one, whereas this reads
   the same at any scale because it is recomputed from the live transform. */
function constrainToViewport(t: ZoomTransform, extent: [[number, number], [number, number]]): ZoomTransform {
  if (!nodesList.value.length) return t;
  const [[vx0, vy0], [vx1, vy1]] = extent;
  const dx = clampAxis(t.applyX(contentBox.minX), t.applyX(contentBox.maxX), vx0, vx1, PAN_SLACK_PX);
  const dy = clampAxis(t.applyY(contentBox.minY), t.applyY(contentBox.maxY), vy0, vy1, PAN_SLACK_PX);
  if (!dx && !dy) return t;
  /* `translate` works in pre-scale units, so convert from the screen px above. */
  return t.translate(dx / t.k, dy / t.k);
}

/** The viewport d3 uses for this SVG, for the two places that build a transform
   themselves and then have to honour the same bound. */
function zoomExtent(): [[number, number], [number, number]] {
  const box = svgRef.value?.getBoundingClientRect();
  return [[0, 0], [box?.width ?? containerWidth, box?.height ?? graphViewportHeight()]];
}

/** Keep the zoom-out floor tied to the current fit, so the graph can always be
   made bigger than half the size it lands at, and never shrink to a speck. */
function applyScaleExtent() {
  if (!zoomBehavior) return;
  /* `fitScale` is capped at 1 before it gets here (resetView): the resting
     view IS 1, and a floor above it would clamp the graph the moment it
     loaded. */
  const min = Math.min(RESET_SCALE, Math.max(ZOOM_MIN, fitScale * ZOOM_OUT_FIT_FRACTION));
  zoomBehavior.scaleExtent([min, ZOOM_MAX]);
}

function resetView(animated = false) {
  /* Centering fix: apply transform to worldSel (the same <g> Vue renders). */
  if (!nodesList.value.length) return; // Guard against empty graph
  const svg = svgRef.value!;
  const box = svg.getBoundingClientRect();
  if (!box.width || !box.height) {
    requestAnimationFrame(() => resetView(animated));
    return;
  }

  if (nodesList.value.length === 0) return;

  /* Vertical fit must use the scroll viewport (the measured container), NOT the
     <svg> client rect: `svgHeight` is applied by Vue on the next tick, so right
     after layoutAndRender() the rect can still report the previous canvas
     height. Fitting against a stale, too-tall rect is what left the graph
     hanging above a band of empty space. */
  const viewportH = graphViewportHeight();
  syncCanvasHeight();            // the legend may have appeared since the layout ran

  const b = contentBounds();
  const contentW = b.maxX - b.minX, contentH = b.maxY - b.minY;

  // Validate bounds
  if (!isFinite(contentW) || !isFinite(contentH) || !isFinite(b.minX) || !isFinite(b.minY)) {
    return;
  }

  /* THE VIEW IS 1:1. The bubble sizes are on-screen pixels (./bubble-size.ts),
     so the resting view may not be scaled at all — "reset view" now means
     "centre the content at zoom 1", not "zoom to fit". The fit scale is still
     computed, capped at 1, purely as the floor for how far out the user may
     zoom manually (applyScaleExtent). */
  const targetScale = RESET_SCALE;
  fitScale = Math.min(RESET_SCALE, Math.min(
    box.width / Math.max(1, contentW),
    viewportH / Math.max(1, contentH),
  ));

  // Center horizontally
  const cx = box.width / 2;
  const worldCenterX = (b.minX + b.maxX) / 2;
  const tx = cx - (worldCenterX * targetScale);
  /* Centre vertically when the graph fits, pin it to the top when it does not:
     at 1:1 a tall graph is read from the root downwards and panned, and
     starting it half-way up would hide the root.

     The free space is measured against the WHOLE canvas box. It used to be
     measured against the box minus a 12px top gutter, and then the gutter was
     added back on top of the centred position — so every graph, a lone bubble
     included, sat exactly half a gutter (6px) below the centre of the box it
     was supposed to be centred in. */
  const scaledContentH = contentH * targetScale;
  const topSpace = scaledContentH + 2 * RESET_TOP_MARGIN <= viewportH
    ? (viewportH - scaledContentH) / 2
    : RESET_TOP_MARGIN;
  const ty = topSpace - (b.minY * targetScale);

  // Validate transform values before applying
  if (!isFinite(tx) || !isFinite(ty)) {
    return;
  }

  /* Record the fit before applying it: the zoom-out floor is relative to it,
     and the constraint needs the bounds that go with the current layout. */
  contentBox = bubbleBounds();
  applyScaleExtent();

  /* d3 does not run `constrain` on a transform set directly, so honour the pan
     bound here. Centred content satisfies it anyway; this keeps the two paths
     from ever disagreeing. */
  const t = constrainToViewport(zoomIdentity.translate(tx, ty).scale(targetScale), zoomExtent());
  (animated ? svgSel.transition().duration(VIEW_TRANSITION_DURATION) : svgSel).call(zoomBehavior.transform as any, t);

  currentK.value = targetScale;
}

/* NOTE: focusNode() lived here — it zoomed the canvas onto the clicked bubble.
   Clicking a bubble grows it in place instead (see the HOVER/OPEN section), so
   the zoom-to-a-node path had no caller left. FOCUS_PADDING went with it. */

/* ─────────────────────────────────────────────────────────────────────────────-
   RENDER PIPELINE (layout→derive arrays→Vue renders)
   ─────────────────────────────────────────────────────────────────────────── */
/** Full re-layout from the data: dials, the graph maximum every bubble's size
   is relative to, then the resting layout. Drops any expanded state, because
   the node it referred to may no longer exist. */
function layoutAndRender() {
  cancelReflow();
  /* Before anything else, and whatever the data: the empty and error states
     return early below and would otherwise keep a stale canvas box. */
  syncCanvasHeight();
  /* The denominator of every size ratio (./bubble-size.ts). Must be set before
     any rFor() call. */
  /* ONLY REAL COUNTS SET THE SCALE. A node whose stats are still generating
     carries a placeholder 1 (buildGraphFromApi); counting those would let a
     graph of placeholders declare its own maximum of 1, every ratio 1, and
     every bubble the top rung. If nothing is real, there is no scale at all
     and rungFor() falls back to BUBBLE_UNKNOWN_RUNG. */
  const realCounts = Object.values(state.graph)
    .filter((n) => !n.statsPending)
    .map((n) => n.contributors);
  state.statsUnknown = Object.keys(state.graph).length > 0 && realCounts.length === 0;
  state.maxContributors = maxContributors(realCounts);
  applyResponsiveDials();          // adapt dials first
  if (!Object.keys(state.graph).length) {
    framePlacements = new Map();
    restingPlacements = new Map();
    nodesList.value = []; edgesList.value = []; trunksList.value = []; jointDots.value = [];
    return;
  }
  /* The RESTING layout first, always: it is the anchor an expanded layout is
     registered against (computeLayout), so it has to exist and be current
     before one is asked for. */
  restingPlacements = computeLayout(state.graph, null);
  /* A re-layout can drop the node the view was pointing at (a refetch). */
  if (expandedId.value !== null && !restingPlacements.has(expandedId.value)) hoveredId.value = null;
  if (detailNode.value && !restingPlacements.has(detailNode.value.id)) finishDetailClose();
  setFrame(state.graph, expandedId.value === null
    ? restingPlacements
    : computeLayout(state.graph, expandedId.value));
}

/* ─────────────────────────────────────────────────────────────────────────────-
   MOUNT (zoom wiring, resize observer, seeds)
   ─────────────────────────────────────────────────────────────────────────── */
onMounted(async () => {
  svgSel = select(svgRef.value!);
  worldSel = select(worldRef.value!);  // CRITICAL: the very group Vue renders into

  zoomBehavior = zoom()
    .scaleExtent([ZOOM_MIN, ZOOM_MAX])
    /* Finite canvas (#104). d3 runs this on every interactive gesture and on
       translateBy/scaleBy — i.e. on the wheel-pan path below. resetView() and
       focusNode() set a transform directly, which d3 does NOT pass through
       constrain, but both centre what they are showing, so they satisfy it by
       construction and stay free to frame the graph however they like. */
    .constrain(constrainToViewport as any)
    /* Filter: pinch and ctrl+wheel zoom; plain wheel should pan (handled below). */
    .filter((event: any) => event.type === "wheel" ? event.ctrlKey : true)
    .on("zoom", (e: any) => {
      const z: ZoomTransform = e.transform; currentK.value = z.k;
      /* Apply pan/zoom to the SAME world group that holds all nodes/edges. */
      worldSel.attr("transform", z.toString());
      /* The History card hangs off a bubble, so it travels with it. */
      if (historyOpen.value) updateHistoryAnchor();
    });

  svgSel.call(zoomBehavior as any);

  /* Background click (outside any bubble) → reset; if on true background (svg), also clear selection */
  svgSel.on("click.bg", (ev: any) => {
    const target = ev.target as Element;
    if (!target.closest("g.node")) {
      if (openArticle.value) return;  // the article owns the view; Back (if any) closes it
      /* Clicking empty canvas drops the hover and re-centres the graph. */
      collapseAll();
      resetView(true);
      applySelection(null, null);
      pendingExternalSelection = null;
      persistSelectionDetail(null);
      window.dispatchEvent(new CustomEvent('repo:bubble-selected', { detail: null }));
      window.dispatchEvent(new CustomEvent('repo:selection-updated', { detail: null }));
    }
  });

  /* Wheel pans (natural trackpad behavior). Ctrl+wheel handled by d3-zoom. */
  svgSel.on("wheel.pan", (ev: any) => {
    if (ev.ctrlKey) return;       // let ctrl+wheel zoom handler run
    ev.preventDefault();
    svgSel.call(zoomBehavior.translateBy as any, -ev.deltaX, -ev.deltaY);
  }, { passive: false });

  /* Observe container width for responsive dials */
  await nextTick();
  const el = containerRef.value;
  if (!el) {
    console.warn('FishboneGraph: container element not available');
    return;
  }
  const rect0 = el.getBoundingClientRect();
  containerWidth = rect0.width;
  containerHeight = rect0.height;
  /* ...so the loading and empty states get a correctly sized box too, not the
     DEFAULT_CONTAINER_HEIGHT placeholder. */
  syncCanvasHeight();
  ro = new ResizeObserver((entries) => {
    const rect = entries[0].contentRect;
    const w = rect.width;
    const h = rect.height;
    let changed = false;
    if (Math.abs(w - containerWidth) > 2) { containerWidth = Math.min(w, 1100); changed = true; }
    if (Math.abs(h - containerHeight) > 2) { containerHeight = h; changed = true; }
    if (changed) {
      /* Synchronously, before the re-layout: a resize with no data (the empty
         state) never reaches layoutAndRender's queue below, and even with data
         the box should not be a frame stale. */
      syncCanvasHeight();
      /* The opened circle is sized from the container, so it has to follow it. */
      if (openArticle.value) { detailSize.value = computeDetailSize(); updateHistoryAnchor(); }
      if (pendingRaf !== null) cancelAnimationFrame(pendingRaf);
      pendingRaf = requestAnimationFrame(() => {
        layoutAndRender();
        resetView();
        pendingRaf = null;
      });
    }
  });
  ro.observe(el);

  /* Where the pointer is. Read only when an article view finishes closing, to
     ask whether the bubble it landed on is genuinely under the cursor. */
  const trackPointer = (ev: PointerEvent) => {
    if (ev.pointerType === 'touch') { pointerClient.x = -1; pointerClient.y = -1; return; }
    pointerClient.x = ev.clientX; pointerClient.y = ev.clientY;
  };
  window.addEventListener('pointermove', trackPointer, {passive: true});
  window.addEventListener('pointerdown', (ev: PointerEvent) => {
    closedByKeyboard = false;      // a real press: not a keyboard close
    trackPointer(ev);
  }, {passive: true});
  pointerCleanup = () => window.removeEventListener('pointermove', trackPointer);

  /* Initial fetch from API */
  await fetchForkGraphAndSet();
  window.addEventListener('repo:selection-updated', handleExternalSelection as EventListener);
  window.addEventListener('repo:compare-mode-toggle', handleCompareModeToggle as EventListener);
  window.addEventListener('keydown', onGraphKeydown);
});

onBeforeUnmount(() => {
  if (ro) ro.disconnect();
  pointerCleanup?.();
  cancelReflow();
  cancelStatsRetry();
  if (hoverTimer !== null) window.clearTimeout(hoverTimer);
  window.removeEventListener('repo:selection-updated', handleExternalSelection as EventListener);
  window.removeEventListener('repo:compare-mode-toggle', handleCompareModeToggle as EventListener);
  window.removeEventListener('keydown', onGraphKeydown);
});

/* Derived for template binding */
const kComputed = computed(() => currentK.value);

function persistSelectionDetail(detail: RepoSelectionDetail | null) {
  if (typeof window === 'undefined') return;
  try {
    if (!detail) {
      window.localStorage.removeItem(LS_OWNER_KEY);
      window.localStorage.removeItem(LS_SUBJECT_KEY);
      window.localStorage.removeItem(LS_REPO_KEY);
    } else {
      window.localStorage.setItem(LS_OWNER_KEY, detail.owner);
      if (detail.subject) {
        window.localStorage.setItem(LS_SUBJECT_KEY, detail.subject);
      } else {
        window.localStorage.removeItem(LS_SUBJECT_KEY);
      }
      window.localStorage.setItem(LS_REPO_KEY, detail.repo);
    }
  } catch {
    // ignore storage quotas
  }
}

/* ──────────────────────────────────────────────────────────────────────────────
   HOVER / OPEN — one bubble grows to 202px and the graph reflows around it

   There is no overlay any more. Hovering a bubble (or focusing it with the
   keyboard) re-runs the SAME layout engine with that node's radius overridden
   to BUBBLE_HOVER_RADIUS and tweens every node from where it is to where it
   now belongs; clicking adds the two actions to the card inside it.

   HOVER is in the graph; OPEN still is not. Clicking hands off to
   ArticleDetailView (imported above, rendered near the bottom of this file),
   which draws the article as a 425px circle in the centre of the canvas with
   the graph not rendered behind it. Two ways in, which is why its "< Back"
   control is conditional rather than always drawn:
     * a click on a bubble  -> detailNode is set, Back and Escape are live;
     * a SOLO subject       -> isSoloSubject/soloPinned, no click happened, so
                               there is nowhere to go back to and no bubble is
                               ever drawn (see openArticle/graphRendered).

   Why re-running the layout rather than scaling one bubble: every geometric
   guarantee the layout makes (no overlapping bubbles, no crossing connectors,
   no connector through a bubble, a positive minimum gap) is a property of the
   engine given a set of radii. Swap one radius and the guarantees still hold,
   for free, in the hovered picture as well as the resting one. Scaling a
   bubble in place would have broken all four.
   ─────────────────────────────────────────────────────────────────────────── */

/** The bubble drawn at 202px in the graph: hovered with a pointer, focused
   with the keyboard, or first-tapped on a touch screen. */
const hoveredId = ref<NodeId | null>(null);
const expandedId = computed<NodeId | null>(() => hoveredId.value);
/** The article OPENED by a click — drawn by ArticleDetailView at 425px in the
   centre of the canvas, with the graph not rendered behind it. */
const detailNode = ref<Node | null>(null);
const historyOpen = ref(false);

/** A subject with exactly ONE article: there is no graph to speak of — no
   forks, no comparison, nothing to hover — so the page IS that article, and it
   opens straight into the 425px view. A live computed, deliberately: add a
   fork and the subject becomes an ordinary graph on the next load of the data,
   with no flag left set from before. (`hasData` keeps the no-article state out
   of this: that one belongs to CreateFirstArticleBubble.) */
const isSoloSubject = computed(() => hasData.value && Object.keys(state.graph).length === 1);

/** The article on screen: the one that was clicked, or — on a solo subject —
   the only one there is. */
const openArticle = computed<Node | null>(
  () => detailNode.value ?? (isSoloSubject.value ? getRoot(state.graph) : null),
);

/** True when the article view IS the page: nothing was clicked to get here, so
   there is nowhere to go Back to and nothing for Escape to dismiss. */
const soloPinned = computed(() => detailNode.value === null && openArticle.value !== null);

/** The graph is not merely covered while an article is open: its bubbles and
   connectors are not rendered at all. It comes back the moment the close
   starts, so the circle visibly shrinks back INTO its bubble. On a solo
   subject it is never rendered at all. */
const graphRendered = computed(() => openArticle.value === null || detailClosing.value);

/** Pointer type of the gesture in progress. Touch has no hover, so a tap has
   to mean one of two things depending on what is already expanded — see
   onBubbleClick(). */
let hoverTimer: number | null = null;
/** Where the pointer is, in client px, so a closing article view can ask
   whether it is genuinely over a bubble. -1 until the pointer is seen. */
const pointerClient = {x: -1, y: -1};
/** True when the article view was closed from the keyboard (Escape, or Back
   activated without a pointer): focus is then handed back to the bubble. */
let closedByKeyboard = false;
/** How long after a tap begins nothing may grow a bubble. Long enough to cover
   the focus the tap itself causes and the click that follows it, short enough
   that the next keyboard or mouse gesture is unaffected. */
const TOUCH_TAP_GUARD_MS = 500;
/* Set by an explicit dismissal (Escape, background click). Escape means "put
   this away", so for a moment afterwards nothing may grow a bubble again —
   without it the dismissal re-expands the bubble it just closed: unmounting
   the two buttons moves focus, the graph reflows under a stationary pointer,
   and either can fire a fresh hover within the same few frames. */
let hoverSuppressedUntil = 0;
const HOVER_DISMISS_GUARD_MS = 250;

/** The node the History card is anchored to, in canvas-box px. */
const historyAnchor = reactive({x: 0, y: 0});

function nodeById(id: NodeId | null): Node | null {
  return id ? state.graph[id] ?? null : null;
}

/** Re-run the layout for the current expanded node and animate into it. */
function reflow() {
  if (!Object.keys(state.graph).length) return;
  const target = expandedId.value === null
    ? restingPlacements
    : computeLayout(state.graph, expandedId.value);
  animateTo(state.graph, target, () => {
    /* The pan clamp works off the bubbles' bounding box, so it has to be told
       about the shape the graph settled into — otherwise a graph that grew
       under the hover could be panned by the old bound. */
    contentBox = bubbleBounds();
    /* ...and the clamp has to be RE-APPLIED, not just updated: d3 only runs
       `constrain` on a gesture, so a reflow that happens after the last one
       (hover a bubble near the edge, then stop moving) can leave the graph
       outside the bound it would now enforce, with nothing to pull it back
       until the user pans again. Re-running it here converges immediately and
       is a no-op whenever the transform already satisfies the bound. */
    if (svgRef.value && zoomBehavior && svgSel) {
      const t = zoomTransform(svgRef.value);
      const clamped = constrainToViewport(t, zoomExtent());
      if (clamped !== t) svgSel.call(zoomBehavior.transform as any, clamped);
    }
    updateHistoryAnchor();
  });
  updateHistoryAnchor();
}

/** Set (or clear) the hovered bubble, debounced against pointer thrash. */
function setHovered(id: NodeId | null, immediate = false) {
  if (hoverTimer !== null) { window.clearTimeout(hoverTimer); hoverTimer = null; }
  const apply = () => {
    hoverTimer = null;
    if (id !== null && performance.now() < hoverSuppressedUntil) return;
    if (hoveredId.value === id) return;
    if (openArticle.value) return;   // an open article owns the view
    hoveredId.value = id;
    reflow();
    const n = nodeById(id);
    if (n) announceToScreenReader(`${n.fullName || n.id}, ${n.contributors} contributor${n.contributors === 1 ? '' : 's'}`);
  };
  if (immediate || HOVER_DEBOUNCE_MS <= 0) apply();
  else hoverTimer = window.setTimeout(apply, HOVER_DEBOUNCE_MS);
}

/** Collapse everything: no hover, no open card, no History. */
function collapseAll() {
  if (hoverTimer !== null) { window.clearTimeout(hoverTimer); hoverTimer = null; }
  historyOpen.value = false;
  hoverSuppressedUntil = performance.now() + HOVER_DISMISS_GUARD_MS;
  /* Give up the focus as well, or the browser re-homes it on the bubble's own
     <g> when the buttons inside it are unmounted — which reads as a fresh
     keyboard hover and re-opens what was just dismissed. */
  const focused = typeof document !== 'undefined' ? document.activeElement : null;
  if (focused instanceof HTMLElement || focused instanceof SVGElement) {
    if (containerRef.value?.contains(focused)) focused.blur();
  }
  if (hoveredId.value === null) return;
  hoveredId.value = null;
  reflow();
}

/** Pointer/focus in and out of a bubble.

   TOUCH NEVER HOVERS. There is no hover on a touch screen — a touch pointer
   fires enter before the tap and leave after it, which is a side effect of the
   tap, not an intention — so a tap goes straight to the article
   (onBubbleClick). The 202px state is simply not reachable that way.

   The tap ALSO focuses the <g> (browsers focus a tabindex element on press),
   and focus is a hover here, so the tap would otherwise sneak the 202px state
   in through the keyboard path for a few frames before the article view
   covered it. Suppressing hover for the length of the gesture closes that
   door without giving up keyboard hover afterwards: the window is short and
   bounded, so a Tab later on behaves normally. */
function onBubbleHover(id: NodeId, on: boolean, pointerType: string) {
  if (pointerType === 'touch') {
    if (on) hoverSuppressedUntil = Math.max(hoverSuppressedUntil, performance.now() + TOUCH_TAP_GUARD_MS);
    return;
  }
  if (openArticle.value) return;   // the graph is not on screen to be hovered
  if (on) setHovered(id);
  else if (hoveredId.value === id) setHovered(null);
}

function onBubbleClick(n: Node) {
  // In compare mode, use compare selection logic instead
  if (isCompareMode.value) {
    onBubbleClickCompare(n);
    return;
  }

  /* One gesture, one outcome, on every input: this opens the article. A mouse
     has already had its hover on the way here and a keyboard its focus; a tap
     has had neither, and gets none — it opens the article on the FIRST tap
     (there is no useful "hover" for a finger to make). openDetail() flies the
     circle out of whatever size the bubble is on screen right now, so a tap
     starts from the resting ladder size and a click from the 202px hover, with
     no special case. */
  openDetail(n);

  const detail = getSelectionDetailFromNode(n);
  if (!detail) return;
  const payload = { ...detail };
  applySelection(n, payload);
  persistSelectionDetail(payload);
  announceToScreenReader(`Selected ${n.fullName || n.id} with ${n.contributors} contributor${n.contributors === 1 ? '' : 's'}`);
  window.dispatchEvent(new CustomEvent('repo:bubble-selected', { detail: payload }));
  window.dispatchEvent(new CustomEvent('repo:selection-updated', { detail: payload }));
}

/* ── THE OPENED ARTICLE (425px, centred) ──────────────────────────────────
   Clicking the hovered bubble opens the article on its own: ArticleDetailView
   draws it as a 425px circle in the middle of the canvas, flying out of the
   202px bubble that was clicked, and the graph stops being rendered behind it.
   Back (or Escape) reverses the flight and puts the graph back exactly as it
   was — the resting layout and the pan/zoom the user had. */

/** Diameter of the opened circle, in screen px. The design draws it at 425,
   but it is drawn INSIDE the canvas box, so it must also fit that box: on a
   narrow viewport or a short canvas it is capped to what the container can
   show (minus a margin), and the spacing inside it scales with it — see
   ArticleDetailView.vue. */
const DETAIL_DIAMETER_DESIGN = 425;   // design size of the opened circle (px)
const DETAIL_CONTAINER_MARGIN = 24;   // breathing room between circle and canvas edge
const DETAIL_DIAMETER_MIN = 200;      // below this the stack is unreadable anyway
/* How long the closing flight owns the view: ArticleDetailView's 120ms content
   fade + 80ms delay + 300ms travel, plus a frame. Nothing may grow a bubble
   while it is in the air. */
const DETAIL_CLOSE_GUARD_MS = 520;
const detailSize = ref(DETAIL_DIAMETER_DESIGN);
/** The bubble the view grew out of, in screen px — the 202px hovered one. */
const detailOrigin = ref<DetailOrigin | null>(null);
/** Set to ask ArticleDetailView for its closing animation; the teardown
   happens in finishDetailClose() when it reports back. */
const detailClosing = ref(false);
/** The pan/zoom the graph was at when the article opened, so Back restores the
   view the user left rather than re-fitting. */
let transformBeforeDetail: ZoomTransform | null = null;

function computeDetailSize(): number {
  const w = containerWidth || DEFAULT_CONTAINER_WIDTH;
  const h = svgHeight.value || graphViewportHeight();
  const fits = Math.min(DETAIL_DIAMETER_DESIGN, w - DETAIL_CONTAINER_MARGIN * 2, h - DETAIL_CONTAINER_MARGIN * 2);
  return Math.round(Math.max(DETAIL_DIAMETER_MIN, fits));
}

/** Where a node's bubble is on screen right now, for the open/close flight.
   Derived from the rendered placement and the live zoom transform rather than
   from the DOM, so it is exact and needs no lookup by id. */
function screenCircleFor(id: NodeId): DetailOrigin | null {
  const svg = svgRef.value;
  const p = framePlacements.get(id);
  if (!svg || !p) return null;
  const t = zoomTransform(svg);
  const box = svg.getBoundingClientRect();
  const r = p.r * t.k;
  if (!(r > 0)) return null;
  return { cx: box.left + t.applyX(p.x), cy: box.top + t.applyY(p.y), r };
}

function openDetail(n: Node) {
  if (detailNode.value?.id === n.id && !detailClosing.value) return;
  if (hoverTimer !== null) { window.clearTimeout(hoverTimer); hoverTimer = null; }
  cancelReflow();
  transformBeforeDetail = svgRef.value ? zoomTransform(svgRef.value) : null;
  detailSize.value = computeDetailSize();
  /* Fly out of whatever is on screen: the 202px hovered bubble if this came
     from a hover (mouse, keyboard, second tap), its resting size otherwise. */
  detailOrigin.value = screenCircleFor(n.id);
  detailClosing.value = false;   // re-opening mid-close cancels the close
  detailNode.value = n;
  historyOpen.value = false;
  nextTick(updateHistoryAnchor);
  announceToScreenReader(`Opened ${n.fullName || n.id}`);
}

/** Ask for the close. The view animates, then calls finishDetailClose(). */
function closeDetail() {
  const node = detailNode.value;
  if (!node || detailClosing.value) return;
  historyOpen.value = false;   /* the popup does not travel with the circle */
  /* Put the graph back NOW, behind the shrinking circle, and at rest: the
     circle is flying back into a bubble, so that bubble has to be there. */
  hoveredId.value = null;
  setFrame(state.graph, restingPlacements);
  contentBox = bubbleBounds();
  if (transformBeforeDetail && svgSel) {
    svgSel.call(zoomBehavior.transform as any, transformBeforeDetail);
    currentK.value = transformBeforeDetail.k;
  }
  /* WHERE THE CIRCLE LANDS, recomputed here rather than reused from the open.
     `detailOrigin` was captured when the article was opened — off the 202px
     HOVERED bubble — so the circle used to shrink to 202 and then be swapped
     for a 126px (or 22px) bubble underneath: "the size of the shrinking bubble
     just before it reveals the original bubble is a bit too big". Reading the
     resting frame now makes the whole close one continuous motion, 425 straight
     down to that bubble's own ladder diameter, ending exactly on it.

     Whether that bubble is hovered afterwards is a separate question, answered
     by the ordinary rules in finishDetailClose() — the animation passing over
     it is not an answer. */
  detailOrigin.value = screenCircleFor(node.id);
  /* Its label waits for the landing, so the resting count fades in at its rung
     size instead of being uncovered halfway through the flight. */
  labelFrozen.value = new Set([node.id]);
  /* Nothing may expand while the circle is on its way — including a pointer
     that happens to be resting over a bubble. That is re-read at the end. */
  hoverSuppressedUntil = performance.now() + DETAIL_CLOSE_GUARD_MS;
  detailClosing.value = true;
}

function finishDetailClose() {
  const closedId = detailNode.value?.id ?? null;
  detailNode.value = null;
  detailClosing.value = false;
  detailOrigin.value = null;
  historyOpen.value = false;
  transformBeforeDetail = null;
  /* The circle has landed on the bubble at its resting size and the graph is
     whole again: release the label (it fades in at its rung font) and drop the
     suppression, because from here the ordinary rules apply. */
  labelFrozen.value = new Set();
  hoverSuppressedUntil = 0;

  /* Should anything be hovered now? Ask the same question the pointer asks
     anywhere else, from live evidence only — never "the animation finished
     here, so hover this". Two honest sources: where the pointer actually is,
     and the focus a keyboard-driven close owes back to the bubble it opened. */
  const byKeyboard = closedByKeyboard;
  closedByKeyboard = false;
  nextTick(() => {
    if (byKeyboard && closedId) {
      /* Returning focus to the control that opened a view is standard; the
         `focusin` that follows expands the bubble exactly as a keyboard hover
         does anywhere else, rather than this function deciding it. */
      const el = svgRef.value?.querySelector(`g.node[data-node-id="${cssEscape(closedId)}"]`);
      (el as SVGGElement | null)?.focus?.();
      return;
    }
    const id = nodeUnderPointer();
    if (id !== null) setHovered(id, true);
  });
}

/** Which bubble the pointer is over right now, or null. A live hit test, not
   remembered state, so it cannot claim a hover the user is not making. */
function nodeUnderPointer(): NodeId | null {
  if (typeof document === 'undefined' || pointerClient.x < 0) return null;
  const el = document.elementFromPoint(pointerClient.x, pointerClient.y);
  const g = (el as Element | null)?.closest?.('g.node') as SVGGElement | null;
  const id = g?.getAttribute('data-node-id') ?? null;
  return id && state.graph[id] ? id : null;
}

/** CSS.escape with a fallback, for the attribute lookup above. */
function cssEscape(value: string): string {
  const api = (window as unknown as {CSS?: {escape?: (v: string) => string}}).CSS;
  return typeof api?.escape === 'function' ? api.escape(value) : value.replace(/["\\]/g, '\\$&');
}

/** "Read full article" inside the opened circle. */
function onDetailRead() {
  const n = openArticle.value;
  if (n) onBubbleView(n);
}

/* ── HISTORY POPUP ────────────────────────────────────────────────────────
   Kept from the detail view, and still the same component (desktop side card,
   bottom sheet under 768px). Only its anchor changed: it used to hang off a
   circle centred in the canvas box, and now follows the opened bubble. */

/** Ancestors of the opened node, oldest last — the lineage the design lists:
   the article itself, then "Fork of:" each parent up to the subject root. */
const historyEntries = computed<HistoryEntry[]>(() => {
  const start = openArticle.value;
  if (!start) return [];
  const out: HistoryEntry[] = [];
  let cur: Node | undefined = start;
  const guard = new Set<NodeId>();
  while (cur && !guard.has(cur.id)) {
    guard.add(cur.id);
    const owner = cur.repoOwner ?? cur.fullName?.split('/')[0] ?? '';
    const title = cur.repoSubject ?? cur.repoName ?? cur.fullName ?? cur.id;
    out.push({
      id: cur.id,
      title: owner ? `${owner} / ${title}` : title,
      isFork: out.length > 0,
      /* NOTE: there is no "point of contention" TEXT anywhere in the model
         today — the graph only marks contention as the junction dots between a
         fork and its parent, and the compare page names the two owners. The
         nearest real text is the fork's own repository description, so that is
         what is shown; a dedicated field would need a server-side change. */
      contention: cur.description,
      owner: owner || undefined,
      isCurrent: cur.id === start.id,
    });
    cur = cur.parentId ? state.graph[cur.parentId] : undefined;
  }
  return out;
});

/** The design expands the oldest ancestor — the original point of contention. */
const historyExpandedId = computed(() => {
  const entries = historyEntries.value;
  return entries.length ? entries[entries.length - 1].id : null;
});

/** Where the opened bubble is inside the canvas box, in px, for the History
   card to sit beside. Derived from the rendered placement and the live zoom
   transform, so it follows a pan.

   The Y is clamped to the middle 30% of the box and the card is limited to 70%
   of its height (see ArticleHistoryPopup), which together guarantee the card is
   inside the box whatever the bubble is doing near an edge. */
function updateHistoryAnchor() {
  const box = containerRef.value?.querySelector('.graph-container') as HTMLElement | null;
  if (!box) return;
  const boxRect = box.getBoundingClientRect();
  let x: number, y: number;
  if (openArticle.value) {
    /* The opened article is a circle centred in this box: the card hangs off
       its right edge, exactly as the design draws it. */
    const d = Math.min(detailSize.value, boxRect.width * 0.84);
    x = boxRect.width / 2 + d / 2;
    y = boxRect.height / 2;
  } else {
    const svg = svgRef.value;
    const p = hoveredId.value !== null ? framePlacements.get(hoveredId.value) : null;
    if (!svg || !p) return;
    const t = zoomTransform(svg);
    const svgBox = svg.getBoundingClientRect();
    x = (svgBox.left - boxRect.left) + t.applyX(p.x) + p.r * t.k;
    y = (svgBox.top - boxRect.top) + t.applyY(p.y);
  }
  historyAnchor.x = Math.round(Math.max(0, Math.min(boxRect.width, x)));
  historyAnchor.y = Math.round(Math.max(boxRect.height * 0.35, Math.min(boxRect.height * 0.65, y)));
}

/* The circle is sized from the container, and on a solo subject nothing
   "opens" it — it is simply there once the data lands. Size it whenever an
   article appears, after the DOM has settled so the canvas box is measured. */
watch(openArticle, async (article) => {
  if (!article) return;
  await nextTick();
  detailSize.value = computeDetailSize();
  updateHistoryAnchor();
}, {immediate: true});

function onHistoryOpen() {
  historyOpen.value = true;
  updateHistoryAnchor();
}

/** "View full history" in the card: the repository's commit log. */
function onDetailFullHistory() {
  const n = openArticle.value;
  if (!n) return;
  const owner = n.repoOwner ?? n.fullName?.split('/')[0] ?? '';
  const repo = n.repoName ?? n.fullName?.split('/')[1] ?? '';
  if (!owner || !repo) return;
  const suburl = window.config?.suburl || '';
  /* WITH A REF. A bare /commits is only absorbed by the `m.Get("/*", ...)`
     catch-all in routers/web/web.go, which is annotated "deprecated, and kept
     for backward compatibility" — every commits link in templates/ carries a
     ref, and so does this one. */
  const branch = props.defaultBranch;
  const base = `${suburl}/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/commits`;
  window.location.href = branch ? `${base}/branch/${encodeURIComponent(branch)}` : base;
}

/** Escape backs out exactly one level each time: the History card, then the
   opened article (same as Back), then the hovered bubble.

   On a solo subject it stops after the History card: `detailNode` is null there
   (nothing was clicked), so there is no close to run — which is the point.
   Escape must not be able to leave that user staring at an empty canvas. */
function onGraphKeydown(ev: KeyboardEvent) {
  if (ev.key !== 'Escape') return;
  if (historyOpen.value) historyOpen.value = false;
  else if (detailNode.value) { closedByKeyboard = true; closeDetail(); }
  else if (expandedId.value !== null) collapseAll();
}

function onBubbleView(n: Node) {
  const detail = getSelectionDetailFromNode(n);
  if (!detail) return;
  const payload = { ...detail };
  applySelection(n, payload);
  persistSelectionDetail(payload);
  window.dispatchEvent(new CustomEvent('repo:selection-updated', { detail: payload }));
  window.dispatchEvent(new CustomEvent('repo:bubble-open-article', { detail: payload }));
}

/* Click handler for joint-parent: navigate to fork comparison page */
function onJointClick(joint: { sourceOwner: string; targetOwner: string; subject: string }) {
  if (!joint.subject || !joint.sourceOwner || !joint.targetOwner) return;
  const suburl = window.config?.suburl || '';
  const compareUrl = `${suburl}/subject/${encodeURIComponent(joint.subject)}/compare/${encodeURIComponent(joint.sourceOwner)}...${encodeURIComponent(joint.targetOwner)}`;
  window.location.href = compareUrl;
}

/* ──────────────────────────────────────────────────────────────────────────────
   COMPARE MODE HANDLERS
   ─────────────────────────────────────────────────────────────────────────── */

/* Toggle compare mode on/off */
function toggleCompareMode() {
  isCompareMode.value = !isCompareMode.value;
  if (!isCompareMode.value) {
    // Exiting compare mode: clear selections and close popup
    compareSelection.value = [];
    showComparePopup.value = false;
  }
  announceToScreenReader(isCompareMode.value ? 'Compare mode activated. Select two articles to compare.' : 'Compare mode deactivated.');
}

/* Handle compare mode toggle from external event (header button) */
function handleCompareModeToggle() {
  toggleCompareMode();
}

/* Handle bubble click in compare mode */
function onBubbleClickCompare(n: Node) {
  const existingIdx = compareSelection.value.findIndex(node => node.id === n.id);

  if (existingIdx !== -1) {
    // Node already selected: remove it
    compareSelection.value.splice(existingIdx, 1);
    showComparePopup.value = false;
    announceToScreenReader(`Deselected ${n.fullName || n.id}. ${compareSelection.value.length} article${compareSelection.value.length === 1 ? '' : 's'} selected.`);
  } else if (compareSelection.value.length < 2) {
    // Add node to selection
    compareSelection.value.push(n);

    if (compareSelection.value.length === 2) {
      // Two nodes selected: show popup
      showComparePopup.value = true;
      announceToScreenReader('Two articles selected. Compare popup opened.');
    } else {
      announceToScreenReader(`Selected ${n.fullName || n.id}. Select one more article to compare.`);
    }
  }
}

/* Close compare popup */
function closeComparePopup() {
  showComparePopup.value = false;
}

/* Navigate to comparison page */
function goToComparison() {
  if (compareSelection.value.length !== 2) return;

  const [first, second] = compareSelection.value;
  const subject1 = first.repoSubject || props.subject || '';
  const subject2 = second.repoSubject || props.subject || '';

  // Validate both articles have the same subject
  if (subject1 !== subject2) {
    console.warn('FishboneGraph: cannot compare articles from different subjects');
    announceToScreenReader('Cannot compare articles from different subjects.');
    return;
  }

  const subject = subject1;
  const owner1 = first.repoOwner || first.fullName?.split('/')[0] || '';
  const owner2 = second.repoOwner || second.fullName?.split('/')[0] || '';

  if (!subject || !owner1 || !owner2) {
    console.warn('FishboneGraph: missing data for comparison URL');
    announceToScreenReader('Unable to compare: missing article information.');
    return;
  }

  const suburl = window.config.suburl || '';
  const compareUrl = `${suburl}/subject/${encodeURIComponent(subject)}/compare/${encodeURIComponent(owner1)}...${encodeURIComponent(owner2)}`;
  window.location.href = compareUrl;
}
</script>

<template>
  <div class="f-fishbone-graph" ref="containerRef">
    <!-- Screen reader announcements -->
    <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ srAnnouncement }}</div>
    <div class="mx-auto max-w-[1100px]">
      <!-- Controls removed; using defaults -->

      <!-- Graph container with relative positioning for overlays -->
      <div class="graph-container">
        <!-- SVG world: IMPORTANT → touch-action:none enables pinch zoom; d3 handles it -->
        <!-- SVG is always rendered to keep refs valid -->
        <svg
          ref="svgRef" class="tw-w-full" :class="{ 'graph-hidden': isLoading || errorMessage || !hasData }"
          :style="{ height: svgHeight + 'px' }" style="touch-action: none;" role="img"
          aria-label="Fork repository graph showing contributors and relationships" tabindex="0"
        >
          <defs>
            <!-- Soft radial bubble gradient -->
            <radialGradient id="bubbleGrad" cx="35%" cy="30%" r="65%">
              <stop offset="0%" class="bubble-grad-start"/>
              <stop offset="60%" class="bubble-grad-mid"/>
              <stop offset="100%" class="bubble-grad-end"/>
            </radialGradient>
            <filter id="softShadow" x="-50%" y="-50%" width="200%" height="200%">
              <feDropShadow dx="0" dy="2" stdDeviation="3" class="bubble-shadow"/>
            </filter>
          </defs>

          <!-- WORLD GROUP: Vue renders here, and d3-zoom transforms this exact
               <g>. Its CONTENTS are v-if'd on `graphRendered`: while an article
               is open there is no bubble and no connector in the document at
               all — the graph is hidden, not merely covered. The <g> itself
               always exists, so the d3 selection taken on mount stays valid. -->
          <g ref="worldRef">
            <template v-if="graphRendered">
              <!-- Trunks (vertical) -->
              <line
                v-for="t in trunksList" :key="t.id" class="trunk" :x1="t.x" :x2="t.x" :y1="t.y1" :y2="t.y2"
                stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round"
              />

              <!-- Branch elbows + runs (one path per edge). `data-edge` pairs a
                 path with the joint dot sitting on it, for the harness check
                 that a dot is ON its connector in EVERY frame of a reflow. -->
              <path
                v-for="e in edgesList" :key="`${e.source.node.id}-${e.target.node.id}`"
                :data-edge="`${e.source.node.id}-${e.target.node.id}`" class="branch" fill="none"
                stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round" opacity="0.9"
                :d="`M ${e.ex} ${e.ey} C ${e.ex} ${e.ey + 0.5522847498307936 * state.elbowR}, ${e.ex + e.side * 0.5522847498307936 * state.elbowR} ${e.hy}, ${e.hx} ${e.hy} L ${e.cx} ${e.cy}`"
              />

              <!-- Child stems -->
              <line
                v-for="e in edgesList" :key="`stem-${e.source.node.id}-${e.target.node.id}`" class="child-stem" :x1="e.sx1"
                :y1="e.sy1" :x2="e.sx2" :y2="e.sy2" stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round"
                opacity="0.9"
              />

              <!-- Joint dots (hollow rings) on trunk side - clickable to compare forks -->
              <circle
                v-for="j in jointDots" :key="`joint-${j.id}`" :data-edge="j.id" class="joint-parent"
                :cx="j.x" :cy="j.y" r="6"
                fill="var(--bubble-joint-fill)" stroke="var(--bubble-joint-stroke)" stroke-width="2"
                style="cursor: pointer;"
                role="button" tabindex="0" :aria-label="`Compare ${j.sourceOwner} with ${j.targetOwner}`"
                @click.stop="() => onJointClick(j)" @keydown.enter.stop="() => onJointClick(j)"
                @keydown.space.stop="() => onJointClick(j)"
              />

              <!-- Bubbles (component handles labels independently). Every
                 coordinate and radius comes from the current frame, so a
                 bubble mid-reflow and the rib attached to it agree. -->
              <BubbleNode
                v-for="f in nodesList" :key="f.node.id" :id="f.node.id" :x="f.x" :y="f.y"
                :r="f.r" :contributors="f.node.contributors" :updated-at="f.node.updatedAt"
                :description="f.node.description" :k="kComputed"
                :detail="detailFor(f.node.contributors)"
                :count-text="countTextFor(f.node.contributors)"
                :count-font-size="countFontFor(f.node.contributors)"
                :expanded="expandedId === f.node.id" :frozen="labelFrozen.has(f.node.id)"
                :is-active="selectedNodeId === f.node.id" :is-compare-mode="isCompareMode"
                :compare-state="getCompareState(f.node.id)"
                @click="() => onBubbleClick(f.node)" @hover="(id, on, pt) => onBubbleHover(id, on, pt)"
              />
            </template>
          </g>
        </svg>

        <!-- State overlays positioned on top of SVG only -->
        <!-- Loading State -->
        <div v-if="isLoading" class="state-overlay loading-state">
          <svg
            class="tw-w-full" viewBox="0 0 1100 400" preserveAspectRatio="xMidYMid meet" role="img"
            aria-label="Loading fork graph"
          >
            <defs>
              <radialGradient id="loadingBubbleGrad" cx="35%" cy="30%" r="65%">
                <stop offset="0%" class="bubble-grad-start"/>
                <stop offset="60%" class="bubble-grad-mid"/>
                <stop offset="100%" class="bubble-grad-end"/>
              </radialGradient>
            </defs>
            <!-- Centered at 50% of viewBox (550, 200) -->
            <g transform="translate(550, 200)">
              <circle
                r="80" fill="url(#loadingBubbleGrad)" stroke="var(--bubble-stroke)" stroke-width="1.2" opacity="0.7"
                class="pulse-animation"
              />
              <text
                text-anchor="middle" dominant-baseline="central" fill="var(--bubble-shadow-color)" font-size="16"
                font-weight="500"
              >Loading...</text>
            </g>
          </svg>
        </div>

        <!-- Error State -->
        <div v-else-if="errorMessage" class="state-overlay error-state">
          <div class="state-message">
            <svg class="state-icon error-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <h3 class="state-title">Failed to Load Fork Graph</h3>
            <p class="state-description">{{ errorMessage }}</p>
            <button class="state-button" @click="fetchForkGraphAndSet">Try Again</button>
          </div>
        </div>

        <!-- Empty State -->
        <CreateFirstArticleBubble
          v-if="!hasData" :owner="props.owner" :repo="props.repo" :subject="props.subject"
          :default-branch="props.defaultBranch"
        />

        <!-- The opened article (#284): one article, 425px, centred, with its
             excerpt and its actions. It MUST stay inside .graph-container: the
             layer is position:absolute/inset:0 with an opaque background, so
             anywhere else it resolves against a positioned ancestor further up
             the page and paints over the page chrome (navbar, view tabs,
             Compare button, legend). Inside the container it covers exactly the
             canvas box and nothing else, and the <svg> stays mounted underneath
             so the box keeps its height. -->
        <ArticleDetailView
          v-if="openArticle"
          :contributors="openArticle.contributors" :description="openArticle.description"
          :updated-at="openArticle.updatedAt" :size="detailSize"
          :origin="soloPinned ? null : detailOrigin" :show-back="!soloPinned"
          :closing="detailClosing"
          @back="closeDetail" @read="onDetailRead" @history="onHistoryOpen"
          @closed="finishDetailClose"
        />

        <!-- History card (#284). It MUST stay inside .graph-container: the
             desktop card is position:absolute and resolves against the nearest
             positioned ancestor, and anywhere else it would paint over the page
             chrome. It anchors to the OPENED bubble (--history-anchor-*) and
             clamps itself to this box; under 768px it is a bottom sheet
             instead, which is fixed to the viewport by design. -->
        <!-- The wrapper carries the anchor: ArticleHistoryPopup has two root
             elements (backdrop + card), so a style binding on the component
             itself would have nowhere to land. It is a zero-height static
             block, so it adds nothing to the box. -->
        <div
          v-if="historyOpen && (openArticle || hoveredId)" class="history-anchor"
          :style="{ '--history-anchor-x': historyAnchor.x + 'px', '--history-anchor-y': historyAnchor.y + 'px' }"
        >
          <ArticleHistoryPopup
            :entries="historyEntries" :expanded-id="historyExpandedId"
            @close="historyOpen = false" @view-full-history="onDetailFullHistory"
          />
        </div>
      </div>
      <!-- End graph-container -->

      <div ref="legendRef">
        <LegendFishbone v-if="hasData"/>
      </div>

      <!-- Compare Popup Modal -->
      <ArticleComparePopup
        v-if="showComparePopup && compareSelection.length === 2" :articles="compareSelection"
        :subject="props.subject || ''" @close="closeComparePopup" @compare="goToComparison"
      />
    </div>
  </div>
</template>

<style scoped>
.f-fishbone-graph {
  width: 100%;
  /* Fill the box #bubble-view-root is given by the page's flex layout (see
     web_src/css/features/bubble-graph.css). "flex-basis: 0" plus
     "min-height: 0" keeps the height coming from the free space rather than
     from the canvas this component sizes off that very height, and any
     leftover (the legend under a min-height canvas) scrolls in here rather
     than growing the page past the footer. Outside a flex parent this falls
     back to an auto height, which is the pre-#149 behaviour minus the
     "calc(100vh - 25rem)" guess. */
  flex: 1 1 0;
  min-height: 0;
  overflow: auto;
}

.f-fishbone-graph svg:focus {
  outline: none;
}

/* Graph container for relative positioning of overlays */
.graph-container {
  position: relative;
}

/* Carries the History card's anchor variables and nothing else. */
.history-anchor {
  height: 0;
}

/* The canvas must be block-level: an inline <svg> also reserves a few px of
   baseline descender space underneath it, which is enough to push the legend
   past the fold and give the (overflow:auto) graph box a scrollbar. */
.graph-container > svg {
  display: block;
}

/* Hide graph content when showing states, but keep SVG rendered */
.graph-hidden {
  visibility: hidden;
  pointer-events: none;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

/* State overlays */
.state-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
  padding: 2rem;
  pointer-events: none;
  /* Allow clicks to pass through to tabs */
}

/* Loading state is transparent and non-blocking */
.loading-state {
  background-color: transparent;
}

/* Error state has opaque background and blocks interaction */
.error-state {
  background-color: var(--bubble-overlay-bg);
  pointer-events: auto !important;
  /* Re-enable interaction for buttons */
}

.state-message {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  max-width: 400px;
}

.state-icon {
  width: 64px;
  height: 64px;
  margin-bottom: 1.5rem;
}

.error-icon {
  color: var(--color-red);
}

.state-title {
  font-size: 1.5rem;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 0.5rem 0;
}

.state-description {
  font-size: 1rem;
  color: var(--color-text-secondary);
  margin: 0 0 1.5rem 0;
  line-height: 1.5;
}

.state-button {
  padding: 0.625rem 1.25rem;
  background-color: var(--color-primary, #2563eb);
  color: white;
  border: none;
  border-radius: 0.375rem;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.2s ease;
}

.state-button:hover {
  background-color: var(--color-primary-dark, #1d4ed8);
}

.state-button:active {
  transform: scale(0.98);
}

.state-button:focus {
  outline: 2px solid var(--color-primary, #2563eb);
  outline-offset: 2px;
}

/* Loading animation */
@keyframes pulse {

  0%,
  100% {
    opacity: 0.7;
    transform: scale(1);
  }

  50% {
    opacity: 0.9;
    transform: scale(1.05);
  }
}

.pulse-animation {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

/* Joint-parent hover/focus effects for clickable points of contention.

   THE TRANSITION IS PER-PROPERTY, NEVER `all`. `cx`/`cy` are CSS-animatable
   geometry properties in Chrome, so an `all 0.15s ease` here (which is what
   this element used to carry inline) gave every per-frame position the JS
   reflow tween writes its OWN 150ms eased animation on top. The dots then
   trailed the ribs they sit on by up to 30px mid-flight and kept drifting for
   ~200ms after the layout had settled — "late and doesn't seem attached to the
   line". The connectors themselves have no transition, which is why only the
   dots came adrift. Colour and stroke may still animate: they are not
   geometry, and they are the whole point of the hover affordance. */
.joint-parent {
  transition: fill 0.15s ease, stroke 0.15s ease, stroke-width 0.15s ease;
}

.joint-parent:hover {
  stroke: var(--color-primary, #2563eb) !important;
  stroke-width: 3 !important;
  fill: var(--color-primary-alpha-10) !important;
}

.joint-parent:focus {
  outline: none;
  stroke: var(--color-primary, #2563eb) !important;
  stroke-width: 3 !important;
  fill: var(--color-primary-alpha-20) !important;
}

.bubble-grad-start {
  stop-color: var(--bubble-grad-start);
}

.bubble-grad-mid {
  stop-color: var(--bubble-grad-mid);
}

.bubble-grad-end {
  stop-color: var(--bubble-grad-end);
}

.bubble-shadow {
  flood-color: var(--bubble-shadow-color);
  flood-opacity: 0.18;
}
</style>
