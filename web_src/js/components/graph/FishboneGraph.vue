<script setup lang="ts">
/* FishboneGraph.vue
   Interactive fork repository graph with fishbone layout.
   
   Key features:
   - Transforms the <g> group (ref="worldRef") that Vue renders into
   - Sets `touch-action: none` on the <svg> for pinch zoom on touch devices
   - All interactions (pan/zoom/click/background reset) wired through d3-zoom
   - Responsive auto-tuning based on container size and graph complexity
   - Accessibility support with ARIA labels and keyboard navigation */

import { onMounted, reactive, ref, onBeforeUnmount, nextTick, computed } from "vue";
// @ts-ignore - d3-selection types may not be available in CI environment
import { select } from "d3-selection";
// @ts-ignore - d3-selection types may not be available in CI environment
import type { Selection } from "d3-selection";
// @ts-ignore - d3-zoom types may not be available in CI environment
import { zoom, zoomIdentity } from "d3-zoom";
// @ts-ignore - d3-zoom types may not be available in CI environment
import type { ZoomBehavior, ZoomTransform } from "d3-zoom";

import LegendFishbone from "./FishboneLegend.vue";
import BubbleNode from "./BubbleNode.vue";
import CreateFirstArticleBubble from "./CreateFirstArticleBubble.vue";
import ArticleComparePopup from "./ArticleComparePopup.vue";
import { bubbleLabelDetailFor, bubbleRadiusFor, singleArticleScreenDiameter } from "./bubble-size.ts";

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
  isEmpty?: boolean;
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
   Bubble radii are NOT computed here: they are snapped to one of four
   predefined tiers defined in ./bubble-size.ts (issue #284). That module is
   the single place where sizes and their contributor thresholds live. */

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

/* === VIEW RESET PARAMETERS === */
const RESET_TOP_MARGIN = 40;    // Minimum top margin when resetting the view
/* NOTE: a MAX_REF_DROP constant used to cap how far a child could be pushed
   down (baseY + 130). It was applied AFTER collision resolution and therefore
   silently threw the result away, which is what made bubbles overlap once the
   tiered radii from ./bubble-size.ts made them larger than the cap. Vertical
   room is now bounded by the zoom-fit in resetView(), not by a magic cap. */

/* === RESPONSIVE BREAKPOINTS & FACTORS === */
const WIDTH_BREAKPOINT_MIN = 480;    // Minimum container width for responsive calculations
const WIDTH_BREAKPOINT_MAX = 1200;   // Maximum container width for responsive calculations
const HEIGHT_BREAKPOINT_MIN = 640;   // Minimum container height for radius scaling
const HEIGHT_BREAKPOINT_MAX = 1080;  // Maximum container height for radius scaling
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

/* === RADIUS SCALING (for vertical fit without scrolling) === */
const RADIUS_HEIGHT_MIN_FACTOR = 0.7;     // Minimum height-based radius multiplier
const RADIUS_HEIGHT_MAX_FACTOR = 1.0;     // Maximum height-based radius multiplier
// Calculate range from min to max for readability
const RADIUS_HEIGHT_RANGE_FACTOR = RADIUS_HEIGHT_MAX_FACTOR - RADIUS_HEIGHT_MIN_FACTOR;
const RADIUS_FORK_MAX_REDUCTION = 0.20;   // Maximum reduction from fork count (20%)
const RADIUS_FORK_STEP = 0.02;            // Reduction per fork (2% per fork)
const RADIUS_MIN_SCALE = 0.65;            // Minimum overall radius scale to avoid over-shrinking

/* === VIEW FITTING (reset/focus zoom calculations) === */
const FILL_FRACTION_MIN = 0.55;      // Minimum horizontal fill fraction for few forks
const FILL_FRACTION_MAX = 0.90;      // Maximum horizontal fill fraction for many forks
const VERTICAL_FILL_FRACTION = 0.86; // Vertical fill fraction of usable height
const FOCUS_PADDING = 24;            // Padding when focusing on a single node
/* Single-article on-screen sizing lives in ./bubble-size.ts
   (singleArticleScreenDiameter) so radius and zoom targets stay in sync. */

/* === SVG LAYOUT === */
const MIN_SVG_HEIGHT = 320;          // Never collapse the canvas below this
const CONTENT_BOUNDS_EXTRA = 16;     // Extra horizontal padding for elbow overhang
const VIEW_TOP_OFFSET = 12;          // Top offset for view calculations
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
  /* True when the subject holds a single article (no forks): its bubble is
     pinned to the smallest tier instead of being sized by contributors. */
  isSingleArticle: false,
  /* Additional global attenuation to reduce bubble sizes for small screens / many forks */
  radiusScale: 1,
});

