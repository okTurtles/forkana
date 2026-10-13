import {test, expect} from '@playwright/test';
import type {Page} from '@playwright/test';

// Each explore tab keeps its own search (#420): switching tabs shows that tab's own last search,
// never searches with the text of the other tab, and coming back restores what was left there.

const searchField = (page: Page) => page.locator('.page-content.explore input[type="search"][name="q"]');
const tabLink = (page: Page, tab: string) => page.locator(`a[data-explore-tab-link][href*="/explore/${tab}"]`);

async function switchTab(page: Page, tab: string) {
  await Promise.all([page.waitForURL(new RegExp(`/explore/${tab}`)), tabLink(page, tab).click()]);
}

async function search(page: Page, keyword: string) {
  await searchField(page).fill(keyword);
  await Promise.all([page.waitForURL(new RegExp(`[?&]q=${keyword}`)), searchField(page).press('Enter')]);
}

test('each explore tab keeps its own search', async ({page}) => {
  await page.goto('/explore/subjects');
  await search(page, 'mars');

  // the other tab opens unsearched
  await switchTab(page, 'users');
  expect(new URL(page.url()).searchParams.has('q')).toBe(false);
  await expect(searchField(page)).toHaveValue('');
  await search(page, 'user2');

  // and each tab comes back with its own search
  await switchTab(page, 'subjects');
  expect(new URL(page.url()).searchParams.get('q')).toBe('mars');
  await expect(searchField(page)).toHaveValue('mars');
  await switchTab(page, 'users');
  expect(new URL(page.url()).searchParams.get('q')).toBe('user2');
  await expect(searchField(page)).toHaveValue('user2');
});

test('switching tabs does not search the text typed in the field', async ({page}) => {
  await page.goto('/explore/subjects?q=mars');
  await searchField(page).fill('forest');

  await switchTab(page, 'users');
  expect(new URL(page.url()).searchParams.has('q')).toBe(false);
  await expect(searchField(page)).toHaveValue('');

  // back on the subjects tab: still the "mars" results, with the unsearched text as it was left
  await switchTab(page, 'subjects');
  expect(new URL(page.url()).searchParams.get('q')).toBe('mars');
  await expect(searchField(page)).toHaveValue('forest');
});
