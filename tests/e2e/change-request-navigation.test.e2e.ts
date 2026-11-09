import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// Fixtures: user2 owns repo1 (subject "example-subject"); /user2/repo1/pulls/3 is an open
// change request posted by user1.

test.describe('Change request navigation (#326)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user4');
  });

  test('the title links to the article and the description no longer names it', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();
    await page.goto('/user2/repo1/pulls/3');

    const titleLink = page.locator('.pr-repo-title a#pr-article-link');
    await expect(titleLink).toHaveText('user2 / example-subject');
    await expect(titleLink).toHaveAttribute('href', '/subject/example-subject/user2');

    const desc = page.locator('#pull-desc-display');
    await expect(desc).toContainText('to this article');
    await expect(desc).not.toContainText('user2/repo1');

    // the navbar carries no "View modes" entry
    await expect(page.locator('#navbar')).not.toContainText('View modes');

    await titleLink.click();
    await expect(page).toHaveURL(/\/subject\/example-subject\/user2$/);
    await expect(page.locator('#article-view-root')).toBeVisible();
    await context.close();
  });
});

// #408 removed the "..." menu's "Something" placeholder; #478 brings the "…" menu back
// after Watch in Read and History (figma Action 7368:264704), holding only Report, not
// available yet. The Edit tab has none (Fork and Submit changes end its row).
test.describe('Article view tabs (#408, #478)', () => {
  for (const mode of ['read', 'history']) {
    test(`the "…" menu after Watch, with no placeholder, in ${mode} mode`, async ({page}) => {
      await page.goto(`/subject/example-subject/user2?mode=${mode}`);
      const tabs = page.locator('#article-tabs');
      await expect(tabs).toBeVisible();
      await expect(tabs).not.toContainText('Something');
      const more = tabs.locator('[data-role="article-more"]');
      await expect(more).toHaveCount(1);
      // after Watch, in the row's trailing part
      await expect(tabs.locator('.article-tabs-trailing > .article-watch + [data-role="article-more"]')).toHaveCount(1);
      const items = more.locator('.menu > .item');
      await expect(items).toHaveCount(1);
      await expect(items).toHaveText('Report');
      await expect(items).toHaveAttribute('aria-disabled', 'true');
    });
  }

  test('no "…" menu in edit mode', async ({page}) => {
    await page.goto('/subject/example-subject/user2?mode=edit');
    const tabs = page.locator('#article-tabs');
    await expect(tabs).toBeVisible();
    await expect(tabs.locator('[data-role="article-more"]')).toHaveCount(0);
    await expect(tabs).not.toContainText('Something');
  });
});
