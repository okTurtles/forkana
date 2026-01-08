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

      // Navigate to article edit page
      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Wait for the article content to be present
      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});

      // Non-owner should see both Fork and Submit Change Request buttons
      // Fork button for fork-and-edit workflow
      const forkButton = page.locator('#fork-article-button');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Submit Change Request button for submit-change-request workflow
      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Button should contain appropriate text
      await expect(submitCRButton).toContainText(/Submit/i);

      await context.close();
    });

    test('owner sees Submit Changes button without data-submit-change-request attribute', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user2');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Wait for the article content to be present
      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});

      // Owner should see Submit Changes button without submit-change-request attribute
      const submitButton = page.locator('#submit-changes-button');
      await expect(submitButton).toBeVisible({timeout: 10000});

      // Should NOT have the data-submit-change-request attribute (owner edits directly)
      const hasSubmitCRAttr = await submitButton.getAttribute('data-submit-change-request');
      expect(hasSubmitCRAttr).toBeNull();

      // Fork button should NOT be visible for owner
      const forkButton = page.locator('#fork-article-button');
      await expect(forkButton).toBeHidden();

      await context.close();
    });
  });

  test.describe('Modal Confirmation Tests', () => {
    test('clicking Submit Change Request shows confirmation modal', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Wait for the article view to be ready
      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Scroll button into view and ensure it's clickable
      await submitCRButton.scrollIntoViewIfNeeded();
      // eslint-disable-next-line playwright/no-wait-for-timeout
      await page.waitForTimeout(500);

      // Use force click for mobile browsers to avoid click interception issues
      // eslint-disable-next-line playwright/no-force-option
      await submitCRButton.click({force: true});

      // Modal should appear with confirmation message (longer timeout for mobile)
      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 10000});

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

      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const urlBefore = page.url();

      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Scroll button into view and ensure it's clickable
      await submitCRButton.scrollIntoViewIfNeeded();
      // eslint-disable-next-line playwright/no-wait-for-timeout
      await page.waitForTimeout(500);

      // Use force click for mobile browsers to avoid click interception issues
      // eslint-disable-next-line playwright/no-force-option
      await submitCRButton.click({force: true});

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 10000});

      // Wait for modal animation to complete
      // eslint-disable-next-line playwright/no-wait-for-timeout
      await page.waitForTimeout(300);

      // Click cancel button - use force click to avoid dimmer interception on mobile
      const cancelButton = modal.locator('.actions .cancel.button');
      // eslint-disable-next-line playwright/no-force-option
      await cancelButton.click({force: true});

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

      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
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

      await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
      await expect(submitCRButton).toBeVisible({timeout: 10000});

      // Scroll button into view and ensure it's clickable
      await submitCRButton.scrollIntoViewIfNeeded();
      // eslint-disable-next-line playwright/no-wait-for-timeout
      await page.waitForTimeout(500);

      // Use force click for mobile browsers to avoid click interception issues
      // eslint-disable-next-line playwright/no-force-option
      await submitCRButton.click({force: true});

      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).toBeVisible({timeout: 10000});

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

      const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
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

    // Wait for the article view to be present
    await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});

    // Unauthenticated user should see a disabled button with sign-in message
    const signInButton = page.locator('button.ui.primary.button.disabled');
    await expect(signInButton).toBeVisible({timeout: 10000});

    // Should contain "Sign in to Edit" text (from locale: repo.editor.sign_in_to_edit)
    await expect(signInButton).toContainText(/Sign in to Edit/i);

    // The button should have the submit-changes-button ID but be disabled
    // (unauthenticated users don't see fork or submit-change-request buttons)
    await expect(signInButton).toHaveAttribute('type', 'button');

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

    await expect(page.locator('#article-view-root')).toBeVisible({timeout: 10000});

    // Fork button should be visible (for fork-and-edit workflow)
    const forkButton = page.locator('#fork-article-button');
    await expect(forkButton).toBeVisible({timeout: 10000});
    await expect(forkButton).toContainText(/Fork/i);

    // Submit Change Request button should also be visible (for in-repo CR workflow)
    const submitCRButton = page.locator('#submit-changes-button[data-submit-change-request="true"]');
    await expect(submitCRButton).toBeVisible({timeout: 10000});
    await expect(submitCRButton).toContainText(/Submit/i);

    // Verify they are different buttons
    await expect(forkButton).not.toHaveAttribute('id', await submitCRButton.getAttribute('id'));

    await context.close();
  });
});
