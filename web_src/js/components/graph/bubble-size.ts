/* bubble-size.ts
   Single source of truth for how large a bubble is drawn in the bubble view.

   WHY THIS EXISTS
   ---------------
   Bubble radii used to be interpolated linearly between a min and a max radius,
   normalised against the largest contributor count *in the current graph*:

       r = R_MIN + (R_MAX - R_MIN) * (contributors / maxContributorsInGraph)

   That had two problems (see issue #284):

     1. A subject with a single article always normalised to 1.0, so the lone
        bubble was rendered at the maximum radius — disproportionately large for
        what is, semantically, the *smallest* possible subject.
     2. It did not scale: because the scale is relative to the current graph,
        the same article changed size whenever a fork was added or removed, and
        two subjects could not be compared visually at all.

   The replacement is a tiered system: a bubble is snapped to one of FOUR
   predefined sizes chosen from an ABSOLUTE contributor threshold. Absolute
   thresholds mean a bubble keeps its size when new forks appear, and the same
   contributor count always looks the same across subjects.

   SIZE SCALE
   ----------
   Radii sit on the 8px design grid used throughout the graph layout
   (LANE_PAD 8, BUBBLE_PAD 8, STEM 12/18, LABEL_PADDING 12, elbow 20-36), and
   the diameters are whole rem steps (16px), ramping by 2rem per tier:

       S  -> d 112px (7rem)   r 56
       M  -> d 144px (9rem)   r 72
       L  -> d 176px (11rem)  r 88
       XL -> d 208px (13rem)  r 104

   The smallest tier is deliberately kept at r=56 (not smaller) because
   BubbleNode.vue needs roughly 110-130px of on-screen diameter before the
   "<n> Contributors" label stops fitting inside the circle. See
   SINGLE_ARTICLE_SCREEN_DIAMETER_MIN below for the on-screen floor.

   NOTE FOR DESIGN REVIEW: the exact values below are derived from the existing
   spacing scale in this codebase, NOT read from Figma node-id=641-61415 (which
   was not accessible when this was implemented). If Figma specifies different
   radii/thresholds, change ONLY the table below — nothing else hardcodes a
   bubble size. */

export type BubbleSizeTier = {
  /** Design name of the tier, used in comments/tests and for debugging. */
  name: 'S' | 'M' | 'L' | 'XL';
  /** Inclusive lower bound on contributors for this tier (absolute, not relative). */
  minContributors: number;
  /** Bubble radius in world units (== CSS px at zoom level 1). */
  radius: number;
};

/* The four predefined bubble sizes, ordered smallest → largest.
   A bubble takes the LAST tier whose `minContributors` it reaches, so the
   thresholds read as: 1-2 → S, 3-5 → M, 6-14 → L, 15+ → XL.
   The bands widen as they go up because contributor counts are long-tailed:
   most articles have a handful of contributors, so the low bands must be
   narrow to stay informative while the top band absorbs the outliers. */
export const BUBBLE_SIZE_TIERS: readonly BubbleSizeTier[] = [
  {name: 'S', minContributors: 0, radius: 56},   // d 112px — solo / brand new article
  {name: 'M', minContributors: 3, radius: 72},   // d 144px — small collaboration
  {name: 'L', minContributors: 6, radius: 88},   // d 176px — established article
  {name: 'XL', minContributors: 15, radius: 104}, // d 208px — flagship article
] as const;

/* A subject with exactly one article has nothing to compare against: the
   tiered scale is meaningless there, so it is pinned to the smallest tier.
   This is the direct fix for issue #284 (1). */
export const SINGLE_ARTICLE_TIER_INDEX = 0;

/* On-screen diameter targets for a subject that has a single article.
   These matter as much as the radius: the view-fitting code zooms a lone
   bubble to a target size, so shrinking the radius alone would just be undone
   by a larger zoom factor.

   MIN is the legibility floor — below ~150px of on-screen diameter the
   stacked "<count>" + "Contributors" labels in BubbleNode.vue start to be
   clipped. MAX/WIDTH_RATIO keep the bubble to roughly a fifth of the viewport
   width instead of the previous 40% (was 220..480px, i.e. 440px on a 1100px
   container — the "disproportionately large" bubble from issue #284). */
export const SINGLE_ARTICLE_SCREEN_DIAMETER_MIN = 180;
export const SINGLE_ARTICLE_SCREEN_DIAMETER_MAX = 240;
export const SINGLE_ARTICLE_SCREEN_WIDTH_RATIO = 0.2;

export type BubbleSizeOptions = {
  /** True when the whole subject holds a single article (no forks at all). */
  singleArticle?: boolean;
  /** Uniform viewport attenuation (small screens / busy graphs). Defaults to 1. */
  scale?: number;
};

/** Index into BUBBLE_SIZE_TIERS for a given contributor count.
   Not exported: `bubbleTierFor` is the tier accessor callers should use. */
function bubbleTierIndexFor(contributors: number, opts: BubbleSizeOptions = {}): number {
  if (opts.singleArticle) return SINGLE_ARTICLE_TIER_INDEX;
  const n = Number.isFinite(contributors) ? contributors : 0;
  let index = 0;
  for (let i = 0; i < BUBBLE_SIZE_TIERS.length; i++) {
    if (n >= BUBBLE_SIZE_TIERS[i].minContributors) index = i;
  }
  return index;
}

/** The tier record a given contributor count maps to. */
export function bubbleTierFor(contributors: number, opts: BubbleSizeOptions = {}): BubbleSizeTier {
  return BUBBLE_SIZE_TIERS[bubbleTierIndexFor(contributors, opts)];
}

/** Bubble radius in world units, tier-snapped and viewport-attenuated. */
export function bubbleRadiusFor(contributors: number, opts: BubbleSizeOptions = {}): number {
  const requested = opts.scale;
  const scale = typeof requested === 'number' && Number.isFinite(requested) && requested > 0 ? requested : 1;
  return bubbleTierFor(contributors, opts).radius * scale;
}

/** Target on-screen diameter for the lone bubble of a single-article subject. */
export function singleArticleScreenDiameter(containerWidth: number): number {
  const fromWidth = Math.floor((containerWidth || 0) * SINGLE_ARTICLE_SCREEN_WIDTH_RATIO);
  return Math.max(
    SINGLE_ARTICLE_SCREEN_DIAMETER_MIN,
    Math.min(fromWidth, SINGLE_ARTICLE_SCREEN_DIAMETER_MAX),
  );
}