/* Derived arrays used for Vue rendering (instead of D3 joins) */
type EdgeGeom = {
  source: Node; target: Node; side: Side;
  ex: number; ey: number; hx: number; hy: number; cx: number; cy: number; sx1: number; sy1: number; sx2: number; sy2: number;
};
const nodesList = ref<Node[]>([]);
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

async function fetchForkGraphAndSet() {
  // Set loading state at the start
  isLoading.value = true;
  errorMessage.value = null;

  try {
    if (!props.apiUrl) {
      console.warn('FishboneGraph: apiUrl not provided');
      errorMessage.value = 'No API URL provided';
      isLoading.value = false;
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
      announceToScreenReader(errorText);
      return;
    }
    const json = await res.json();
    const graph = buildGraphFromApi(json?.root);
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
      announceToScreenReader('No fork data available');
    }
  } catch (err) {
    const errorText = err instanceof Error ? err.message : 'Failed to load fork graph';
    console.error('FishboneGraph: failed to fetch graph', err);
    errorMessage.value = errorText;
    isLoading.value = false;
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

    // If repository is not empty but contributors shows 0, it means stats are still generating
    // In this case, we know there's at least 1 contributor (the person who created the content)
    if (!isEmpty && contributors === 0) {
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
      isEmpty: isEmpty,
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

/* Radius for a bubble with `n` contributors.
   Sizes come from the four-tier table in ./bubble-size.ts (absolute
   contributor thresholds, so a bubble does not resize when siblings appear).
   `radiusScale` is the uniform viewport attenuation applied on top. */
function rFor(n: number) {
  return bubbleRadiusFor(n, {
    singleArticle: state.isSingleArticle,
    scale: state.radiusScale || 1,
  });
}

/* What a bubble of this size tier is meant to say. Paired with rFor(): the
   same tier decides both, so size and content can never disagree. */
function detailFor(n: number) {
  return bubbleLabelDetailFor(n, { singleArticle: state.isSingleArticle });
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
  const ch = containerHeight || DEFAULT_CONTAINER_HEIGHT;

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

  // Compute a gentle attenuation for bubble sizes to improve vertical fit without scrolling
  // Height factor: smaller screens → smaller bubbles
  const heightNorm = Math.min(1, Math.max(0, (ch - HEIGHT_BREAKPOINT_MIN) / (HEIGHT_BREAKPOINT_MAX - HEIGHT_BREAKPOINT_MIN)));
  const heightFactor = RADIUS_HEIGHT_MIN_FACTOR + RADIUS_HEIGHT_RANGE_FACTOR * heightNorm;
  // Fork factor: more forks → smaller bubbles
  const forksFactor = 1 - Math.min(RADIUS_FORK_MAX_REDUCTION, forks * RADIUS_FORK_STEP);
  // Combine and clamp to avoid over-shrinking
  state.radiusScale = Math.max(RADIUS_MIN_SCALE, Math.min(1, heightFactor * forksFactor));
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

  const nx = n.x ?? 0, ny = n.y ?? 0, nr = rFor(n.contributors);
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
    const cr = rFor(c.contributors);
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

function layoutFishbone(g: Graph) {
  const nodeCount = Object.keys(g).length;
  if (nodeCount === 0) {
    nodesList.value = [];
    edgesList.value = [];
    trunksList.value = [];
    jointDots.value = [];
    return;
  }

  computeDepths(g);
  const root: any = getRoot(g);
  // Note: root is guaranteed to exist here since we checked nodeCount above
  root.x = 0; root.y = 0;
  layoutSubtree(g, root);

  // Prepare arrays for Vue rendering
  nodesList.value = Object.values(g) as any;

  const links = nodesList.value
    .filter(n => n.parentId && g[n.parentId])
    .map(n => ({ source: (g as any)[n.parentId!], target: n }));
  const R = state.elbowR;
  const edges = links.map(l => {
    const side: Side = (l.target.x! >= l.source.x!) ? +1 : -1;
    const ex = l.source.x!, ey = l.target.y! - R, hx = ex + side * R, hy = l.target.y!;
    const rt = rFor(l.target.contributors), cx = l.target.x! - side * (rt + STEM_LEN_CHILD), cy = hy;
    const sx1 = l.target.x! - side * rt, sy1 = hy, sx2 = cx, sy2 = cy;
    return { source: l.source, target: l.target, side, ex, ey, hx, hy, cx, cy, sx1, sy1, sx2, sy2 };
  });
  edgesList.value = edges;

  trunksList.value = nodesList.value.filter(n => n.children.length > 0).map(n => {
    const rs = rFor(n.contributors);
    const yStart = n.y! + rs + STEM_LEN_PARENT;
    const ys = n.children.map(id => g[id]).filter((c): c is Node => c !== undefined).map(c => (c as any).y! - R);
    const y2 = Math.max(yStart, ...ys);
    return { x: n.x!, y1: n.y! + rs, y2, id: n.id };
  });

  jointDots.value = edges.map(e => ({
    x: e.ex,
    y: e.ey,
    id: `${e.source.id}-${e.target.id}`,
    sourceOwner: e.source.repoOwner || e.source.fullName?.split('/')[0] || '',
    targetOwner: e.target.repoOwner || e.target.fullName?.split('/')[0] || '',
    subject: e.source.repoSubject || e.target.repoSubject || props.subject || '',
  }));

  /* The canvas is the VIEWPORT, not the content: resetView() zoom-fits the
     world into it. It used to be sized from the lowest bubble in WORLD units
     (`maxY + 240`), which ignored the zoom factor and left the graph stranded
     at the top of an over-tall, scrolling canvas — the empty band under the
     first bubble. */
  svgHeight.value = graphViewportHeight();
}

/* ─────────────────────────────────────────────────────────────────────────────-
   VIEW FITTING (responsive reset + tiny-graph elegance)
   ─────────────────────────────────────────────────────────────────────────── */
function contentBounds() {
  if (nodesList.value.length === 0) {
    return { minX: 0, maxX: 100, minY: 0, maxY: 100 };
  }
  const minX = Math.min(...nodesList.value.map(n => (n.x ?? 0) - rFor(n.contributors)));
  const maxX = Math.max(...nodesList.value.map(n => (n.x ?? 0) + rFor(n.contributors)));
  const minY = Math.min(...nodesList.value.map(n => (n.y ?? 0) - rFor(n.contributors)));
  const maxY = Math.max(...nodesList.value.map(n => (n.y ?? 0) + rFor(n.contributors)));
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
    const r = rFor(n.contributors);
    xs0.push((n.x ?? 0) - r); xs1.push((n.x ?? 0) + r);
    ys0.push((n.y ?? 0) - r); ys1.push((n.y ?? 0) + r);
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
  const min = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, fitScale * ZOOM_OUT_FIT_FRACTION));
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
  svgHeight.value = viewportH;   // the legend may have appeared since the layout ran
  const usableH = viewportH - VIEW_TOP_OFFSET;

  const forks = forkCount(state.graph);
  const b = contentBounds();
  const contentW = b.maxX - b.minX, contentH = b.maxY - b.minY;

  // Validate bounds
  if (!isFinite(contentW) || !isFinite(contentH) || !isFinite(b.minX) || !isFinite(b.minY)) {
    return;
  }

  // Calculate fill fraction based on number of forks (more forks = fill more of viewport)
  const fillFrac = FILL_FRACTION_MIN + (FILL_FRACTION_MAX - FILL_FRACTION_MIN) * Math.min(1, forks / COMPLEXITY_THRESHOLD);

  // Calculate scale to fit content horizontally and vertically
  const scaleW = (box.width * fillFrac) / Math.max(1, contentW);
  const scaleH = (usableH * VERTICAL_FILL_FRACTION) / Math.max(1, contentH);

  let targetScale = Math.min(scaleW, scaleH);
  /* Special handling for a subject with a SINGLE article: there is nothing to
     fit against, so pin the lone bubble to a modest on-screen diameter.
     Previously this applied to graphs with up to one fork and targeted
     220-480px, which blew the bubble up to ~40% of the viewport (issue #284).
     Graphs that already have a fork now go through the normal fit path. */
  if (state.isSingleArticle) {
    const root = getRoot(state.graph);
    if (root) {
      const r = rFor(root.contributors);
      const sBubble = singleArticleScreenDiameter(box.width) / (2 * r);
      // `min` (not `max`): never zoom past the target just to fill the canvas.
      targetScale = Math.min(ZOOM_MAX, sBubble);
    }
  }

  // Center horizontally
  const cx = box.width / 2;
  const worldCenterX = (b.minX + b.maxX) / 2;
  const tx = cx - (worldCenterX * targetScale);
  /* Center vertically: distribute the leftover height above and below the
     content instead of pinning it to the top (which left a large dead area
     under a small graph, most visibly under a lone bubble). RESET_TOP_MARGIN
     is now a floor, used when the content is as tall as the viewport. */
  const scaledContentH = contentH * targetScale;
  const topSpace = Math.max(RESET_TOP_MARGIN, (usableH - scaledContentH) / 2);
  const ty = VIEW_TOP_OFFSET + topSpace - (b.minY * targetScale);

  // Validate transform values before applying
  if (!isFinite(tx) || !isFinite(ty) || !isFinite(targetScale)) {
    return;
  }

  /* Record the fit before applying it: the zoom-out floor is relative to it,
     and the constraint needs the bounds that go with the current layout. */
  fitScale = targetScale;
  contentBox = bubbleBounds();
  applyScaleExtent();

  /* d3 does not run `constrain` on a transform set directly, so honour the pan
     bound here. Centred content satisfies it anyway; this keeps the two paths
     from ever disagreeing. */
  const t = constrainToViewport(zoomIdentity.translate(tx, ty).scale(targetScale), zoomExtent());
  (animated ? svgSel.transition().duration(VIEW_TRANSITION_DURATION) : svgSel).call(zoomBehavior.transform as any, t);

  currentK.value = targetScale;
}

/* Click focus: center selected bubble and fit fully */
function focusNode(n: Node) {
  /* Note: also applied to worldSel via zoomBehavior, so it now works. */
  const svg = svgRef.value!;
  const box = svg.getBoundingClientRect();
  // Same reason as in resetView(): fit against the measured viewport.
  const usableH = graphViewportHeight() - VIEW_TOP_OFFSET;
  const r = rFor(n.contributors);

  // Calculate scale to fit the bubble with padding
  const sx = (box.width - 2 * FOCUS_PADDING) / (2 * r);
  const sy = (usableH - 2 * FOCUS_PADDING) / (2 * r);
  const scale = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.min(sx, sy)));

  // Center the node in the viewport
  const cx = box.width / 2;
  const cy = VIEW_TOP_OFFSET + usableH / 2;
  const tx = cx - (n.x! * scale);
  const ty = cy - (n.y! * scale);
  /* Same bound as a manual pan: focusing a bubble at the edge of the graph
     frames it as closely as the clamp allows rather than pulling the rest of
     the graph off screen (and then snapping back on the next gesture). */
  const t = constrainToViewport(zoomIdentity.translate(tx, ty).scale(scale), zoomExtent());
  svgSel.transition().duration(VIEW_TRANSITION_DURATION).call(zoomBehavior.transform as any, t);
  currentK.value = scale;
}

