import {test, expect, request} from '@playwright/test';
import {env} from 'node:process';
import {create_article_via_api, login_user, load_logged_in_context} from './utils_e2e.ts';

// #156: an article whose owner is fork-only (an import account) takes no change requests.
// Its Edit tab says so, and Submit changes creates the editor's own article instead.
// user18 owns the article (no other e2e test uses that account, so flagging it, and never
// clearing it, is harmless: concurrent projects only ever set it), user4 edits it. That an
// article of a user accepting change requests shows no notice is in the integration test
// (TestForkOnlyArticle).
const owner = 'user18';
const editor = 'user4';
const run = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const subject = `fork-only-${run}`;
const editURL = `/subject/${encodeURIComponent(subject)}/${owner}?mode=edit`;

// mark the owner fork-only as the site admin, user1, through the admin API
async function markForkOnly() {
  const baseURL = `${(env.GITEA_URL || 'http://localhost:3000').replace(/\/+$/, '')}/`;
  const api = await request.newContext({
    baseURL,
    extraHTTPHeaders: {Authorization: `Basic ${Buffer.from('user1:password').toString('base64')}`},
  });
  try {
    const response = await api.patch(`api/v1/admin/users/${owner}`, {
      data: {source_id: 0, login_name: owner, fork_only: true},
    });
    expect(response.status(), await response.text()).toBe(200);
  } finally {
    await api.dispose();
  }
}

test.beforeAll(async ({browser}, workerInfo) => {
  await create_article_via_api(owner, subject, `${subject}-${owner}`, 'An imported article');
  await markForkOnly();
  await login_user(browser, workerInfo, editor);
});

test('Edit tab of a fork-only article: the notice, and Submit changes forks it', async ({browser}, workerInfo) => {
  const context = await load_logged_in_context(browser, workerInfo, editor);
  const page = await context.newPage();
  await page.goto(editURL);
  await expect(page.locator('.article-editor-box .toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});

  const notice = page.locator('[data-role="article-fork-only-notice"]');
  await expect(notice).toBeVisible();
  await expect(notice).toHaveText(`This will create a new article because the user ${owner} doesn't accept change requests`);
  // the Edit tab's info message, as for an editor who already has an article for the subject
  await expect(notice).toHaveClass(/\bui info message\b/);

  // no change request modal: Submit changes commits to a new fork
  await expect(page.locator('#pre-submit-changes-button')).toHaveCount(0);
  const submit = page.locator('#submit-changes-button');
  await expect(submit).toHaveText('Submit changes');
  await submit.click();
  await page.waitForURL((url) => url.pathname.includes(`/${editor}`) && !url.pathname.includes('/pulls/'), {timeout: 20000});
  expect(page.url()).toContain('mode=read');
});
