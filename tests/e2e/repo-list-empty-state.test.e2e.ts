import {test, expect} from '@playwright/test';

// The article list on profiles and org homes never offers to create a subject (#369): an empty
// search there only says that nothing matched, even for a subject that already exists elsewhere.

test('profile search with no match does not offer to create a subject', async ({page}) => {
  await page.goto('/user4?tab=repositories&q=example-subject');
  await expect(page.locator('.flex-list')).toContainText('No articles match “example-subject”.');
  await expect(page.locator('a[href*="/repo/create?subject="]')).toHaveCount(0);
});

test('org home search with no match does not offer to create a subject', async ({page}) => {
  await page.goto('/org3?q=example-subject');
  await expect(page.locator('.flex-list')).toContainText('No articles match “example-subject”.');
  await expect(page.locator('a[href*="/repo/create?subject="]')).toHaveCount(0);
});
