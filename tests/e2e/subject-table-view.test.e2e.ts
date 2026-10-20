// #436 / #437: the subject's Table view after figma "Table view / Articles" (641:61176) and
// "Table view / Comparing articles" (641:61039).
import {test, expect} from '@playwright/test';
import {create_article_via_api} from './utils_e2e.ts';

// Subjects of their own: one with an article by each of two users (compare mode needs
// two), and one with more articles than a page of the table holds (8, as in figma).
// Every project (and retry) runs the setup again, with subjects of its own.
const run = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const subject = `table-view-${run}`;
const pagedSubject = `table-paged-${run}`;
const pagedUsers = ['user2', 'user4', 'user8', 'user10', 'user11', 'user12', 'user13', 'user14', 'user15'];

// Light-theme values of the tokens figma names (they differ in the dark theme):
// Row 20 641:61208, #e9e7ff at 40% (--color-hover); the owner 641:61216, #4e40fa
// (--color-primary); Frame 169 641:61219, white.
const selectedRowBg = 'rgba(233, 231, 255, 0.4)';
const brand = 'rgb(78, 64, 250)';
const white = 'rgb(255, 255, 255)';

// through the API: the create form's layout is not what this spec is about
function addArticle(user: string, subjectName: string) {
  return create_article_via_api(user, subjectName, `${subjectName}-${user}`, `An article by ${user}`);
}

test.beforeAll(async () => {
  for (const user of ['user12', 'user13']) await addArticle(user, subject);
});

test('the Table view matches figma 641:61176', async ({page}) => {
  await page.goto(`/subject/${encodeURIComponent(subject)}?view=table`);
  const table = page.locator('#articles-table');
  const rows = table.locator('tr.article-row');
  await expect(rows).toHaveCount(2);

  // 1. the Sort menu checks the current order, Most contributors by default
  await page.locator('.history-table-sort-button').click();
  const current = page.locator('.history-table-sort.is-current');
  await expect(current).toHaveCount(1);
  await expect(current).toHaveAttribute('data-sort', 'most_contrib');
  await expect(current.locator('svg.octicon-check')).toBeVisible();
  await page.keyboard.press('Escape');

  // 2. the footer: "2 of 2", Previous / Next (both lead nowhere on a single page)
  await expect(page.locator('[data-role="table-counter"]')).toHaveText('2 of 2');
  await expect(page.locator('[data-role="table-prev"]')).toHaveAttribute('aria-disabled', 'true');
  await expect(page.locator('[data-role="table-next"]')).toHaveAttribute('aria-disabled', 'true');

  // 4. the owner alone in its cell (no subject under it); 5. dates as "03 Feb 2025"
  const first = rows.first();
  await expect(first.locator('.history-table-owner')).not.toContainText(subject);
  await expect(first.locator('td').nth(2)).toHaveText(/^\d{2} [A-Z][a-z]{2} \d{4}$/);

  // 3, 6, 8. opening a row selects it (highlighted, owner in the brand colour) and shows
  // its summary and "Go to article"
  await first.locator('.row-toggle').click();
  await expect(first).toHaveClass(/is-selected/);
  await expect(first).toHaveCSS('background-color', selectedRowBg);
  await expect(first.locator('.history-table-owner')).toHaveCSS('color', brand);
  const detail = page.locator('tr.article-detail-row:not(.tw-hidden)');
  await expect(detail).toHaveCount(1);
  await expect(detail.locator('[data-role="summary"]')).not.toBeEmpty();
  await expect(detail.locator('.go-to-article')).toHaveText('Go to article');

  // 7. the open row is not highlighted on hover, and a hovered row does not look selected
  await detail.hover();
  await expect(detail.locator('td')).toHaveCSS('background-color', white);
  await rows.nth(1).hover();
  await expect(rows.nth(1)).not.toHaveCSS('background-color', selectedRowBg);
});

