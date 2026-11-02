import {test, expect, request} from '@playwright/test';
import type {Locator} from '@playwright/test';
import {env} from 'node:process';
import {create_article_via_api, delete_repo, disableGeneratedHooks, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// An image stored in the article's repository and shown by a relative path, as markdown or as
// HTML, must display in the Read view (signed in or not) and in the Visual editor (#167). The
// Read view resolved it against the article url, which serves no files, and the Visual editor
// showed an HTML `<img>` against the page url.

// a 2×2 PNG
const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAEElEQVR4nGM4IScHRAwQCgAfJgQRoo8irwAAAABJRU5ErkJggg==';

const README = [
  '# Images',
  '',
  'Markdown ![md-image](pic.png) image.',
  '',
  'HTML <img src="./img/pic.png" alt="html-image"> image.',
  '',
].join('\n');

async function expectLoaded(img: Locator) {
  await img.scrollIntoViewIfNeeded();
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth), {timeout: 10000}).toBe(2);
}

test.describe('Article images', () => {
  test.setTimeout(90000);

  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('relative images display in the Read view and the Visual editor (#167)', async ({browser}, workerInfo) => {
    const subject = `e2e-images-${workerInfo.workerIndex}-${Date.now()}`;
    const repoName = subject;
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    const api = await request.newContext({
      baseURL: `${(env.GITEA_URL || 'http://localhost:3000').replace(/\/+$/, '')}/`,
      extraHTTPHeaders: {Authorization: `Basic ${Buffer.from('user2:password').toString('base64')}`},
    });

    try {
      await create_article_via_api('user2', subject, repoName);
      await disableGeneratedHooks('user2', repoName);
      const contents = `api/v1/repos/user2/${repoName}/contents`;
      for (const path of ['pic.png', 'img/pic.png']) {
        const added = await api.post(`${contents}/${path}`, {data: {content: PNG, message: `add ${path}`}});
        expect(added.status(), await added.text()).toBe(201);
      }
      const readme = await (await api.get(`${contents}/README.md`)).json();
      const updated = await api.put(`${contents}/README.md`, {data: {sha: readme.sha, message: 'images', content: Buffer.from(README).toString('base64')}});
      expect(updated.status(), await updated.text()).toBe(200);

      const article = `/subject/${normalizeSubjectTitle(subject)}/user2`;
      const anonymous = await browser.newContext();
      try {
        for (const reader of [page, await anonymous.newPage()]) {
          await reader.goto(article);
          const content = reader.locator('#article-view-root .markup');
          await expectLoaded(content.locator('img[alt="md-image"]'));
          await expectLoaded(content.locator('img[alt="html-image"]'));
        }
      } finally {
        await anonymous.close();
      }

      await page.goto(`${article}?mode=edit`);
      const visual = page.locator('.toastui-editor-ww-container .ProseMirror');
      await expectLoaded(visual.locator('img[alt="md-image"]'));
      await expectLoaded(visual.locator('img[alt="html-image"]'));
      // from the "/media/" route, like the Read view, which also serves images stored in LFS
      for (const alt of ['md-image', 'html-image']) {
        await expect(visual.locator(`img[alt="${alt}"]`)).toHaveAttribute('src', /\/user2\/[^/]+\/media\/branch\//);
      }
      // the markdown keeps the paths as written
      expect((await page.locator('#edit_area').inputValue()).trimEnd()).toBe(README.trimEnd());
    } finally {
      await api.dispose();
      try {
        await delete_repo(page, workerInfo, 'user2', repoName);
      } catch {
        // Best-effort cleanup only.
      }
      await context.close();
    }
  });
});
