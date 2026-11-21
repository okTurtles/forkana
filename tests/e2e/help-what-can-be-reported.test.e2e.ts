import {test, expect} from '@playwright/test';

// "What can be reported" (#541, Figma 8159:873 and, on a phone, 8159:1144). The page is public.

test.describe('What can be reported (#541)', () => {
  test('the example image stays blurred until shown, for this visit only', async ({page}) => {
    await page.setViewportSize({width: 1440, height: 900});
    await page.goto('/help/what-can-be-reported');

    const warning = page.locator('#graphic-violence .help-cw');
    const show = warning.getByRole('button', {name: 'Show image'});
    const hide = warning.getByRole('button', {name: 'Hide image'});
    await expect(show).toBeVisible();
    await expect(hide).toBeHidden();
    await expect(warning.locator('.help-cw-image')).toHaveCSS('filter', /blur/);

    await show.click();
    await expect(hide).toBeVisible();
    await expect(hide).toBeFocused();
    await expect(warning.locator('.help-cw-image')).toHaveCSS('filter', 'none');

    await hide.click();
    await expect(show).toBeVisible();
    await expect(show).toBeFocused();

    // a reload blurs it again
    await show.click();
    await page.reload();
    await expect(warning.getByRole('button', {name: 'Show image'})).toBeVisible();
  });

  test('on a desktop every reason is shown, with the Help sidebar', async ({page}) => {
    await page.setViewportSize({width: 1440, height: 900});
    await page.goto('/help/what-can-be-reported');

    await expect(page.locator('.help-nav .help-nav-item.active')).toHaveText('What can be reported');
    for (const reason of await page.locator('.help-reason').all()) {
      await expect(reason.locator('.help-reason-body')).toBeVisible();
    }
  });

  test('on a phone the reasons are an accordion, the first one open', async ({page}) => {
    await page.setViewportSize({width: 375, height: 812});
    await page.goto('/help/what-can-be-reported');

    await expect(page.locator('.help-nav')).toBeHidden();
    const childSafety = page.locator('#child-safety');
    const doxxing = page.locator('#doxxing');
    await expect(childSafety.locator('.help-reason-body')).toBeVisible();
    await expect(doxxing.locator('.help-reason-body')).toBeHidden();

    const toggle = doxxing.getByRole('button', {name: 'Doxxing'});
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(doxxing.locator('.help-reason-body')).toBeVisible();
    await toggle.click();
    await expect(doxxing.locator('.help-reason-body')).toBeHidden();

    // a link to a reason opens it
    await page.goto('/help/what-can-be-reported#off-topic-or-spam');
    await expect(page.locator('#off-topic-or-spam .help-reason-body')).toBeVisible();
  });
});
