// #469: the subject header is the same on every view (Bubble, Table, Article) and every
// article tab (Read, Edit, History); an info bar sits in the flow above it. figma
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
  // the views' script is running (it records the view in the history), so that tabs are
  // switched in the page rather than by following their links
  await page.waitForFunction(() => window.history.state !== null);
}

// waits for the info bars' slides (and the views' fade) to end
async function settle(page: Page) {
  await expect.poll(() => page.evaluate(() => document.getAnimations().filter((a) => a.playState === 'running').length)).toBe(0);
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

function intersects(a: Box, b: Box): boolean {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

// a subject whose title is wider than the screen, for the compare page's Back below
const longSubject = `header-long-${run} the title of a subject long enough to wrap over two or three lines`;

test.beforeAll(async () => {
  // two articles, so that the Bubble and Table views have something to compare
  for (const user of [owner, 'user13']) await create_article_via_api(user, subject, `${subject}-${user}`, `An article by ${user}`);
  for (const user of [owner, 'user13']) await create_article_via_api(user, longSubject, `header-long-${run}-${user}`, `An article by ${user}`);
});

// The compare page (Point of contention) has the same header, with its Back laid over the
// title block (figma Button 7368:261776, 16px into the block at the header's left margin):
// the header is where the subject page has it, and Back never covers the title.
for (const width of [1440, 375]) {
  test(`compare page at ${width}px: the header is the subject page's, and Back stays clear of the title`, async ({page}) => {
    await page.setViewportSize({width, height: 900});
    await open(page, 'bubble');
    const reference = await headerOf(page);
    await page.goto(`${subjectUrl}/compare/${owner}...user13`);
    await expect(page.locator('.subject-compare-back')).toBeVisible();
    const header = await headerOf(page);
    expect(header.title.y).toBeCloseTo(reference.title.y, 0);
    expect(header.tabs).toEqual(reference.tabs);

    for (const name of [subject, longSubject]) {
      await page.goto(`/subject/${encodeURIComponent(name)}/compare/${owner}...user13`);
      await expect(page.locator('.subject-compare-back')).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      const back = await boxOf(page, '.subject-compare-back');
      const title = await boxOf(page, '.subject-header h1');
      expect(intersects(back, title), `${name}: Back ${JSON.stringify(back)}, title ${JSON.stringify(title)}`).toBe(false);
    }
  });
}

// Per width, signed out: the views whose header is compared with the Bubble view's, the
// article tabs switched in turn, and the width of the content column. A signed-out
// reader's Edit tab has the sign-in banner, which pushes the header down (tested below);
// the signed-in tests cover the Edit tab without it.
const layouts = [
  {width: 1440, column: 640},
  {width: 375, column: 343},
];
const compared = ['table', 'read', 'history'];
const tabs = ['history', 'read'];

// the url of each tab (the Read tab is the default one: its url names no mode)
const tabUrl: Record<string, RegExp> = {read: /view=article(?!.*mode=)/, edit: /mode=edit/, history: /mode=history/};

for (const {width, column} of layouts) {
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

    test('the Read / History row stays still when tabs are switched', async ({page}) => {
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
      await expect(page.locator(shown.history)).toBeVisible({timeout: 10000});
    });

    // Info bars stay in the flow on every width (Pierre's decision on #469): a bar sits
    // right under the navbar, above the header, and pushes the header down by its height.
    test('an info bar sits in the flow above the header and pushes it down by its height', async ({page}) => {
      await open(page, 'read');
      const reference = await headerOf(page);
      const navbar = await boxOf(page, '#navbar');

      // the Edit tab's sign-in banner
      await open(page, 'edit');
      await expect(page.locator('#article-signin-banner')).toBeVisible();
      await settle(page);
      const banner = await boxOf(page, '#article-signin-banner');
      expect(banner.y).toBeCloseTo(navbar.y + navbar.height, 0);
      const pushed = await headerOf(page);
      expect(pushed.title.y).toBeCloseTo(reference.title.y + banner.height, 0);
      expect(pushed.follow.y).toBeCloseTo(reference.follow.y + banner.height, 0);
      expect(pushed.tabs.y).toBeCloseTo(reference.tabs.y + banner.height, 0);

      // the Bubble view's Compare banner
      await open(page, 'bubble');
      await page.locator('#compare-mode-button').click();
      await expect(page.locator('.compare-announcement')).toBeVisible();
      await settle(page);
      const compareBanner = await boxOf(page, '.compare-announcement');
      expect(compareBanner.y).toBeCloseTo(navbar.y + navbar.height, 0);
      expect((await headerOf(page)).tabs.y).toBeCloseTo(reference.tabs.y + compareBanner.height, 0);
    });
  });
}

