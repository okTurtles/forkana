import {
  BUBBLE_SIZE_TIERS,
  SINGLE_ARTICLE_SCREEN_DIAMETER_MAX,
  SINGLE_ARTICLE_SCREEN_DIAMETER_MIN,
  bubbleLabelDetailFor,
  bubbleRadiusFor,
  bubbleTierFor,
  singleArticleScreenDiameter,
} from './bubble-size.ts';

test('exactly four predefined sizes, strictly increasing', () => {
  expect(BUBBLE_SIZE_TIERS).toHaveLength(4);
  for (let i = 1; i < BUBBLE_SIZE_TIERS.length; i++) {
    expect(BUBBLE_SIZE_TIERS[i].radius).toBeGreaterThan(BUBBLE_SIZE_TIERS[i - 1].radius);
    expect(BUBBLE_SIZE_TIERS[i].minContributors).toBeGreaterThan(BUBBLE_SIZE_TIERS[i - 1].minContributors);
  }
});

test('contributor count maps to the expected tier', () => {
  expect(bubbleTierFor(0).name).toBe('S');
  expect(bubbleTierFor(2).name).toBe('S');
  expect(bubbleTierFor(3).name).toBe('M');
  expect(bubbleTierFor(23).name).toBe('M');
  expect(bubbleTierFor(24).name).toBe('L');
  expect(bubbleTierFor(149).name).toBe('L');
  expect(bubbleTierFor(150).name).toBe('XL');
  expect(bubbleTierFor(5000).name).toBe('XL');
});

test('the ramp is steep enough to read as a hierarchy', () => {
  const [S, M, L, XL] = BUBBLE_SIZE_TIERS;
  // Figma node-id=641-61415: the biggest bubble is ~2.5x a mid one across and
  // ~4x a small one. A flat ramp makes every bubble look the same.
  expect(XL.radius / S.radius).toBeGreaterThanOrEqual(3);
  expect(XL.radius / L.radius).toBeGreaterThanOrEqual(1.4);
  expect(M.radius / S.radius).toBeGreaterThanOrEqual(1.4);
});

test('label detail follows the size tier, matching the design frame', () => {
  // The numbers are the ones drawn in Figma node-id=641-61415.
  for (const small of [1, 23]) expect(bubbleLabelDetailFor(small)).toBe('count');
  for (const mid of [67, 111, 119]) expect(bubbleLabelDetailFor(mid)).toBe('label');
  for (const large of [180, 309, 335, 538]) expect(bubbleLabelDetailFor(large)).toBe('full');
  // A lone article is pinned to the smallest tier, so it asks for nothing more
  // than its count — the view-fit still draws it big enough to show more.
  expect(bubbleLabelDetailFor(400, {singleArticle: true})).toBe('count');
});

test('a single-article subject always uses the smallest tier', () => {
  // Regression for issue #284: a lone bubble used to normalise to the maximum
  // radius no matter how few contributors it had.
  expect(bubbleTierFor(1, {singleArticle: true}).name).toBe('S');
  expect(bubbleTierFor(400, {singleArticle: true}).name).toBe('S');
  expect(bubbleRadiusFor(400, {singleArticle: true})).toBe(BUBBLE_SIZE_TIERS[0].radius);
  // ...and is smaller than the largest tier a multi-fork subject can reach.
  expect(bubbleRadiusFor(400, {singleArticle: true})).toBeLessThan(bubbleRadiusFor(400));
});

test('radius is the tier radius, not a value relative to the graph', () => {
  // The old formula normalised against the largest contributor count in the
  // graph, so these numbers moved whenever a sibling was added or removed.
  // Assert the actual table values instead of comparing a call to itself.
  const [S, M, L, XL] = BUBBLE_SIZE_TIERS;
  expect(bubbleRadiusFor(1)).toBe(S.radius);
  expect(bubbleRadiusFor(4)).toBe(M.radius);
  expect(bubbleRadiusFor(50)).toBe(L.radius);
  expect(bubbleRadiusFor(200)).toBe(XL.radius);
  // A 4-contributor article is the same size whether or not a 200-contributor
  // fork exists, and is smaller than it.
  expect(bubbleRadiusFor(4)).toBeLessThan(bubbleRadiusFor(200));
});

test('viewport attenuation multiplies the tier radius', () => {
  expect(bubbleRadiusFor(1, {scale: 0.5})).toBe(BUBBLE_SIZE_TIERS[0].radius * 0.5);
  // Invalid/absent scales fall back to 1 rather than collapsing the bubble.
  expect(bubbleRadiusFor(1, {scale: 0})).toBe(BUBBLE_SIZE_TIERS[0].radius);
  expect(bubbleRadiusFor(1, {scale: Number.NaN})).toBe(BUBBLE_SIZE_TIERS[0].radius);
});

test('non-finite contributor counts fall back to the smallest tier', () => {
  expect(bubbleTierFor(Number.NaN).name).toBe('S');
  expect(bubbleTierFor(-3).name).toBe('S');
});

test('single-article on-screen diameter stays within its clamps', () => {
  expect(singleArticleScreenDiameter(400)).toBe(SINGLE_ARTICLE_SCREEN_DIAMETER_MIN);
  expect(singleArticleScreenDiameter(1100)).toBe(220);
  expect(singleArticleScreenDiameter(4000)).toBe(SINGLE_ARTICLE_SCREEN_DIAMETER_MAX);
  // Much smaller than the previous 220..480px band (440px on a 1100px canvas).
  expect(singleArticleScreenDiameter(1100)).toBeLessThan(440);
});
