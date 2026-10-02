import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// Test users:
// - user2: owns repo1 (public, has subject_id: 1 "example-subject")

test.describe('Article Editor Direct Submit', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  // Regression: the article editor used to be initialized twice on a full page load, so the
  // owner's Submit Changes posted the unchanged content of a detached editor instead of the edit.
  test('owner Submit Changes posts the edited content', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const submitButton = page.locator('#submit-changes-button');
    await expect(submitButton).toBeVisible({timeout: 10000});
    await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

    const marker = `edited-by-e2e-${Date.now()}`;
    const editor = page.locator('.toastui-editor-ww-container .ProseMirror');
    await editor.click();
    await page.keyboard.press('ControlOrMeta+End');
    await page.keyboard.type(` ${marker}`);

    // Capture the commit request and answer it here, so the fixture repository is not modified.
    let postedBody = '';
    let editRequests = 0;
    await page.route('**/_edit/**', async (route) => {
      editRequests++;
      postedBody = route.request().postData() ?? '';
      await route.fulfill({status: 200, contentType: 'application/json', body: '{}'});
    });

    // Use force click for mobile browsers to avoid click interception issues
    await submitButton.scrollIntoViewIfNeeded();
    // eslint-disable-next-line playwright/no-force-option
    await submitButton.click({force: true});

    await expect.poll(() => editRequests, {timeout: 10000}).toBe(1);
    expect(postedBody).toContain(marker);

    await context.close();
  });
});
