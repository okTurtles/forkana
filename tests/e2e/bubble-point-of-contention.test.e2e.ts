// #431 / #432: the Bubble view's point of contention, the compare page it opens, and the
// way back. user2 owns repo1 ("example-subject"); user5 forks it here (through the API)
// so the subject has two articles and one point of contention between them.
import {test, expect} from '@playwright/test';
import type {Page} from '@playwright/test';

const subjectPath = '/subject/example-subject';

test.beforeAll(async ({request}) => {
  const res = await request.post('/api/v1/repos/user2/repo1/forks', {
    headers: {Authorization: `Basic ${Buffer.from('user5:password').toString('base64')}`},
    data: {},
  });
  // 403/409: user5 has forked it already (an earlier project of the run); openGraph()
  // then checks that the subject does have a point of contention
  expect([202, 403, 409]).toContain(res.status());
});

async function openGraph(page: Page) {
  await page.goto(subjectPath);
  const joint = page.locator('circle.joint-parent[role=button]').first();
  await expect(joint).toBeVisible();
  return joint;
}

test('a point of contention is selected on its own, and Back returns to it (#431 items 1, 3, 5)', async ({page}) => {
  const joint = await openGraph(page);
  await joint.click();
  await expect(page).toHaveURL(/\/subject\/example-subject\/compare\//);

  // item 2: figma 641:62395, Inter 600 12/20 in #25292e, not the brand colour
  const back = page.locator('a[data-subject-back]');
  await expect(back).toHaveCSS('color', 'rgb(37, 41, 46)');
  await expect(back).toHaveCSS('font-size', '12px');
  await expect(back).toHaveCSS('font-weight', '600');

  // item 1: the page's Back goes back in the history, to the very entry that was left
  const historyLength = await page.evaluate(() => window.history.length);
  await back.click();
  await expect(page).toHaveURL(new RegExp(`${subjectPath}(\\?|$)`));
  expect(await page.evaluate(() => window.history.length)).toBe(historyLength);

  // items 3 and 5: the point of contention is selected (21×21, opaque white, 1px brand ring),
  // and the article it leads to is not
  const selected = page.locator('circle.joint-parent.is-selected');
  await expect(selected).toHaveCount(1);
  await expect(selected).toHaveAttribute('r', '10');
  await expect(selected).toHaveCSS('fill', 'rgb(255, 255, 255)');
  await expect(selected).toHaveCSS('stroke-width', '1px');
  await expect(page.locator('g.node[aria-pressed="true"]')).toHaveCount(0);
});

test('selecting an article clears the point of contention, and browser Back closes it (#431 items 4, 5)', async ({page}) => {
  const joint = await openGraph(page);
  await joint.click();
  await expect(page).toHaveURL(/\/compare\//);
  await page.goBack();
  await expect(page.locator('circle.joint-parent.is-selected')).toHaveCount(1);

  await page.locator('g.node[role=button]').first().click();
  await expect(page.locator('.detail-layer')).toHaveCount(1);
  // the browser's Back closes the opened article and stays on the graph
  await page.goBack();
  await expect(page.locator('.detail-layer')).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`${subjectPath}\\?`));
  await expect(page.locator('g.node[aria-pressed="true"]')).toHaveCount(1);
  await expect(page.locator('circle.joint-parent.is-selected')).toHaveCount(0);
});

test('compare mode clears the selected point of contention (#432)', async ({page}) => {
  const joint = await openGraph(page);
  await joint.click();
  await expect(page).toHaveURL(/\/compare\//);
  await page.goBack();
  await expect(page.locator('circle.joint-parent.is-selected')).toHaveCount(1);

  await page.locator('#compare-mode-button').click();
  await expect(page.locator('circle.joint-parent.is-selected')).toHaveCount(0);
});
