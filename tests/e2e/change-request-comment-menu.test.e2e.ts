import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';
import type {Locator, Page} from '@playwright/test';

// Fixtures: user2 owns repo1 (subject "example-subject"); /user2/repo1/pulls/3 is an open
// change request posted by user1, who is also a site admin.

async function setCommentEditorValue(page: Page, value: string): Promise<void> {
  const container = page.locator('#comment-form .toast-comment-editor');
  await expect(container.locator('.toastui-editor-defaultUI')).toBeVisible({timeout: 20000});
  await container.evaluate((el, text) => {
    const editor = (el as HTMLElement & {_giteaToastCommentEditor?: {value: (v?: string) => string}})._giteaToastCommentEditor;
    if (!editor) throw new Error('Toast editor is not initialized');
    editor.value(text);
    el.dispatchEvent(new CustomEvent('ce-editor-content-changed'));
  }, value);
}

async function expectLockedMenu(item: Locator): Promise<void> {
  await item.locator('.context-dropdown .context-menu').click();
  const menu = item.locator('.context-dropdown .menu');
  await expect(menu).toBeVisible();
  await expect(menu).toContainText(/copy link/i);
  await expect(menu).toContainText(/quote reply/i);
  await expect(menu.locator('.edit-content')).toHaveCount(0);
  await expect(menu.locator('.delete-comment')).toHaveCount(0);
  await expect(menu.locator('.reference-issue')).toHaveCount(0);
  await item.page().keyboard.press('Escape');
}

test.describe('Change request comment menu (#146)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user1');
  });

  test('no Edit, Delete or Reference in new issue, even for the admin poster', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user1');
    const page = await context.newPage();
    await page.goto('/user2/repo1/pulls/3');

    const text = `e2e comment ${workerInfo.project.name} ${Date.now()}`;
    await setCommentEditorValue(page, text);
    await page.locator('#comment-button').click();
    await page.waitForURL(/#issuecomment-[0-9]+$/, {timeout: 20000});
    await page.goto('/user2/repo1/pulls/3');

    // the change request description, then the comment just posted
    await expectLockedMenu(page.locator('.timeline-item.comment.first'));
    await expectLockedMenu(page.locator('.timeline-item.comment', {hasText: text}));
    await expect(page.locator('#reference-issue-modal')).toHaveCount(0);
    await context.close();
  });
});
