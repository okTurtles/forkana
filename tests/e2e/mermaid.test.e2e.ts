import {expect, test, type Page} from '@playwright/test';
import {chmod, readFile, writeFile} from 'node:fs/promises';
import {isAbsolute, join, resolve} from 'node:path';
import {delete_repo, load_logged_in_context, login_user} from './utils_e2e.ts';

const articleDiagram = `flowchart LR
  ArticleAlpha --> ArticleBeta`;
const issueDiagram = `flowchart LR
  IssueAlpha --> IssueBeta`;
const commentDiagram = `flowchart LR
  CommentAlpha --> CommentBeta`;

function fencedMermaid(source: string): string {
  return `\`\`\`mermaid\n${source}\n\`\`\``;
}

async function setTextareaValue(page: Page, selector: string, value: string): Promise<void> {
  await page.locator(selector).evaluate((el, text) => {
    const textarea = el as HTMLTextAreaElement;
    textarea.value = text;
    textarea.dispatchEvent(new Event('input', {bubbles: true}));
    textarea.dispatchEvent(new Event('change', {bubbles: true}));
  }, value);
}

async function expectMermaidFrame(page: Page, index: number, text: RegExp): Promise<void> {
  const iframe = page.locator('iframe.markup-content-iframe').nth(index);
  await expect(iframe).toBeVisible({timeout: 20000});
  await expect(iframe.contentFrame().locator('svg')).toContainText(text, {timeout: 20000});
}

async function getRepositoryRoot(): Promise<string> {
  const configPath = resolve(process.cwd(), process.env.GITEA_CONF ?? 'tests/sqlite.ini');
  const config = await readFile(configPath, 'utf8');
  let inRepositorySection = false;

  for (const line of config.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed.startsWith('[')) {
      inRepositorySection = trimmed === '[repository]';
      continue;
    }
    if (!inRepositorySection) continue;

    const match = /^ROOT\s*=\s*(.+)$/.exec(trimmed);
    if (match) {
      const repoRoot = match[1].trim();
      return isAbsolute(repoRoot) ? repoRoot : resolve(process.cwd(), repoRoot);
    }
  }

  throw new Error(`Repository root not found in ${configPath}`);
}

async function disableGeneratedHooks(owner: string, repoName: string): Promise<void> {
  const repositoryRoot = await getRepositoryRoot();
  const repoPath = join(repositoryRoot, owner, `${repoName}.git`);
  const hookScript = '#!/usr/bin/env bash\n# Disabled for this E2E-created repository.\nexit 0\n';
  const hookPaths = [
    join(repoPath, 'hooks/pre-receive.d/gitea'),
    join(repoPath, 'hooks/update.d/gitea'),
    join(repoPath, 'hooks/post-receive.d/gitea'),
    join(repoPath, 'hooks/proc-receive'),
  ];

  await Promise.all(hookPaths.map(async (hookPath) => {
    await writeFile(hookPath, hookScript);
    await chmod(hookPath, 0o755);
  }));
}

function getRepoNameFromCurrentURL(page: Page, owner: string): string {
  const segments = new URL(page.url()).pathname.split('/').filter(Boolean);
  const ownerIndex = segments.indexOf(owner);
  if (ownerIndex < 0 || !segments[ownerIndex + 1]) {
    throw new Error(`Could not determine repository name from ${page.url()}`);
  }
  return decodeURIComponent(segments[ownerIndex + 1]);
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
      await page.goto(`/repo/create-first-article?subject=${encodeURIComponent(subject)}`);
      await page.waitForURL(/\/user2\/.*\/_new\/.*\/README\.md/, {timeout: 20000});
      repoName = getRepoNameFromCurrentURL(page, 'user2');
      await disableGeneratedHooks('user2', repoName);

      await setTextareaValue(page, '#edit_area', `${fencedMermaid(articleDiagram)}\n`);
      await page.locator('#commit-button').click();
      await page.waitForURL(`**/article/user2/${subject}**`, {timeout: 30000});

      await expectMermaidFrame(page, 0, /ArticleAlpha[\s\S]*ArticleBeta/);

      const copyButton = page.locator('.mermaid-block button[data-clipboard-text]').first();
      await expect(copyButton).toHaveAttribute('data-clipboard-text', /ArticleAlpha[\s\S]*ArticleBeta/);
      await copyButton.click();
      await expect.poll(() => page.evaluate(() => (window as typeof window & {__copiedText?: string}).__copiedText))
        .toContain(articleDiagram);

      await page.goto(`/user2/${subject}/issues/new`);
      await expect(page.locator('#new-issue')).toBeVisible({timeout: 10000});
      await page.locator('input[name="title"]').fill('Mermaid issue');
      await page.locator('#new-issue textarea[name="content"]').fill(fencedMermaid(issueDiagram));
      await page.locator('#new-issue button.ui.primary.button').click();
      await page.waitForURL(/\/user2\/.*\/issues\/[0-9]+/, {timeout: 20000});

      await expectMermaidFrame(page, 0, /IssueAlpha[\s\S]*IssueBeta/);

      await page.locator('#comment-form textarea[name="content"]').fill(fencedMermaid(commentDiagram));
      await page.locator('#comment-button').click();
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
