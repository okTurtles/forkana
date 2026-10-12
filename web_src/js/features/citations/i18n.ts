// Texts of the references and of the editor's link and cite tools (issue #399). They come from
// the locale through the hidden <template id="forkana-citation-i18n"> of every page
// (custom/templates/custom/footer.tmpl): one data attribute per "citation.*" key, with "{1}",
// "{2}" for the arguments. A key without text (no template, e.g. in unit tests) shows the key.

let texts: Record<string, string> | null = null;

function loadTexts(): Record<string, string> {
  if (texts) return texts;
  const el = document.querySelector<HTMLTemplateElement>('#forkana-citation-i18n');
  texts = {};
  for (const attr of el?.attributes ?? []) {
    if (attr.name.startsWith('data-')) texts[attr.name.slice(5)] = attr.value;
  }
  return texts;
}

// setCitationTexts replaces the texts (unit tests).
export function setCitationTexts(t: Record<string, string> | null): void {
  texts = t;
}

// "used_times" → "used-times", the name of its "data-used-times" attribute.
function attributeKey(key: string): string {
  return key.replaceAll('_', '-');
}

// citationText returns the text of the "citation.<key>" locale key with its arguments.
export function citationText(key: string, ...args: Array<string | number>): string {
  const text = loadTexts()[attributeKey(key)];
  if (text === undefined) return args.length ? `${key} ${args.join(' ')}` : key;
  return text.replace(/\{(\d)\}/g, (match, n: string) => {
    const arg = args[Number(n) - 1];
    return arg === undefined ? match : String(arg);
  });
}
