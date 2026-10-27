import {test, expect} from '@playwright/test';
import type {Locator, Page} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// The Article view's Edit tab against figma "Article view / Logged out - Edit"
// (7368:264887): the Submit changes button at the end of the tabs' row, the editor's
// header, and the space around the Summary. user2 owns repo1, the article of
// "example-subject". The viewport is pinned, so the values are the same in every
// Playwright project.
const editURL = '/subject/example-subject/user2?mode=edit';
const viewport = {width: 1440, height: 900};

type Box = {x: number, y: number, width: number, height: number};

async function box(locator: Locator): Promise<Box> {
  const b = await locator.boundingBox();
  if (!b) throw new Error('not displayed');
  return b;
}

async function style(locator: Locator, ...props: string[]): Promise<Record<string, string>> {
  return locator.evaluate((el, names) => {
    const cs = getComputedStyle(el);
    return Object.fromEntries(names.map((n) => [n, cs.getPropertyValue(n)]));
  }, props);
}

// The colour as 0-255 sRGB channels, whatever notation the browser computes it in
// (rgb(), or color(srgb …) for a color-mix()).
async function rgb(locator: Locator, prop: string): Promise<number[]> {
  return locator.evaluate((el, name) => {
    const c = document.createElement('canvas').getContext('2d');
    c.fillStyle = getComputedStyle(el).getPropertyValue(name);
    c.fillRect(0, 0, 1, 1);
    return Array.from(c.getImageData(0, 0, 1, 1).data.slice(0, 3));
  }, prop);
}

function expectColor(actual: number[], hex: string) {
  const want = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  for (const [i, v] of want.entries()) expect(Math.abs(actual[i] - v), `${actual} vs ${hex}`).toBeLessThanOrEqual(2);
}

