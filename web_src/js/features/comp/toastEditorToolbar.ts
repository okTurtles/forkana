// The default toolbar shared by the article editor (toast-editor.ts) and the comment editor
// (ToastCommentEditor.ts). Keep it in one place: the two copies it replaces drifted once
// already, when a styling commit rewrote one of them and silently dropped an item.
//
// `codeblock` is what lets a Visual-mode author produce a fenced code block at all
// (issue #367). Without it there is no way to write a ```mermaid block except by
// switching to the Source editor: typing the backticks in Visual mode just escapes
// them, so diagrams get saved as plain paragraphs and never render.
//
// The groups follow the article editor's toolbar in the figma (#434 item 11, "Article view /
// Logged out - Edit" 641:65657, _Markdown toolbar 641:65763): heading, bold, italic | quote,
// code, link | bulleted, numbered and task lists. Its fourth group (mention, cross-reference,
// saved replies) has no counterpart in an article, so the editor's own codeblock (see above),
// image and table take its place. Their icons are the figma's octicons, set in
// custom/public/assets/css/toast-editor.css.
export const defaultToolbarItems: string[][] = [
  ['heading', 'bold', 'italic'],
  ['quote', 'code', 'link'],
  ['ul', 'ol', 'task'],
  ['codeblock', 'image', 'table'],
];
