// #460: purple radio buttons, the landing search and feeds sharing one column, and the atom
// icon for subjects.
import {test, expect, request} from '@playwright/test';
import {env} from 'node:process';
import {create_article_via_api, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// --color-primary, the brand purple #4E40FA
const brandPurple = 'rgb(78, 64, 250)';

test.beforeAll(async ({browser}, workerInfo) => {
  await login_user(browser, workerInfo, 'user2');
  await login_user(browser, workerInfo, 'user4');
});

// An API client signed in as a fixture user (they all have the password "password")
const apiAs = (user: string) => request.newContext({
  baseURL: `${(env.GITEA_URL || 'http://localhost:3000').replace(/\/+$/, '')}/`,
  extraHTTPHeaders: {Authorization: `Basic ${Buffer.from(`${user}:password`).toString('base64')}`},
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
    // "Your feeds" (7368:416712): 36px from the card's outer edge, Inter 600 16/24
    const header = page.locator('.home-feeds-card-header');
    await expect(header).toHaveCSS('font-size', '16px');
    await expect(header).toHaveCSS('font-weight', '600');
    const headerBox = await header.boundingBox();
    expect(headerBox.y + headerBox.height - feeds.y).toBeCloseTo(36, 0);
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

// A feed item after Figma Home > Frame 1063 (7368:416733 and its siblings 7368:416721,
// 7368:416742), at the desktop and phone widths
for (const viewport of [{width: 1440, height: 900}, {width: 375, height: 812}]) {
  test(`landing feed items match the Figma rows at ${viewport.width}px`, async ({browser}, workerInfo) => {
    // user4 watches an article of user2, which user2 then edits: user4's feed gets an item
    const name = `feed-item-${workerInfo.workerIndex}-${viewport.width}-${Date.now()}`;
    await create_article_via_api('user2', name, name);
    const reader = await apiAs('user4');
    expect((await reader.put(`api/v1/repos/user2/${name}/subscription`)).status()).toBe(200);
    await reader.dispose();
    const writer = await apiAs('user2');
    const readme = await (await writer.get(`api/v1/repos/user2/${name}/contents/README.md`)).json();
    const edit = await writer.put(`api/v1/repos/user2/${name}/contents/README.md`, {
      data: {content: Buffer.from('# Edited\n').toString('base64'), sha: readme.sha, message: 'Edit'},
    });
    expect(edit.status()).toBe(200);
    await writer.dispose();

    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();
    await page.setViewportSize(viewport);
    await page.goto('/');

    const muted = 'rgb(89, 99, 110)'; // #59636e
    const card = await page.locator('.home-feeds-card').boundingBox();
    const item = page.locator('.home-feed-item').first();
    const itemBox = await item.boundingBox();
    // 16px in from the card's outer edges, 8px above and below
    expect(itemBox.x - card.x).toBeCloseTo(16, 0);
    expect(card.x + card.width - (itemBox.x + itemBox.width)).toBeCloseTo(16, 0);
    await expect(item).toHaveCSS('padding-top', '8px');
    await expect(item).toHaveCSS('padding-bottom', '8px');

    // the avatar (7368:416736): 24px, radius 6, a 1px #d1d9e0 stroke, 4px under the text's top
    const avatar = item.locator('.home-feed-item-avatar img');
    const avatarBox = await avatar.boundingBox();
    expect([avatarBox.width, avatarBox.height]).toEqual([24, 24]);
    await expect(avatar).toHaveCSS('border-radius', '6px');
    await expect(avatar).toHaveCSS('border-top-color', 'rgb(209, 217, 224)');

    // the sentence (7368:416739): Inter 12/18 in #59636e, 400 with the names in 600, 12px
    // after the avatar
    const action = item.locator('.home-feed-item-action');
    const actionBox = await action.boundingBox();
    expect(actionBox.x - (avatarBox.x + avatarBox.width)).toBeCloseTo(12, 0);
    expect(avatarBox.y - actionBox.y).toBeCloseTo(4, 0);
    await expect(action).toHaveCSS('font-size', '12px');
    await expect(action).toHaveCSS('line-height', '18px');
    await expect(action).toHaveCSS('font-weight', '400');
    await expect(action).toHaveCSS('color', muted);
    await expect(action.locator('a').first()).toHaveCSS('font-weight', '600');
    await expect(action.locator('a').first()).toHaveCSS('color', muted);

    // the time (7368:416740): 10px in #59636e, 4px under the sentence
    const time = item.locator('.home-feed-item-time');
    await expect(time).toHaveCSS('font-size', '10px');
    await expect(time).toHaveCSS('font-weight', '400');
    await expect(time).toHaveCSS('color', muted);
    expect((await time.boundingBox()).y - (actionBox.y + actionBox.height)).toBeCloseTo(4, 0);

    // the action icon (7368:416741): 16px in #1f2328, 12px after the text
    const icon = item.locator('.home-feed-item-icon svg');
    const iconBox = await icon.boundingBox();
    expect([iconBox.width, iconBox.height]).toEqual([16, 16]);
    await expect(icon).toHaveCSS('color', 'rgb(31, 35, 40)');
    expect(iconBox.x - (actionBox.x + actionBox.width)).toBeCloseTo(12, 0);
    await context.close();
  });
}
