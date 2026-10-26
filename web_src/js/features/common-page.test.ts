import {bfcacheRestoreNeedsReload} from './common-page.ts';

test('bfcacheRestoreNeedsReload', () => {
  // same viewer (signed out, or the same user): shown as it was left
  expect(bfcacheRestoreNeedsReload('', '')).toBe(false);
  expect(bfcacheRestoreNeedsReload('5', '5')).toBe(false);
  // signed in or out since the page was rendered
  expect(bfcacheRestoreNeedsReload('', '5')).toBe(true);
  expect(bfcacheRestoreNeedsReload('5', '')).toBe(true);
  expect(bfcacheRestoreNeedsReload('5', '7')).toBe(true);
  // cannot tell
  expect(bfcacheRestoreNeedsReload(null, '5')).toBe(true);
  expect(bfcacheRestoreNeedsReload('5', null)).toBe(true);
});
