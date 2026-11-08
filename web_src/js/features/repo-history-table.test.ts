import {
  fillMessage, formatTableDate, isEmptySpaceClick, nextComparePicks, tableCompareState, tableCounterText, tablePageCount, tablePageOfRow, tablePageRange,
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
  const msg = '%[1]d of %[2]d';
  expect(tableCounterText(msg, 1, 8, 12)).toEqual('8 of 12');
  expect(tableCounterText(msg, 2, 8, 12)).toEqual('12 of 12');
  expect(tableCounterText(msg, 1, 8, 3)).toEqual('3 of 3');
  expect(fillMessage('%[2]s, %[1]d, %[3]d', 1, 'two')).toEqual('two, 1, %[3]d');
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

test('what a click must land on to clear the selection (#495)', () => {
  document.body.innerHTML = `
    <nav id="navbar"><span class="navbar-gap"></span></nav>
    <div id="table-compare-announcement-root"><div class="compare-announcement"><span>1/2</span></div></div>
    <div class="page-content">
      <h1 class="subject-title">Moon Landing</h1>
      <div class="ui floating dropdown"><span class="text">Sort</span><div class="menu"><a class="item">Latest</a></div></div>
      <table><thead><tr><th class="head">Contributors</th></tr></thead><tbody>
        <tr class="article-row"><td class="cell">user2</td><td><input type="checkbox"></td></tr>
        <tr class="article-detail-row"><td><p class="summary">A summary</p><button><span class="label">Go</span></button></td></tr>
      </tbody></table>
      <div class="history-table-footer"><span class="counter">2 of 2</span></div>
    </div>
    <footer class="page-footer"><p class="copy">Forkana</p></footer>`;
  const at = (selector: string) => document.querySelector(selector);
  // empty space: the page around the table, its heading row and its footer's counter
  for (const selector of ['.page-content', '.subject-title', 'th.head', '.history-table-footer', '.counter']) {
    expect([selector, isEmptySpaceClick(at(selector))]).toEqual([selector, true]);
  }
  // the rows, their summaries and controls, menus, the Compare banner and the site chrome
  for (const selector of ['td.cell', 'input', '.summary', 'button .label', '.dropdown .text', '.menu .item', '.compare-announcement span', '.navbar-gap', '.copy']) {
    expect([selector, isEmptySpaceClick(at(selector))]).toEqual([selector, false]);
  }
  expect(isEmptySpaceClick(null)).toBe(false);
  expect(isEmptySpaceClick(document.createElement('div'))).toBe(false); // not on the page
});
