import {test, expect} from '@playwright/test';
import type {Page} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// user2 owns repo1, the article of subject "example-subject" (#477, #461; figma section
// "08 · Settings and Collaborators flow" 7570:71939).
const articleURL = '/subject/example-subject/user2?view=article';
const settingsURL = `${articleURL}&mode=settings`;

// The projects run at the same time against the same server: each invites its own user,
// so that one project's invite does not show up in another's checks.
const inviteeByProject: Record<string, string> = {
  'chromium': 'user4',
  'webkit': 'user5',
  'Mobile Chrome': 'user8',
  'Mobile Safari': 'user10',
};

async function openPage(page: Page, url: string) {
  await page.goto(url);
  // the views' script is running (it records the view in the history), so that links
  // are followed in the page
  await page.waitForFunction(() => window.history.state !== null);
}

// the Invite button: in the box's header, or in the empty state when nobody is listed yet
function inviteButton(page: Page) {
  return page.locator('#article-settings-collaborators [data-article-settings-modal="#article-collaborator-invite-modal"]').first();
}

test.describe('Article settings', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  // #477: no Settings tab; the owner's "…" menu holds Settings, a divider, then Report
  // ("Profile" 7621:70837), and the page has "‹ Back" and a title instead of the tabs
  test('opens from the "…" menu, switches sections, and goes back to Read', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openPage(page, `${articleURL}&mode=read`);

    await expect(page.locator('#article-tabs .tabular.menu a.item')).toHaveText([/Read/, /Edit/, /History/]);
    await page.locator('#article-tabs .article-more-button').click();
    const menu = page.locator('[data-role="article-more"] > .menu');
    await expect(menu).toBeVisible();
    await expect(menu.locator(':scope > *')).toHaveClass([/article-more-settings/, /article-more-divider/, /article-more-report/]);
    await menu.locator('.article-more-settings').click();

    await expect(page.locator('#article-view-root[data-article-mode="settings"]')).toBeVisible({timeout: 10000});
    await expect(page.locator('#article-tabs')).toHaveCount(0);
    await expect(page.locator('.article-settings-heading')).toHaveText('Settings');
    // the selected segment is white with a purple border and label
    const general = page.locator('[data-article-settings-section="general"]');
    await expect(general).toHaveAttribute('aria-pressed', 'true');
    await expect(general).toHaveCSS('color', 'rgb(78, 64, 250)');
    await expect(general).toHaveCSS('background-color', 'rgb(255, 255, 255)');
    // General keeps its fields, read-only
    await expect(page.locator('#article-settings-owner')).toHaveAttribute('readonly', '');
    await expect(page.locator('#article-settings-owner')).toHaveValue('user2');

    await page.locator('[data-article-settings-section="collaborators"]').click();
    await expect(page.locator('#article-settings-collaborators')).toBeVisible();
    await expect(page.locator('#article-settings-general')).toBeHidden();

    await page.locator('.article-settings-back').click();
    await expect(page.locator('#article-view-root[data-article-mode="read"]')).toBeVisible({timeout: 10000});
    await context.close();
  });

  // #477: the dialogs' standard layout (grey header with the title and ×, grey footer
  // with Cancel and the action), "(required)" labels and no "*", Cancel without an icon
  test('dialogs: standard layout, "(required)" labels, Cancel without icon', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openPage(page, settingsURL);

    await page.locator('[data-article-settings-modal="#article-transfer-modal"]').click();
    const transfer = page.locator('#article-transfer-modal');
    await expect(transfer).toBeVisible();
    await expect(transfer.locator('.forkana-dialog-title')).toHaveText('Transfer ownership');
    await expect(transfer.locator('.forkana-dialog-banner-warning')).toHaveText(/You will lose access to this article once the new owner accepts the transfer\./);
    await expect(transfer.locator('.forkana-field-label')).toHaveText(['Article name (required)', 'New owner (required)']);
    await expect(transfer.locator('#article-transfer-submit')).toBeDisabled();
    const cancel = transfer.locator('.actions .cancel');
    await expect(cancel).toHaveText('Cancel');
    await expect(cancel.locator('svg')).toHaveCount(0);
    await transfer.locator('[data-dialog-close]').click();
    await expect(transfer).toBeHidden();

    // user2 owns the unarchived repo1, so all five dialogs are rendered: transfer, archive,
    // delete, invite collaborator and remove collaborator
    const cancelButtons = page.locator('.ui.modal.forkana-dialog[id^="article-"] .actions .cancel');
    await expect(cancelButtons).toHaveCount(5);
    await expect(cancelButtons.locator('svg')).toHaveCount(0);
    await expect(page.locator('.ui.modal[id^="article-"] .required.field')).toHaveCount(0);
    await expect(page.locator('.ui.modal[id^="article-"] label')).not.toContainText(['*']);
    await context.close();
  });

  // #461: "Invite" stays unavailable until someone is picked, the footer says why; then
  // the picked person is a chip, the access level shows and the button reads
  // "Invite {username}"
  test('invite: the button waits for a person, then names them', async ({browser}, workerInfo) => {
    const invitee = inviteeByProject[workerInfo.project.name] ?? 'user4';
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openPage(page, `${settingsURL}&section=collaborators`);

    await inviteButton(page).click();
    const dialog = page.locator('#article-collaborator-invite-modal');
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('.forkana-dialog-title')).toHaveText('Invite a collaborator');
    await expect(dialog.locator('label[for="article-collaborator-invitee"]')).toHaveText('User (required)');
    const submit = dialog.locator('#article-collaborator-invite-submit');
    await expect(submit).toHaveText('Invite');
    await expect(submit).toBeDisabled();
    await expect(dialog.locator('[data-role="invite-reason"]')).toHaveText('Pick a person to send the invite.');
    await expect(dialog.locator('[data-role="invite-access"]')).toBeHidden();

    await dialog.locator('#article-collaborator-invitee').fill(invitee);
    await dialog.locator(`.results .result:has(.description:text-is("@${invitee}"))`).click();
    await expect(dialog.locator('.article-user-search-login')).toHaveText(`@${invitee}`);
    await expect(submit).toHaveText(`Invite ${invitee}`);
    await expect(submit).toBeEnabled();
    await expect(dialog.locator('[data-role="invite-reason"]')).toBeHidden();
    await expect(dialog.locator('[data-role="invite-access"]')).toBeVisible();
    await expect(dialog.locator('[data-role="invite-access"] .forkana-field-label')).toHaveText('Access level (required)');

    // clearing the chip puts the dialog back
    await dialog.locator('.article-user-search-clear').click();
    await expect(submit).toBeDisabled();
    await expect(submit).toHaveText('Invite');
    await context.close();
  });

  // #461: the invited person shows "Waiting invite"; Remove asks first, shows a refusal in
  // the dialog ("Remove – states" 7675:74699), and once removed the banner says so
  test('remove: confirmation, error state, then success banner', async ({browser}, workerInfo) => {
    const invitee = inviteeByProject[workerInfo.project.name] ?? 'user4';
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openPage(page, `${settingsURL}&section=collaborators`);

    await inviteButton(page).click();
    const dialog = page.locator('#article-collaborator-invite-modal');
    await dialog.locator('#article-collaborator-invitee').fill(invitee);
    await dialog.locator(`.results .result:has(.description:text-is("@${invitee}"))`).click();
    await dialog.locator('#article-collaborator-invite-submit').click();
    await expect(page.locator('.flash-message.flash-success')).toContainText('has been invited to collaborate on this article.');

    const row = page.locator(`[data-article-collaborator]:has(a[href$="/${invitee}"])`);
    await expect(row.locator('.forkana-label-attention')).toHaveText('Waiting invite');
    await expect(page.locator('[data-article-collaborator-owner]')).toContainText('(you)');

    await row.locator('[data-article-collaborator-remove]').click();
    const remove = page.locator('#article-collaborators-remove-modal');
    await expect(remove).toBeVisible();
    await expect(remove.locator('.forkana-dialog-title')).toHaveText(/^Cancel the invite for .+\?$/);
    const confirm = remove.locator('#article-collaborators-remove-submit');
    await expect(confirm).toHaveText('Cancel invite');

    // a refusal: the banner under the header, and the buttons work again
    await page.route('**/article_collaborators/remove', (route) => route.fulfill({
      status: 400, contentType: 'application/json', body: JSON.stringify({errorMessage: 'Something went wrong.'}),
    }));
    await confirm.click();
    await expect(remove.locator('[data-role="remove-error"]')).toBeVisible();
    await expect(remove.locator('[data-role="remove-error"]')).toHaveText('Something went wrong.');
    await expect(confirm).toBeEnabled();
    await expect(confirm).toHaveText('Cancel invite');
    await page.unroute('**/article_collaborators/remove');

    await confirm.click();
    await expect(page.locator('.flash-message.flash-success')).toContainText(/^The invite for .+ has been cancelled\.$/);
    await expect(page.locator(`[data-article-collaborator]:has(a[href$="/${invitee}"])`)).toHaveCount(0);
    await context.close();
  });
});

// #477: on a phone, Read · Edit · History without icons, then Watch (an eye) and "…" at
// the end of the same row ("Read – Watch icon + "…" (mobile, owner)" 7596:66930)
test('on a phone: tabs without icons, Watch and "…" on their row', async ({page}) => {
  await page.setViewportSize({width: 375, height: 812});
  await page.goto(`${articleURL}&mode=read`);
  const row = await page.locator('#article-tabs').boundingBox();
  expect(row.height).toBe(40);
  await expect(page.locator('#article-tabs .tabular.menu a.item .svg').first()).toBeHidden();
  const read = await page.locator('#article-tabs a[data-article-tab="read"]').boundingBox();
  expect(read.width).toBeCloseTo(66, -1); // TabNav.Link 7596:67066: 16 + "Read" + 16
  const more = await page.locator('#article-tabs .article-more-button').boundingBox();
  expect(more.y).toBeGreaterThanOrEqual(row.y);
  expect(more.y + more.height).toBeLessThanOrEqual(row.y + row.height);
  expect(more.x + more.width).toBeCloseTo(row.x + row.width, 0);
});
