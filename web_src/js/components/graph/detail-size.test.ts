import {DETAIL_DIAMETER_DESIGN, DETAIL_DIAMETER_MIN, detailDiameter, renderedDetailDiameter} from './detail-size.ts';

test('a roomy box gets the design size', () => {
  expect(detailDiameter(1100, 800)).toBe(DETAIL_DIAMETER_DESIGN);
  expect(detailDiameter(1100)).toBe(DETAIL_DIAMETER_DESIGN);
});

test('a small box shrinks the circle to fit inside its margins, down to the minimum', () => {
  expect(detailDiameter(400, 800)).toBe(352);
  expect(detailDiameter(1100, 300)).toBe(252);
  expect(detailDiameter(100, 100)).toBe(DETAIL_DIAMETER_MIN);
});

test('the rendered circle also obeys the 84vw cap', () => {
  expect(renderedDetailDiameter(1100, 1440)).toBe(DETAIL_DIAMETER_DESIGN);
  expect(renderedDetailDiameter(390, 390)).toBeCloseTo(327.6);
});
