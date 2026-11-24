import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// The notifications list (custom/templates/user/notification/notification_div.tmpl), after Figma
// 7648:74304 (desktop), 7648:74435 (mobile) and 7895:6430 (empty). Fixtures: user2 has unread
// and pinned issue notifications (models/fixtures/notification.yml); user5 has none.

test.beforeAll(async ({browser}, workerInfo) => {
  await login_user(browser, workerInfo, 'user2');
  await login_user(browser, workerInfo, 'user5');
});

test('the list: title, toolbar, day groups and rows', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.setViewportSize({width: 1440, height: 1024});
  await page.goto('/notifications');

  // Title 7648:74309: Inter 600 24/32
  const title = page.locator('.notifications-title h1');
  await expect(title).toHaveText('Notifications');
  await expect(title).toHaveCSS('font-size', '24px');
  await expect(title).toHaveCSS('font-weight', '600');

  // the SegmentedControl: Unread is current
  const unread = page.locator('.notifications-toolbar .forkana-segmented-item').first();
  await expect(unread).toHaveClass(/active/);
  await expect(unread).toHaveAttribute('aria-current', 'page');

  // a day group, then rows with an icon, a title, a label and a time
  await expect(page.locator('#notification_table .notifications-group').first()).toBeVisible();
  const row = page.locator('#notification_table .notifications-item').first();
  await expect(row).toBeVisible();
  await expect(row.locator('.notifications-icon')).toHaveCSS('width', '32px');
  await expect(row.locator('.notifications-text-title')).toHaveCSS('font-weight', '600');
  await expect(row.locator('.notifications-label')).toHaveCSS('border-radius', '100px');
  await expect(row.locator('a.notifications-link')).toHaveAttribute('href', /\/repo\d+\//);

  // Gitea's actions are still there, shown on hover
  await row.hover();
  await expect(row.locator('.notifications-buttons button[value="mark_as_read"], .notifications-buttons button[value="pin"]').first()).toBeVisible();

  // the Type menu filters the list
  await page.locator('.notifications-type-menu > summary').click();
  await page.locator('.notifications-type-options a', {hasText: 'Invites'}).click();
  await expect(page).toHaveURL(/source=invites/);
  await expect(page.locator('.notifications-type-menu > summary')).toContainText('Invites');
  await context.close();
});

test('the Type menu closes on Escape and on a click outside it', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.goto('/notifications');
  const menu = page.locator('.notifications-type-menu');
  const options = menu.locator('.notifications-type-options');

  await menu.locator('> summary').click();
  await expect(options).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(options).toBeHidden();
  await expect(menu.locator('> summary')).toBeFocused();

  await menu.locator('> summary').click();
  await expect(options).toBeVisible();
  await page.locator('.notifications-title h1').click();
  await expect(options).toBeHidden();
  await context.close();
});

test('with a Type filter on: no Mark all as read, and an empty list says it is filtered', async ({browser}, workerInfo) => {
  // user2 has unread notifications, none of them invites
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.goto('/notifications');
  await expect(page.locator('.notifications-mark-all')).toHaveCount(1);

  await page.goto('/notifications?type=unread&source=invites');
  await expect(page.locator('.notifications-mark-all')).toHaveCount(0);
  const empty = page.locator('.notifications-empty');
  await expect(empty.locator('h2')).toHaveText('No unread notifications.');
  await expect(empty.locator('p')).toContainText('Invites');
  await empty.locator('a').click();
  await expect(page).not.toHaveURL(/source=/);
  await expect(page.locator('#notification_table .notifications-item').first()).toBeVisible();
  await context.close();
});

test('the empty state: all caught up', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user5');
  const page = await context.newPage();
  await page.goto('/notifications');
  const empty = page.locator('.notifications-empty');
  await expect(empty.locator('h2')).toHaveText('You’re all caught up');
  await expect(empty.locator('.notifications-empty-icon')).toHaveCSS('width', '56px');
  await expect(page.locator('.notifications-mark-all')).toHaveCount(0);
  await empty.locator('a').click();
  await expect(page).toHaveURL(/type=read/);
  await expect(page.locator('.notifications-empty h2')).toHaveText('No read notifications.');
  await context.close();
});
