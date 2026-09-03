import {expect, test, type Page} from '@playwright/test';
import {create_first_article, delete_repo, disableGeneratedHooks, load_logged_in_context, login_user} from './utils_e2e.ts';

const articleDiagram = `flowchart LR
  ArticleAlpha --> ArticleBeta`;
const issueDiagram = `flowchart LR
  IssueAlpha --> IssueBeta`;
const commentDiagram = `flowchart LR
  CommentAlpha --> CommentBeta`;

function fencedMermaid(source: string): string {
  return `\`\`\`mermaid\n${source}\n\`\`\``;
}

// Every editor in the app (issue, comment, article) hides its textarea and owns the content,
// so the value has to go through the editor instance: writing to the textarea directly is
// overwritten by the lossless tracker's syncTextarea() on the next editor event, and the
// editor is what the submit paths read. Comment forms expose the ToastCommentEditor wrapper
// (`.value()`), the repo/article editor exposes the Toast editor itself (`setMarkdown()`);
// both go through the lossless layer and sync the textarea the same way a user edit does.
async function setToastEditorValue(page: Page, selector: string, value: string): Promise<void> {
  const container = page.locator(selector);
  await expect(container.locator('.toastui-editor-defaultUI')).toBeVisible({timeout: 20000});
  await container.evaluate((el, text) => {
    type ToastEditorContainer = HTMLElement & {
      _giteaToastCommentEditor?: {value: (v?: string) => string};
      _giteaToastEditor?: {setMarkdown: (markdown: string) => void};
    };
    const {_giteaToastCommentEditor: commentEditor, _giteaToastEditor: editor} = el as ToastEditorContainer;
    if (commentEditor) {
      commentEditor.value(text);
      el.dispatchEvent(new CustomEvent('ce-editor-content-changed'));
    } else if (editor) {
      editor.setMarkdown(text);
    } else {
      throw new Error('Toast editor is not initialized');
    }
  }, value);
}

async function expectMermaidFrame(page: Page, index: number, text: RegExp): Promise<void> {
  const iframe = page.locator('iframe.markup-content-iframe').nth(index);
  await expect(iframe).toBeVisible({timeout: 20000});
  await expect(iframe.contentFrame().locator('svg')).toContainText(text, {timeout: 20000});
}

test.describe('Mermaid rendering', () => {
  test.setTimeout(60000);

  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('renders diagrams in articles, issues, comments, and supports copying source', async ({browser}, workerInfo) => {
    const subject = `e2e-mermaid-${workerInfo.workerIndex}-${Date.now()}`;
    let repoName = subject;
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();

    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            (window as typeof window & {__copiedText?: string}).__copiedText = text;
          },
        },
      });
    });

    try {
      repoName = await create_first_article(page, 'user2', subject);
      await disableGeneratedHooks('user2', repoName);

      await setToastEditorValue(page, '#toast-editor-container', `${fencedMermaid(articleDiagram)}\n`);
      // the commit button stays disabled until areYouSure sees the textarea change the
      // editor's textarea sync dispatches, which is also what enables it for a real user
      await expect(page.locator('#commit-button')).toBeEnabled({timeout: 10000});
      await page.locator('#commit-button').click();
      await page.waitForURL(`**/article/user2/${subject}**`, {timeout: 30000});

      await expectMermaidFrame(page, 0, /ArticleAlpha[\s\S]*ArticleBeta/);

      const mermaidBlock = page.locator('.mermaid-block').first();
      const copyButton = mermaidBlock.locator('button[data-clipboard-text]');
      await expect(copyButton).toHaveAttribute('data-clipboard-text', /ArticleAlpha[\s\S]*ArticleBeta/);
      await expect(page.locator('.code-block-container button[data-clipboard-text]')).toHaveCount(1);
      await mermaidBlock.hover();
      await expect(copyButton).toBeVisible();
      await copyButton.click();
      await expect.poll(() => page.evaluate(() => (window as typeof window & {__copiedText?: string}).__copiedText))
        .toContain(articleDiagram);

      await page.goto(`/user2/${subject}/issues/new`);
      await expect(page.locator('#new-issue')).toBeVisible({timeout: 10000});
      await page.locator('input[name="title"]').fill('Mermaid issue');
      await setToastEditorValue(page, '#new-issue .toast-comment-editor', fencedMermaid(issueDiagram));
      await page.locator('#new-issue button.ui.primary.button').click();
      await page.waitForURL(/\/issues\/[0-9]+$/, {timeout: 20000});
      // creating an issue redirects to the article-scoped URL, which has no view route, so
      // continue on the repository issue URL where the issue is rendered
      const issueIndex = new URL(page.url()).pathname.split('/').pop();
      const issueUrl = `/user2/${repoName}/issues/${issueIndex}`;
      await page.goto(issueUrl);

      await expectMermaidFrame(page, 0, /IssueAlpha[\s\S]*IssueBeta/);

      await setToastEditorValue(page, '#comment-form .toast-comment-editor', fencedMermaid(commentDiagram));
      await page.locator('#comment-button').click();
      // posting a comment also redirects to the article-scoped URL
      await page.waitForURL(/#issuecomment-[0-9]+$/, {timeout: 20000});
      await page.waitForLoadState('load');
      await page.goto(issueUrl);
      await expect(page.locator('iframe.markup-content-iframe')).toHaveCount(2, {timeout: 20000});
      await expectMermaidFrame(page, 1, /CommentAlpha[\s\S]*CommentBeta/);
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
