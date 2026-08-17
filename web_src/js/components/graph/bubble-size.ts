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
   Radii sit on the 8px design grid used throughout the graph layout (LANE_PAD
   8, BUBBLE_PAD 8, STEM 12/18, LABEL_PADDING 12, elbow 20-36):

       S  -> d  80px  r 40
       M  -> d 128px  r 64
       L  -> d 192px  r 96
       XL -> d 288px  r 144

   The ramp is deliberately steep — XL is 3.6x S across, and 1.5x L. An earlier
   version of this table ran 56/72/88/104, only 1.9x end to end, and the result
   read as one uniform blob: in Figma node-id=641-61415 the biggest bubble is
   roughly 2.5x the diameter of a mid one and 4x a small one. The hierarchy is
   the point of the tiers, so the steps have to be obvious.

   Size also decides how much a bubble SAYS — see `labelDetail`. That is why
   the two matter together and live in the same table.

   THRESHOLDS
   ----------
   The bands are read off the same Figma frame, whose bubbles are 1, 23 (small,
   count only), 67/111/119 (mid, count + "Contributors") and 180/309/335/538
   (large, count + "Contributors" + "Last updated" + date). So the boundaries
   sit between 23 and 67, and between 119 and 180; the lowest boundary keeps a
   brand-new article visibly smaller than an established one.

   CONSEQUENCE WORTH CONFIRMING WITH DESIGN: on these thresholds a typical
   repository with 5-20 contributors lands in M and shows only its count. That
   follows the Figma, where 23 contributors is already drawn as a small bubble,
   but it does mean the "Contributors" line is reserved for genuinely large
   articles. If real data should read differently, change ONLY the table below
   — nothing else hardcodes a bubble size or its label detail. */

/** How much text a bubble carries. Each step adds to the one before it:
   'count' = the number only, 'label' = + "Contributor(s)",
   'full'  = + "Last updated" and the date. */
export type BubbleLabelDetail = 'count' | 'label' | 'full';

export type BubbleSizeTier = {
  /** Design name of the tier, used in comments/tests and for debugging. */
  name: 'S' | 'M' | 'L' | 'XL';
  /** Inclusive lower bound on contributors for this tier (absolute, not relative). */
  minContributors: number;
  /** Bubble radius in world units (== CSS px at zoom level 1). */
  radius: number;
  /** The text this tier is meant to carry, whatever the current zoom is.
     BubbleNode treats it as a FLOOR: it shrinks the type to honour it, and
     shows more when a bubble happens to be drawn large enough. */
  labelDetail: BubbleLabelDetail;
};

/* The four predefined bubble sizes, ordered smallest → largest.
   A bubble takes the LAST tier whose `minContributors` it reaches, so the
   thresholds read as: 1-2 → S, 3-5 → M, 6-14 → L, 15+ → XL.
   The bands widen as they go up because contributor counts are long-tailed:
   the low bands stay narrow to stay informative while the top band absorbs
   the outliers. */
export const BUBBLE_SIZE_TIERS: readonly BubbleSizeTier[] = [
  {name: 'S', minContributors: 0, radius: 40, labelDetail: 'count'},    // solo / brand new
  {name: 'M', minContributors: 3, radius: 64, labelDetail: 'count'},    // small collaboration
  {name: 'L', minContributors: 24, radius: 96, labelDetail: 'label'},   // established article
  {name: 'XL', minContributors: 150, radius: 144, labelDetail: 'full'}, // flagship article
] as const;

/* A subject with exactly one article has nothing to compare against: the
   tiered scale is meaningless there, so it is pinned to the smallest tier.
   This is the direct fix for issue #284 (1). */
export const SINGLE_ARTICLE_TIER_INDEX = 0;

/* On-screen diameter targets for a subject that has a single article.
   These matter as much as the radius: the view-fitting code zooms a lone
   bubble to a target size, so shrinking the radius alone would just be undone
   by a larger zoom factor.

   MIN is the legibility floor: a lone bubble is the whole view, so it should
   comfortably carry its count, its label and its date. It is expressed on
   SCREEN, not in world units, so it is unaffected by the tier radius above —
   the view-fit zoom makes up the difference either way.
   MAX/WIDTH_RATIO keep the bubble to roughly a fifth of the viewport
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

/** The text a bubble of this size is meant to carry. See BubbleLabelDetail. */
export function bubbleLabelDetailFor(contributors: number, opts: BubbleSizeOptions = {}): BubbleLabelDetail {
  return bubbleTierFor(contributors, opts).labelDetail;
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
