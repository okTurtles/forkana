import {test, expect} from '@playwright/test';
import {
  login_user,
  load_logged_in_context,
} from './utils_e2e.ts';

/**
 * E2E tests for the "first user to create article is owner" feature.
 */

// Use unique subject name per test run to avoid conflicts
const TEST_SUBJECT_PREFIX = 'e2e-first-article';

test.describe('First Article Becomes Root', () => {
  // Login users before all tests
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('first user can create article with subject', async ({browser}, workerInfo) => {
    const subjectName = `${TEST_SUBJECT_PREFIX}-${workerInfo.workerIndex}-${Date.now()}`;
    const repoName = subjectName.toLowerCase().replace(/\s+/g, '-');

    // Login as user2
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();

    try {
      // Step 1: Navigate to create repository page with subject pre-filled
      await page.goto(`/repo/create?subject=${encodeURIComponent(subjectName)}`);

      // Step 2: Verify subject field is populated
      await expect(page.locator('input#subject')).toHaveValue(subjectName);

      // Step 3: Check the template requirements checkbox (required to enable submit button)
      const templateRequirementsCheckbox = page.locator('input#template_requirements');
      await templateRequirementsCheckbox.waitFor({state: 'visible', timeout: 10000});
      await templateRequirementsCheckbox.check();

      // Step 4: Wait for submit button to be enabled and click
      const submitButton = page.locator('#create-repo-submit-button');
      await expect(submitButton).toBeEnabled({timeout: 5000});
      await submitButton.click();

      // Step 5: Wait for navigation to the subject page
      await page.waitForURL(/\/subject\//, {timeout: 30000});

      // Step 6: Verify we are on the subject page
      expect(page.url()).toContain('/subject/');

      // Step 7: Navigate to the repository page to verify it was created
      await page.goto(`/user2/${repoName}`);
      await page.waitForLoadState('domcontentloaded');

      // Step 8: Verify the repository exists (either empty or has content)
      // The page should load successfully (not 404)
      const pageTitle = page.locator('title');
      const titleText = await pageTitle.textContent({timeout: 10000});
      expect(titleText).toContain(repoName);
    } finally {
      // Cleanup: try to delete the repository
      try {
        await page.goto(`/user2/${repoName}/settings`);
        await page.waitForLoadState('domcontentloaded');

        // Look for delete button in danger zone and click it
        const deleteButton = page.locator('button:has-text("Delete This Repository")');
        await expect(deleteButton).toBeVisible({timeout: 5000});
        await deleteButton.click();

        // Wait for modal
        const modal = page.locator('#delete-repo-modal');
        await modal.waitFor({state: 'visible', timeout: 5000});

        // Fill repo name and confirm
        await page.locator('#delete-repo-modal input[name=repo_name]').fill(repoName);
        await page.locator('#delete-repo-modal button:has-text("Delete Repository")').click();

        // Wait for navigation away from settings
        await page.waitForURL('**/', {timeout: 10000});
      } catch {
        // Cleanup failed, that's okay for test purposes
      }
      await context.close();
    }
  });
});
