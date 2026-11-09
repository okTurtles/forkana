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
const inviteeFor = (project: string) => inviteeByProject[project] ?? 'user4';

async function openPage(page: Page, url: string) {
  await page.goto(url);
  // the views' script is running (it records the view in the history), so that links
  // are followed in the page
  await page.waitForFunction(() => window.history.state !== null);
}

// the delegate each project makes, for the same reason
const delegateByProject: Record<string, string> = {
  'chromium': 'user13',
  'webkit': 'user15',
  'Mobile Chrome': 'user16',
  'Mobile Safari': 'user18',
};
const delegateFor = (project: string) => delegateByProject[project] ?? 'user13';

// the person each project's delegate picks in the invite dialog (never invited)
const delegatePickByProject: Record<string, string> = {
  'chromium': 'user20',
  'webkit': 'user21',
  'Mobile Chrome': 'user24',
  'Mobile Safari': 'user27',
};
const delegatePickFor = (project: string) => delegatePickByProject[project] ?? 'user20';

// the Invite button: in the box's header, or in the empty state when nobody is listed yet
function inviteButton(page: Page) {
  return page.locator('#article-settings-collaborators [data-article-settings-modal="#article-collaborator-invite-modal"]').first();
}

// Removes the user from the collaborators shown on the owner's page, if listed (a retry
// after a failure, or the end of a test), with the page's own form and CSRF token, then
// reloads the collaborators.
async function removeIfListed(page: Page, name: string) {
  const listed = await page.evaluate((user) => {
    const checkbox = document.querySelector<HTMLInputElement>(`[data-article-collaborator]:has(a[href$="/${user}"]) [data-article-collaborator-select]`);
    const form = document.querySelector<HTMLFormElement>('#article-collaborators-remove-form');
    return {
      uid: checkbox?.value ?? '',
      action: form?.action ?? '',
      csrf: form?.querySelector<HTMLInputElement>('input[name="_csrf"]')?.value ?? '',
    };
  }, name);
  await Promise.all(listed.uid ? [page.request.post(listed.action, {form: {_csrf: listed.csrf, uid: listed.uid}})] : []);
  await openPage(page, `${settingsURL}&section=collaborators`);
}

