import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// Test users:
// - user2: owns repo1 (public, has subject_id: 1 "example-subject")
// - user4: non-owner user who can fork

test.describe('Fork Article Confirmation Modal', () => {
  // Log in users once per worker to avoid race conditions
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
    await login_user(browser, workerInfo, 'user4');
  });

  test.describe('Modal Appearance Tests', () => {
    test('modal appears when clicking Fork button', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      // Listen for console errors to debug JavaScript issues
      page.on('console', (msg) => {
        if (msg.type() === 'error') {
          console.log(`Console error: ${msg.text()}`);
        }
      });

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      const forkButton = page.locator('#submit-changes-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates a .toastui-editor element when initialized
      await page.waitForSelector('.toastui-editor', {state: 'attached', timeout: 20000});

      await forkButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      const header = modal.locator('.header');
      await expect(header).toBeVisible();
      await expect(header).toContainText('Are you sure you want to Fork');

      const content = modal.locator('.content');
      await expect(content).toBeVisible();
      await expect(content).toContainText('Forking creates a separate version');

      const cancelButton = modal.locator('.actions .cancel.button');
      const confirmButton = modal.locator('.actions .ok.button');
      await expect(cancelButton).toBeVisible();
      await expect(confirmButton).toBeVisible();

      await context.close();
    });

    test('modal does not appear for direct edit (repo owner)', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user2');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      const submitButton = page.locator('#submit-changes-button');
      await expect(submitButton).toBeVisible({timeout: 10000});

      await expect(submitButton).not.toHaveAttribute('data-fork-and-edit', 'true');

      await context.close();
    });

    test('modal button text matches locale strings', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      const forkButton = page.locator('#submit-changes-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      await page.waitForSelector('.toastui-editor', {state: 'attached', timeout: 20000});

      await forkButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      const cancelButton = modal.locator('.actions .cancel.button');
      await expect(cancelButton).toContainText('Go back');

      const confirmButton = modal.locator('.actions .ok.button');
      await expect(confirmButton).toContainText('Yes, Fork article');

      await expect(cancelButton.locator('svg.octicon-x')).toBeVisible();
      await expect(confirmButton.locator('svg.octicon-check')).toBeVisible();

      await context.close();
    });
  });

  test.describe('Modal Cancel Action Tests', () => {
    test('clicking Go back closes modal without action', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      const forkButton = page.locator('#submit-changes-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      await page.waitForSelector('.toastui-editor', {state: 'attached', timeout: 20000});

      const urlBefore = page.url();

      await forkButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      const cancelButton = modal.locator('.actions .cancel.button');
      await cancelButton.click();

      await expect(modal).not.toBeVisible({timeout: 5000});
      expect(page.url()).toBe(urlBefore);

      await context.close();
    });

    test('pressing Escape closes modal without action', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      const forkButton = page.locator('#submit-changes-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      await page.waitForSelector('.toastui-editor', {state: 'attached', timeout: 20000});

      const urlBefore = page.url();

      await forkButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      await page.keyboard.press('Escape');

      await expect(modal).not.toBeVisible({timeout: 5000});
      expect(page.url()).toBe(urlBefore);

      await context.close();
    });
  });
});

test.describe('Fork Button Tooltip', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user4');
  });

  test('fork button has tooltip with confirmation message', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const forkButton = page.locator('#submit-changes-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    await expect(forkButton).toHaveAttribute('data-tooltip-content', /Forking creates a separate version/);

    await context.close();
  });
});
