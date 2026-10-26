// #460: purple radio buttons, the landing search and feeds sharing one column, and the atom
// icon for subjects.
import {test, expect} from '@playwright/test';
import {create_article_via_api, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// --color-primary, the brand purple #4E40FA
const brandPurple = 'rgb(78, 64, 250)';

test.beforeAll(async ({browser}, workerInfo) => {
  await login_user(browser, workerInfo, 'user2');
});

test('radio buttons are drawn in the brand purple, like the checkboxes', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user2');
  const page = await context.newPage();
  await page.goto('/org/create');

  // the New Organization form's visibility radios: one is checked by default
  const checked = page.locator('form input[type="radio"]:checked').first();
  await expect(checked).toBeVisible();
  await expect(checked).toHaveCSS('appearance', 'none');
  await expect(checked).toHaveCSS('border-top-color', brandPurple);
  await expect(checked).toHaveCSS('border-radius', '50%');
  expect(await checked.evaluate((el) => getComputedStyle(el).backgroundImage)).toContain('rgb(78, 64, 250)');

  // an unchecked one has no dot, and turns purple when picked
  const unchecked = page.locator('form input[type="radio"]:not(:checked)').first();
  expect(await unchecked.evaluate((el) => getComputedStyle(el).backgroundImage)).toBe('none');
  await unchecked.check();
  await expect(unchecked).toHaveCSS('border-top-color', brandPurple);
  await context.close();
});

for (const viewport of [{width: 1440, height: 900}, {width: 375, height: 812}]) {
  test(`landing search and feeds card line up at ${viewport.width}px`, async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.setViewportSize(viewport);
    await page.goto('/');

    const search = await page.locator('#home-search-input').boundingBox();
    const feeds = await page.locator('.home-feeds-card').boundingBox();
    expect(search.x).toBeCloseTo(feeds.x, 0);
    expect(search.x + search.width).toBeCloseTo(feeds.x + feeds.width, 0);
    // never wider than the page (the hero keeps 16px on each side on a phone)
    expect(feeds.x).toBeGreaterThanOrEqual(16);
    expect(feeds.x + feeds.width).toBeLessThanOrEqual(viewport.width - 16);
    await context.close();
  });
}

test('subjects are shown with the atom icon', async ({page}) => {
  const subject = `atom-${Date.now()}`;
  await create_article_via_api('user2', subject, subject);

  await page.goto(`/explore/subjects?q=${subject}`);
  // the Subjects tab
  await expect(page.locator('a[data-explore-tab-link][href*="/explore/subjects"] svg.forkana-atom')).toBeVisible();
  // the subject's row in the results
  const row = page.locator('.flex-item', {has: page.locator(`a[href$="/subject/${normalizeSubjectTitle(subject)}"]`)});
  await expect(row.locator('.flex-item-leading svg.forkana-atom')).toBeVisible();
  await expect(page.locator('svg.octicon-goal')).toHaveCount(0);
});