async function invite(page: Page, name: string, delegate = false) {
  await inviteButton(page).click();
  const dialog = page.locator('#article-collaborator-invite-modal');
  await dialog.locator('#article-collaborator-invitee').fill(name);
  await dialog.locator(`.results .result:has(.description:text-is("@${name}"))`).click();
  if (delegate) {
    await dialog.locator('#article-collaborator-role').click();
    await dialog.locator('#article-collaborator-role .item[data-value="2"]').click();
  }
  await dialog.locator('#article-collaborator-invite-submit').click();
  await expect(page.locator('.flash-message.flash-success')).toContainText('has been invited to collaborate on this article.');
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
    // a dialog, named by its title
    await expect(transfer).toHaveAttribute('role', 'dialog');
    await expect(page.getByRole('dialog', {name: 'Transfer ownership'})).toBeVisible();
    await transfer.locator('[data-dialog-close]').click();
    await expect(transfer).toBeHidden();

    // the confirmation is the article as shown, "user2 / example-subject", the spaces
    // around the slash being optional, as on the server
    await page.locator('[data-article-settings-modal="#article-archive-modal"]').click();
    const archive = page.locator('#article-archive-modal');
    const archiveSubmit = archive.locator('#article-archive-submit');
    await expect(archiveSubmit).toBeDisabled();
    for (const typed of ['user2 / example-subject', 'user2/example-subject']) {
      await archive.locator('#article-archive-confirm-name').fill(typed);
      await expect(archiveSubmit).toBeEnabled();
    }
    await archive.locator('#article-archive-confirm-name').fill('user2 / example');
    await expect(archiveSubmit).toBeDisabled();
    await archive.locator('.actions .cancel').click();
    await expect(archive).toBeHidden();

    // user2 owns the unarchived repo1, so all five dialogs are rendered: transfer, archive,
    // delete, invite collaborator and remove collaborator
    const cancelButtons = page.locator('.ui.modal.forkana-dialog[id^="article-"] .actions .cancel');
    await expect(cancelButtons).toHaveCount(5);
    await expect(cancelButtons.locator('svg')).toHaveCount(0);
    await expect(page.locator('.ui.modal[id^="article-"] .required.field')).toHaveCount(0);
    await expect(page.locator('.ui.modal[id^="article-"] label')).not.toContainText(['*']);
    await context.close();
  });

  // #477: on a phone the dialogs are bottom sheets (Dialog 7622:69567): full width, their
  // bottom edge on the screen's, 16px top corners
  test('on a phone: the dialogs open as bottom sheets', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.setViewportSize({width: 375, height: 812});
    await openPage(page, settingsURL);
    await page.locator('[data-article-settings-modal="#article-delete-modal"]').click();
    const dialog = page.locator('#article-delete-modal');
    await expect(dialog).toBeVisible();
    // (once the dimmer's opening animation is over)
    await expect.poll(async () => {
      const box = await dialog.boundingBox();
      return [Math.round(box.x), Math.round(box.width), Math.round(box.y + box.height)];
    }).toEqual([0, 375, 812]);
    await expect(dialog).toHaveCSS('border-top-left-radius', '16px');
    await expect(dialog).toHaveCSS('border-bottom-left-radius', '0px');
    // the main action first, Cancel under it
    const action = await dialog.locator('#article-delete-submit').boundingBox();
    const cancel = await dialog.locator('.actions .cancel').boundingBox();
    expect(action.y).toBeLessThan(cancel.y);
    expect(action.width).toBeCloseTo(343, 0);
    await context.close();
  });

  // #461: "Invite" stays unavailable until someone is picked, the footer says why; then
  // the picked person is a chip, the access level shows and the button reads
  // "Invite {username}"
  test('invite: the button waits for a person, then names them', async ({browser}, workerInfo) => {
    const invitee = inviteeFor(workerInfo.project.name);
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
    const invitee = inviteeFor(workerInfo.project.name);
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openPage(page, `${settingsURL}&section=collaborators`);

    // a retry after a failure finds the invitee still listed: they go first
    await removeIfListed(page, invitee);
    await invite(page, invitee);

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
    await expect(page.locator('.flash-message.flash-success')).toContainText(/The invite for .+ has been canceled\./);
    await expect(page.locator(`[data-article-collaborator]:has(a[href$="/${invitee}"])`)).toHaveCount(0);
    await context.close();
  });

  // #477 (issuecomment-6727300520): a Maintainer delegate's "…" menu has "Collaborators"
  // instead of Settings; it opens the Collaborators section alone, without the General |
  // Collaborators switch or the General boxes
  test('a delegate: "Collaborators" in the "…" menu, and that section alone', async ({browser}, workerInfo) => {
    const delegate = delegateFor(workerInfo.project.name);
    await login_user(browser, workerInfo, delegate);
    const ownerContext = await load_logged_in_context(browser, workerInfo, 'user2');
    const owner = await ownerContext.newPage();
    await openPage(owner, `${settingsURL}&section=collaborators`);
    await removeIfListed(owner, delegate);
    await invite(owner, delegate, true);

    const delegateContext = await load_logged_in_context(browser, workerInfo, delegate);
    const page = await delegateContext.newPage();
    await openPage(page, `${articleURL}&mode=read`);
    await page.locator('#article-collaborator-invite-notice form[action$="/accept"] button').click();
    await expect(page.locator('.flash-message.flash-success')).toBeVisible();
    await openPage(page, `${articleURL}&mode=read`);

    await page.locator('#article-tabs .article-more-button').click();
    const item = page.locator('[data-role="article-more"] > .menu > .article-more-settings');
    await expect(item).toHaveText('Collaborators');
    await item.click();
    await expect(page.locator('#article-view-root[data-article-mode="settings"]')).toBeVisible({timeout: 10000});
    await expect(page.locator('.article-settings-heading')).toHaveText('Collaborators');
    await expect(page.locator('#article-settings-collaborators')).toBeVisible();
    await expect(page.locator('.forkana-segmented')).toHaveCount(0);
    await expect(page.locator('[data-article-settings-panel="general"]')).toHaveCount(0);

    // Pam's rules ("NEW · Delegate – Collaborators" 8020:64472): no actions on the owner's
    // row or on a delegate's, their own included; the line says what they can do
    await expect(page.locator('[data-role="collaborators-summary"]')).toContainText('(owner), you');
    await expect(page.locator('[data-role="collaborators-summary"]')).toContainText('You can invite and remove Maintainers.');
    const ownRow = page.locator(`[data-article-collaborator]:has(a[href$="/${delegate}"])`);
    await expect(ownRow).toContainText('(you)');
    await expect(ownRow.locator('.article-collaborator-role-text')).toHaveText('Maintainer delegate');
    await expect(ownRow.locator('input, button, .ui.dropdown')).toHaveCount(0);
    await expect(page.locator('[data-article-collaborator-owner]').locator('input, button, .ui.dropdown')).toHaveCount(0);

    // the invite ("– Invite, access level fixed" 8020:65052), on a desktop and as the
    // phone's bottom sheet, opened from the title's row there (8020:64781, 8022:64527)
    const pick = delegatePickFor(workerInfo.project.name);
    for (const {width, opener} of [
      {width: 1280, opener: '.article-collaborators-header [data-article-settings-modal]'},
      {width: 375, opener: '.article-settings-title-row [data-article-settings-modal]'},
    ]) {
      await page.setViewportSize({width, height: 812});
      await page.locator(opener).click();
      const dialog = page.locator('#article-collaborator-invite-modal');
      await expect(dialog).toBeVisible();
      await dialog.locator('#article-collaborator-invitee').fill(pick);
      await dialog.locator(`.results .result:has(.description:text-is("@${pick}"))`).click();
      const role = dialog.locator('#article-collaborator-role');
      await expect(role).toBeDisabled();
      await expect(role).toHaveText('Maintainer');
      await expect(dialog.locator('#article-collaborator-role-reason')).toHaveText('Only the owner can add Maintainer delegates.');
      await expect(dialog.locator('input[name="role"]')).toHaveValue('1');
      await expect(dialog.locator('#article-collaborator-invite-submit')).toHaveText(`Invite ${pick}`);
      await dialog.locator('.actions .cancel').click();
      await expect(dialog).toBeHidden();
    }
    // on the phone, the section's own heading row gives way to the title's
    await expect(page.locator('.article-collaborators-header')).toBeHidden();
    await expect(page.locator('.article-settings-title-row [data-article-settings-modal]')).toHaveText('Invite');
    await delegateContext.close();

    // so that the next run starts without them
    await openPage(owner, `${settingsURL}&section=collaborators`);
    await removeIfListed(owner, delegate);
    await expect(owner.locator(`[data-article-collaborator]:has(a[href$="/${delegate}"])`)).toHaveCount(0);
    await ownerContext.close();
  });
});

// #477: on a phone, Read · Edit · History without icons, then Watch (an eye) and "…" at
// the end of the same row ("Read – Watch icon + "…" (mobile, owner)" 7596:66930)
test('on a phone: tabs without icons, Watch and "…" on their row', async ({page}) => {
  await page.setViewportSize({width: 375, height: 812});
  await page.goto(`${articleURL}&mode=read`);
  const row = await page.locator('#article-tabs').boundingBox();
  expect(row.height).toBe(41); // the 40px tabs and the 1px rule under them
  await expect(page.locator('#article-tabs .tabular.menu a.item .svg').first()).toBeHidden();
  const read = await page.locator('#article-tabs a[data-article-tab="read"]').boundingBox();
  expect(read.width).toBeCloseTo(66, -1); // TabNav.Link 7596:67066: 16 + "Read" + 16
  const more = await page.locator('#article-tabs .article-more-button').boundingBox();
  expect(more.y).toBeGreaterThanOrEqual(row.y);
  expect(more.y + more.height).toBeLessThanOrEqual(row.y + row.height);
  expect(more.x + more.width).toBeCloseTo(row.x + row.width, 0);
});
