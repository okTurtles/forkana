import {expect, test, type Page} from '@playwright/test';
import {create_first_article, delete_repo, disableGeneratedHooks, load_logged_in_context, login_user, normalizeSubjectTitle} from './utils_e2e.ts';

// A code block copied from a rendered page (ChatGPT, GitHub, any highlighter) puts
// `<pre><code class="language-mermaid">` on the clipboard. Pasted into the Visual editor it
// used to become a fence without its language, so the diagram never rendered (#373).

const diagram = 'flowchart LR\n  PastedAlpha --> PastedBeta';
// eslint-disable-next-line github/unescaped-html-literal -- the clipboard HTML under test, never rendered
const pastedHtml = `<pre><code class="hljs language-mermaid">${diagram.replace('>', '&gt;')}\n</code></pre>`;

type EditorContainer = HTMLElement & {_giteaToastEditor: {getMarkdown: () => string}};

async function pasteHtml(page: Page, browserName: string, html: string, text: string) {
  const visual = page.locator('#toast-editor-container .toastui-editor-ww-container .ProseMirror');
  await visual.click();
  if (browserName === 'chromium') {
    // A real paste: the HTML goes through the system clipboard and the browser's own
    // clipboard handling, then Ctrl/Cmd+V.
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.evaluate(async ([h, t]) => {
      await navigator.clipboard.write([new ClipboardItem({
        'text/html': new Blob([h], {type: 'text/html'}),
        'text/plain': new Blob([t], {type: 'text/plain'}),
      })]);
    }, [html, text]);
    await page.keyboard.press('ControlOrMeta+V');
  } else {
    // WebKit gives no clipboard write permission to automation: the same paste event, built.
    await visual.evaluate((el, [h, t]) => {
      const dt = new DataTransfer();
      dt.setData('text/html', h);
      dt.setData('text/plain', t);
      el.dispatchEvent(new ClipboardEvent('paste', {bubbles: true, cancelable: true, clipboardData: dt}));
    }, [html, text]);
  }
}

test.describe('Pasting a code block into the Visual editor', () => {
  test.setTimeout(60000);

  test.beforeAll(async ({browser}, workerInfo) => {
    await login_user(browser, workerInfo, 'user2');
  });

  test('keeps its language, so a pasted mermaid diagram renders (#373)', async ({browser, browserName}, workerInfo) => {
    const subject = `e2e-paste-code-${workerInfo.workerIndex}-${Date.now()}`;
    let repoName = subject;
    const context = await load_logged_in_context(browser, workerInfo, 'user2');
    const page = await context.newPage();

    try {
      repoName = await create_first_article(page, 'user2', subject);
      await disableGeneratedHooks('user2', repoName);
      const container = page.locator('#toast-editor-container');
      await expect(container.locator('.toastui-editor-ww-container .ProseMirror')).toBeVisible({timeout: 20000});

      await pasteHtml(page, browserName, pastedHtml, diagram);

      // the Visual editor shows a code block with the language, and saves it as ```mermaid
      await expect(container.locator('.toastui-editor-ww-container pre code[data-language="mermaid"]')).toContainText('PastedAlpha');
      await expect.poll(() => container.evaluate((el) => (el as EditorContainer)._giteaToastEditor.getMarkdown()))
        .toContain(`\`\`\`mermaid\n${diagram}`);

      await expect(page.locator('#commit-button')).toBeEnabled({timeout: 10000});
      await page.locator('#commit-button').click();
      await page.waitForURL(`**/subject/${normalizeSubjectTitle(subject)}/user2**`, {timeout: 30000});

      const iframe = page.locator('iframe.markup-content-iframe').first();
      await expect(iframe).toBeVisible({timeout: 20000});
      await expect(iframe.contentFrame().locator('body > svg')).toContainText(/PastedAlpha[\s\S]*PastedBeta/, {timeout: 20000});
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
