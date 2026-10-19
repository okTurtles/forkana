import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// The Article view's Read, Edit and History tabs as a signed-out reader sees them
// (#433, #434, #435). user2 owns repo1, the article of "example-subject".
const article = '/subject/example-subject/user2';

test.describe('Article view layout (signed out)', () => {
  test('Read tab: tokens, Watch, path, no placeholders', async ({page}) => {
    await page.goto(`${article}?mode=read`);
    await expect(page.locator('#article-view-root[data-article-mode="read"]')).toBeVisible();

    // #433 items 1, 5 and 9: no Compare button, no placeholder line, no back link
    await expect(page.locator('#compare-mode-button')).toBeHidden();
    await expect(page.locator('[data-role="back-to-bubble"]')).toHaveCount(0);
    await expect(page.locator('#article-view-root')).not.toContainText('marked this article as correct');

    // item 2: "N Contributors" and "N Last updated: …"
    const tokens = page.locator('[data-role="article-tokens"]');
    await expect(tokens).toContainText(/\d+ Contributors?/);
    await expect(page.locator('[data-role="article-last-updated"]')).toContainText(/\d+ Last updated:/);
    await expect(page.locator('[data-role="article-last-updated"] [data-tooltip-content]').first()).toHaveAttribute('data-tooltip-content', /^\d+ commits?$/);

    // item 3: Watch at the end of the tabs, leading a signed-out reader to sign in
    const watch = page.locator('#article-tabs [data-role="article-watch"]');
    await expect(watch).toBeVisible();
    await expect(watch).toContainText('Watch');
    await expect(watch).toHaveAttribute('href', /\/user\/login\?redirect_to=/);
    await expect(page.locator('.repo-header-follow')).toBeHidden();

    // item 4: the path names the article by its subject, both parts are links
    const breadcrumb = page.locator('[data-role="article-breadcrumb"]');
    await expect(breadcrumb.locator('a')).toHaveCount(2);
    await expect(breadcrumb.locator('a').first()).toHaveText('user2');
    await expect(breadcrumb.locator('a').last()).toHaveText('example-subject');

    // the sign-in banner belongs to the Edit tab
    await expect(page.locator('#article-signin-banner')).toBeHidden();
  });

  test('Edit tab: banner, Fork and Submit changes lead to sign in, framed editor', async ({page}) => {
    await page.goto(`${article}?mode=edit`);
    await expect(page.locator('#article-view-root[data-article-mode="edit"]')).toBeVisible();

    // #434 item 13: the banner, which can be dismissed
    const banner = page.locator('#article-signin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('You are not logged in. You have to Sign in/Sign up in order to do edits.');
    await expect(banner.locator('a[href*="/user/login"]')).toHaveCount(1);
    await expect(banner.locator('a[href*="/user/sign_up"]')).toHaveCount(1);

    // items 8 and 9: Fork and Submit changes instead of "Sign in to Edit"
    await expect(page.locator('#article-tabs')).not.toContainText(/Sign in to Edit/i);
    for (const id of ['#signed-out-fork-button', '#signed-out-submit-button']) {
      await expect(page.locator(id)).toBeVisible();
      await expect(page.locator(id)).toHaveAttribute('href', /\/user\/login\?redirect_to=/);
    }

    // items 5, 10, 11 and 12: summary, framed editor with the figma's toolbar, the tip
    await expect(page.locator('[data-role="article-summary"]')).toContainText('Summary');
    await expect(page.locator('.article-editor-box .toastui-editor-defaultUI')).toBeVisible({timeout: 10000});
    for (const icon of ['heading', 'bold', 'italic', 'quote', 'code', 'link', 'bullet-list', 'ordered-list', 'task-list']) {
      // (on a narrow screen Toast UI folds the last ones into its "more" menu)
      await expect(page.locator(`.article-editor-box .toastui-editor-toolbar-icons.${icon}`)).toHaveCount(1);
    }
    const tip = page.locator('[data-role="article-contribution-tip"]');
    await expect(tip).toContainText('Remember, contributions to this article should follow its contributing guidelines and code of conduct.');
    await expect(tip.locator('a')).toHaveCount(2);

    await banner.locator('.article-signin-banner-dismiss').click();
    await expect(banner).toBeHidden();
  });

  test('History tab: rounded table, avatars, expandable messages, Sort and footer', async ({page}) => {
    await page.goto(`${article}?mode=history`);
    await expect(page.locator('#article-view-root[data-article-mode="history"]')).toBeVisible();

    // #435 item 7: no CopyRef here, item 6: the Sort menu instead
    await expect(page.locator('.article-copy-ref')).toHaveCount(0);
    await expect(page.locator('[data-role="article-history-sort"]')).toBeVisible();

    // items 8 and 10: rounded table frame, avatars with 6px corners
    const table = page.locator('[data-role="article-history"]');
    await expect(table).toHaveCSS('border-top-left-radius', '6px');
    await expect(table.locator('img.article-history-avatar').first()).toHaveCSS('border-top-left-radius', '6px');

    // item 12: a message expands to the full commit
    const details = table.locator('.article-history-details').first();
    await details.locator('summary').click();
    await expect(details).toHaveAttribute('open', '');
    await expect(details.locator('.article-history-sha')).toHaveText(/^[0-9a-f]{40}$/);

    // item 13: the footer
    await expect(table.locator('.article-history-count')).toHaveText(/^\d+-\d+ of \d+$/);
    await expect(table.locator('.article-history-pager')).toBeVisible();

    // Sort > Forks lists the article's forks (or says there are none)
    await page.locator('[data-role="article-history-sort"]').click();
    await page.locator('[data-role="article-history-sort"] .menu a.item[href*="history=forks"]').click();
    await expect(page).toHaveURL(/history=forks/);
    await expect(page.locator('[data-role="article-history"][data-sort="forks"], [data-role="article-history-no-forks"]').first()).toBeVisible();
  });

  test('switching to the Edit tab without reloading shows the banner', async ({page}) => {
    await page.goto(`${article}?mode=read`);
    await expect(page.locator('#article-signin-banner')).toBeHidden();
    await page.locator('#article-tabs a[data-article-tab="edit"]').click();
    await expect(page.locator('#article-view-root[data-article-mode="edit"]')).toBeVisible();
    await expect(page.locator('#article-signin-banner')).toBeVisible();
  });
});

test.describe('Article view Watch button (signed in)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user4');
  });

  // #433 item 3: the Watch button of a tab swapped in without a reload still posts through
  // htmx (with its CSRF header) and is answered with itself, and the subject header's
  // (hidden) Follow star follows it
  test('Watch works after switching tabs, and keeps the Follow star in step', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();
    await page.goto(`${article}?mode=edit`);
    // the sign-in banner is never in a signed-in reader's page
    await expect(page.locator('#article-view-root[data-article-mode="edit"]')).toBeVisible();
    await expect(page.locator('#article-signin-banner')).toHaveCount(0);
    await page.locator('#article-tabs a[data-article-tab="read"]').click();
    await expect(page.locator('#article-view-root[data-article-mode="read"]')).toBeVisible();

    const watch = page.locator('#article-tabs button[data-role="article-watch"]');
    const star = page.locator('#repo-header-follow-form .follow-article-button');
    const watching = await watch.getAttribute('aria-pressed') === 'true';
    await expect(star).toHaveAttribute('aria-label', watching ? 'Following' : 'Follow');
    const url = page.url();

    await watch.click();
    await expect(watch).toHaveAttribute('aria-pressed', watching ? 'false' : 'true');
    await expect(star).toHaveAttribute('aria-label', watching ? 'Follow' : 'Following');
    expect(page.url()).toBe(url);

    // nor after switching back to the Edit tab without a reload
    await page.locator('#article-tabs a[data-article-tab="edit"]').click();
    await expect(page.locator('#article-view-root[data-article-mode="edit"]')).toBeVisible();
    await expect(page.locator('#article-signin-banner')).toHaveCount(0);
    await page.locator('#article-tabs a[data-article-tab="read"]').click();
    await expect(page.locator('#article-view-root[data-article-mode="read"]')).toBeVisible();

    // and back, so that the fixture keeps its state
    await watch.click();
    await expect(watch).toHaveAttribute('aria-pressed', watching ? 'true' : 'false');
    await expect(star).toHaveAttribute('aria-label', watching ? 'Following' : 'Follow');
    await context.close();
  });
});
