import {
  fillMessage, formatTableDate, nextComparePicks, tableCompareState, tableCounterText, tablePageCount, tablePageOfRow, tablePageRange,
} from './repo-history-table.ts';

const article = (owner: string) => ({owner, repo: 'moon-landing', subject: 'Moon Landing'});

test('pages of the table', () => {
  expect(tablePageCount(0, 8)).toEqual(1);
  expect(tablePageCount(8, 8)).toEqual(1);
  expect(tablePageCount(12, 8)).toEqual(2);
  expect(tablePageOfRow(0, 8)).toEqual(1);
  expect(tablePageOfRow(7, 8)).toEqual(1);
  expect(tablePageOfRow(8, 8)).toEqual(2);
  expect(tablePageRange(1, 8, 12)).toEqual([0, 8]);
  expect(tablePageRange(2, 8, 12)).toEqual([8, 12]);
  expect(tablePageRange(5, 8, 12)).toEqual([8, 12]); // clamped to the last page
  expect(tablePageRange(0, 8, 12)).toEqual([0, 8]);
});

test('the footer counter reads as figma\'s "8 of 12"', () => {
  const msg = '{shown} of {total}';
  expect(tableCounterText(msg, 1, 8, 12)).toEqual('8 of 12');
  expect(tableCounterText(msg, 2, 8, 12)).toEqual('12 of 12');
  expect(tableCounterText(msg, 1, 8, 3)).toEqual('3 of 3');
  expect(fillMessage('{a} {unknown}', {a: 1})).toEqual('1 {unknown}');
});

test('compare picks: two at most, in order, a third refused', () => {
  let picks = nextComparePicks([], article('a'), true);
  expect(picks.map((p) => p.owner)).toEqual(['a']);
  picks = nextComparePicks(picks, article('a'), true); // already picked
  expect(picks.map((p) => p.owner)).toEqual(['a']);
  picks = nextComparePicks(picks, article('b'), true);
  expect(picks.map((p) => p.owner)).toEqual(['a', 'b']);
  expect(nextComparePicks(picks, article('c'), true).map((p) => p.owner)).toEqual(['a', 'b']);
  expect(nextComparePicks(picks, article('a'), false).map((p) => p.owner)).toEqual(['b']);
});

test('the banner follows the picks', () => {
  expect(tableCompareState(false, 0, false)).toBeNull();
  expect(tableCompareState(false, 0, true)).toEqual('unavailable');
  expect(tableCompareState(true, 0, false)).toEqual('select');
  expect(tableCompareState(true, 1, false)).toEqual('one');
  expect(tableCompareState(true, 2, false)).toEqual('ready');
});

test(`dates read as figma's "03 Feb 2025", in the given time zone`, () => {
  const date = new Date(Date.UTC(2025, 1, 3, 23, 30));
  expect(formatTableDate(date, 'en-US', 'UTC')).toEqual('03 Feb 2025');
  expect(formatTableDate(date, 'en-GB', 'Asia/Tokyo')).toEqual('04 Feb 2025');
});
