import {
  BUBBLE_HOVER_DIAMETER,
  BUBBLE_HOVER_RADIUS,
  BUBBLE_SIZE_LADDER,
  bubbleDiameterFor,
  bubbleLabelDetailFor,
  bubbleRadiusFor,
  bubbleRungFor,
  contributorRatio,
  maxContributors,
} from './bubble-size.ts';

const LADDER = [22, 34, 58, 90, 126];

test('the ladder is exactly the five agreed diameters, smallest first', () => {
  expect(BUBBLE_SIZE_LADDER.map((r) => r.diameter)).toEqual(LADDER);
  for (let i = 1; i < BUBBLE_SIZE_LADDER.length; i++) {
    expect(BUBBLE_SIZE_LADDER[i].minRatio).toBeGreaterThan(BUBBLE_SIZE_LADDER[i - 1].minRatio);
  }
  expect(BUBBLE_SIZE_LADDER[0].minRatio).toBe(0);
  expect(BUBBLE_SIZE_LADDER.map((r) => r.minRatio)).toEqual([0, 0.07, 0.2, 0.45, 0.75]);
});

test('the thresholds are read off the ratio, at and just under each boundary', () => {
  const d = (ratio: number) => bubbleDiameterFor(ratio * 100, 100);
  expect(d(1)).toBe(126);
  expect(d(0.75)).toBe(126);
  expect(d(0.749)).toBe(90);
  expect(d(0.45)).toBe(90);
  expect(d(0.449)).toBe(58);
  expect(d(0.2)).toBe(58);
  expect(d(0.199)).toBe(34);
  expect(d(0.07)).toBe(34);
  expect(d(0.069)).toBe(22);
  expect(d(0)).toBe(22);
});

test('a diameter is never anything but a ladder value', () => {
  for (let max = 1; max <= 40; max++) {
    for (let n = 0; n <= max; n++) {
      expect(LADDER).toContain(bubbleDiameterFor(n, max));
      expect(bubbleRadiusFor(n, max)).toBe(bubbleDiameterFor(n, max) / 2);
    }
  }
});

test('the biggest bubble in a graph is always 126', () => {
  for (const counts of [[1], [1, 1], [3, 300], [0, 0, 7], [5, 5, 5], [1, 2, 3, 400]]) {
    const max = maxContributors(counts);
    const biggest = counts.filter((c) => c === Math.max(...counts));
    for (const c of biggest) expect(bubbleDiameterFor(c, max)).toBe(126);
  }
});

test('a single article is one 126px bubble (no special case left)', () => {
  // Issue #284 (1): a lone bubble used to normalise to the maximum radius and
  // was then special-cased down to the smallest tier. It is now simply the
  // biggest bubble in a graph of one.
  expect(bubbleDiameterFor(1, maxContributors([1]))).toBe(126);
  expect(bubbleDiameterFor(400, maxContributors([400]))).toBe(126);
});

test('similar contributor counts share the top rung', () => {
  // Pierre's example: two bubbles with similar counts are both 126.
  const counts = [40, 38];
  const max = maxContributors(counts);
  expect(counts.map((c) => bubbleDiameterFor(c, max))).toEqual([126, 126]);
  // ...and 0.75 is exactly where that stops.
  expect(bubbleDiameterFor(75, 100)).toBe(126);
  expect(bubbleDiameterFor(74, 100)).toBe(90);
});

test('the wider the proportional gap, the smaller the lesser bubble', () => {
  const max = 300;
  const sizes = [300, 240, 140, 70, 25, 4].map((c) => bubbleDiameterFor(c, max));
  expect(sizes).toEqual([126, 126, 90, 58, 34, 22]);
  for (let i = 1; i < sizes.length; i++) {
    expect(sizes[i]).toBeLessThanOrEqual(sizes[i - 1]);
  }
});

test('ties: every article with the maximum count gets 126', () => {
  const counts = [12, 12, 12];
  const max = maxContributors(counts);
  expect(counts.map((c) => bubbleDiameterFor(c, max))).toEqual([126, 126, 126]);
});

test('zero contributors', () => {
  // Zero against a real maximum is the bottom rung...
  expect(bubbleDiameterFor(0, 50)).toBe(22);
  // ...but a graph where NOTHING has contributors has no scale to compare
  // against, so every bubble ties for biggest rather than collapsing to 22.
  expect(maxContributors([0, 0, 0])).toBe(0);
  expect(bubbleDiameterFor(0, 0)).toBe(126);
  expect(contributorRatio(0, 0)).toBe(1);
});

test('nonsense inputs fall back instead of producing a non-ladder size', () => {
  expect(bubbleDiameterFor(Number.NaN, 100)).toBe(22);
  expect(bubbleDiameterFor(-3, 100)).toBe(22);
  expect(bubbleDiameterFor(50, Number.NaN)).toBe(126);
  expect(bubbleDiameterFor(50, -1)).toBe(126);
  // More contributors than the graph maximum (a stale maximum) clamps to 1.
  expect(contributorRatio(200, 100)).toBe(1);
  expect(bubbleDiameterFor(200, 100)).toBe(126);
  expect(maxContributors([])).toBe(0);
  expect(maxContributors([Number.NaN, 4])).toBe(4);
});

test('label detail is a property of the rung, not of the article', () => {
  expect(BUBBLE_SIZE_LADDER.map((r) => r.labelDetail))
    .toEqual(['none', 'none', 'count', 'label', 'full']);
  // 22 and 34 carry no text at all; hovering is how a small bubble is read.
  expect(bubbleLabelDetailFor(1, 100)).toBe('none');
  expect(bubbleLabelDetailFor(10, 100)).toBe('none');
  expect(bubbleLabelDetailFor(25, 100)).toBe('count');
  expect(bubbleLabelDetailFor(50, 100)).toBe('label');
  expect(bubbleLabelDetailFor(90, 100)).toBe('full');
  expect(bubbleRungFor(90, 100).name).toBe('XL');
});

test('the hover size is one number, bigger than every rung', () => {
  expect(BUBBLE_HOVER_DIAMETER).toBe(202);
  expect(BUBBLE_HOVER_RADIUS).toBe(101);
  for (const rung of BUBBLE_SIZE_LADDER) {
    expect(BUBBLE_HOVER_DIAMETER).toBeGreaterThan(rung.diameter);
  }
  expect(BUBBLE_HOVER_DIAMETER / 126).toBeGreaterThan(1.5);
});
