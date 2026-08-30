/* graph-viewport.ts — how the bubble graph turns a MEASURED container into the
   numbers the layout runs at (#348).

   These four lines used to live inside FishboneGraph.vue, mixed into the
   ResizeObserver callback. They are pure functions of a measurement, so they
   live here where a test can pin them down: the whole point of #348 is that the
   canvas must be derived from the container EVERY time the container changes,
   and a re-measure that mis-reports "nothing changed" is exactly how the graph
   kept its mount-time height. */

/** Never collapse the canvas below this, however short the container is. */
export const MIN_SVG_HEIGHT = 320;
/** Container width the layout is capped at: the graph is centred in wider
   containers rather than spread across them. */
export const MAX_LAYOUT_WIDTH = 1100;
/** Width used before anything has been measured (a container that is not
   rendered yet reports 0, and a layout at width 0 draws nothing). */
export const DEFAULT_CONTAINER_WIDTH = MAX_LAYOUT_WIDTH;
/** ONE default for "container height we have not measured yet". There used to
   be two (DEFAULT_SVG_HEIGHT 1000 and DEFAULT_CONTAINER_HEIGHT 800) that meant
   the same thing and disagreed, so the first layout was fitted against one
   value and drawn at the other. */
export const DEFAULT_CONTAINER_HEIGHT = 800;
/** Sub-pixel jitter (zoom levels, fractional layout) must not count as a
   resize: a re-layout per frame would fight the scrollbar it can create. */
export const SIZE_EPSILON = 1;

export type ContainerSize = {width: number; height: number};

/** Height available to the SVG canvas: the measured container minus anything
   else inside its scroll box (the legend sits under the graph). Without this
   the canvas was given the FULL container height, the legend was pushed past
   the fold and the container grew a scrollbar. */
export function canvasHeightFor(containerHeight: number, legendHeight: number): number {
  return Math.max(MIN_SVG_HEIGHT, (containerHeight || DEFAULT_CONTAINER_HEIGHT) - legendHeight);
}

/** The width a layout runs at, from a raw measurement. Clamped — and the clamp
   is why the raw measurement has to be kept separately: comparing a clamped
   value against the next raw one reported "changed" on every single delivery
   for any container wider than the cap, which re-ran the layout (and threw
   away the user's pan/zoom) for resizes that never happened. */
export function layoutWidthFor(rawWidth: number): number {
  return Math.min(rawWidth || DEFAULT_CONTAINER_WIDTH, MAX_LAYOUT_WIDTH);
}

/** Is this a real, drawable measurement? A container that is `display:none`
   (the other view is showing) measures 0×0; sizing the canvas from that would
   bake the placeholder in, which is symptom two of #348. */
export function isMeasurable(size: ContainerSize): boolean {
  return size.width > 0 && size.height > 0;
}

/** Bail-out test for the re-measure path: compares RAW measurement to RAW
   measurement, so an unchanged container never triggers a re-render and the
   observer cannot feed itself. */
export function sizeChanged(prev: ContainerSize, next: ContainerSize, epsilon = SIZE_EPSILON): boolean {
  return Math.abs(next.width - prev.width) > epsilon || Math.abs(next.height - prev.height) > epsilon;
}
