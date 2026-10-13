/* detail-size.ts
   Single source of truth for the size of the OPENED article's circle
   (ArticleDetailView). Two callers depend on agreeing about it:

     * FishboneGraph.computeDetailSize() picks the diameter it hands the view;
     * FishboneGraph.articleCanvasFloor() grows the canvas so that circle fits
       at its design size (#422) — if the two disagreed, the floor would be too
       short and the circle would shrink under text that keeps its size.

   ArticleDetailView then caps the circle with CSS at DETAIL_VIEWPORT_CAP of
   the viewport width (a phone), fed from the same constant. */

/** Design size of the opened circle, in px (figma 1996:36736). */
export const DETAIL_DIAMETER_DESIGN = 425;
/** Below this the stack inside the circle is unreadable anyway. */
export const DETAIL_DIAMETER_MIN = 200;
/** Breathing room between the circle and the canvas box's edges. */
export const DETAIL_EDGE_MARGIN = 24;
/** The circle is never wider than this share of the viewport (`84vw`). */
export const DETAIL_VIEWPORT_CAP = 0.84;

/** The diameter FishboneGraph asks for: the design size, shrunk to fit the
   box (`width` × `height`, minus the margin), never below the minimum. */
export function detailDiameter(width: number, height = Infinity): number {
  const fits = Math.min(DETAIL_DIAMETER_DESIGN, width - DETAIL_EDGE_MARGIN * 2, height - DETAIL_EDGE_MARGIN * 2);
  return Math.round(Math.max(DETAIL_DIAMETER_MIN, fits));
}

/** The diameter the circle is actually drawn at: detailDiameter() under the
   CSS viewport cap. */
export function renderedDetailDiameter(width: number, viewportWidth: number, height = Infinity): number {
  return Math.min(detailDiameter(width, height), viewportWidth * DETAIL_VIEWPORT_CAP);
}