/* ─────────────────────────────────────────────────────────────────────────────-
   RENDER PIPELINE (layout→derive arrays→Vue renders)
   ─────────────────────────────────────────────────────────────────────────── */
function layoutAndRender() {
  // A lone article (root with no forks) is pinned to the smallest bubble tier;
  // must be decided before any rFor() call. See ./bubble-size.ts.
  state.isSingleArticle = Object.keys(state.graph).length <= 1;
  applyResponsiveDials();          // adapt dials first
  layoutFishbone(state.graph);     // compute x,y and derive edges/trunks/lists
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
    });

  svgSel.call(zoomBehavior as any);

  /* Background click (outside any bubble) → reset; if on true background (svg), also clear selection */
  svgSel.on("click.bg", (ev: any) => {
    const target = ev.target as Element;
    if (!target.closest("g.node")) {
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
  ro = new ResizeObserver((entries) => {
    const rect = entries[0].contentRect;
    const w = rect.width;
    const h = rect.height;
    let changed = false;
    if (Math.abs(w - containerWidth) > 2) { containerWidth = Math.min(w, 1100); changed = true; }
    if (Math.abs(h - containerHeight) > 2) { containerHeight = h; changed = true; }
    if (changed) {
      if (pendingRaf !== null) cancelAnimationFrame(pendingRaf);
      pendingRaf = requestAnimationFrame(() => {
        layoutAndRender();
        resetView();
        pendingRaf = null;
      });
    }
  });
  ro.observe(el);

  /* Initial fetch from API */
  await fetchForkGraphAndSet();
  window.addEventListener('repo:selection-updated', handleExternalSelection as EventListener);
  window.addEventListener('repo:compare-mode-toggle', handleCompareModeToggle as EventListener);
});

onBeforeUnmount(() => {
  if (ro) ro.disconnect();
  window.removeEventListener('repo:selection-updated', handleExternalSelection as EventListener);
  window.removeEventListener('repo:compare-mode-toggle', handleCompareModeToggle as EventListener);
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

/* Click handler: focus and persist selected article (owner/subject) */
function onBubbleClick(n: Node) {
  // In compare mode, use compare selection logic instead
  if (isCompareMode.value) {
    onBubbleClickCompare(n);
    return;
  }

  focusNode(n);
  const detail = getSelectionDetailFromNode(n);
  if (!detail) return;
  const payload = { ...detail };
  applySelection(n, payload);
  persistSelectionDetail(payload);
  announceToScreenReader(`Selected ${n.fullName || n.id} with ${n.contributors} contributor${n.contributors === 1 ? '' : 's'}`);
  window.dispatchEvent(new CustomEvent('repo:bubble-selected', { detail: payload }));
  window.dispatchEvent(new CustomEvent('repo:selection-updated', { detail: payload }));
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

          <!-- WORLD GROUP: Vue renders here, and d3-zoom transforms this exact <g> -->
          <g ref="worldRef">
            <!-- Trunks (vertical) -->
            <line
              v-for="t in trunksList" :key="t.id" class="trunk" :x1="t.x" :x2="t.x" :y1="t.y1" :y2="t.y2"
              stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round"
            />

            <!-- Branch elbows + runs (one path per edge) -->
            <path
              v-for="e in edgesList" :key="`${e.source.id}-${e.target.id}`" class="branch" fill="none"
              stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round" opacity="0.9"
              :d="`M ${e.ex} ${e.ey} C ${e.ex} ${e.ey + 0.5522847498307936 * state.elbowR}, ${e.ex + e.side * 0.5522847498307936 * state.elbowR} ${e.hy}, ${e.hx} ${e.hy} L ${e.cx} ${e.cy}`"
            />

            <!-- Child stems -->
            <line
              v-for="e in edgesList" :key="`stem-${e.source.id}-${e.target.id}`" class="child-stem" :x1="e.sx1"
              :y1="e.sy1" :x2="e.sx2" :y2="e.sy2" stroke="var(--bubble-edge-stroke)" stroke-width="2" stroke-linecap="round"
              opacity="0.9"
            />

            <!-- Joint dots (hollow rings) on trunk side - clickable to compare forks -->
            <circle
              v-for="j in jointDots" :key="`joint-${j.id}`" class="joint-parent" :cx="j.x" :cy="j.y" r="6"
              fill="var(--bubble-joint-fill)" stroke="var(--bubble-joint-stroke)" stroke-width="2" style="cursor: pointer; transition: all 0.15s ease;"
              role="button" tabindex="0" :aria-label="`Compare ${j.sourceOwner} with ${j.targetOwner}`"
              @click.stop="() => onJointClick(j)" @keydown.enter.stop="() => onJointClick(j)"
              @keydown.space.stop="() => onJointClick(j)"
            />

            <!-- Bubbles (component handles labels independently) -->
            <BubbleNode
              v-for="n in nodesList" :key="n.id" :id="n.id" :x="(n as any).x" :y="(n as any).y"
              :r="(rFor(n.contributors))" :contributors="n.contributors" :updated-at="n.updatedAt" :k="kComputed"
              :detail="detailFor(n.contributors)"
              :is-active="selectedNodeId === n.id" :is-compare-mode="isCompareMode"
              :compare-state="getCompareState(n.id)" @click="() => onBubbleClick(n)" @view="() => onBubbleView(n)"
            />
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
  height: calc(100vh - 25rem);
  overflow: auto;
}

.f-fishbone-graph svg:focus {
  outline: none;
}

/* Graph container for relative positioning of overlays */
.graph-container {
  position: relative;
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

/* Joint-parent hover/focus effects for clickable points of contention */
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
