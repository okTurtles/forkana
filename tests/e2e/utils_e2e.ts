import {expect} from '@playwright/test';
import {env} from 'node:process';
import type {Browser, Page, WorkerInfo} from '@playwright/test';

const ARTIFACTS_PATH = `tests/e2e/test-artifacts`;
const LOGIN_PASSWORD = 'password';

// log in user and store session info. This should generally be
//  run in test.beforeAll(), then the session can be loaded in tests.
export async function login_user(browser: Browser, workerInfo: WorkerInfo, user: string) {
  // Set up a new context
  const context = await browser.newContext();
  const page = await context.newPage();

  // Route to login page
  // Note: this could probably be done more quickly with a POST
  const response = await page.goto('/user/login');
  expect(response?.status()).toBe(200); // Status OK

  // Fill out form
  await page.locator('input[name=user_name]').fill(user);
  await page.locator('input[name=password]').fill(LOGIN_PASSWORD);
  await page.click('button:has-text("Sign In")');

  await page.waitForURL(`${workerInfo.project.use.baseURL}/`, {timeout: 10000});

  expect(page.url(), {message: `Failed to login user ${user}`}).toBe(`${workerInfo.project.use.baseURL}/`);

  // Save state
  await context.storageState({path: `${ARTIFACTS_PATH}/state-${user}-${workerInfo.workerIndex}.json`});

  return context;
}

export async function load_logged_in_context(browser: Browser, workerInfo: WorkerInfo, user: string) {
  let context;
  try {
    context = await browser.newContext({storageState: `${ARTIFACTS_PATH}/state-${user}-${workerInfo.workerIndex}.json`});
  } catch (err) {
    if (err.code === 'ENOENT') {
      throw new Error(`Could not find state for '${user}'. Did you call login_user(browser, workerInfo, '${user}') in test.beforeAll()?`);
    }
  }
  return context;
}

export async function save_visual(page: Page) {
  // Optionally include visual testing
  if (env.VISUAL_TEST) {
    await page.waitForLoadState('networkidle'); // eslint-disable-line playwright/no-networkidle
    // Mock page/version string
    await page.locator('footer div.ui.left').evaluate((node) => node.innerHTML = 'MOCK');
    await expect(page).toHaveScreenshot({
      fullPage: true,
      timeout: 20000,
      mask: [
        page.locator('.secondary-nav span>img.ui.avatar'),
        page.locator('.ui.dropdown.jump.item span>img.ui.avatar'),
      ],
    });
  }
}

/**
 * Create an article (repository with subject) via the UI.
 * This navigates to the repo creation page with the subject prefilled,
 * submits the form, and returns the created repository URL.
 */