test('desktop: the title block is 136px and the view tabs row 48px (7368:260781, 7368:260789)', async ({page}) => {
  await page.setViewportSize({width: 1440, height: 900});
  await open(page, 'bubble');
  const {title, follow, tabs} = await headerOf(page);
  const navbar = await boxOf(page, '#navbar');
  const row = await boxOf(page, '.subject-header overflow-menu');
  expect(title.y).toBeCloseTo(navbar.y + navbar.height + 24, 0);
  expect(title.height).toBe(56);
  expect(follow.y).toBeCloseTo(title.y + 56, 0);
  expect(follow.height).toBe(32);
  expect(tabs.y).toBeCloseTo(follow.y + 32 + 24, 0);
  expect(row.height).toBe(48);
  // 1344px wide: 48px margins
  expect(row.x).toBe(48);
  expect(row.width).toBe(1344);
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

// Switches the article tab in the page and waits for it (and, for Edit, its editor).
async function switchTab(page: Page, tab: string) {
  await page.locator(`#article-tabs a[data-article-tab="${tab}"]`).click();
  await expect(page.locator(shown[tab])).toBeVisible({timeout: 10000});
  await expect(page.locator(`${shown[tab]} .toastui-editor-defaultUI`)).toHaveCount(tab === 'edit' ? 1 : 0, {timeout: 10000});
  // an info bar slides for 200ms, and the editor used to focus itself (and scroll) in a
  // timer right after it was created
  await settle(page);
  await page.evaluate(() => new Promise((resolve) => setTimeout(resolve, 50)));
}

// #469: opening the Edit tab used to scroll the page down to the editor (Toast UI focused
// it, through the lossless tracker's initial setMarkdown). The page stays where it is now.
test.describe('switching tabs does not scroll the page', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, owner);
  });

  test('signed in: window.scrollY is unchanged', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, owner);
    const page = await context.newPage();
    await page.setViewportSize({width: 1440, height: 400});
    await open(page, 'read');
    await page.evaluate(() => window.scrollTo(0, 120));
    const scrollY = await page.evaluate(() => window.scrollY);
    expect(scrollY).toBeGreaterThan(0);
    for (const tab of ['edit', 'read', 'history', 'edit']) {
      await switchTab(page, tab);
      expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollY), tab).toBeLessThanOrEqual(1);
    }
    // the editor was not focused on its own
    expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.toastui-editor-defaultUI')))).toBe(false);
    await context.close();
  });

  // Signed out, the Edit tab's sign-in banner slides open above the header, above the part
  // of the page on screen: the browser keeps what is on screen in place (scroll
  // anchoring), so the tab row does not move in the window.
  test('signed out: what is on screen stays in place', async ({page, browserName}) => {
    // WebKit does not always keep what is on screen in place when the bar above it closes
    // while the article below is swapped (its scroll anchor goes with the old article)
    // eslint-disable-next-line playwright/no-skipped-test
    test.skip(browserName === 'webkit', 'WebKit scroll anchoring');
    await page.setViewportSize({width: 1440, height: 400});
    await open(page, 'read');
    await page.evaluate(() => window.scrollTo(0, 120));
    const rowY = () => page.evaluate(() => document.querySelector('#article-tabs').getBoundingClientRect().y);
    const before = await rowY();
    for (const tab of ['edit', 'read', 'history']) {
      await switchTab(page, tab);
      expect(Math.abs(await rowY() - before), tab).toBeLessThanOrEqual(1);
    }
  });
});

type BarSample = {height: number, gap: number};

