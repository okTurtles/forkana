// Pasting into the REAL Toast UI Editor (issue #373): a code block copied as HTML keeps its
// language. The transform itself is tested in pastedCodeLanguage.test.ts.

// @ts-expect-error - @toast-ui/editor has type definition issues with package.json exports
import Editor from '@toast-ui/editor';
import {pastedCodeLanguagePlugin} from './pastedCodeLanguage.ts';

// `>` escaped, as a clipboard holds it: happy-dom's parser garbles a raw `-->` in text.
const htmlDiagram = 'flowchart TD\n  Start --&gt; Stop\n';

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
