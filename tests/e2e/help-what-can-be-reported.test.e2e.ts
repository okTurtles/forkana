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
    // screen readers don't read the image's description before the warning
    await expect(warning.locator('.help-cw-image')).toHaveAttribute('aria-hidden', 'true');

    await show.click();
    await expect(hide).toBeVisible();
    await expect(hide).toBeFocused();
    await expect(warning.locator('.help-cw-image')).toHaveCSS('filter', 'none');
    await expect(warning.locator('.help-cw-image')).not.toHaveAttribute('aria-hidden');

    await hide.click();
    await expect(show).toBeVisible();
    await expect(show).toBeFocused();

    // a reload blurs it again
    await show.click();
    await page.reload();
    await expect(warning.getByRole('button', {name: 'Show image'})).toBeVisible();
  });

  test('leaving the page blurs the image again, for a return through Back', async ({page}) => {
    // Back and Forward may restore the page from the back/forward cache, as it was left. The
    // headless browsers don't reliably use that cache, so this sends the event leaving the page
    // fires ("pagehide", persisted as when the page goes into the cache) and checks what the
    // cached page would show.
    await page.setViewportSize({width: 1440, height: 900});
    await page.goto('/help/what-can-be-reported');
    const warning = page.locator('#graphic-violence .help-cw');
    await warning.getByRole('button', {name: 'Show image'}).click();
    await expect(warning.locator('.help-cw-image')).toHaveCSS('filter', 'none');

    await page.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pagehide', {persisted: true})));
    await expect(warning.getByRole('button', {name: 'Show image'})).toBeVisible();
    await expect(warning.locator('.help-cw-image')).toHaveCSS('filter', /blur/);
    await expect(warning.locator('.help-cw-image')).toHaveAttribute('aria-hidden', 'true');
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

    // a link to a reason on this page opens it (hashchange)
    await page.goto('/help/what-can-be-reported#off-topic-or-spam');
    await expect(page.locator('#off-topic-or-spam .help-reason-body')).toBeVisible();
  });

  test('on a phone a link from another page opens its reason', async ({page}) => {
    // the report form's links: the page loads with the hash already set
    await page.setViewportSize({width: 375, height: 812});
    await page.goto('/help/what-can-be-reported?redirect_to=%2Fexplore%2Fsubjects#doxxing');
    await expect(page.locator('#doxxing .help-reason-body')).toBeVisible();

    // Back leads to the report form, as the phone has no sidebar
    const back = page.locator('a.help-back');
    await expect(back).toBeVisible();
    await expect(back).toHaveAttribute('href', '/explore/subjects');
  });

  test('a malformed hash leaves the page working', async ({page}) => {
    const errors: string[] = [];
    page.on('pageerror', (err) => errors.push(err.message));
    await page.setViewportSize({width: 375, height: 812});
    await page.goto('/help/what-can-be-reported#%');

    // the accordion still answers, so the page's start-up ran to the end
    const toggle = page.locator('#doxxing').getByRole('button', {name: 'Doxxing'});
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(errors).toEqual([]);
  });
});