// Clicks `action` in the page, then samples every frame the height of a bar and how far
// the title is below it: from the click until `settleMs` after the bar first changed (an
// article tab is fetched first), or 8s.
function sampleBar(page: Page, bar: string, action: string, settleMs = 400): Promise<BarSample[]> {
  return page.evaluate(async ({bar, action, settleMs}) => {
    const measure = () => {
      const el = document.querySelector(bar);
      const rect = el?.getBoundingClientRect();
      const height = rect && el.getClientRects().length ? rect.height : 0;
      const title = document.querySelector('.subject-header h1').getBoundingClientRect().y;
      return {height, gap: title - (height ? rect.bottom : 0)};
    };
    const samples = [measure()];
    document.querySelector<HTMLElement>(action).click();
    let end = performance.now() + 8000;
    let changed = false;
    while (performance.now() < end) {
      await new Promise((resolve) => requestAnimationFrame(resolve));
      const sample = measure();
      samples.push(sample);
      if (!changed && Math.abs(sample.height - samples[0].height) > 0.5) {
        changed = true;
        end = performance.now() + settleMs;
      }
    }
    return samples;
  }, {bar, action, settleMs});
}

// heights going from 0 to full (opening) or from full to 0 (closing), through frames in
// between and never back the other way
function expectSlide(samples: BarSample[], opening: boolean, label: string) {
  const heights = samples.map((s) => s.height);
  const full = Math.max(...heights);
  const trace = `${label}: ${heights.map(Math.round).join(',')}`;
  expect(full, trace).toBeGreaterThan(30);
  expect(heights.some((h) => h > 1 && h < full - 1), trace).toBe(true);
  const direction = opening ? 1 : -1;
  expect(heights.every((h, i) => i === 0 || (h - heights[i - 1]) * direction >= -0.5), trace).toBe(true);
  expect(heights.at(-1), trace).toBeCloseTo(opening ? full : 0, 0);
}

test.describe('info bars slide open and closed', () => {
  // (frames are sampled: a busy machine can drop the few frames of a 200ms slide)
  test.describe.configure({retries: 1});

  test.beforeEach(async ({page}) => {
    await page.setViewportSize({width: 1440, height: 900});
  });

  test('the Bubble view Compare banner, with the header riding on it', async ({page}) => {
    await open(page, 'bubble');
    const opening = await sampleBar(page, '.info-bar-slide', '#compare-mode-button');
    expectSlide(opening, true, 'opening');
    const closing = await sampleBar(page, '.info-bar-slide', '#compare-mode-button');
    expectSlide(closing, false, 'closing');
    // the header moves with the bar frame by frame: never a jump between the two
    const gaps = [...opening, ...closing].filter((s) => s.height > 0).map((s) => Math.round(s.gap));
    expect(new Set(gaps).size, gaps.join(',')).toBe(1);
  });

  test('the Edit tab sign-in banner (signed out)', async ({page}) => {
    await open(page, 'read');
    expectSlide(await sampleBar(page, '#article-signin-banner', '#article-tabs a[data-article-tab="edit"]'), true, 'opening');
    expectSlide(await sampleBar(page, '#article-signin-banner', '#article-tabs a[data-article-tab="read"]'), false, 'closing');
    await expect(page.locator('#article-signin-banner')).toBeHidden();
  });

  test('with reduced motion they appear and go at once', async ({page}) => {
    await page.emulateMedia({reducedMotion: 'reduce'});
    await open(page, 'bubble');
    // (the first sample is taken before the click)
    const opening = (await sampleBar(page, '.info-bar-slide', '#compare-mode-button', 150)).slice(1);
    expect(opening[0].height).toBeGreaterThan(30);
    expect(opening.every((s) => s.height === opening[0].height)).toBe(true);
    const closing = (await sampleBar(page, '.info-bar-slide', '#compare-mode-button', 150)).slice(1);
    expect(closing.every((s) => s.height === 0)).toBe(true);
  });

  test('a bar shown on the first load does not slide in', async ({page}) => {
    await page.goto(views.edit);
    const heights = await page.evaluate(async () => {
      const out: number[] = [];
      for (let i = 0; i < 5; i++) {
        out.push(document.querySelector('#article-signin-banner').getBoundingClientRect().height);
        await new Promise((resolve) => requestAnimationFrame(resolve));
      }
      return out;
    });
    expect(heights.every((h) => h === heights[0] && h > 30), heights.join(',')).toBe(true);
  });
});
