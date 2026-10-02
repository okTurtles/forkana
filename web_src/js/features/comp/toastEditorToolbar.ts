// The default toolbar shared by the article editor (toast-editor.ts) and the comment editor
// (ToastCommentEditor.ts). Keep it in one place: the two copies it replaces drifted once
// already, when a styling commit rewrote one of them and silently dropped an item.
//
// `codeblock` is what lets a Visual-mode author produce a fenced code block at all
// (issue #367). Without it there is no way to write a ```mermaid block except by
// switching to the Source editor: typing the backticks in Visual mode just escapes
// them, so diagrams get saved as plain paragraphs and never render.
export const defaultToolbarItems: string[][] = [
  ['heading', 'bold', 'italic'],
  ['indent', 'outdent', 'code', 'codeblock', 'link'],
  ['ul', 'ol', 'task'],
  ['image', 'table'],
];
