// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {pastedCodeLanguagePlugin, transformPastedCodeLanguage} from './pastedCodeLanguage.ts';

const diagram = 'flowchart TD\n  Start --> Stop\n';
// As a clipboard holds it, with `>` escaped: happy-dom's parser garbles a raw `-->` in text
// (it repeats the text before it), which a browser does not.
const htmlDiagram = 'flowchart TD\n  Start --&gt; Stop\n';

function codeOf(html: string) {
  const doc = new DOMParser().parseFromString(transformPastedCodeLanguage(html), 'text/html');
  const pre = doc.querySelector('pre');
  const code = pre.firstElementChild;
  return {pre, code, language: code.getAttribute('data-language'), text: pre.textContent};
}

describe('transformPastedCodeLanguage', () => {
  test.each([
    ['class="language-X" on <code>', `<pre><code class="language-mermaid">${htmlDiagram}</code></pre>`, 'mermaid'],
    ['among other classes', `<pre><code class="hljs language-mermaid !whitespace-pre">${htmlDiagram}</code></pre>`, 'mermaid'],
    ['class="lang-X" on <code>', `<pre><code class="lang-js">x()</code></pre>`, 'js'],
    ['class="language-X" on <pre> (Prism)', `<pre class="language-c++"><code>x</code></pre>`, 'c++'],
    ['<pre lang="X"> with a <code>', `<pre lang="mermaid"><code>${htmlDiagram}</code></pre>`, 'mermaid'],
    ['<pre lang="X"> without a <code>', `<pre lang="mermaid">${htmlDiagram}</pre>`, 'mermaid'],
    ['GitHub highlight-source-X wrapper', `<div class="highlight highlight-source-js"><pre><span class="pl-k">const</span> a = 1;</pre></div>`, 'js'],
  ])('reads the language from %s', (_, html, language) => {
    const result = codeOf(html);
    expect(result.code.tagName).toBe('CODE');
    expect(result.language).toBe(language);
  });

  test('keeps the code text, and drops a toolbar before the <code>', () => {
    const result = codeOf(`<pre><div><span>mermaid</span><button>Copy code</button></div><div><code class="language-mermaid">${htmlDiagram}</code></div></pre>`);
    expect(result.language).toBe('mermaid');
    expect(result.text).toBe(diagram);
    expect(result.pre.children).toHaveLength(1);
  });

  test('wraps a <pre> of highlighter spans without losing text', () => {
    const result = codeOf(`<div class="highlight-source-python"><pre><span>def</span> f():\n  <span>pass</span></pre></div>`);
    expect(result.language).toBe('python');
    expect(result.text).toBe('def f():\n  pass');
  });

  test.each([
    ['no <pre>', '<p class="language-mermaid">text</p>'],
    ['a <pre> without a language', '<pre><code>plain</code></pre>'],
    ['Toast UI\'s own form', '<pre><code data-language="js">x</code></pre>'],
    ['a language that cannot be an info string', '<pre><code class="language-a`b">x</code></pre>'],
  ])('returns the HTML unchanged for %s', (_, html) => {
    expect(transformPastedCodeLanguage(html)).toBe(html);
  });
});

describe('pasting into the real Visual editor', () => {
  function createEditor(plugins: unknown[]) {
    const el = document.createElement('div');
    document.body.append(el);
    return new Editor({el, initialEditType: 'wysiwyg', usageStatistics: false, plugins});
  }

  test('a pasted language-mermaid block is saved as a ```mermaid fence', () => {
    const editor = createEditor([pastedCodeLanguagePlugin]);
    editor.wwEditor.view.pasteHTML(`<pre><code class="language-mermaid">${htmlDiagram}</code></pre>`);
    expect(editor.getMarkdown()).toContain('```mermaid\nflowchart TD\n  Start --> Stop');
    editor.destroy();
  });

  test('without the plugin the language is lost (the bug)', () => {
    const editor = createEditor([]);
    editor.wwEditor.view.pasteHTML(`<pre><code class="language-mermaid">${htmlDiagram}</code></pre>`);
    expect(editor.getMarkdown()).not.toContain('```mermaid');
    editor.destroy();
  });
});
