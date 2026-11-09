import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// user2 owns repo1, the article of subject "example-subject".
const settingsURL = '/subject/example-subject/user2?view=article&mode=settings';

test.describe('Article settings modals', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  // Figma (#337, frames "477 · Transfer/Archive/Delete – dialog" and the
  // collaborator dialogs) shows the Cancel button as text only, without an X icon.
  test('Cancel buttons have no icon', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto(settingsURL);

    await page.locator('[data-article-settings-modal="#article-transfer-modal"]').click();
    const transferModal = page.locator('#article-transfer-modal');
    await expect(transferModal).toBeVisible();
    const cancel = transferModal.locator('.actions .cancel.button');
    await expect(cancel).toHaveText('Cancel');
    await expect(cancel.locator('svg')).toHaveCount(0);

    // user2 owns the unarchived repo1, so all five dialogs are rendered: transfer,
    // archive, delete, invite collaborator and remove collaborator.
    const cancelButtons = page.locator('.ui.modal[id^="article-"] .actions .cancel.button');
    await expect(cancelButtons).toHaveCount(5);
    await expect(cancelButtons.locator('svg')).toHaveCount(0);

    await context.close();
  });
});
