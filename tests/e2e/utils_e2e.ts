import {expect} from '@playwright/test';
import {chmod, readFile, writeFile} from 'node:fs/promises';
import {isAbsolute, join, resolve} from 'node:path';
import {cwd, env} from 'node:process';
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
 * Read the repository name out of the current repo-scoped URL. The server derives it from
 * the subject, so it is not necessarily equal to the subject that was requested.
 */
function getRepoNameFromCurrentURL(page: Page, owner: string): string {
  const segments = new URL(page.url()).pathname.split('/').filter(Boolean);
  const ownerIndex = segments.indexOf(owner);
  if (ownerIndex < 0 || !segments[ownerIndex + 1]) {
    throw new Error(`Could not determine repository name from ${page.url()}`);
  }
  return decodeURIComponent(segments[ownerIndex + 1]);
}

/**
 * Create an article for a subject via the "create first article" flow, which redirects to
 * the README editor of the newly created repository. Returns the repository name.
 */
export async function create_first_article(page: Page, owner: string, subjectName: string): Promise<string> {
  const response = await page.goto(`/repo/create-first-article?subject=${encodeURIComponent(subjectName)}`);
  expect(response?.status()).toBe(200);

  await page.waitForURL(new RegExp(`/${owner}/.*/_new/.*/README\\.md`), {timeout: 20000});

  return getRepoNameFromCurrentURL(page, owner);
}

/**
 * Resolve the configured repository storage root. Relative ROOT values are resolved against
 * the current working directory, which only matches the server's work path when the tests are
 * started the way the Makefile does (`make test-e2e-sqlite`, from the repository root).
 */
async function getRepositoryRoot(): Promise<string> {
  const configPath = resolve(cwd(), env.GITEA_CONF ?? 'tests/sqlite.ini');
  const config = await readFile(configPath, 'utf8');
  let inRepositorySection = false;

  for (const line of config.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed.startsWith('[')) {
      inRepositorySection = trimmed === '[repository]';
      continue;
    }
    if (!inRepositorySection) continue;

    const match = /^ROOT\s*=\s*(.+)$/.exec(trimmed);
    if (match) {
      const repoRoot = match[1].trim();
      return isAbsolute(repoRoot) ? repoRoot : resolve(cwd(), repoRoot);
    }
  }

  throw new Error(`Repository root not found in ${configPath}`);
}

/**
 * Neutralize the generated git hooks of a repository created by an E2E test.
 *
 * A web-editor commit is pushed through git-receive-pack, which runs the repository's
 * generated hooks; each hook spawns a separate `gitea hook` process that calls back into the
 * server's internal API. Under the in-process E2E server that callback is unreliable, so the
 * push fails for reasons unrelated to what the test is asserting. Call this after creating a
 * repository and before committing through the web editor.
 */
export async function disableGeneratedHooks(owner: string, repoName: string): Promise<void> {
  const repositoryRoot = await getRepositoryRoot();
  const repoPath = join(repositoryRoot, owner, `${repoName}.git`);
  const hookScript = '#!/usr/bin/env bash\n# Disabled for this E2E-created repository.\nexit 0\n';
  const hookPaths = [
    join(repoPath, 'hooks/pre-receive.d/gitea'),
    join(repoPath, 'hooks/update.d/gitea'),
    join(repoPath, 'hooks/post-receive.d/gitea'),
    join(repoPath, 'hooks/proc-receive'),
  ];

  await Promise.all(hookPaths.map(async (hookPath) => {
    await writeFile(hookPath, hookScript);
    await chmod(hookPath, 0o755);
  }));
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