export async function create_article(page: Page, _workerInfo: WorkerInfo, subjectName: string): Promise<string> {
  // Go to the create first article endpoint
  const response = await page.goto(`/repo/create-first-article?subject=${encodeURIComponent(subjectName)}`);
  expect(response?.status()).toBe(200);

  // Wait for redirect to the editor (for empty repo) or bubble view
  await page.waitForURL(/\/_new\/|\/subject\//, {timeout: 15000});

  // Return the current URL which should be either the editor or subject/bubble view
  return page.url();
}

/**
 * Create an article by going through the standard repo create form.
 * This is useful when we want to create an empty repo without auto-redirect to editor.
 */
export async function create_repo_with_subject(page: Page, _workerInfo: WorkerInfo, subjectName: string, repoName?: string): Promise<string> {
  // Navigate to create repository page with subject pre-filled
  const response = await page.goto(`/repo/create?subject=${encodeURIComponent(subjectName)}`);
  expect(response?.status()).toBe(200);

  // If a custom repo name is provided, fill it in
  if (repoName) {
    await page.locator('input[name=repo_name]').fill(repoName);
  }

  // Submit the form - button text is "Create subject" in the custom locale
  await page.click('button.ui.primary.button');

  // Wait for redirect (should go to subject bubble view on success)
  await page.waitForURL(/\/subject\//, {timeout: 30000});

  return page.url();
}

/**
 * Commit a file to the current repository via the web editor.
 * The page should already be on the editor for the repository.
 */
export async function commit_file_via_editor(page: Page, _workerInfo: WorkerInfo, options: {
  filename: string;
  content: string;
  commitMessage?: string;
}): Promise<void> {
  const {filename, content, commitMessage = 'Add file via e2e test'} = options;

  // Wait for the editor to be ready (CodeMirror)
  await page.locator('.cm-content').waitFor({state: 'visible', timeout: 10000});

  // Set filename if the field exists
  const filenameInput = page.locator('input[name=tree_path]');
  if (await filenameInput.isVisible()) {
    await filenameInput.fill(filename);
  }

  // Fill in the content via CodeMirror
  const editor = page.locator('.cm-content');
  await editor.click();
  // Select all and replace
  await page.keyboard.press('Meta+a');
  await page.keyboard.type(content);

  // Set commit message
  const commitMsgInput = page.locator('input[name=commit_summary]');
  if (await commitMsgInput.isVisible()) {
    await commitMsgInput.fill(commitMessage);
  }

  // Submit the commit
  await page.click('button:has-text("Commit Changes")');

  // Wait for redirect after commit
  await page.waitForURL(/\/src\/|\/article\//, {timeout: 15000});
}

/**
 * Delete a subject by name via the admin API.
 * Note: This requires the logged-in user to have admin permissions.
 * For non-admin cleanup, use delete_repo instead.
 */
export async function delete_subject_repos(page: Page, _workerInfo: WorkerInfo, subjectName: string): Promise<void> {
  // Navigate to the subject page to get repository links
  const response = await page.goto(`/subject/${encodeURIComponent(subjectName)}?view=bubble`);

  // If subject doesn't exist (404), nothing to clean up
  if (response?.status() === 404) {
    // Subject not found, nothing to delete
  }

  // TODO: Get all repos that belong to this subject and delete them
  // This is done through the settings page of each repo
  // For simplicity in tests, we'll navigate to each repo's settings and delete
}

/**
 * Delete a repository via its settings page.
 */
export async function delete_repo(page: Page, _workerInfo: WorkerInfo, owner: string, repoName: string): Promise<boolean> {
  // Navigate to the repository settings page
  const settingsUrl = `/${owner}/${repoName}/settings`;
  const response = await page.goto(settingsUrl);

  // If repo doesn't exist, nothing to delete
  if (response?.status() === 404) {
    return false;
  }

  // Scroll to danger zone and click delete
  const deleteButton = page.locator('button[data-modal="#delete-repo-modal"]');
  if (!(await deleteButton.isVisible())) {
    // Try alternative selector
    const altDeleteButton = page.locator('button:has-text("Delete This Repository")');
    if (!(await altDeleteButton.isVisible())) {
      return false;
    }
    await altDeleteButton.click();
  } else {
    await deleteButton.click();
  }

  // Wait for modal and confirm deletion
  await page.locator('#delete-repo-modal').waitFor({state: 'visible', timeout: 5000});

  // Type the repo name to confirm
  await page.locator('#delete-repo-modal input[name=repo_name]').fill(repoName);

  // Click the final delete button
  await page.click('#delete-repo-modal button:has-text("Delete Repository")');

  // Wait for redirect to dashboard or user page
  await page.waitForURL(/\/\?|\/[^/]+$/, {timeout: 10000});

  return true;
}

/**
 * Check if a repository is a fork by examining the page content.
 */
export async function is_repo_fork(page: Page, owner: string, repoName: string): Promise<boolean> {
  await page.goto(`/${owner}/${repoName}`);

  // Check for fork indicator - repos that are forks show "forked from" text
  const forkIndicator = page.locator('a:has-text("forked from")');
  return forkIndicator.isVisible();
}

/**
 * Get repository info including fork status.
 */
export async function get_repo_info(page: Page, owner: string, repoName: string): Promise<{
  isFork: boolean;
  isEmpty: boolean;
  forkParent?: string;
}> {
  await page.goto(`/${owner}/${repoName}`);

  const isFork = await page.locator('a:has-text("forked from")').isVisible();
  const isEmpty = await page.locator('text=This repository is empty').isVisible();

  let forkParent: string | undefined;
  if (isFork) {
    const forkLink = page.locator('a:has-text("forked from")').first();
    const href = await forkLink.getAttribute('href');
    if (href) {
      forkParent = href.replace(/^\//, '');
    }
  }

  return {isFork, isEmpty, forkParent};
}
