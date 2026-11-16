import {test, expect, request} from '@playwright/test';
import {env} from 'node:process';
import {login_user, load_logged_in_context} from './utils_e2e.ts';
import type {Locator, Page} from '@playwright/test';

// The navbar's states (#532, Figma "Navbar – how it works (for dev)" 40001058:1049 in the
// Design System file). Fixtures: user2 owns repo1, which receives the open change requests
// #2, #3 and #5 (/pulls/<index>; issue ids 2, 3 and 11).

async function api_as(user: string) {
  return request.newContext({
    baseURL: `${(env.GITEA_URL || 'http://localhost:3000').replace(/\/+$/, '')}/`,
    extraHTTPHeaders: {Authorization: `Basic ${Buffer.from(`${user}:password`).toString('base64')}`},
  });
}

// the current page: bold dark text, aria-current, and neither a line nor a background
async function expect_current(item: Locator, label: Locator = item) {
  await expect(item).toHaveAttribute('aria-current', 'page');
  await expect(item).toHaveCSS('font-weight', '600');
  await expect(item).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
  for (const node of [item, label]) {
    await expect(node).toHaveCSS('text-decoration-line', 'none');
    await expect(node).toHaveCSS('box-shadow', 'none');
    await expect(node).toHaveCSS('border-bottom-width', '0px');
  }
}

async function expect_normal(item: Locator) {
  await expect(item).not.toHaveAttribute('aria-current');
  await expect(item).toHaveCSS('font-weight', '400');
  await expect(item).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
}

// on a phone the items are in the menu, where the current page is bold too
async function open_page(page: Page, url: string, isMobile: boolean) {
  await page.goto(url);
  if (isMobile) await page.locator('#navbar-expand-toggle').click();
}

// focus with the keyboard, so that it is :focus-visible. WebKit on macOS doesn't Tab to
// links by default: there, a key press then focus() is a keyboard focus too.
async function keyboard_focus(page: Page, item: Locator, browserName: string) {
  if (browserName === 'webkit') {
    await page.keyboard.press('Shift');
    await item.focus();
    return;
  }
  await item.focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab');
}

test.describe('Navbar (#532)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('the current page is bold, without a line or a background', async ({browser, isMobile}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    const changeRequests = page.locator('#navbar-change-requests');
    const label = changeRequests.locator('.navbar-item-label');
    const explore = page.locator('#navbar-explore');
    await open_page(page, '/pulls', isMobile);
    await expect(changeRequests).toHaveCSS('border-radius', '6px');
    await expect(label).toHaveText('Change requests');
    await expect_current(changeRequests, label);
    await expect_normal(explore);

    await open_page(page, '/explore/subjects', isMobile);
    await expect_current(explore);
    await expect_normal(changeRequests);

    // any other page: no current page
    await open_page(page, '/user/settings', isMobile);
    await expect_normal(changeRequests);
    await expect_normal(explore);
    await context.close();
  });

  test('hover and keyboard focus', async ({browser, browserName, isMobile}, workerInfo) => {
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(isMobile, 'no hover on touch screens');
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto('/user/settings');
    const explore = page.locator('#navbar-explore');

    await explore.hover();
    await expect(explore).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
    // the logo has no hover effect
    await page.locator('#navbar-logo').hover();
    await expect(page.locator('#navbar-logo')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');

    // the desktop item is a 32px invisible Button
    await expect(explore).toHaveCSS('height', '32px');
    await expect(explore).toHaveCSS('padding', '6px 12px');

    // keyboard focus: a 2px ring inside the item
    await keyboard_focus(page, explore, browserName);
    await expect(explore).toBeFocused();
    await expect(explore).toHaveCSS('outline-style', 'solid');
    await expect(explore).toHaveCSS('outline-width', '2px');
    await context.close();
  });

  test('the counter is primary while a counted change request is unread', async ({browser, isMobile}, workerInfo) => {
    // the notifications are user2's in every project: one project only, so that another
    // one doesn't read them while this one checks them
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(isMobile || workerInfo.project.name !== 'chromium', 'one project changes user2\'s notifications');
    test.setTimeout(60000);
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    const counter = page.locator('#navbar-change-requests .navbar-counter');

    // user1 mentions user2 on user2's change request issue2, whose notification user2 had
    // read: it is unread again (issue3's is pinned, which a new comment doesn't change)
    const user1 = await api_as('user1');
    try {
      const response = await user1.post('api/v1/repos/user2/repo1/issues/2/comments', {data: {body: '@user2 e2e #532: something new'}});
      expect(response.status(), await response.text()).toBe(201);
    } finally {
      await user1.dispose();
    }
    // notifications are created by a queue: reload until it has run
    await expect.poll(async () => {
      await page.goto('/explore/subjects');
      return counter.getAttribute('class');
    }, {timeout: 20000}).toContain('navbar-counter-primary');
    await expect(counter).toHaveText(/^[1-9]\d*$/);
    // CounterLabel primary: white on #59636e (fgColor/muted)
    await expect(counter).toHaveCSS('background-color', 'rgb(89, 99, 110)');
    // on every page, the hub included
    await page.goto('/pulls');
    await expect(counter).toHaveClass(/navbar-counter-primary/);

    // read: nothing new, the counter turns secondary
    const user2 = await api_as('user2');
    try {
      const response = await user2.put('api/v1/repos/user2/repo1/notifications?all=true&to-status=read');
      expect(response.status(), await response.text()).toBe(205);
    } finally {
      await user2.dispose();
    }
    await expect.poll(async () => {
      await page.goto('/explore/subjects');
      return counter.getAttribute('class');
    }, {timeout: 10000}).toContain('navbar-counter-secondary');
    await expect(counter).toHaveText(/^[1-9]\d*$/);
    await context.close();
  });
});
