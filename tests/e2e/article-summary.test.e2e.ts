import {test, expect} from '@playwright/test';
import type {Locator, Page} from '@playwright/test';
import {login_user, load_logged_in_context, create_article_via_api, normalizeSubjectTitle} from './utils_e2e.ts';

// #475: the article's Summary is edited on its Edit tab (figma Textarea 7368:264938) and is
// required. #248: a subject row shows its original article's summary and "Updated …" (figma
// "Explore_ Articles" 7368:273049 and 7368:273128). user2 owns repo1, the article of
// "example-subject", which has no summary in the fixtures.
const editURL = '/subject/example-subject/user2?mode=edit';
const viewport = {width: 1440, height: 900};

async function style(locator: Locator, ...props: string[]): Promise<Record<string, string>> {
  return locator.evaluate((el, names) => {
    const cs = getComputedStyle(el);
    return Object.fromEntries(names.map((n) => [n, cs.getPropertyValue(n)]));
  }, props);
}

// The colour as 0-255 sRGB channels, whatever notation the browser computes it in
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

async function box(locator: Locator) {
  const b = await locator.boundingBox();
  if (!b) throw new Error('not displayed');
  return b;
}

// Answers the editor's POST here, so the fixture article is not modified; returns the bodies.
async function captureEdits(page: Page): Promise<string[]> {
  const bodies: string[] = [];
  await page.route('**/_edit/**', async (route) => {
    bodies.push(route.request().postData() ?? '');
    await route.fulfill({status: 200, contentType: 'application/json', body: '{}'});
  });
  return bodies;
}

test.describe('Article summary on the Edit tab (#475)', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('an editable, required Summary as in the figma', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.setViewportSize(viewport);
    await page.goto(editURL);
    await expect(page.locator('.article-editor-box .toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});

    const label = page.locator('.article-summary-field .article-summary-label');
    await expect(label).toHaveText(/^Summary\s+\(required\)$/);
    const field = page.locator('#article-summary-input');
    await expect(field).toBeEditable();
    await expect(field).toHaveAttribute('maxlength', '200');
    await expect(page.locator('.article-summary-caption')).toHaveText('Up to 200 characters.');

    // Textarea 7368:264938: a 64px box, 8/16 padding inside its 1px #d1d9e0 border, 6px
    // corners, Inter 400 16/24 #59636e; the label Inter 600 14/20 #1f2328
    expect((await box(field)).height).toBe(64);
    expect(await style(field, 'padding-top', 'padding-left', 'border-top-width', 'border-top-left-radius', 'font-size', 'line-height', 'font-weight')).toEqual({
      'padding-top': '7px', 'padding-left': '15px', 'border-top-width': '1px', 'border-top-left-radius': '6px', 'font-size': '16px', 'line-height': '24px', 'font-weight': '400',
    });
    expectColor(await rgb(field, 'color'), '#59636e');
    expectColor(await rgb(field, 'border-top-color'), '#d1d9e0');
    expect(await style(label, 'font-size', 'line-height', 'font-weight')).toEqual({'font-size': '14px', 'line-height': '20px', 'font-weight': '600'});
    expectColor(await rgb(label, 'color'), '#1f2328');
    await context.close();
  });

  test('Submit changes asks for a summary first, then sends it with the article', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await page.goto(editURL);
    await expect(page.locator('.article-editor-box .toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});
    const bodies = await captureEdits(page);
    const field = page.locator('#article-summary-input');
    const submit = page.locator('#submit-changes-button');

    // the fixture article has no summary: nothing is sent, and the field is the one to fix
    await field.fill('   ');
    // eslint-disable-next-line playwright/no-force-option
    await submit.click({force: true});
    await expect(field).toHaveAttribute('aria-invalid', 'true');
    await expect(field).toBeFocused();
    expect(bodies).toHaveLength(0);

    await field.fill('The summary from the Edit tab');
    await expect(field).not.toHaveAttribute('aria-invalid', 'true');
    // eslint-disable-next-line playwright/no-force-option
    await submit.click({force: true});
    await expect.poll(() => bodies.length).toBe(1);
    expect(bodies[0]).toContain('The summary from the Edit tab');
    expect(bodies[0]).toMatch(/name="summary"|summary=/);
    await context.close();
  });
});

test.describe('Subject rows in the search results (#248)', () => {
  test('a row shows the name, the original article\'s summary and "Updated …"', async ({page}, workerInfo) => {
    const typed = `e2e-summary-${workerInfo.workerIndex}-${Date.now()}`;
    const subject = normalizeSubjectTitle(typed);
    await create_article_via_api('user2', subject, typed.toLowerCase(), 'Earth’s only natural satellite.');
    await page.setViewportSize(viewport);
    await page.goto(`/explore/subjects?q=${encodeURIComponent(subject)}`);

    const row = page.locator('[data-role="subject-row"]').first();
    const name = row.locator('.flex-item-title a');
    await expect(name).toHaveText(subject);
    const summary = row.locator('[data-role="subject-summary"]');
    await expect(summary).toHaveText('Earth’s only natural satellite.');
    const updated = row.locator('[data-role="subject-updated"]');
    await expect(updated).toHaveText(/^Updated\s/);

    // Frame 592 7368:273077: the name Inter 600 14/20 #4e40fa, 2px, the summary Inter 400
    // 12/20 #1f2328, 2px, "Updated …" Inter 400 12/18 #59636e
    expect(await style(name, 'font-size', 'line-height', 'font-weight')).toEqual({'font-size': '14px', 'line-height': '20px', 'font-weight': '600'});
    expectColor(await rgb(name, 'color'), '#4e40fa');
    expect(await style(summary, 'font-size', 'line-height', 'font-weight')).toEqual({'font-size': '12px', 'line-height': '20px', 'font-weight': '400'});
    expectColor(await rgb(summary, 'color'), '#1f2328');
    expect(await style(updated, 'font-size', 'line-height')).toEqual({'font-size': '12px', 'line-height': '18px'});
    expectColor(await rgb(updated, 'color'), '#59636e');
    const [n, s, u] = [await box(name), await box(summary), await box(updated)];
    expect(Math.round(s.y - (n.y + n.height))).toBe(2);
    expect(Math.round(u.y - (s.y + s.height))).toBe(2);

    // "Search results for …" (7368:273125): Inter 600 16/24 #1f2328, 16px above the row
    const heading = page.locator('[data-role="subject-search-heading"]');
    expect(await style(heading, 'font-size', 'line-height', 'font-weight')).toEqual({'font-size': '16px', 'line-height': '24px', 'font-weight': '600'});
    expect(Math.round((await box(row)).y - ((await box(heading)).y + (await box(heading)).height))).toBe(16);
  });

  test('"Create a new subject" is the small secondary button (#248 item 3)', async ({page}) => {
    await page.setViewportSize(viewport);
    await page.goto(`/explore/subjects?q=${encodeURIComponent(`No such subject ${Date.now()}`)}`);
    const button = page.locator('a[data-role="create-new-subject"]');
    await expect(button).toHaveText('Create a new subject');
    // Button 7368:273149: 26px, #f6f8fa with a 1px #d1d9e0 border, Inter 600 12/18 #25292e
    expect((await box(button)).height).toBe(26);
    expectColor(await rgb(button, 'background-color'), '#f6f8fa');
    expectColor(await rgb(button, 'border-top-color'), '#d1d9e0');
    expect(await style(button, 'font-size', 'font-weight')).toEqual({'font-size': '12px', 'font-weight': '600'});
  });
});
