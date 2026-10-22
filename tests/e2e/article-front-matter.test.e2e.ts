import {test, expect} from '@playwright/test';
import type {Locator, Page, Request} from '@playwright/test';
import {login_user, load_logged_in_context} from './utils_e2e.ts';

// An article whose file opens with YAML front matter (the Wikipedia imports do) keeps it
// byte for byte when it is edited, in either editor (#433: the Visual editor used to save it
// as `***` and the keys run together, which rendered as a rule and a big heading).
//
// user2 owns repo1, the article of "example-subject". Its edit page gets front matter put
// into its editor's textarea, and the commit request is answered here, so the fixture
// repository is neither needed nor modified.

const FRONT_MATTER = [
  '---',
  'title: "Prodromus"',
  'source: https://en.wikipedia.org/wiki/Prodromus',
  'license: CC BY-SA 4.0',
  'attribution: Wikipedia contributors',
  'fetched_at: 2025-11-11T13:56:22Z',
  '---',
].join('\n');
const ARTICLE = `${FRONT_MATTER}\n\nA prodromus is a preliminary publication.\n`;
const editURL = '/subject/example-subject/user2?mode=edit';

// the "content" field of the commit request, urlencoded or multipart
function postedContent(request: Request): string {
  const body = request.postData() ?? '';
  const boundary = /boundary=(\S+)/.exec(request.headers()['content-type'] ?? '')?.[1];
  if (!boundary) return new URLSearchParams(body).get('content') ?? '';
  // (up to the part's boundary: the front matter's own `---` lines must not end it)
  for (const part of body.split(`--${boundary}`)) {
    const at = part.indexOf('name="content"\r\n\r\n');
    if (at >= 0) return part.slice(at + 'name="content"\r\n\r\n'.length).replace(/\r\n$/, '');
  }
  return '';
}

// The article file is given front matter in the page itself, before the editor reads its
// textarea: the init script's DOMContentLoaded listener is registered before the page's own.
async function openEditorWithFrontMatter(page: Page) {
  await page.addInitScript((article: string) => {
    document.addEventListener('DOMContentLoaded', () => {
      const textarea = document.querySelector<HTMLTextAreaElement>('#edit_area');
      if (textarea) textarea.value = article;
    });
  }, ARTICLE);
  await page.goto(editURL, {waitUntil: 'domcontentloaded'});
  await expect(page.locator('#submit-changes-button')).toBeVisible({timeout: 10000});
  await expect(page.locator('.toastui-editor-ww-container .ProseMirror p', {hasText: 'preliminary publication'})).toBeVisible({timeout: 20000});
}

// Puts the caret at the end of the given line by clicking its bottom-right corner, which
// works the same with a mouse and a touch screen, wherever the line wraps.
async function clickAtEnd(line: Locator) {
  await line.scrollIntoViewIfNeeded();
  const box = await line.boundingBox();
  if (!box) throw new Error('the line is not displayed');
  await line.click({position: {x: box.width - 1, y: box.height - 2}});
}

async function submitAndCapture(page: Page): Promise<string> {
  let posted = '';
  await page.route('**/_edit/**', async (route) => {
    posted = postedContent(route.request());
    await route.fulfill({status: 200, contentType: 'application/json', body: '{}'});
  });
  const submit = page.locator('#submit-changes-button');
  await submit.scrollIntoViewIfNeeded();
  // eslint-disable-next-line playwright/no-force-option
  await submit.click({force: true});
  await expect.poll(() => posted, {timeout: 10000}).not.toBe('');
  return posted;
}

test.describe('Article front matter in the editor', () => {
  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('an edit in the Visual editor keeps the front matter', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openEditorWithFrontMatter(page);

    await clickAtEnd(page.locator('.toastui-editor-ww-container .ProseMirror p', {hasText: 'preliminary publication'}));
    await page.keyboard.type(' Edited.');

    const posted = (await submitAndCapture(page)).replaceAll('\r\n', '\n');
    expect(posted.startsWith(`${FRONT_MATTER}\n`)).toBe(true);
    expect(posted).toContain('A prodromus is a preliminary publication. Edited.');
    expect(posted).not.toContain('***');
    await context.close();
  });

  test('an edit in the Source editor keeps the front matter', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openEditorWithFrontMatter(page);

    await page.locator('.toastui-editor-mode-switch .tab-item', {hasText: 'Source editor'}).click();
    await clickAtEnd(page.locator('.toastui-editor-md-container .ProseMirror > div', {hasText: 'preliminary publication'}));
    await page.keyboard.type(' Edited.');

    const posted = (await submitAndCapture(page)).replaceAll('\r\n', '\n');
    expect(posted.startsWith(`${FRONT_MATTER}\n`)).toBe(true);
    expect(posted).toContain('A prodromus is a preliminary publication. Edited.');
    await context.close();
  });
});