test('Compare on the Table view stays on the table (#437)', async ({page}) => {
  await page.goto(`/subject/${encodeURIComponent(subject)}?view=table`);
  await page.locator('#compare-mode-button').click();

  // still the Table view, with figma's Comparing articles banner
  await expect(page).toHaveURL(/view=table/);
  await expect(page.locator('#subject-view-tabs a[data-view="table"]')).toHaveClass(/active/);
  await expect(page.locator('.history-view-section--table')).toBeVisible();
  await expect(page.locator('.history-view-section--bubble')).toBeHidden();
  const banner = page.locator('#table-compare-announcement-root .compare-announcement');
  await expect(banner).toContainText('Select 2 articles to compare');
  await expect(page.locator('#compare-mode-button')).toHaveClass(/primary/);

  // the rows' checkboxes pick the articles, and so does a click on a row
  const rows = page.locator('#articles-table tr.article-row');
  const checks = page.locator('#articles-table .row-check');
  await checks.nth(0).check();
  await expect(banner).toContainText('1/2 article selected. Select another to compare.');
  await expect(rows.first()).toHaveClass(/is-selected/);
  // a checked box (641:61075): the brand fill with a white check, not the global
  // checkbox's brand-coloured tick, which would vanish on that fill
  await expect(checks.nth(0)).toHaveCSS('background-color', brand);
  expect(await checks.nth(0).evaluate((el) => getComputedStyle(el, '::after').backgroundColor)).toBe(white);
  await rows.nth(1).locator('td').nth(2).click();
  await expect(page).toHaveURL(/view=table/);
  await expect(checks.nth(1)).toBeChecked();
  await expect(banner).toContainText('2/2 selected');
  await expect(banner.locator('.compare-announcement-primary')).toBeVisible();

  // the x leaves compare mode and clears the picks
  await banner.locator('.compare-announcement-dismiss').click();
  await expect(banner).toHaveCount(0);
  await expect(checks.nth(0)).not.toBeChecked();
  await expect(page.locator('#compare-mode-button')).not.toHaveClass(/primary/);

  // ticking a box outside compare mode enters it with that article picked
  await checks.nth(1).check();
  await expect(banner).toContainText('1/2 article selected.');
  await expect(page.locator('#compare-mode-button')).toHaveClass(/primary/);
});

test('the Table view is paged 8 rows at a time (641:61303)', async ({page}) => {
  for (const user of pagedUsers) await addArticle(user, pagedSubject);

  await page.goto(`/subject/${encodeURIComponent(pagedSubject)}?view=table`);
  const visibleRows = page.locator('#articles-table tr.article-row:not([hidden])');
  const counter = page.locator('[data-role="table-counter"]');
  const prev = page.locator('[data-role="table-prev"]');
  const next = page.locator('[data-role="table-next"]');
  await expect(visibleRows).toHaveCount(8);
  await expect(counter).toHaveText('8 of 9');
  await expect(prev).toHaveAttribute('aria-disabled', 'true');

  // Next onto the last page keeps the keyboard focus and is announced
  await next.focus();
  await page.keyboard.press('Enter');
  await expect(visibleRows).toHaveCount(1);
  await expect(counter).toHaveText('9 of 9');
  await expect(next).toHaveAttribute('aria-disabled', 'true');
  await expect(next).toBeFocused();
  await expect(page.locator('[data-role="table-status"]')).toHaveText('Articles 9 to 9 of 9');

  // the page of the selected article opens first
  const last = visibleRows.first();
  const owner = await last.getAttribute('data-owner');
  const repo = await last.getAttribute('data-repo');
  await page.goto(`/subject/${encodeURIComponent(pagedSubject)}?view=table&selected=${encodeURIComponent(`${owner}/${repo}`)}`);
  await expect(counter).toHaveText('9 of 9');
  await expect(visibleRows.first()).toHaveClass(/is-selected/);

  await prev.click();
  await expect(counter).toHaveText('8 of 9');
});
