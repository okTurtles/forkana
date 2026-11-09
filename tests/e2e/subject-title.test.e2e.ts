import {test, expect} from '@playwright/test';
import {create_article_via_api, load_logged_in_context, login_user} from './utils_e2e.ts';

// #401: subject titles follow the subject title rule (Wikipedia's technical restrictions).

// A title made only of symbols, unique to this run: the digits of the worker index and the time
// spelled with symbols the rule allows, since a digit would give the title an ASCII slug.
function symbolTitle(prefix: string, workerIndex: number): string {
  const symbols = `'!*()@$^=-`;
  return prefix + Array.from(`${workerIndex}${Date.now()}`, (d) => symbols[Number(d)]).join('');
}

// The subject a /subject/{title} URL names
function subjectOfURL(url: URL): string {
  const [, segment] = /^\/subject\/([^/]+)/.exec(url.pathname) ?? [];
  return segment ? decodeURIComponent(segment) : '';
}

test.describe('Subject title rule', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('the new subject form refuses a title that breaks the rule', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    try {
      await page.goto('/repo/create');
      const input = page.locator('input#subject');
      const hint = page.locator('[data-subject-title-hint]');

      await input.fill('Song #3');
      await expect(hint).toBeVisible();
      await expect(hint).toContainText('# < > [ ] | { }');
      expect(await input.evaluate((el: HTMLInputElement) => el.validity.valid)).toBe(false);

      // the browser does not submit the form
      await page.locator('#create-repo-submit-button').click();
      await expect(page).toHaveURL(/\/repo\/create$/);
      await expect(input).toHaveJSProperty('validationMessage', await hint.textContent());

      // apostrophes and other punctuation are allowed
      await input.fill(`'''`);
      await expect(hint).toBeHidden();
      expect(await input.evaluate((el: HTMLInputElement) => el.validity.valid)).toBe(true);
    } finally {
      await context.close();
    }
  });

  test(`"'''" is a subject of its own, not a longer run of apostrophes`, async ({browser}, workerInfo) => {
    const short = symbolTitle(`'''`, workerInfo.workerIndex);
    const long = `''''''''''''${short}`;
    // someone else created the longer title first
    await create_article_via_api('user5', long, `long-symbols-${workerInfo.workerIndex}-${Date.now()}`);

    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    try {
      // the search offers to create the short title, it does not say it already exists
      await page.goto(`/explore/subjects?q=${encodeURIComponent(short)}`);
      const create = page.locator('a.ui.primary.button[href^="/repo/create?subject="]');
      await expect(create).toBeVisible();
      await expect(page.locator('a.ui.basic.button[href^="/subject/"]')).toHaveCount(0);
      await create.click();

      await expect(page.locator('input#subject')).toHaveValue(short);
      await expect(page.locator('[data-subject-title-hint]')).toBeHidden();
      await page.locator('#create-repo-submit-button').click();

      // the new subject has the title as typed
      await page.waitForURL((url) => subjectOfURL(url) === short, {timeout: 30000});

      // and the search now finds it, under its own title
      await page.goto(`/explore/subjects?q=${encodeURIComponent(short)}`);
      await expect(create).toHaveCount(0);
      const subjectLinks = await page.locator('a[href^="/subject/"]').evaluateAll(
        (links) => links.map((link) => (link as HTMLAnchorElement).href),
      );
      const subjects = subjectLinks.map((href) => subjectOfURL(new URL(href)));
      expect(subjects).toContain(short);
      // the longer title is still listed as a similar subject
      expect(subjects).toContain(long);
    } finally {
      await context.close();
    }
  });
});
