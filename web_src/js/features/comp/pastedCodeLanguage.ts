// Keep the language of a code block pasted as HTML into the Visual editor (issue #373).
//
// A code block copied from a rendered page (ChatGPT, GitHub, any highlighter) arrives as
// `<pre><code class="language-mermaid">`, but Toast UI's codeBlock parser only reads the
// language from `data-language` on the <pre>'s first element child, the form Toast UI itself
// renders. The block was pasted without its language, so a pasted mermaid diagram was saved
// as a plain ``` fence and never rendered.
//
// The fix is a ProseMirror `transformPastedHTML` prop, given to the editor through Toast UI's
// plugin API (`wysiwygPlugins`). ProseMirror runs every such prop in turn (Toast UI's own,
// which cleans up Office HTML, then this one) before parsing the HTML with the schema, so the
// language is moved where the codeBlock parser reads it without touching vendor code.

// What an info string may hold once written after ``` (```mermaid, ```c++, ```objective-c).
const languagePattern = /^[\w#+.-]+$/;

function languageFromClass(el: Element | null): string | null {
  if (!el) return null;
  for (const name of el.classList) {
    // `language-X` (CommonMark, highlight.js, Prism, ChatGPT), `lang-X` (Prettify)
    const match = /^(?:language|lang)-(.+)$/.exec(name);
    if (match && languagePattern.test(match[1])) return match[1];
  }
  return null;
}

// GitHub names a few grammars after their TextMate scope rather than their fence name.
const githubScopeAliases: Record<string, string> = {'html-basic': 'html'};

function languageOf(pre: HTMLElement, code: Element | null): string | null {
  // The classes first: `lang` is, in HTML, the natural language of the text (`<pre lang="en">`),
  // and only a legacy code-language mark when a page uses it so.
  const fromClass = languageFromClass(code) ?? languageFromClass(pre);
  if (fromClass) return fromClass;
  const lang = pre.getAttribute('lang');
  if (lang && languagePattern.test(lang)) return lang;
  // GitHub's rendered Markdown: <div class="highlight highlight-source-js"><pre>…</pre></div>,
  // or highlight-text-X for text-like grammars (md, html-basic, xml).
  const wrapper = pre.parentElement?.closest('[class*="highlight-source-"], [class*="highlight-text-"]');
  if (wrapper) {
    for (const name of wrapper.classList) {
      const match = /^highlight-(?:source|text)-(.+)$/.exec(name);
      if (match && languagePattern.test(match[1])) return githubScopeAliases[match[1]] ?? match[1];
    }
  }
  return null;
}

function isBlank(node: Node): boolean {
  return node.nodeType === Node.TEXT_NODE && !node.textContent?.trim();
}

// transformPastedCodeLanguage rewrites every <pre> whose language is given in a form Toast UI
// does not read into `<pre><code data-language="X">…</code></pre>`. HTML with no such block is
// returned unchanged, byte for byte.
export function transformPastedCodeLanguage(html: string): string {
  if (!/<pre[\s>]/i.test(html)) return html;
  const doc = new DOMParser().parseFromString(html, 'text/html');
  let changed = false;
  for (const pre of doc.querySelectorAll('pre')) {
    const first = pre.firstElementChild;
    if (first?.hasAttribute('data-language')) continue; // Toast UI's own form, e.g. copied from the editor
    const code = pre.querySelector('code');
    const language = languageOf(pre, code);
    if (!language) continue;
    let target = code;
    if (!target) {
      // `<pre lang="X">` or a highlighter's <pre> of bare <span>s: wrap the content.
      target = doc.createElement('code');
      target.append(...pre.childNodes);
      pre.append(target);
    } else if (pre.firstElementChild !== target) {
      // A <code> after a toolbar (ChatGPT puts the language name and a "Copy" button in the
      // <pre> before it, and the code in a <div>): bring the <code> up to the <pre> so Toast UI
      // reads its language, and drop the elements before it, or the toolbar text is pasted
      // into the block. Everything from the <code> on is kept (more <code> regions, a note),
      // and so is text before it: Toast UI pastes all the text of a <pre>.
      for (let wrapper = target.parentElement; wrapper && wrapper !== pre; wrapper = target.parentElement) {
        wrapper.replaceWith(...wrapper.childNodes);
      }
      const children = Array.from(pre.childNodes);
      const before = children.slice(0, children.indexOf(target));
      for (const node of before) {
        if (node.nodeType === Node.ELEMENT_NODE || isBlank(node)) node.remove();
      }
    }
    target.setAttribute('data-language', language);
    changed = true;
  }
  if (!changed) return html;
  return doc.body.innerHTML;
}

type ToastPluginContext = {
  pmState: {Plugin: new (spec: {props: {transformPastedHTML: (html: string) => string}}) => unknown};
};

// pastedCodeLanguagePlugin is a Toast UI plugin (the editor's `plugins` option).
export function pastedCodeLanguagePlugin(context: ToastPluginContext) {
  const {Plugin} = context.pmState;
  return {
    wysiwygPlugins: [() => new Plugin({props: {transformPastedHTML: transformPastedCodeLanguage}})],
  };
}