async function openEdit(page: Page) {
  await page.setViewportSize(viewport);
  await page.goto(editURL);
  await expect(page.locator('.article-editor-box .toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});
}

// Button 7368:264915: 32px, 6px corners, 6/12 padding (11px + the 1px border inside), an
// Inter 600 14/20 label "Submit changes" and no icon.
async function expectSubmitBox(button: Locator) {
  await expect(button).toHaveText('Submit changes');
  await expect(button.locator('svg')).toHaveCount(0);
  const b = await box(button);
  expect(b.height).toBe(32);
  expect(b.width).toBeGreaterThan(128);
  expect(b.width).toBeLessThan(140);
  expect(await style(button, 'border-radius', 'padding-left', 'padding-right', 'font-weight', 'font-size', 'line-height', 'border-top-width')).toEqual({
    'border-radius': '6px', 'padding-left': '11px', 'padding-right': '11px', 'font-weight': '600', 'font-size': '14px', 'line-height': '20px', 'border-top-width': '1px',
  });
}

test.describe('Article Edit tab against figma 7368:264887', () => {
  test('signed out: Submit changes unavailable, 9px after Fork, at the end of the row', async ({page}) => {
    await openEdit(page);
    const submit = page.locator('#signed-out-submit-button');
    await expectSubmitBox(submit);
    // #dad5ff with a white 80% label
    expectColor(await rgb(submit, 'background-color'), '#dad5ff');
    expect((await style(submit, 'color')).color).toBe('rgba(255, 255, 255, 0.8)');

    const tabs = await box(page.locator('#article-tabs'));
    const fork = await box(page.locator('#signed-out-fork-button'));
    const s = await box(submit);
    expect(Math.round(s.x - (fork.x + fork.width))).toBe(9);
    expect(Math.round(s.x + s.width)).toBe(Math.round(tabs.x + tabs.width));
    // vertically centred in the 40px row (y 4 in Frame 9 7368:264912)
    expect(Math.round(s.y + s.height / 2)).toBe(Math.round(tabs.y + 20));
  });

  test('signed out: Fork, its count, the tabs, and no Watch in the Edit tab', async ({page}) => {
    await openEdit(page);
    // Button 7368:264913: 84×32, #f6f8fa, a #818b98 10% border, Inter 600 14/20 #818b98, no icon
    const fork = page.locator('#signed-out-fork-button');
    await expect(fork.locator('svg')).toHaveCount(0);
    const f = await box(fork);
    expect(f.height).toBe(32);
    expect(Math.abs(f.width - 84)).toBeLessThanOrEqual(2);
    expectColor(await rgb(fork, 'background-color'), '#f6f8fa');
    expectColor(await rgb(fork, 'color'), '#818b98');
    expect((await style(fork, 'border-top-color'))['border-top-color']).toBe('rgba(129, 139, 152, 0.1)');
    expect(await style(fork, 'font-weight', 'font-size', 'line-height', 'border-radius')).toEqual({
      'font-weight': '600', 'font-size': '14px', 'line-height': '20px', 'border-radius': '6px',
    });
    // CounterLabel I7368:264913;30258:5608: an 18px pill, 0 6px, #818b98 at 12%, 8px after the label
    const count = fork.locator('[data-role="article-fork-count"]');
    await expect(count).toHaveText(/^\d+$/);
    const c = await box(count);
    expect(c.height).toBe(18);
    expect(await style(count, 'padding-left', 'border-top-left-radius')).toEqual({'padding-left': '6px', 'border-top-left-radius': '9999px'});
    expect((await style(count, 'background-color'))['background-color']).toMatch(/0\.12\)$/);

    // TabNav 7368:264911: Read / Edit / History, 0 16px, 4px apart; its Trailing (Watch)
    // is hidden in this frame, so the Edit tab has no Watch; Read keeps it
    const tabs = page.locator('#article-tabs .ui.tabular.menu > a.item');
    await expect(tabs).toHaveText(['Read', 'Edit', 'History']);
    expect(await style(tabs.first(), 'padding-left', 'padding-right')).toEqual({'padding-left': '16px', 'padding-right': '16px'});
    expect(Math.round((await box(tabs.first())).height)).toBe(40);
    const t0 = await box(tabs.nth(0));
    const t1 = await box(tabs.nth(1));
    expect(Math.round(t1.x - (t0.x + t0.width))).toBe(4);
    await expect(page.locator('#article-tabs .article-watch')).toBeHidden();
    await page.locator('#article-tabs a[data-article-tab="read"]').click();
    await expect(page.locator('#article-view-root[data-article-mode="read"]')).toBeVisible({timeout: 10000});
    await expect(page.locator('#article-tabs .article-watch')).toBeVisible();
  });

  test('editor toolbar: the figma\'s button set on the tabs\' row, and the Insert menu', async ({page}) => {
    await openEdit(page);
    // _Markdown toolbar 7368:265005: H B I | quote code link | lists | (mention,
    // cross-reference) and a 40px button with a dropdown: here Cite, with the
    // cross-reference icon, and the menu of the code block, image and table
    const toolbar = page.locator('.article-editor-box .toastui-editor-defaultUI-toolbar');
    const names = await toolbar.locator('button.toastui-editor-toolbar-icons:visible').evaluateAll((els) => els.map((el) => Array.from(el.classList).find((c) => !['toastui-editor-toolbar-icons', 'forkana-toolbar-icon'].includes(c))));
    expect(names).toEqual(['heading', 'bold', 'italic', 'quote', 'code', 'forkana-subject-link', 'bullet-list', 'ordered-list', 'task-list', 'forkana-cite', 'forkana-more']);
    await expect(toolbar.locator('.forkana-cite svg.octicon-cross-reference')).toHaveCount(1);
    await expect(toolbar.locator('.forkana-more svg.octicon-reply')).toHaveCount(1);
    await expect(toolbar.locator('.forkana-more svg.octicon-triangle-down')).toHaveCount(1);
    const groups = toolbar.locator('.toastui-editor-toolbar-group:visible');
    const widths = [];
    for (let i = 0; i < await groups.count(); i++) widths.push(Math.round((await box(groups.nth(i))).width));
    expect(widths).toEqual([80, 80, 80, 68]);
    const more = await box(toolbar.locator('.forkana-more'));
    expect([Math.round(more.width), Math.round(more.height)]).toEqual([40, 24]);

    // one row: the Visual / Source tabs and every button in the 47px band
    const band = await box(page.locator('.article-editor-box .toastui-editor-defaultUI .toastui-editor-toolbar'));
    expect(band.height).toBe(48);
    const tab = await box(page.locator('.article-editor-box .toastui-editor-mode-switch .tab-item.active'));
    // (the open tab stands 1px over the rule under the band)
    for (const b of [tab, more]) expect(b.y + b.height).toBeLessThanOrEqual(band.y + band.height + 1);
    expect(more.x).toBeGreaterThan(tab.x + tab.width);

    // the menu
    await page.locator('.toastui-editor-ww-container .ProseMirror p').first().click();
    await toolbar.locator('.forkana-more').click();
    const items = page.locator('.forkana-more-popup .forkana-more-item');
    await expect(items).toHaveText(['Code block', 'Image', 'Table']);
    await items.filter({hasText: 'Table'}).click();
    await expect(page.locator('.forkana-more-popup')).toBeHidden();
    await expect(page.locator('.toastui-editor-ww-container table')).toHaveCount(1);
  });

  test('editor header: tabs and toolbar', async ({page}) => {
    await openEdit(page);
    const frame = await box(page.locator('.article-editor-box'));
    const band = page.locator('.article-editor-box .toastui-editor-defaultUI .toastui-editor-toolbar');
    // Toolbar 7368:264999: a 47px #f6f8fa band over a 1px #d1d9e0 rule
    const b = await box(band);
    expect(b.height).toBe(48);
    expectColor(await rgb(band, 'background-color'), '#f6f8fa');
    expectColor(await rgb(band, 'border-bottom-color'), '#d1d9e0');
    expect((await style(band, 'border-bottom-width'))['border-bottom-width']).toBe('1px');

    // Tabnav 7368:265001: tabs 8px from the band's top and left, 4px apart, 40px, 0 16px,
    // Inter 400 14/20; the open one white, #1f2328, 1px #d1d9e0 sides and top, 6px corners
    const active = page.locator('.article-editor-box .toastui-editor-mode-switch .tab-item.active');
    const other = page.locator('.article-editor-box .toastui-editor-mode-switch .tab-item:not(.active)');
    await expect(active).toHaveText('Visual editor');
    await expect(other).toHaveText('Source editor');
    const a = await box(active);
    const o = await box(other);
    expect(Math.round(a.x - b.x)).toBe(8);
    expect(Math.round(a.y - b.y)).toBe(8);
    expect(Math.round(o.x - (a.x + a.width))).toBe(4);
    expect(Math.round(o.height)).toBe(40);
    expect(await style(active, 'padding-left', 'padding-right', 'font-weight', 'font-size', 'line-height', 'border-top-left-radius', 'border-top-width', 'border-bottom-width')).toEqual({
      'padding-left': '16px', 'padding-right': '16px', 'font-weight': '400', 'font-size': '14px', 'line-height': '20px', 'border-top-left-radius': '6px', 'border-top-width': '1px', 'border-bottom-width': '0px',
    });
    expectColor(await rgb(active, 'background-color'), '#ffffff');
    expectColor(await rgb(active, 'color'), '#1f2328');
    expectColor(await rgb(active, 'border-top-color'), '#d1d9e0');
    expectColor(await rgb(other, 'color'), '#59636e');

    // Frame 386 7368:265002: 24px buttons 13px from the band's top, the last one ending
    // 8px from its right edge; 4px apart in a group, 16px between groups; #59636e icons
    const icons = page.locator('.article-editor-box .toastui-editor-defaultUI-toolbar button.toastui-editor-toolbar-icons:visible');
    const first = await box(icons.first());
    const last = await box(icons.last());
    expect(first.height).toBe(24);
    expect(Math.round(first.y - b.y)).toBe(13);
    expect(Math.round(b.x + b.width - (last.x + last.width))).toBe(8);
    // (one that is not active: the one for the block the caret is in is brand purple)
    expectColor(await rgb(page.locator('.article-editor-box .toastui-editor-defaultUI-toolbar button.toastui-editor-toolbar-icons:not(.active):visible').first(), 'color'), '#59636e');
    const groups = page.locator('.article-editor-box .toastui-editor-defaultUI-toolbar .toastui-editor-toolbar-group');
    const g0 = await box(groups.nth(0));
    const g1 = await box(groups.nth(1));
    expect(Math.round(g1.x - (g0.x + g0.width))).toBe(16);

    // the writing area 8px inside the frame, under the band
    const writing = await box(page.locator('.article-editor-box .toastui-editor-ww-container'));
    expect(Math.round(writing.x - frame.x)).toBe(9); // 8px + the frame's 1px border
    expect(Math.round(writing.y - (b.y + b.height))).toBe(8);
  });

  test('Summary: 24px from the path above and from the editor below', async ({page}) => {
    await openEdit(page);
    const path = await box(page.locator('.article-path'));
    const summary = await box(page.locator('[data-role="article-summary"]'));
    const editor = await box(page.locator('.article-editor-box'));
    // Frame 952 7368:264918 (gap 24) and Frame 307 7368:264941 (y 625, after 601)
    expect(Math.round(summary.y - (path.y + path.height))).toBe(24);
    expect(Math.round(editor.y - (summary.y + summary.height))).toBe(24);
    // Textarea 7368:264938: label, 4px, a 64px box with 8/16 padding inside its 1px border, 4px, caption
    const field = page.locator('.article-summary-box');
    expect((await box(field)).height).toBe(64);
    expect(await style(field, 'padding-top', 'padding-left', 'border-top-width', 'border-top-left-radius')).toEqual({
      'padding-top': '7px', 'padding-left': '15px', 'border-top-width': '1px', 'border-top-left-radius': '6px',
    });
  });
});

test.describe('Article Edit tab signed in (figma 7368:264887)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('the owner\'s Submit changes is the primary button at the end of the row', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openEdit(page);
    const submit = page.locator('#submit-changes-button');
    await expectSubmitBox(submit);
    // the primary variant: #4e40fa, white label
    expectColor(await rgb(submit, 'background-color'), '#4e40fa');
    expectColor(await rgb(submit, 'color'), '#ffffff');
    await expect(submit).toBeEnabled();
    const tabs = await box(page.locator('#article-tabs'));
    const s = await box(submit);
    expect(Math.round(s.x + s.width)).toBe(Math.round(tabs.x + tabs.width));
    await context.close();
  });
});
