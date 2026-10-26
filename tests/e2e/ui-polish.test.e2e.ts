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
  // Figma Radio 7416:433670: a #4e40fa circle with a white dot
  await expect(checked).toHaveCSS('border-top-color', brandPurple);
  await expect(checked).toHaveCSS('background-color', brandPurple);
  await expect(checked).toHaveCSS('border-radius', '50%');
  expect(await checked.evaluate((el) => getComputedStyle(el).backgroundImage)).toContain('rgb(255, 255, 255)');

  // an unchecked one ("Limited", Figma Radio 7416:433672) is white with a #818b98 ring, and
  // turns purple when picked
  const unchecked = page.locator('form input[type="radio"]').nth(1);
  await expect(unchecked).not.toBeChecked();
  await expect(unchecked).toHaveCSS('border-top-color', 'rgb(129, 139, 152)');
  expect(await unchecked.evaluate((el) => getComputedStyle(el).backgroundImage)).toBe('none');
  await unchecked.check();
  await page.mouse.move(0, 0); // off the radio, so its border is not the hover one
  await expect(unchecked).toHaveCSS('border-top-color', brandPurple);
  await expect(unchecked).toHaveCSS('background-color', brandPurple);
  await context.close();
});

// Figma Home > Frame 1060 (7368:416692): the field and the card in one 348px column, 48px apart;
// on a phone (Frame 1081, 7368:418412) the 375px frame minus 16px on each side
for (const {viewport, columnWidth} of [{viewport: {width: 1440, height: 900}, columnWidth: 348}, {viewport: {width: 375, height: 812}, columnWidth: 343}]) {
  test(`landing search and feeds card line up at ${viewport.width}px`, async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.setViewportSize(viewport);
    await page.goto('/');

    const search = await page.locator('#home-search-input').boundingBox();
    const feeds = await page.locator('.home-feeds-card').boundingBox();
    expect(search.x).toBeCloseTo(feeds.x, 0);
    expect(search.x + search.width).toBeCloseTo(feeds.x + feeds.width, 0);
    expect(feeds.width).toBeCloseTo(columnWidth, 0);
    expect(feeds.y - (search.y + search.height)).toBeCloseTo(48, 0);
    // "Your feeds" (7368:416712): a 36px header in Inter 600 16/24
    const header = page.locator('.home-feeds-card-header');
    await expect(header).toHaveCSS('font-size', '16px');
    expect((await header.boundingBox()).height).toBeCloseTo(36, 0);
    // never wider than the page (the hero keeps 16px on each side on a phone)
    expect(feeds.x).toBeGreaterThanOrEqual(16);
    expect(feeds.x + feeds.width).toBeLessThanOrEqual(viewport.width - 16);
    await context.close();
  });
}

test('subjects are shown with the atom icon', async ({page}, workerInfo) => {
  // one subject per Playwright worker, which run at the same time
  const subject = `atom-${workerInfo.workerIndex}-${Date.now()}`;
  await create_article_via_api('user2', subject, subject);

  await page.goto(`/explore/subjects?q=${subject}`);
  // the Subjects tab
  await expect(page.locator('a[data-explore-tab-link][href*="/explore/subjects"] svg.forkana-atom')).toBeVisible();
  // the subject's row in the results
  const row = page.locator('.flex-item', {has: page.locator(`a[href$="/subject/${normalizeSubjectTitle(subject)}"]`)});
  await expect(row.locator('.flex-item-leading svg.forkana-atom')).toBeVisible();
  await expect(page.locator('svg.octicon-goal')).toHaveCount(0);
});
