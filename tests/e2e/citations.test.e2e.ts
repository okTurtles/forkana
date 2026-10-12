import {expect, test, type Page} from '@playwright/test';
import {create_first_article, delete_repo, disableGeneratedHooks, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// Internal links and external references (issue #399).
// Fixtures: user2 owns repo1, the only article of the subject "example-subject".

type ToastEditorContainer = HTMLElement & {_giteaToastEditor?: {getMarkdown: () => string, setMarkdown: (m: string) => void, changeMode: (m: string) => void}};

async function openArticleEditor(page: Page): Promise<void> {
  await page.goto('/article/user2/example-subject?mode=edit');
  await expect(page.locator('.toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});
}

function markdownOf(page: Page): Promise<string> {
  return page.locator('#toast-editor-container').evaluate((el) => (el as ToastEditorContainer)._giteaToastEditor.getMarkdown());
}

async function setMarkdown(page: Page, markdown: string): Promise<void> {
  await page.locator('#toast-editor-container').evaluate((el, m) => (el as ToastEditorContainer)._giteaToastEditor.setMarkdown(m), markdown);
}

// puts the caret at the end of the Visual editor's document
async function caretAtEnd(page: Page): Promise<void> {
  await page.locator('.toastui-editor-ww-container .ProseMirror').click();
  await page.keyboard.press('ControlOrMeta+End');
}

test.describe('Citations and internal links', () => {
  test.setTimeout(60000);
  // desktop flows: on a phone the tools open as full-screen sheets and the markers as a
  // bottom sheet instead of hover cards
  // eslint-disable-next-line playwright/no-skipped-test
  test.skip(({isMobile}) => isMobile, 'desktop layout');

  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('"[[" links to a subject, and to an author\'s article with Tab', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openArticleEditor(page);
    await setMarkdown(page, 'Landings.\n');
    await caretAtEnd(page);

    await page.keyboard.type(' See [[example');
    const search = page.locator('.citation-subject-search');
    await expect(search.locator('.citation-subject-row').first()).toContainText('example-subject', {timeout: 10000});
    await expect(search.locator('.citation-subject-row').first()).toContainText('1 article');
    await page.keyboard.press('Enter');
    await expect(search).toHaveCount(0);
    await expect.poll(() => markdownOf(page)).toContain('See [example-subject](subject/example-subject)');

    // Tab lists the subject's articles: link to user2's
    await page.keyboard.type(' and [[example');
    await expect(search.locator('.citation-subject-row').first()).toContainText('example-subject', {timeout: 10000});
    await page.keyboard.press('Tab');
    await expect(search.locator('.citation-subject-row').first()).toContainText('By user2', {timeout: 10000});
    await page.keyboard.press('Enter');
    await expect.poll(() => markdownOf(page)).toContain('and [example-subject](subject/example-subject/user2)');

    // a subject nobody created yet
    await page.keyboard.type(' or [[Nobody wrote this');
    await expect(search.locator('.citation-subject-new')).toContainText('Nobody wrote this', {timeout: 10000});
    await search.locator('.citation-subject-new').click();
    await expect.poll(() => markdownOf(page)).toContain('or [Nobody wrote this](subject/Nobody wrote this)');

    // Esc leaves the words
    await page.keyboard.type(' [[just words');
    await expect(search).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(search).toHaveCount(0);
    await expect.poll(() => markdownOf(page)).toContain(' just words');
    expect(await markdownOf(page)).not.toContain('[[just words');

    // the links stay links after a round trip through the Source editor
    await page.locator('.toastui-editor-mode-switch .tab-item', {hasText: 'Source editor'}).click();
    await page.locator('.toastui-editor-mode-switch .tab-item', {hasText: 'Visual editor'}).click();
    await expect(page.locator('.toastui-editor-ww-container .ProseMirror a', {hasText: 'Nobody wrote this'})).toHaveAttribute('href', 'subject/Nobody wrote this');
    expect(await markdownOf(page)).toContain('[Nobody wrote this](subject/Nobody wrote this)');
    await context.close();
  });

  test('a web address in the link search becomes a citation or a plain link', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    await openArticleEditor(page);
    await setMarkdown(page, 'Text.\n');
    await caretAtEnd(page);

    await page.locator('.forkana-subject-link').click();
    await page.keyboard.type('https://www.nasa.gov/');
    const popover = page.locator('.citation-link-url');
    await expect(popover).toBeVisible({timeout: 10000});
    await expect(popover.locator('.citation-banner')).toContainText('reference [1]');
    await popover.locator('[data-action="plain"]').click();
    await expect.poll(() => markdownOf(page)).toContain('Text.[https://www.nasa.gov/](https://www.nasa.gov/)');
    await context.close();
  });

  test('Cite adds a numbered reference, and reuses a source', async ({browser}, workerInfo) => {
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    // the lookup reaches the internet: answer it here
    await page.route('**/-/citation/lookup', (route) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({type: 'website', url: 'https://nssdc.gsfc.nasa.gov/luna2', title: 'Luna 2', site: 'NASA–NSSDC'}),
    }));
    await openArticleEditor(page);
    await setMarkdown(page, 'Luna 2 came first.\n');
    await caretAtEnd(page);

    await page.locator('.forkana-cite').click();
    const popover = page.locator('.citation-cite-popover');
    await expect(popover).toBeVisible();
    await popover.locator('input').fill('https://nssdc.gsfc.nasa.gov/luna2');
    await popover.locator('input').press('Enter');

    const dialog = page.locator('.citation-dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('.citation-type-tab[aria-checked="true"]')).toContainText('Website');
    // filled in from the lookup
    await expect(dialog.locator('.citation-field', {hasText: 'Title'}).locator('input')).toHaveValue('Luna 2');
    await expect(dialog.locator('.citation-dialog-preview-text')).toContainText('"Luna 2". NASA–NSSDC. Retrieved');
    await dialog.locator('[data-action="insert"]').click();
    await expect(dialog).toHaveCount(0);

    await expect.poll(() => markdownOf(page)).toContain('Luna 2 came first.[^1]');
    let markdown = await markdownOf(page);
    expect(markdown).toMatch(/\n\[\^1\]: \["Luna 2"\]\(https:\/\/nssdc\.gsfc\.nasa\.gov\/luna2\)\. \*NASA–NSSDC\*\. Retrieved \d+ \w+ \d{4}\.\n/);

    // Ctrl+Shift+K, then reuse the source: no second definition
    await page.keyboard.type(' Again.');
    await page.keyboard.press('ControlOrMeta+Shift+K');
    await expect(popover).toBeVisible();
    await popover.locator('.citation-reuse-row', {hasText: '[1]'}).click();
    await expect.poll(() => markdownOf(page)).toContain('Again.[^1]');
    markdown = await markdownOf(page);
    expect(markdown.match(/\[\^1\]:/g)).toHaveLength(1);

    // add a citation by hand
    await page.keyboard.type(' A book.');
    await page.locator('.forkana-cite').click();
    await popover.locator('[data-action="manual"]').click();
    await dialog.locator('.citation-type-tab', {hasText: 'Book'}).click();
    await dialog.locator('[data-action="insert"]').click();
    await expect(dialog.locator('.citation-dialog-error')).toBeVisible(); // title and authors are required
    await dialog.locator('.citation-field', {hasText: 'Title'}).locator('input').fill('A Man on the Moon');
    await dialog.locator('.citation-field', {hasText: 'Author(s)'}).locator('input').fill('Chaikin, Andrew');
    await dialog.locator('.citation-field', {hasText: 'Year'}).locator('input').fill('1994');
    await expect(dialog.locator('[data-action="insert"]')).toHaveText('Insert [2]');
    await dialog.locator('[data-action="insert"]').click();
    await expect.poll(() => markdownOf(page)).toContain('A book.[^2]');
    markdown = await markdownOf(page);
    expect(markdown).toContain('[^2]: Chaikin, Andrew (1994). *A Man on the Moon*.');
    await context.close();
  });

  test('articles render internal links and references', async ({browser}, workerInfo) => {
    const subject = `e2e-citations-${workerInfo.workerIndex}-${Date.now()}`;
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();
    let repoName = subject;
    try {
      repoName = await create_first_article(page, 'user2', subject);
      await disableGeneratedHooks('user2', repoName);
      await setMarkdown(page, [
        'See [example-subject](subject/example-subject), [a missing one](subject/Nobody wrote this) and [NASA](https://www.nasa.gov/).',
        '',
        'Luna 2 came first.[^luna] Apollo 11 landed in 1969.[^apollo] Luna again.[^luna]',
        '',
        '[^luna]: ["Luna 2"](https://nssdc.gsfc.nasa.gov/luna2). *NASA–NSSDC*.',
        '[^apollo]: Dhillon, Amrit (23 August 2023). ["India lands"](https://www.theguardian.com/x). *The Guardian*.',
        '',
      ].join('\n'));
      await expect(page.locator('#commit-button')).toBeEnabled({timeout: 10000});
      await page.locator('#commit-button').click();
      await page.waitForURL(`**/subject/${normalizeSubjectTitle(subject)}/user2**`, {timeout: 30000});

      const markup = page.locator('.file-view.markup');
      await expect(markup.locator('a.internal-link', {hasText: 'example-subject'})).toHaveAttribute('href', '/subject/example-subject');
      await expect(markup.locator('a.internal-link-missing', {hasText: 'a missing one'})).toHaveAttribute('href', '/repo/create?subject=Nobody+wrote+this');
      await expect(markup.locator('a.external-link', {hasText: 'NASA'}).first()).toBeVisible();

      const markers = markup.locator('sup.footnote-ref');
      await expect(markers).toHaveText(['[1]', '[2]', '[1]']);
      await expect(markup.locator('.footnotes-heading')).toHaveText('References');
      await expect(markup.locator('.footnotes-count')).toHaveText('2 sources from 2 publishers');
      await expect(markup.locator('.footnotes li').first().locator('.footnote-backrefs')).toHaveText('^ a b');
      await expect(markup.locator('.citation-summary')).toContainText('Website 1');
      await expect(markup.locator('.citation-summary')).toContainText('News 1');

      // the hover card
      await markers.nth(1).locator('a').hover();
      const card = page.locator('.citation-hover-card');
      await expect(card).toBeVisible();
      await expect(card).toContainText('News · Reference 2');
      await expect(card).toContainText('Used 1 time');

      // jump to the reference
      await markers.nth(0).locator('a').click();
      await expect(page).toHaveURL(/#fn:user-content-luna$/);

      // the filter chips
      await markup.locator('.citation-filter', {hasText: 'News'}).click();
      await expect(markup.locator('.footnotes li:visible')).toHaveCount(1);
      // it keeps its number (review #16)
      await expect(markup.locator('.footnotes li:visible')).toHaveAttribute('data-number', '2');
    } finally {
      try {
        await delete_repo(page, workerInfo, 'user2', repoName);
      } catch {
        // best effort: the assertion above is what matters
      }
      await context.close();
    }
  });
});
