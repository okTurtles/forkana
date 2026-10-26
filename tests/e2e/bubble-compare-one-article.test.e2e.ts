// #451: the Bubble view of a subject with a single article (figma "Bubble view /
// Comparing disabled", 6661:52942). Compare has nothing to compare there: its tooltip
// says why, below the button, and pressing it shows the "No forks yet" notice in the
// header, whose "Fork this article" leads to the fork flow (signing in first).
import {test, expect} from '@playwright/test';
import type {Page} from '@playwright/test';
import {create_article_via_api, load_logged_in_context, login_user} from './utils_e2e.ts';

// A subject of its own, with one article by user12. Every project (and retry) runs the
// setup again, with a subject of its own.
const run = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const subject = `one-article-${run}`;
const subjectPath = `/subject/${encodeURIComponent(subject)}`;

test.beforeAll(async () => {
  await create_article_via_api('user12', subject, `${subject}-user12`, 'The only article');
});

async function openBubbleView(page: Page) {
  await page.goto(`${subjectPath}?view=bubble`);
  const button = page.locator('#compare-mode-button');
  // the graph has loaded and reported that there is nothing to compare
  await expect(button).toHaveClass(/is-unavailable/);
  return button;
}

test('the Compare tooltip is shown below the button (#451 item 1)', async ({page}) => {
  const button = await openBubbleView(page);
  await button.hover();
  const tooltip = page.locator('.tippy-box[data-theme="tooltip"]', {hasText: 'No forks yet'});
  await expect(tooltip).toBeVisible();
  // tippy flips it above when there is no room below: there is plenty here
  await expect(tooltip).toHaveAttribute('data-placement', /^bottom/);
  const buttonBox = await button.boundingBox();
  const tooltipBox = await tooltip.boundingBox();
  expect(tooltipBox.y).toBeGreaterThanOrEqual(buttonBox.y + buttonBox.height);
});

test('signed out, "Fork this article" in the notice leads to sign in, then the fork flow (#451 item 2)', async ({page}) => {
  const button = await openBubbleView(page);
  // aria-disabled, which Playwright refuses to click: a reader can still press it
  await button.dispatchEvent('click');
  const banner = page.locator('#compare-announcement-root .compare-announcement');
  await expect(banner).toContainText('No forks yet. Compare needs at least 2 articles. Fork this article to start comparing.');
  const link = banner.getByRole('link', {name: 'Fork this article'});
  await expect(link).toBeVisible();
  const href = await link.getAttribute('href');
  const target = new URL(href, page.url());
  expect(target.pathname).toBe('/user/login');
  // back to the article's Edit tab, which has the Fork button
  expect(target.searchParams.get('redirect_to')).toMatch(/^\/subject\/[^?]+\/user12\?view=article&mode=edit$/);
  await link.click();
  await expect(page).toHaveURL(/\/user\/login\?redirect_to=/);
  await expect(page.locator('input[name=user_name]')).toBeVisible();
});

test.describe('signed in', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('"Fork this article" in the notice leads to the article\'s Fork button (#451 item 2)', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    const button = await openBubbleView(page);
    // aria-disabled, which Playwright refuses to click: a reader can still press it
    await button.dispatchEvent('click');
    const link = page.locator('#compare-announcement-root .compare-announcement').getByRole('link', {name: 'Fork this article'});
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute('href', /\/subject\/[^?]+\/user12\?view=article&mode=edit$/);
    await link.click();
    await expect(page.locator('#fork-article-button')).toBeVisible();
    await context.close();
  });
});
