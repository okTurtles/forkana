import {test, expect} from '@playwright/test';
import type {Locator, Page, Request} from '@playwright/test';
import {create_first_article, delete_repo, disableGeneratedHooks, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// An article with raw HTML that Toast UI cannot convert to its Visual editor (a declaration,
// a processing instruction, a comment it does not recognize, an indented HTML block) used to
// make the Edit page throw "Cannot read properties of null (reading '1')" and never show the
// editor (#446). It must open, show that HTML as written, and save it unchanged.

const ARTICLE = [
  '# Mark down',
  '',
  'First paragraph.',
  '',
  '<!DOCTYPE html>',
  '',
  '<!-- a -- b -->',
  '',
  '  <div class="footer">indented</div>',
  '',
  'Inline <?php echo 1; ?> instruction.',
  '',
  'Last paragraph.',
  '',
].join('\n');

const visual = (page: Page) => page.locator('.toastui-editor-ww-container .ProseMirror');

// the "content" field of the commit request, urlencoded or multipart
function postedContent(request: Request): string {
  const body = request.postData() ?? '';
  const boundary = /boundary=(\S+)/.exec(request.headers()['content-type'] ?? '')?.[1];
  if (!boundary) return new URLSearchParams(body).get('content') ?? '';
  for (const part of body.split(`--${boundary}`)) {
    const at = part.indexOf('name="content"\r\n\r\n');
    if (at >= 0) return part.slice(at + 'name="content"\r\n\r\n'.length).replace(/\r\n$/, '');
  }
  return '';
}

// Puts the caret at the end of the given line, the same with a mouse and a touch screen.
async function clickAtEnd(line: Locator) {
  await line.scrollIntoViewIfNeeded();
  const box = await line.boundingBox();
  if (!box) throw new Error('the line is not displayed');
  await line.click({position: {x: box.width - 1, y: box.height - 2}});
}

test.describe('Article with raw HTML in the editor', () => {
  test.setTimeout(90000);

  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('opens in the Visual editor and keeps the HTML (#446)', async ({browser}, workerInfo) => {
    const subject = `e2e-raw-html-${workerInfo.workerIndex}-${Date.now()}`;
    let repoName = subject;
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    try {
      // Create the article with that content, written in the Source editor (which never
      // converts it to the Visual editor's model).
      repoName = await create_first_article(page, 'user2', subject);
      await disableGeneratedHooks('user2', repoName);
      const container = page.locator('#toast-editor-container');
      await expect(container.locator('.toastui-editor-defaultUI')).toBeVisible({timeout: 20000});
      await container.locator('.toastui-editor-mode-switch .tab-item', {hasText: 'Source editor'}).click();
      await container.evaluate((el, text) => {
        (el as HTMLElement & {_giteaToastEditor: {setMarkdown: (md: string) => void}})._giteaToastEditor.setMarkdown(text);
      }, ARTICLE);
      await expect(page.locator('#commit-button')).toBeEnabled({timeout: 10000});
      await page.locator('#commit-button').click();
      await page.waitForURL(`**/subject/${normalizeSubjectTitle(subject)}/user2**`, {timeout: 30000});
      pageErrors.length = 0;

      // Edit it again: the Visual editor opens with the content.
      await page.goto(`/subject/${normalizeSubjectTitle(subject)}/user2?mode=edit`, {waitUntil: 'domcontentloaded'});
      await expect(page.locator('#submit-changes-button')).toBeVisible({timeout: 20000});
      await expect(visual(page).locator('p', {hasText: 'Last paragraph.'})).toBeVisible({timeout: 20000});
      await expect(visual(page).locator('h1', {hasText: 'Mark down'})).toBeVisible();
      const rawBlocks = visual(page).locator('[data-html-comment]');
      await expect(rawBlocks.filter({hasText: '<!DOCTYPE html>'})).toBeVisible();
      await expect(rawBlocks.filter({hasText: '<!-- a -- b -->'})).toBeVisible();
      await expect(rawBlocks.filter({hasText: 'div class="footer">indented'})).toBeVisible();
      await expect(visual(page).locator('p', {hasText: 'Inline <?php echo 1; ?> instruction.'})).toBeVisible();
      expect(pageErrors).toEqual([]);

      // An edit in the Visual editor saves the HTML unchanged. The commit request is
      // answered here, so the article is not modified.
      await clickAtEnd(visual(page).locator('p', {hasText: 'Last paragraph.'}));
      await page.keyboard.type(' Edited.');
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
      // (the Source editor that wrote the article may have dropped its final newline)
      expect(posted.replaceAll('\r\n', '\n').trimEnd()).toBe(ARTICLE.replace('Last paragraph.', 'Last paragraph. Edited.').trimEnd());
      expect(pageErrors).toEqual([]);
    } finally {
      try {
        await delete_repo(page, workerInfo, 'user2', repoName);
      } catch {
        // Best-effort cleanup only.
      }
      await context.close();
    }
  });
});
