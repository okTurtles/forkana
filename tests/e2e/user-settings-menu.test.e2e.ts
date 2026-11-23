import {test, expect} from '@playwright/test';
import {load_logged_in_context} from './utils_e2e.ts';

// The account settings menu (custom/templates/user/settings/navbar.tmpl): Forkana's tabs only,
// and on a phone one "Settings · <section>" row that opens the list in place.

test('the menu lists Forkana tabs only', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.goto('/user/settings/security');
  const menu = page.locator('#settings-menu');
  await expect(menu.locator('a[href="/user/settings/security"]')).toHaveClass(/active/);
  for (const hidden of ['applications', 'keys', 'actions/runners', 'packages', 'hooks', 'organization', 'repos']) {
    await expect(menu.locator(`a[href="/user/settings/${hidden}"]`)).toHaveCount(0);
  }
  // the hidden pages still answer by URL
  const resp = await page.goto('/user/settings/applications');
  expect(resp.status()).toBe(200);
  await context.close();
});

test('on a phone the menu folds into one row', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.setViewportSize({width: 375, height: 812});
  await page.goto('/user/settings/security');
  const toggle = page.locator('.settings-menu-toggle');
  const menu = page.locator('#settings-menu');

  await expect(toggle).toBeVisible();
  await expect(toggle).toContainText('Security');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(menu).toBeHidden();

  // keyboard: Enter opens, Space closes, without a reload
  await toggle.focus();
  await page.keyboard.press('Enter');
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
  await expect(menu).toBeVisible();
  await expect(menu.locator('a[href="/user/settings/account"]')).toBeVisible();
  await page.keyboard.press('Space');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(menu).toBeHidden();

  // a desktop shows the list and no row
  await page.setViewportSize({width: 1440, height: 900});
  await expect(toggle).toBeHidden();
  await expect(menu).toBeVisible();
  await context.close();
});
