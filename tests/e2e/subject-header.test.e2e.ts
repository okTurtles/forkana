// #469: the subject header is the same on every view (Bubble, Table, Article) and every
// article tab (Read, Edit, History), and an info bar never moves it on a desktop. figma
// "Notes – Width system" (7368:275653): the title block is 136px (24px, the 56px title,
// the 32px Follow, 24px), then the 48px view tabs row; below it, the Table view and the
// Article view's tabs are a 640px column.
import {test, expect} from '@playwright/test';
import {create_article_via_api, login_user, load_logged_in_context} from './utils_e2e.ts';
import type {Page} from '@playwright/test';

const run = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const subject = `header-${run}`;
const owner = 'user12';
const subjectUrl = `/subject/${encodeURIComponent(subject)}`;
const articleUrl = `${subjectUrl}/${owner}`;
const views: Record<string, string> = {
  bubble: `${subjectUrl}?view=bubble`,
  table: `${subjectUrl}?view=table`,
  read: `${articleUrl}?view=article&mode=read`,
  edit: `${articleUrl}?view=article&mode=edit`,
  history: `${articleUrl}?view=article&mode=history`,
};
// what shows that a view is on screen
const shown: Record<string, string> = {
  bubble: '.history-view-section--bubble',
  table: '.history-view-section--table',
  read: '#article-view-root[data-article-mode="read"]',
  edit: '#article-view-root[data-article-mode="edit"]',
  history: '#article-view-root[data-article-mode="history"]',
};

type Box = {x: number, y: number, width: number, height: number};

// in page coordinates, so that a scroll (the editor takes the focus) does not count
function boxOf(page: Page, selector: string): Promise<Box> {
  return page.locator(selector).first().evaluate((el) => {
    const r = el.getBoundingClientRect();
    return {x: r.x + window.scrollX, y: r.y + window.scrollY, width: r.width, height: r.height};
  });
}

async function headerOf(page: Page) {
  await page.evaluate(() => document.fonts.ready);
  return {
    title: await boxOf(page, '.subject-header h1'),
    follow: await boxOf(page, '.repo-header-follow'),
    tabs: await boxOf(page, '#subject-view-tabs'),
    compare: await boxOf(page, '#compare-mode-button'),
  };
}

async function open(page: Page, view: string) {
  await page.goto(views[view]);
  await expect(page.locator(shown[view])).toBeVisible({timeout: 10000});
  await expect(page.locator('.repo-header-follow')).toBeVisible();
}

// The Follow label reads "Follow" or "Following", for the article each view stands for,
// and Compare's "Compare on" while compare mode is on: Follow's centre and Compare's
// right edge are what must not move, with the rows they are on.
function expectSameHeader(actual: Awaited<ReturnType<typeof headerOf>>, expected: Awaited<ReturnType<typeof headerOf>>, label: string) {
  expect(actual.title, `${label}: title`).toEqual(expected.title);
  expect(actual.tabs, `${label}: view tabs`).toEqual(expected.tabs);
  expect(actual.compare.y, `${label}: Compare`).toBe(expected.compare.y);
  expect(actual.compare.height, `${label}: Compare`).toBe(expected.compare.height);
  expect(actual.compare.x + actual.compare.width, `${label}: Compare`).toBeCloseTo(expected.compare.x + expected.compare.width, 0);
  expect(actual.follow.y, `${label}: Follow`).toBe(expected.follow.y);
  expect(actual.follow.height, `${label}: Follow`).toBe(expected.follow.height);
  expect(actual.follow.x + actual.follow.width / 2, `${label}: Follow`).toBeCloseTo(expected.follow.x + expected.follow.width / 2, 0);
}

test.beforeAll(async () => {
  // two articles, so that the Bubble and Table views have something to compare
  for (const user of [owner, 'user13']) await create_article_via_api(user, subject, `${subject}-${user}`, `An article by ${user}`);
});

// Per width: the views whose header is compared with the Bubble view's (a phone keeps the
// Edit tab's sign-in banner in the flow, above the header: tested on its own below), the
// article tabs switched in turn, and the width of the content column.
const layouts = [
  {width: 1440, compared: ['table', 'read', 'history', 'edit'], tabs: ['history', 'edit', 'read'], column: 640},
  {width: 375, compared: ['table', 'read', 'history'], tabs: ['history', 'read'], column: 343},
];

// the url of each tab (the Read tab is the default one: its url names no mode)
const tabUrl: Record<string, RegExp> = {read: /view=article(?!.*mode=)/, edit: /mode=edit/, history: /mode=history/};

