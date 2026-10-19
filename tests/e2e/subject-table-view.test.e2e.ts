// #436 / #437: the subject's Table view after figma "Table view / Articles" (641:61176) and
// "Table view / Comparing articles" (641:61039).
import {test, expect} from '@playwright/test';
import {load_logged_in_context, login_user} from './utils_e2e.ts';

// a subject of its own, with an article by each of two users: compare mode needs two
const subject = `table-view-${Date.now()}`;

test.beforeAll(async ({browser}, workerInfo) => {
  for (const user of ['user12', 'user13']) {
    await login_user(browser, workerInfo, user);
    const context = await load_logged_in_context(browser, workerInfo, user);
    const page = await context.newPage();
    // an article with a README (an empty repository is no article of the fork graph):
    // the create form's hidden "initialize" fields are set as their defaults would
    const response = await page.goto(`/repo/create?subject=${encodeURIComponent(subject)}`);
    expect(response?.status()).toBe(200);
    await page.evaluate((description) => {
      document.querySelector<HTMLTextAreaElement>('textarea[name=description]').value = description;
      document.querySelector<HTMLInputElement>('input[name=auto_init]').checked = true;
      document.querySelector<HTMLInputElement>('input[name=readme]').value = 'Default';
    }, `An article by ${user}`);
    await page.click('button.ui.primary.button');
    await page.waitForURL(/\/subject\//, {timeout: 30000});
    await context.close();
  }
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
  await expect(page.locator('[data-role="table-prev"]')).toBeDisabled();
  await expect(page.locator('[data-role="table-next"]')).toBeDisabled();

  // 4. the owner alone in its cell (no subject under it); 5. dates as "03 Feb 2025"
  const first = rows.first();
  await expect(first.locator('.history-table-owner')).not.toContainText(subject);
  await expect(first.locator('td').nth(2)).toHaveText(/^\d{2} [A-Z][a-z]{2} \d{4}$/);

  // 3, 6, 8. opening a row selects it (highlighted, owner in the brand colour) and shows
  // its summary and "Go to article"
  await first.locator('.row-toggle').click();
  await expect(first).toHaveClass(/is-selected/);
  await expect(first).toHaveCSS('background-color', 'rgba(233, 231, 255, 0.4)');
  await expect(first.locator('.history-table-owner')).toHaveCSS('color', 'rgb(78, 64, 250)');
  const detail = page.locator('tr.article-detail-row:not(.tw-hidden)');
  await expect(detail).toHaveCount(1);
  await expect(detail.locator('[data-role="summary"]')).not.toBeEmpty();
  await expect(detail.locator('.go-to-article')).toHaveText('Go to article');

  // 7. the open row is not highlighted on hover
  await detail.hover();
  await expect(detail.locator('td')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
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

  // the rows' checkboxes pick the articles
  const checks = page.locator('#articles-table .row-check');
  await checks.nth(0).check();
  await expect(banner).toContainText('1/2 article selected. Select another to compare.');
  await expect(page.locator('#articles-table tr.article-row').first()).toHaveClass(/is-selected/);
  await checks.nth(1).check();
  await expect(banner).toContainText('2/2 selected');
  await expect(banner.locator('.compare-announcement-primary')).toBeVisible();

  // the x leaves compare mode and clears the picks
  await banner.locator('.compare-announcement-dismiss').click();
  await expect(banner).toHaveCount(0);
  await expect(checks.nth(0)).not.toBeChecked();
  await expect(page.locator('#compare-mode-button')).not.toHaveClass(/primary/);
});
