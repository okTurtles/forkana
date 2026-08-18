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
   'count' = the number only, 'label' = + "Contributor(s)",
   'full'  = + "Last updated" and the date.
   THE COUNT IS ON EVERY RUNG. There is no "shows nothing" step any more: a
   22px bubble still says how many contributors it has, at 9px. */
export type BubbleLabelDetail = 'count' | 'label' | 'full';

export type BubbleRung = {
  /** Design name of the rung, used in comments/tests and for debugging. */
  name: 'XS' | 'S' | 'M' | 'L' | 'XL';
  /** Inclusive lower bound on `contributors / maxContributorsInGraph`. */
  minRatio: number;
  /** On-screen diameter in px (== world units, because the graph is at zoom 1). */
  diameter: number;
  /** Size of the CONTRIBUTOR COUNT, in px. A fixed ladder — 9/12/14/22 — not a
     function of the radius: the count used to be `radius * 0.95` clamped, which
     meant a different size on every bubble and a size that CHANGED while a
     bubble was growing. Four values, mapped rung by rung, in this table so the
     diameter and its type can never drift apart. 90 and 126 share 22px: 22 is
     "the big bubble size", and a heading larger than that leaves no room for
     the lines underneath it. */
  countFontSize: number;
  /** The text this rung carries BEYOND the count. BubbleNode treats it as a
     CEILING and drops a secondary line (or shrinks it) only if it will not fit
     inside the arc. The count itself is never dropped and never shrunk — if it
     is too long it is abbreviated instead (formatContributorCount). */
  labelDetail: BubbleLabelDetail;
};

/* THE LADDER. Five diameters, five thresholds — the only thing anyone should
   need to edit. Ordered smallest → largest; `bubbleRungFor` takes the LAST
   rung whose `minRatio` the ratio reaches. */
export const BUBBLE_SIZE_LADDER: readonly BubbleRung[] = [
  {name: 'XS', minRatio: 0, diameter: 22, countFontSize: 9, labelDetail: 'count'},
  {name: 'S', minRatio: 0.07, diameter: 34, countFontSize: 12, labelDetail: 'count'},
  {name: 'M', minRatio: 0.2, diameter: 58, countFontSize: 14, labelDetail: 'count'},
  {name: 'L', minRatio: 0.45, diameter: 90, countFontSize: 22, labelDetail: 'label'},
  {name: 'XL', minRatio: 0.75, diameter: 126, countFontSize: 22, labelDetail: 'full'},
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

/** Size of the contributor count for this bubble, in px. One of 9/12/14/22. */
export function bubbleCountFontFor(contributors: number, maxInGraph: number): number {
  return bubbleRungFor(contributors, maxInGraph).countFontSize;
}

/* ──────────────────────────────────────────────────────────────────────────
   THE COUNT ALWAYS FITS

   The count is the one thing a bubble may never drop and may never shrink
   below its rung's size, so when it is too wide for the circle it is
   ABBREVIATED. How much room there is is pure geometry: a line of text of
   height `font` centred in a circle of radius `r` may be as wide as the chord
   at that height, 2*sqrt(r^2 - (font/2)^2).

   CHAR_WIDTH is measured, not guessed: digits render at 0.696em in the app's
   font stack (the old 0.62 estimate came from mixed-case text and made a
   three-digit count look unfittable at 22px, which is where this matters
   most). 0.70 is that measurement plus a hair. */
const COUNT_CHAR_WIDTH = 0.7;
/* The count gets the WHOLE circle bar half a pixel — it is the last thing to
   be given up, so it is not held to the margin the multi-line block keeps. */
const COUNT_ARC_PADDING = 0.5;

/** Widest a count line may be inside this circle, in px. */
export function countWidthBudget(diameter: number, fontSize: number): number {
  const r = Math.max(0, diameter / 2 - COUNT_ARC_PADDING);
  const half = fontSize / 2;
  return 2 * Math.sqrt(Math.max(0, r * r - half * half));
}

/** ...expressed in characters, which is what the formatter picks against. */
export function countCharBudget(diameter: number, fontSize: number): number {
  return Math.max(1, Math.floor(countWidthBudget(diameter, fontSize) / (fontSize * COUNT_CHAR_WIDTH)));
}

/** Compact forms of `n`, longest (most precise) first:
   exact → one decimal → integer, at the largest unit that applies.
   1234 → "1234", "1.2k", "1k";  12345 → "12345", "12.3k", "12k". */
function countCandidates(n: number): string[] {
  const out = [String(n)];
  const units: [number, string][] = [[1e9, 'B'], [1e6, 'M'], [1e3, 'k']];
  for (const [size, suffix] of units) {
    if (n < size) continue;
    const scaled = n / size;
    const oneDecimal = `${(Math.floor(scaled * 10) / 10).toFixed(1).replace(/\.0$/, '')}${suffix}`;
    const integer = `${Math.floor(scaled)}${suffix}`;
    if (!out.includes(oneDecimal)) out.push(oneDecimal);
    if (!out.includes(integer)) out.push(integer);
    break;
  }
  return out;
}

/** Last resort for a circle too small even for "12k": the largest round value
   that fits, marked as a floor — "9k+" reads "more than nine thousand", which
   is true and short. Only reachable at the 22px rung with six-figure counts,
   which no real contributor list produces; it exists so the promise "the count
   is never dropped and never crosses the arc" has no exceptions. */
function clampedPlus(n: number, maxChars: number): string {
  const units: [number, string][] = [[1e9, 'B'], [1e6, 'M'], [1e3, 'k'], [1, '']];
  for (const [size, suffix] of units) {
    if (n < size) continue;
    const digits = Math.max(1, maxChars - suffix.length - 1);
    const cap = 10 ** digits - 1;                    // 9, 99, 999...
    const value = Math.min(cap, Math.floor(n / size));
    const text = `${value}${suffix}+`;
    if (text.length <= maxChars) return text;
  }
  return '9+';
}

/** The string a bubble shows for `count`, given how many characters fit.
   Never empty, never longer than `maxChars` unless nothing shorter exists. */
export function formatContributorCount(count: number, maxChars: number): string {
  const n = Number.isFinite(count) && count > 0 ? Math.floor(count) : 0;
  const budget = Math.max(1, Math.floor(maxChars));
  for (const candidate of countCandidates(n)) {
    if (candidate.length <= budget) return candidate;
  }
  return clampedPlus(n, budget);
}

/** What this bubble writes in its circle: the count, abbreviated if the rung
   it landed on is too small to spell it out. */
export function bubbleCountTextFor(contributors: number, maxInGraph: number): string {
  const rung = bubbleRungFor(contributors, maxInGraph);
  return formatContributorCount(contributors, countCharBudget(rung.diameter, rung.countFontSize));
}
