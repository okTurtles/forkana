import {registerGlobalSelectorFunc} from '../modules/observer.ts';

/* Moderation passage anchors. A report anchors to one block of the rendered article: the
   renderer gives each block it can map a data-source-range="start-end" attribute
   (RenderOptions.SourceRanges, modules/markup/markdown/source_range.go).

   The values are byte offsets into the README; the DOM's text is UTF-16, so offsets must
   never be computed from it. This module only ever reads the attribute and sends its value
   back as it is; the server re-checks it against the README. A block without a range (raw
   HTML, a rule) can only be reported by its quote (anchor "quote_only"). */

export const sourceRangeAttr = 'data-source-range';
const highlightClass = 'moderation-passage-highlight';
const hoverClass = 'moderation-passage-hover';
// rendered quotes are capped like the server's (markdown.MaxSourceBlockQuote)
const maxQuoteLength = 500;

export type Passage =
  | {anchor: 'exact', range: string, quote: string} |
  {anchor: 'quote_only', quote: string, section: string};

const reRange = /^(0|[1-9]\d{0,9})-([1-9]\d{0,9})$/;

// parseSourceRange returns the offsets of a "start-end" value, or null. It is only used to
// check a value: the value itself is what is sent.
export function parseSourceRange(value: string | null): {start: number, end: number} | null {
  const m = reRange.exec(value ?? '');
  if (!m) return null;
  const start = Number(m[1]), end = Number(m[2]);
  return start < end ? {start, end} : null;
}

function collapse(text: string): string {
  return Array.from(text.replace(/\s+/g, ' ').trim()).slice(0, maxQuoteLength).join('');
}

function elementOf(node: Node | null): Element | null {
  if (!node) return null;
  return node.nodeType === Node.ELEMENT_NODE ? node as Element : node.parentElement;
}

// findPassageBlock returns the innermost block with a source range that contains the node,
// inside root, or null.
export function findPassageBlock(node: Node | null, root: Element): HTMLElement | null {
  const block = elementOf(node)?.closest<HTMLElement>(`[${sourceRangeAttr}]`);
  if (!block || !root.contains(block) || !parseSourceRange(block.getAttribute(sourceRangeAttr))) return null;
  return block;
}

// sectionOf returns the id of the nearest heading before the element in root (without the
// renderer's "user-content-" prefix kept or removed: as rendered), or ''.
function sectionOf(el: Element, root: Element): string {
  let section = '';
  for (const h of root.querySelectorAll<HTMLElement>('h1[id], h2[id], h3[id], h4[id], h5[id], h6[id]')) {
    // DOCUMENT_POSITION_FOLLOWING: el comes after h
    if (h === el || h.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING) section = h.id;
    else break;
  }
  return section;
}

// passageFromBlock returns the exact passage of a block with a source range.
export function passageFromBlock(block: HTMLElement): Passage {
  return {anchor: 'exact', range: block.getAttribute(sourceRangeAttr), quote: collapse(block.textContent ?? '')};
}

// passageFromSelection snaps a text selection inside root to a passage: the innermost block
// with a range that contains the whole selection, or, when there is none (raw HTML), a
// quote-only passage of the selected text. Null when nothing inside root is selected.
export function passageFromSelection(selection: Selection | null, root: Element): Passage | null {
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  const common = elementOf(range.commonAncestorContainer);
  if (!common || !root.contains(common)) return null;
  const block = findPassageBlock(common, root);
  if (block) return passageFromBlock(block);
  const quote = collapse(selection.toString());
  if (!quote) return null;
  return {anchor: 'quote_only', quote, section: sectionOf(common, root)};
}

// highlightPassage marks the block whose range is exactly the value, and unmarks the others.
// It returns the block, or null when the article has no such block (another revision).
export function highlightPassage(root: Element, range: string): HTMLElement | null {
  for (const el of root.querySelectorAll(`.${highlightClass}`)) el.classList.remove(highlightClass);
  if (!parseSourceRange(range)) return null;
  const block = root.querySelector<HTMLElement>(`[${sourceRangeAttr}="${range}"]`);
  block?.classList.add(highlightClass);
  return block;
}

/* initPassageRoot wires one rendered article:
   - data-passage-highlight="start-end" highlights that block (the case review);
   - data-passage-select makes blocks selectable: hovering marks the block, a click or a text
     selection picks it, and a "moderation:passage" event carries the Passage (the report
     modal of feature 1.1 listens to it). */
export function initPassageRoot(root: HTMLElement) {
  const highlight = root.getAttribute('data-passage-highlight');
  if (highlight) highlightPassage(root, highlight)?.scrollIntoView({block: 'center'});

  if (!root.hasAttribute('data-passage-select')) return;
  const pick = (passage: Passage | null) => {
    if (!passage) return;
    if (passage.anchor === 'exact') highlightPassage(root, passage.range);
    root.dispatchEvent(new CustomEvent<Passage>('moderation:passage', {detail: passage, bubbles: true}));
  };
  let hovered: HTMLElement | null = null;
  root.addEventListener('mouseover', (e) => {
    const block = findPassageBlock(e.target as Node, root);
    if (block === hovered) return;
    hovered?.classList.remove(hoverClass);
    hovered = block;
    hovered?.classList.add(hoverClass);
  });
  root.addEventListener('mouseleave', () => {
    hovered?.classList.remove(hoverClass);
    hovered = null;
  });
  root.addEventListener('mouseup', (e) => {
    const selected = passageFromSelection(window.getSelection(), root);
    if (selected) return pick(selected);
    const block = findPassageBlock(e.target as Node, root);
    if (block) pick(passageFromBlock(block));
  });
}

export function initModerationPassage() {
  registerGlobalSelectorFunc('.moderation-passage-root', initPassageRoot);
}
