// #404: design updates on the landing search, the sign-in page and the feeds.
import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// The design system's secondary button background (#f6f8fa) and text colour (#25292e)
const secondaryBg = 'rgb(246, 248, 250)';
const secondaryText = 'rgb(37, 41, 46)';

test.beforeAll(async ({browser}, workerInfo) => {
  // user5 has no activity in the fixtures, so its feed is empty
  await login_user(browser, workerInfo, 'user5');
});

// The landing hero after the two Landing frames of Figma. Each test pins the frame's own viewport,
// so the values are the same in every Playwright project (the mobile ones run at 390-393px):
// Desktop 6313:56396 (1440 wide) and Mobile 6313:56412 (375 wide, below tailwind's 640px "sm").
const landingFrames = [
  {name: 'Desktop', viewport: {width: 1440, height: 900}, titleSize: '32px', titleLineHeight: '48px', logo: [124, 122], logoGap: 60, fieldWidth: 348, fieldX: 546},
  {name: 'Mobile', viewport: {width: 375, height: 812}, titleSize: '24px', titleLineHeight: '36px', logo: [68, 67], logoGap: 48, fieldWidth: 343, fieldX: 16},
];

for (const frame of landingFrames) {
  test(`landing hero matches the ${frame.name} Landing frame`, async ({page}) => {
    await page.setViewportSize(frame.viewport);
    await page.goto('/');

    // logo, then the title in Inter 700 (6313:56405 / 6313:56424)
    const logoBox = await page.locator('.home-hero img.logo').boundingBox();
    expect([logoBox.width, logoBox.height]).toEqual(frame.logo);
    const title = page.locator('.home-hero-title');
    await expect(title).toHaveCSS('font-size', frame.titleSize);
    await expect(title).toHaveCSS('font-weight', '700');
    await expect(title).toHaveCSS('line-height', frame.titleLineHeight);
    await expect(title).toHaveCSS('letter-spacing', 'normal');
    const titleBox = await title.boundingBox();
    expect(titleBox.y - (logoBox.y + logoBox.height)).toBeCloseTo(frame.logoGap, 0);

    // the field (641:65360 / 6313:56425): 16px under the title, 40px tall, Inter 14/20,
    // 12px side padding + 16px icon + 8px gap before the text (35px inside the 1px border);
    // on mobile it spans the 375px frame minus 16px on each side
    const input = page.locator('#home-search-input');
    const box = await input.boundingBox();
    expect(box.y - (titleBox.y + titleBox.height)).toBeCloseTo(16, 0);
    expect(box.x).toBeCloseTo(frame.fieldX, 0);
    expect(box.width).toBeCloseTo(frame.fieldWidth, 0);
    expect(box.height).toBeCloseTo(40, 0);
    await expect(input).toHaveCSS('font-size', '14px');
    await expect(input).toHaveCSS('line-height', '20px');
    await expect(input).toHaveCSS('padding-left', '35px');
  });
}

test('landing search button is a secondary button and Enter still searches', async ({page}) => {
  await page.goto('/');
  const input = page.locator('#home-search-input');
  const button = page.locator('#home-search-button');
  await expect(button).toBeHidden();

  await input.fill('moon');
  await expect(button).toBeVisible();
  await expect(button).toHaveCSS('background-color', secondaryBg);
  await expect(button).toHaveCSS('color', secondaryText);
  await expect(button).toHaveCSS('height', '26px');

  await input.press('Enter');
  await page.waitForURL(/\/explore\/subjects\?q=moon/);
});

test('sign-in page: tertiary "Forgot password?" button and "Create an account" link', async ({page}) => {
  await page.goto('/user/login');

  const forgot = page.locator('a[href$="/user/forgot_password"]');
  await expect(forgot).toHaveCSS('text-decoration-line', 'none');
  await expect(forgot).toHaveCSS('height', '26px');
  await expect(forgot).toHaveCSS('padding-left', '7px');
  await forgot.hover();
  await expect(forgot).toHaveCSS('text-decoration-line', 'underline');

  const createAccount = page.locator('.auth-alternative-box a[href$="/user/sign_up"]');
  await expect(createAccount).toHaveText('Create an account');
  await expect(createAccount).toHaveCSS('text-decoration-line', 'underline');
  await expect(createAccount).toHaveCSS('padding-left', '0px');
  // the box around the sentence is not a link itself
  await expect(page.locator('a.auth-alternative-box')).toHaveCount(0);
});

test('empty feed shows the empty state on the home page and on /feeds', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, 'user5');
  const page = await context.newPage();
  try {
    await page.goto('/');
    await expect(page.locator('.home-feeds-empty')).toHaveText('It\'s quiet in here… for now');
    await expect(page.locator('.home-feeds-see-all')).toHaveCount(0);

    await page.goto('/feeds');
    await expect(page.locator('.feeds-page-empty')).toHaveText('It\'s quiet in here… for now');

    // an empty "?filter=" (hand-crafted URL) is not a filter
    await page.goto('/feeds?filter=');
    await expect(page.locator('.feeds-page-empty')).toHaveText('It\'s quiet in here… for now');

    // a search that matches nothing still says so
    await page.goto('/feeds?q=no-such-activity');
    await expect(page.locator('.feeds-page-empty')).toHaveText('No results found.');
  } finally {
    await context.close();
  }
});