for (const {width, compared, tabs, column} of layouts) {
  test.describe(`at ${width}px`, () => {
    test.beforeEach(async ({page}) => {
      await page.setViewportSize({width, height: 900});
    });

    test('signed out: the header does not move between views and tabs', async ({page}) => {
      await open(page, 'bubble');
      const reference = await headerOf(page);
      for (const view of compared) {
        await open(page, view);
        expectSameHeader(await headerOf(page), reference, view);
        const content = await boxOf(page, '.history-view-section:not([hidden]) :is(.history-table, #article-view-root)');
        expect(content.width, `${view}: content column`).toBe(column);
      }
    });

    test('the Read / Edit / History row stays still when tabs are switched', async ({page}) => {
      await open(page, 'read');
      const row = await boxOf(page, '#article-tabs');
      const header = await headerOf(page);
      for (const tab of tabs) {
        await page.locator(`#article-tabs a[data-article-tab="${tab}"]`).click();
        await expect(page.locator(shown[tab])).toBeVisible({timeout: 10000});
        await expect(page).toHaveURL(tabUrl[tab]);
        expect(await boxOf(page, '#article-tabs'), tab).toEqual(row);
        expectSameHeader(await headerOf(page), header, tab);
      }
      // the tabs were switched without a reload, and Back still returns to the one before
      await page.goBack();
      await expect(page.locator(shown[tabs[tabs.length - 2]])).toBeVisible({timeout: 10000});
    });
  });
}

test('desktop: the title block is 136px and the view tabs row 48px (7368:260781, 7368:260789)', async ({page}) => {
  await page.setViewportSize({width: 1440, height: 900});
  await open(page, 'bubble');
  const {title, follow, tabs} = await headerOf(page);
  const navbar = await boxOf(page, '#navbar');
  const row = await boxOf(page, '.subject-header overflow-menu');
  expect(title.y).toBe(navbar.y + navbar.height + 24);
  expect(title.height).toBe(56);
  expect(follow.y).toBe(title.y + 56);
  expect(follow.height).toBe(32);
  expect(tabs.y).toBe(follow.y + 32 + 24);
  expect(row.height).toBe(48);
  // 1344px wide: 48px margins
  expect(row.x).toBe(48);
  expect(row.width).toBe(1344);
});

test('desktop: an info bar is laid over the header instead of moving it', async ({page}) => {
  await page.setViewportSize({width: 1440, height: 900});

  // the Bubble view's Compare banner
  await open(page, 'bubble');
  const reference = await headerOf(page);
  const navbar = await boxOf(page, '#navbar');
  await page.locator('#compare-mode-button').click();
  await expect(page.locator('#subject-info-bars .compare-announcement')).toBeVisible();
  expect((await boxOf(page, '#subject-info-bars .compare-announcement')).y).toBe(navbar.y + navbar.height);
  expectSameHeader(await headerOf(page), reference, 'bubble, compare banner');

  // the Table view's
  await open(page, 'table');
  await page.locator('#compare-mode-button').click();
  await expect(page.locator('#table-compare-announcement-root .compare-announcement')).toBeVisible();
  expectSameHeader(await headerOf(page), reference, 'table, compare banner');

  // the Edit tab's sign-in banner, under the navbar
  await open(page, 'edit');
  await expect(page.locator('#article-signin-banner')).toBeVisible();
  expect((await boxOf(page, '#article-signin-banner')).y).toBe(navbar.y + navbar.height);
  expectSameHeader(await headerOf(page), reference, 'edit, sign-in banner');
});

test('phone: an info bar stays in the flow, above the header', async ({page}) => {
  await page.setViewportSize({width: 375, height: 900});
  await open(page, 'read');
  const reference = await headerOf(page);
  await open(page, 'edit');
  const banner = await boxOf(page, '#article-signin-banner');
  expect(banner.height).toBeGreaterThan(0);
  expect((await headerOf(page)).title.y).toBe(reference.title.y + banner.height);
});

test.describe('signed in', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, owner);
  });

  for (const width of [1440, 375]) {
    test(`at ${width}px the header does not move between views and tabs`, async ({browser}, workerInfo) => {
      const context = await load_logged_in_context(browser, workerInfo, owner);
      const page = await context.newPage();
      await page.setViewportSize({width, height: 900});
      await open(page, 'bubble');
      const reference = await headerOf(page);
      for (const view of ['table', 'read', 'edit', 'history']) {
        await open(page, view);
        expectSameHeader(await headerOf(page), reference, view);
      }
      // a signed-in reader has no sign-in banner
      await expect(page.locator('#article-signin-banner')).toHaveCount(0);
      await context.close();
    });
  }
});
