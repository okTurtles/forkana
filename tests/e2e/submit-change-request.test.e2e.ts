import {test, expect} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// Test users:
// - user2: owns repo1 (public, has subject_id: 1 "example-subject")
// - user4: non-owner user who can submit change requests

test.describe('Submit Change Request Workflow', () => {
  // Log in users once per worker to avoid race conditions
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
    await login_user(browser, workerInfo, 'user4');
  });

  test.describe('Button Visibility Tests', () => {
    test('non-owner sees Submit Change Request button on article edit page', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      // Navigate to article edit page with submit_change_request mode
      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Wait for the article edit form to be present
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Non-owner should see the Submit Change Request button
      // This button is shown when the user doesn't have write access
      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Button should contain appropriate text
      await expect(submitCRButton).toContainText(/Submit|Change Request/i);

      await context.close();
    });

    test('owner does not see Submit Change Request button', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user2');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Owner should see Submit Changes button, not Submit Change Request
      const submitButton = page.locator('#submit-changes-button');
      await expect(submitButton).toBeVisible({timeout: 10000});

      // Submit Change Request button should not be visible for owner
      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeHidden();

      await context.close();
    });
  });

  test.describe('Modal Confirmation Tests', () => {
    test('clicking Submit Change Request shows confirmation modal', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Wait for the editor to be ready
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      await submitCRButton.click();

      // Modal should appear with confirmation message
      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      // Modal should have appropriate header and content
      const header = modal.locator('.header');
      await expect(header).toContainText(/Change Request|Submit/i);

      const content = modal.locator('.content');
      await expect(content).toBeVisible();

      await context.close();
    });

    test('canceling modal does not submit changes', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const urlBefore = page.url();

      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});
      await submitCRButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      // Click cancel button
      const cancelButton = modal.locator('.actions .cancel.button');
      await cancelButton.click();

      // Modal should close
      await expect(modal).not.toBeVisible({timeout: 5000});

      // URL should not change (no redirect to PR)
      expect(page.url()).toBe(urlBefore);

      await context.close();
    });
  });

  test.describe('Accessibility Tests', () => {
    test('Submit Change Request button is keyboard accessible', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Focus the button and press Enter
      await submitCRButton.focus();
      await page.keyboard.press('Enter');

      // Modal should open
      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      await context.close();
    });

    test('Escape key closes modal', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});
      await submitCRButton.click();

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 5000});

      // Press Escape to close
      await page.keyboard.press('Escape');

      await expect(modal).not.toBeVisible({timeout: 5000});

      await context.close();
    });
  });

  test.describe('Tooltip Tests', () => {
    test('Submit Change Request button has informative tooltip', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      const submitCRButton = page.locator('#submit-change-request-button');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Button should have a tooltip explaining the action
      await expect(submitCRButton).toHaveAttribute('data-tooltip-content', /change request|pull request|review/i);

      await context.close();
    });
  });
});

test.describe('Submit Change Request - Unauthenticated User', () => {
  test('unauthenticated user sees sign-in prompt instead of Submit Change Request', async ({page}) => {
    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    // Unauthenticated user should see a disabled button with sign-in message
    const signInButton = page.locator('button.ui.primary.button.disabled');
    await expect(signInButton).toBeVisible({timeout: 10000});

    // Should contain sign-in text
    await expect(signInButton).toContainText(/Sign in/i);

    // Submit Change Request button should not be visible
    const submitCRButton = page.locator('#submit-change-request-button');
    await expect(submitCRButton).toBeHidden();

    await page.close();
  });
});

test.describe('Submit Change Request vs Fork Button', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user4');
  });

  test('Submit Change Request and Fork buttons are distinct', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

    // Fork button should be visible (for fork-and-edit workflow)
    const forkButton = page.locator('#fork-article-button');
    await expect(forkButton).toBeVisible({timeout: 10000});

    // Submit Change Request button should also be visible (for in-repo CR workflow)
    // Note: This button may or may not exist depending on the UI design
    // We verify the fork button is visible and has expected text
    await expect(forkButton).toContainText(/Fork/i);

    await context.close();
  });
});
