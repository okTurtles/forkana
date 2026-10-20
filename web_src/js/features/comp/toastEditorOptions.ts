// Toast UI options the article editor (toast-editor.ts) depends on for what it saves.
//
// frontMatter: an article file may open with YAML front matter (the Wikipedia imports
// carry title, source, license, attribution and fetched_at). Without this option Toast UI
// reads its `---` opener as a thematic break and the lines under it as one paragraph, so
// the first edit made in the Visual editor saved `***` and the keys run together on one
// line, which then rendered as a rule and a big heading (#433, seen on Prodromus). With
// it the block is a front matter node that both editors keep byte for byte.
export const articleEditorMarkdownOptions = {
  frontMatter: true,
} as const;
