import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';
import type {Page} from '@playwright/test';

// The change requests hub (#94, Figma "09 · Changes hub" 7368:274515).
// Fixtures: user2 owns repo1 (subject "example-subject"), where user1 opened the change
// requests issue2, issue3 and pull5; user2 opened change requests on other articles.

const isPhone = (page: Page) => (page.viewportSize()?.width ?? 1280) < 768;

test.describe('Change requests hub (#94)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('the navbar entry counts the open change requests received', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto('/explore/subjects');

    const entry = page.locator('#navbar-change-requests');
    await expect(entry).toHaveAttribute('href', '/pulls');
    await expect(entry.locator('.navbar-item-label')).toHaveText('Change request');
    await expect(entry.locator('.navbar-counter')).toHaveText(/^[1-9]\d*$/);
    await expect(entry).not.toHaveClass(/navbar-item-current/);
    await context.close();
  });

  test('Received lists the change requests to the user\'s articles', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto('/pulls');

    await expect(page.locator('#navbar-change-requests')).toHaveClass(/navbar-item-current/);
    await expect(page.locator('.changes-hub-title')).toHaveText('Change requests');
    await expect(page.locator('.changes-hub-description')).toHaveText('Proposals made to your articles, and the ones you\'ve made to others.');

    const tabs = page.locator('.changes-hub-tab');
    await expect(tabs).toHaveCount(3);
    await expect(tabs.locator('.changes-hub-tab-label')).toHaveText(['Received', 'Sent', 'Mentions']);
    await expect(page.locator('.changes-hub-tab.active')).toHaveAttribute('data-tab', 'received');
    await expect(page.locator('.changes-hub-tab.active .changes-hub-counter')).toHaveText(/^[1-9]\d*$/);

    const rows = page.locator('.changes-hub-row');
    const row = rows.filter({hasText: 'example-subject'}).first();
    await expect(row).toBeVisible();
    await expect(row.locator('.changes-hub-line')).toContainText(' on example-subject');
    await expect(row).toHaveAttribute('href', /\/pulls\/\d+$/);
    await expect(page.locator('.changes-hub-shown')).toHaveText(/^\d+ of \d+$/);

    // a row opens its change request
    await row.click();
    await expect(page).toHaveURL(/\/pulls\/\d+$/);
    await context.close();
  });

  test('Sent, Filter and Sort', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto('/pulls');

    await page.locator('.changes-hub-tab[data-tab="sent"]').click();
    await expect(page).toHaveURL(/\/pulls\?tab=sent$/);
    await expect(page.locator('.changes-hub-tab.active')).toHaveAttribute('data-tab', 'sent');
    await expect(page.locator('.changes-hub-row').first()).toBeVisible();

    // Filter: the states are checkboxes; picking one keeps the tab
    await page.locator('#changes-hub-filter').click();
    const filterMenu = page.locator('#changes-hub-filter .menu');
    await expect(filterMenu).toBeVisible();
    await expect(filterMenu.locator('.item')).toHaveText(['Clear filters', 'Open', 'Merged', 'Closed']);
    await filterMenu.locator('.changes-hub-filter-item[data-state="open"]').click();
    await expect(page).toHaveURL(/tab=sent&state=open/);
    await expect(page.locator('.changes-hub-filter-item[data-state="open"]')).toHaveAttribute('aria-checked', 'true');
    for (const state of await page.locator('.changes-hub-row').evaluateAll((els) => els.map((el) => el.getAttribute('data-state')))) {
      expect(state).toBe('open');
    }

    // Sort: one choice at a time, highlighted
    await page.locator('#changes-hub-sort').click();
    const sortMenu = page.locator('#changes-hub-sort .menu');
    await expect(sortMenu).toBeVisible();
    await expect(sortMenu.locator('.item.active')).toHaveText('Most recently updated');
    await sortMenu.locator('.item[data-sort="oldest"]').click();
    await expect(page).toHaveURL(/tab=sent&state=open&sort=oldest/);
    await expect(page.locator('#changes-hub-sort .item.active')).toHaveAttribute('data-sort', 'oldest');

    // Clear filters drops the state but keeps the rest
    await page.locator('#changes-hub-filter').click();
    await page.locator('#changes-hub-filter .changes-hub-clear').click();
    await expect(page).toHaveURL(/\/pulls\?tab=sent&sort=oldest$/);

    // the search keeps the tab, filter and sort
    await page.locator('.changes-hub-search input[name=q]').fill('pull');
    await page.locator('.changes-hub-search input[name=q]').press('Enter');
    await expect(page).toHaveURL(/tab=sent&sort=oldest&q=pull/);
    await expect(page.locator('.changes-hub-search input[name=q]')).toHaveValue('pull');
    await context.close();
  });

  test('the desktop layout follows the frame', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(isPhone(page), 'the desktop frame');
    await page.goto('/pulls');

    const toolbar = await page.locator('.changes-hub-toolbar').boundingBox();
    const tabs = await page.locator('.changes-hub-tabs').boundingBox();
    const row = page.locator('.changes-hub-row').first();
    await expect(page.locator('.changes-hub-back')).toBeVisible();
    expect(tabs.y).toBeLessThan(toolbar.y);
    expect(tabs.height).toBe(48);
    expect(toolbar.height).toBe(32);
    await expect(row.locator('> .changes-hub-state')).toBeVisible();
    await expect(row.locator('.changes-hub-mobile-meta')).toBeHidden();
    await expect(row.locator('.changes-hub-sub')).toContainText(' · ');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await context.close();
  });

  test('the phone layout follows the Mobile frame', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(!isPhone(page), 'the Mobile frame');
    await page.goto('/pulls');

    // no Back; Search, Filter and Sort above the tabs; the state under the first line
    const toolbar = await page.locator('.changes-hub-toolbar').boundingBox();
    const tabs = await page.locator('.changes-hub-tabs').boundingBox();
    const row = page.locator('.changes-hub-row').first();
    await expect(page.locator('.changes-hub-back')).toBeHidden();
    expect(toolbar.y).toBeLessThan(tabs.y);
    await expect(row.locator('.changes-hub-mobile-meta .changes-hub-state')).toBeVisible();
    await expect(row.locator('.changes-hub-sub')).toBeHidden();
    await expect(page.locator('.changes-hub-pages')).toBeHidden();
    // no sideways scroll
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await context.close();
  });
});
