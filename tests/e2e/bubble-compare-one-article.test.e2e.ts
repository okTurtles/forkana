// #451: the Bubble view of a subject with a single article (figma "Bubble view /
// Comparing disabled", 7368:260887). Compare has nothing to compare there: its tooltip
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
  // figma's Tooltip (7368:260912): #25292e, 8px around Inter 12/18 in white, "No forks
  // yet." in 600
  const tooltip = page.locator('.tippy-box[data-theme="forkana"]', {hasText: 'No forks yet'});
  await expect(tooltip).toBeVisible();
  // tippy flips it above when there is no room below: there is plenty here
  await expect(tooltip).toHaveAttribute('data-placement', 'bottom-end');
  await expect(tooltip).toHaveCSS('background-color', 'rgb(37, 41, 46)');
  await expect(tooltip).toHaveCSS('border-radius', '6px');
  const content = tooltip.locator('.tippy-content');
  await expect(content).toHaveText('No forks yet. Compare needs at least 2 articles.');
  await expect(content).toHaveCSS('padding', '8px');
  await expect(content).toHaveCSS('font-size', '12px');
  await expect(content).toHaveCSS('line-height', '18px');
  await expect(content).toHaveCSS('font-weight', '400');
  await expect(content).toHaveCSS('color', 'rgb(255, 255, 255)');
  await expect(content.locator('strong')).toHaveText('No forks yet.');
  await expect(content.locator('strong')).toHaveCSS('font-weight', '600');

  // below the button, flush with its right edge: the arrow tip 10px under it, the box
  // 18px, the arrow's centre 21.25px in from the box's right edge
  const buttonBox = await button.boundingBox();
  const tooltipBox = await tooltip.boundingBox();
  const arrowBox = await tooltip.locator('.tippy-svg-arrow svg').boundingBox();
  const buttonBottom = buttonBox.y + buttonBox.height;
  // within 1px: tippy rounds the tooltip to whole pixels, the button is not
  const near = (value: number, want: number) => expect(Math.abs(value - want)).toBeLessThanOrEqual(1);
  near(tooltipBox.y - buttonBottom, 18);
  near(arrowBox.y - buttonBottom, 10);
  near(tooltipBox.x + tooltipBox.width, buttonBox.x + buttonBox.width);
  near(tooltipBox.x + tooltipBox.width - (arrowBox.x + arrowBox.width / 2), 21.25);
});

test('signed out, "Fork this article" in the notice leads to sign in, then the fork flow (#451 item 2)', async ({page}) => {
  const button = await openBubbleView(page);
  // aria-disabled, which Playwright refuses to click: a reader can still press it
  await button.dispatchEvent('click');
  const banner = page.locator('#compare-announcement-root .compare-announcement');
  await expect(banner).toContainText('No forks yet. Compare needs at least 2 articles. Fork this article to start comparing.');
  const link = banner.getByRole('link', {name: 'Fork this article'});
  // figma's Message (I7368:260916;38494:155070): Inter 400 14/20 in #1f2328, its
  // "Fork this article" run 600, underlined, #4e40fa
  const message = banner.locator('.compare-announcement-message');
  await expect(message).toHaveCSS('font-weight', '400');
  await expect(message).toHaveCSS('font-size', '14px');
  await expect(message).toHaveCSS('line-height', '20px');
  await expect(message).toHaveCSS('color', 'rgb(31, 35, 40)');
  await expect(link).toHaveCSS('font-weight', '600');
  await expect(link).toHaveCSS('color', 'rgb(78, 64, 250)');
  await expect(link).toHaveCSS('text-decoration-line', 'underline');
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
    await login_user(browser, workerInfo, 'user12');
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

  test('the article\'s owner, who cannot fork it, is told so instead of offered a Fork link (#451 item 2)', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user12');
    const page = await context.newPage();
    const button = await openBubbleView(page);
    await button.dispatchEvent('click');
    const banner = page.locator('#compare-announcement-root .compare-announcement');
    await expect(banner).toContainText('No forks yet. Compare needs at least 2 articles. Comparing starts once someone forks this article.');
    // the message's own style, as every banner message (figma's Message, Inter 400 14/20 #1f2328)
    const message = banner.locator('.compare-announcement-message');
    await expect(message).toHaveCSS('font-weight', '400');
    await expect(message).toHaveCSS('color', 'rgb(31, 35, 40)');
    await expect(banner.getByRole('link')).toHaveCount(0);
    await context.close();
  });
});
