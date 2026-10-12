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

    // a search that matches nothing still says so
    await page.goto('/feeds?q=no-such-activity');
    await expect(page.locator('.feeds-page-empty')).toHaveText('No results found.');
  } finally {
    await context.close();
  }
});
