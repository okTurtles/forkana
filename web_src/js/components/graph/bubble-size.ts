/* bubble-size.ts
   Single source of truth for how large a bubble is drawn in the bubble view.

   WHY THIS EXISTS
   ---------------
   Bubble size has been through three models (issue #284):

     1. A linear interpolation between a min and a max radius, normalised
        against the largest contributor count in the graph. A lone article
        normalised to 1.0 and was therefore drawn at the maximum radius.
     2. ABSOLUTE tiers: a contributor count mapped to one of four fixed radii,
        whatever the rest of the graph looked like. That fixed (1) but was a
        misreading of the requirement — it made a graph of five similar
        articles render as five identical bubbles, and a graph of five small
        ones render as five specks.
     3. This file: a FIXED LADDER of five on-screen diameters, chosen by the
        bubble's contributor count RELATIVE to the biggest bubble in the same
        graph. Size is a comparison inside one subject, not an absolute
        measure of popularity.

   THE MODEL
   ---------
       ratio = contributors / maxContributorsInGraph

   and `ratio` picks a rung of BUBBLE_SIZE_LADDER (the last rung whose
   `minRatio` it reaches). Consequences, all of them deliberate:

     * the bubble with the most contributors has ratio 1 and is ALWAYS 126px —
       every graph has at least one 126px bubble;
     * a subject with a single article is that bubble, so a lone article is
       126px (it is no longer a special case anywhere in the code);
     * two articles with similar counts (say 40 and 38) are both 126px;
     * the wider the proportional gap, the smaller the lesser bubble — 4
       contributors against 300 is ratio 0.013 and lands on the bottom rung.

   Nothing outside this file hardcodes a bubble diameter or its label detail:
   change the table below and the layout, the labels and the tests follow.

   EXACT PIXELS
   ------------
   The ladder is in SCREEN pixels, and the graph renders at zoom 1 (see
   FishboneGraph.resetView), so a 126px bubble measures 126px on screen. There
   is no viewport attenuation and no zoom-to-fit scaling of the resting view:
   they would make the measured diameter something other than the ladder. */

/** How much text a bubble carries. Each step adds to the one before it:
   'none'  = no text at all (the circle is a size cue only),
   'count' = the number only, 'label' = + "Contributor(s)",
   'full'  = + "Last updated" and the date. */
export type BubbleLabelDetail = 'none' | 'count' | 'label' | 'full';

export type BubbleRung = {
  /** Design name of the rung, used in comments/tests and for debugging. */
  name: 'XS' | 'S' | 'M' | 'L' | 'XL';
  /** Inclusive lower bound on `contributors / maxContributorsInGraph`. */
  minRatio: number;
  /** On-screen diameter in px (== world units, because the graph is at zoom 1). */
  diameter: number;
  /** The text this rung carries. BubbleNode treats it as a CEILING and drops
     detail (or shrinks the type) only if a line will not fit inside the arc.
     The two bottom rungs are too small for legible type at any size: 22px and
     34px circles carry nothing, and hovering is how you read them. */
  labelDetail: BubbleLabelDetail;
};

/* THE LADDER. Five diameters, five thresholds — the only thing anyone should
   need to edit. Ordered smallest → largest; `bubbleRungFor` takes the LAST
   rung whose `minRatio` the ratio reaches. */
export const BUBBLE_SIZE_LADDER: readonly BubbleRung[] = [
  {name: 'XS', minRatio: 0, diameter: 22, labelDetail: 'none'},
  {name: 'S', minRatio: 0.07, diameter: 34, labelDetail: 'none'},
  {name: 'M', minRatio: 0.2, diameter: 58, labelDetail: 'count'},
  {name: 'L', minRatio: 0.45, diameter: 90, labelDetail: 'label'},
  {name: 'XL', minRatio: 0.75, diameter: 126, labelDetail: 'full'},
] as const;

/* The hovered/opened bubble: one fixed size, ~1.6x the largest rung, big
   enough to carry the article's whole card (count, excerpt, date, and — once
   clicked — its two actions). The layout is re-run with this radius in place
   of the hovered node's own, which is what pushes its neighbours aside. */
export const BUBBLE_HOVER_DIAMETER = 202;
export const BUBBLE_HOVER_RADIUS = BUBBLE_HOVER_DIAMETER / 2;

/** Largest contributor count in a graph — the denominator of every ratio.
   Returns 0 for an empty graph, which `contributorRatio` reads as "no
   comparison possible" and answers 1 (everything ties for biggest). */
export function maxContributors(counts: Iterable<number>): number {
  let max = 0;
  for (const c of counts) {
    const n = Number.isFinite(c) ? c : 0;
    if (n > max) max = n;
  }
  return max;
}

/** `contributors / maxInGraph`, clamped to 0..1 and defined at the edges:
   a non-positive maximum (an empty graph, or one where every article reports
   zero contributors) means every bubble ties for the biggest, so they all get
   ratio 1 and therefore the top rung. */
export function contributorRatio(contributors: number, maxInGraph: number): number {
  const n = Number.isFinite(contributors) && contributors > 0 ? contributors : 0;
  const m = Number.isFinite(maxInGraph) && maxInGraph > 0 ? maxInGraph : 0;
  if (m <= 0) return 1;
  return Math.min(1, n / m);
}

/** The rung a bubble sits on, given its own count and the graph's maximum. */
export function bubbleRungFor(contributors: number, maxInGraph: number): BubbleRung {
  const ratio = contributorRatio(contributors, maxInGraph);
  let rung = BUBBLE_SIZE_LADDER[0];
  for (const candidate of BUBBLE_SIZE_LADDER) {
    if (ratio >= candidate.minRatio) rung = candidate;
  }
  return rung;
}

/** On-screen diameter in px — one of the five ladder values, nothing else. */
export function bubbleDiameterFor(contributors: number, maxInGraph: number): number {
  return bubbleRungFor(contributors, maxInGraph).diameter;
}

/** Bubble radius in world units (== screen px at zoom 1). */
export function bubbleRadiusFor(contributors: number, maxInGraph: number): number {
  return bubbleRungFor(contributors, maxInGraph).diameter / 2;
}

/** The text a bubble of this size carries. See BubbleLabelDetail. */
export function bubbleLabelDetailFor(contributors: number, maxInGraph: number): BubbleLabelDetail {
  return bubbleRungFor(contributors, maxInGraph).labelDetail;
}
