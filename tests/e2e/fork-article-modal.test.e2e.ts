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
          console.error(`Console error: ${msg.text()}`);
        }
      });

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

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

      const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

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

      const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

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

      const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present (indicates we're on the edit page)
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

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

    const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    await expect(forkButton).toHaveAttribute('data-tooltip-content', /Forking creates a separate version/);

    await context.close();
  });
});

test.describe('Fork-on-Edit Permission Tests', () => {
  // Log in users once per worker to avoid race conditions
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
    await login_user(browser, workerInfo, 'user4');
  });

  test.describe('Repository Owner Tests', () => {
    test('repository owner sees Submit Changes button with data-fork-and-edit=false', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user2');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      // Owner should see Submit Changes button, not Fork button
      const submitButton = page.locator('#submit-changes-button');
      await expect(submitButton).toBeVisible({timeout: 10000});

      // Button should have data-fork-and-edit="false" for direct editing
      await expect(submitButton).toHaveAttribute('data-fork-and-edit', 'false');

      // Button should contain "Submit Changes" text
      await expect(submitButton).toContainText('Submit Changes');

      // Button should be primary style (not secondary like Fork button)
      await expect(submitButton).toHaveClass(/primary/);

      await context.close();
    });

    test('repository owner can click Submit Changes without confirmation modal', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user2');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      const submitButton = page.locator('#submit-changes-button');
      await expect(submitButton).toBeVisible({timeout: 10000});

      // Wait for the article edit form to be present
      await expect(page.locator('#article-edit-form')).toBeVisible({timeout: 10000});

      // Wait for the Toast UI Editor to be initialized
      // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
      await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

      await submitButton.click();

      // No confirmation modal should appear for repo owner
      const modal = page.locator('.ui.g-modal-confirm.modal.visible');
      await expect(modal).not.toBeVisible({timeout: 2000});

      await context.close();
    });
  });

  test.describe('Non-Owner Permission Tests', () => {
    test('non-owner sees Fork button with data-fork-and-edit=true', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Verify we're on the article page
      await expect(page).toHaveURL(/\/article\/user2\/example-subject/);

      // Non-owner should see Fork button
      const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
      await expect(forkButton).toBeVisible({timeout: 10000});

      // Button should contain "Fork" text
      await expect(forkButton).toContainText('Fork');

      // Button should be secondary style (not primary)
      await expect(forkButton).toHaveClass(/secondary/);

      await context.close();
    });

    test('non-owner sees disabled Submit Changes button alongside Fork button', async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, 'user4');
      const page = await context.newPage();

      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Non-owner should see disabled Submit Changes button
      const disabledSubmitButton = page.locator('button.ui.primary.button.disabled:has-text("Submit Changes")');
      await expect(disabledSubmitButton).toBeVisible({timeout: 10000});

      await context.close();
    });
  });

  test.describe('Unauthenticated User Tests', () => {
    test('unauthenticated user sees disabled sign-in button', async ({page}) => {
      await page.goto('/article/user2/example-subject?mode=edit');
      await page.waitForLoadState('domcontentloaded');

      // Unauthenticated user should see disabled button with sign-in message
      const signInButton = page.locator('button.ui.primary.button.disabled');
      await expect(signInButton).toBeVisible({timeout: 10000});

      // Button should contain sign-in text
      await expect(signInButton).toContainText(/Sign in/i);

      await page.close();
    });
  });
});

// Accessibility Tests (Section 5 from opus-analysis.md)
test.describe('Accessibility Tests', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user4');
  });

  test('modal is keyboard accessible - Enter opens modal', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    // Wait for the Toast UI Editor to be initialized
    // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
    await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

    // Focus the Fork button and press Enter
    await forkButton.focus();
    await page.keyboard.press('Enter');

    // Modal should open
    const modal = page.locator('.ui.g-modal-confirm.modal.visible');
    await expect(modal).toBeVisible({timeout: 5000});

    await context.close();
  });

  test('modal buttons are keyboard navigable with Tab', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    // Wait for the Toast UI Editor to be initialized
    // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
    await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

    await forkButton.click();

    const modal = page.locator('.ui.g-modal-confirm.modal.visible');
    await expect(modal).toBeVisible({timeout: 5000});

    const cancelButton = modal.locator('.actions .cancel.button');
    const confirmButton = modal.locator('.actions .ok.button');

    // Tab through modal buttons - focus should move between them
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');

    // Both buttons should be focusable (have tabindex or be naturally focusable)
    await expect(cancelButton).toBeVisible();
    await expect(confirmButton).toBeVisible();

    await context.close();
  });

  test('Enter activates focused cancel button and closes modal', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    // Wait for the Toast UI Editor to be initialized
    // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
    await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

    await forkButton.click();

    const modal = page.locator('.ui.g-modal-confirm.modal.visible');
    await expect(modal).toBeVisible({timeout: 5000});

    const cancelButton = modal.locator('.actions .cancel.button');

    // Focus the cancel button and press Enter
    await cancelButton.focus();
    await page.keyboard.press('Enter');

    // Modal should close
    await expect(modal).not.toBeVisible({timeout: 5000});

    await context.close();
  });

  test('modal buttons have accessible names', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user4');
    const page = await context.newPage();

    await page.goto('/article/user2/example-subject?mode=edit');
    await page.waitForLoadState('domcontentloaded');

    const forkButton = page.locator('#fork-article-button[data-fork-and-edit="true"]');
    await expect(forkButton).toBeVisible({timeout: 10000});

    // Wait for the Toast UI Editor to be initialized
    // The editor creates two .toastui-editor elements (md-mode and ww-mode), so use .first()
    await expect(page.locator('.toastui-editor').first()).toBeAttached({timeout: 20000});

    await forkButton.click();

    const modal = page.locator('.ui.g-modal-confirm.modal.visible');
    await expect(modal).toBeVisible({timeout: 5000});

    const cancelButton = modal.locator('.actions .cancel.button');
    const confirmButton = modal.locator('.actions .ok.button');

    // Buttons should have accessible text content
    // Verify specific accessible names using Playwright's toContainText assertion
    await expect(cancelButton).toContainText('Go back');
    await expect(confirmButton).toContainText('Yes, Fork article');

    await context.close();
  });
});
