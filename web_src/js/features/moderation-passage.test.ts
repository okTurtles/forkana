import {
  findPassageBlock,
  highlightPassage,
  initPassageRoot,
  parseSourceRange,
  passageFromBlock,
  passageFromSelection,
  type Passage,
} from './moderation-passage.ts';

// what the renderer writes with RenderOptions.SourceRanges: byte offsets, which differ from
// the UTF-16 positions of the text ("Açaí 🍓" is 11 bytes, 7 UTF-16 units)
const article = `
<h1 id="user-content-acai" data-source-range="22-33">Açaí 🍓</h1>
<p data-source-range="35-80">O açaí é <strong>popular</strong> — 東京</p>
<ul><li data-source-range="84-90">um ç<ul><li data-source-range="95-104">nested 子供</li></ul></li></ul>
<h2 id="user-content-fim" data-source-range="110-113">Fim</h2>
<div class="raw">Raw <em>HTML</em> block</div>
<p data-source-range="x">forged</p>`;

function setup(): HTMLElement {
  document.body.innerHTML = `<div class="moderation-passage-root">${article}</div><p data-source-range="1-2">outside</p>`;
  return document.querySelector('.moderation-passage-root');
}

function select(node: Node, start: number, endNode: Node, end: number) {
  const range = document.createRange();
  range.setStart(node, start);
  range.setEnd(endNode, end);
  const sel = window.getSelection();
  sel.removeAllRanges();
  sel.addRange(range);
  return sel;
}

test('parseSourceRange', () => {
  expect(parseSourceRange('12-40')).toEqual({start: 12, end: 40});
  expect(parseSourceRange('0-1')).toEqual({start: 0, end: 1});
  for (const bad of [null, '', '12', '12-', '-4', '40-12', '12-12', '012-40', '12-40x', ' 12-40', '1e3-2000']) {
    expect(parseSourceRange(bad)).toBeNull();
  }
});

test('findPassageBlock', () => {
  const root = setup();
  const strong = root.querySelector('strong');
  expect(findPassageBlock(strong.firstChild, root)?.getAttribute('data-source-range')).toBe('35-80');
  const nested = root.querySelectorAll('li')[1];
  expect(findPassageBlock(nested.firstChild, root)?.getAttribute('data-source-range')).toBe('95-104');
  expect(findPassageBlock(root.querySelector('.raw em'), root)).toBeNull();
  expect(findPassageBlock(root.querySelector('p:last-child').firstChild, root)).toBeNull(); // forged value
  expect(findPassageBlock(document.body.lastElementChild.firstChild, root)).toBeNull(); // outside the root
});

test('passageFromBlock echoes the attribute, never offsets from the text', () => {
  const root = setup();
  expect(passageFromBlock(root.querySelector('h1'))).toEqual({anchor: 'exact', range: '22-33', quote: 'Açaí 🍓'});
});

test('passageFromSelection snaps to the containing block', () => {
  const root = setup();
  const p = root.querySelector('p');
  // from inside "O açaí é" to inside "popular"
  const sel = select(p.firstChild, 2, root.querySelector('strong').firstChild, 3);
  expect(passageFromSelection(sel, root)).toEqual({anchor: 'exact', range: '35-80', quote: 'O açaí é popular — 東京'});
});

test('passageFromSelection across blocks uses the block containing both ends, or the quote', () => {
  const root = setup();
  const items = root.querySelectorAll('li');
  // from the outer item into the nested one: the outer item contains both
  let sel = select(items[0].firstChild, 0, items[1].firstChild, 3);
  expect(passageFromSelection(sel, root)).toMatchObject({anchor: 'exact', range: '84-90'});

  // in a raw HTML block: quote only, with the section of the heading before it
  const raw = root.querySelector('.raw');
  sel = select(raw.firstChild, 0, raw.querySelector('em').firstChild, 4);
  expect(passageFromSelection(sel, root)).toEqual({anchor: 'quote_only', quote: 'Raw HTML', section: 'user-content-fim'});
});

test('passageFromSelection ignores empty and outside selections', () => {
  const root = setup();
  const p = root.querySelector('p');
  expect(passageFromSelection(select(p.firstChild, 1, p.firstChild, 1), root)).toBeNull();
  const outside = document.body.lastElementChild;
  expect(passageFromSelection(select(outside.firstChild, 0, outside.firstChild, 3), root)).toBeNull();
});

test('highlightPassage marks one block', () => {
  const root = setup();
  expect(highlightPassage(root, '35-80')?.tagName).toBe('P');
  expect(highlightPassage(root, '95-104')?.tagName).toBe('LI');
  expect(root.querySelectorAll('.moderation-passage-highlight')).toHaveLength(1);
  expect(highlightPassage(root, '1-2')).toBeNull(); // another revision: no such block
  expect(highlightPassage(root, '"] *, [x="')).toBeNull(); // never reaches the selector
  expect(root.querySelectorAll('.moderation-passage-highlight')).toHaveLength(0);
});

test('initPassageRoot highlights and selects', () => {
  const root = setup();
  root.setAttribute('data-passage-highlight', '110-113');
  root.setAttribute('data-passage-select', '');
  Element.prototype.scrollIntoView = Element.prototype.scrollIntoView ?? (() => {});
  initPassageRoot(root);
  expect(root.querySelector('h2').classList.contains('moderation-passage-highlight')).toBe(true);

  const picked: Passage[] = [];
  root.addEventListener('moderation:passage', (e) => picked.push((e as CustomEvent<Passage>).detail));
  window.getSelection().removeAllRanges();
  const strong = root.querySelector('strong');
  strong.dispatchEvent(new MouseEvent('mouseover', {bubbles: true}));
  expect(root.querySelector('p').classList.contains('moderation-passage-hover')).toBe(true);
  strong.dispatchEvent(new MouseEvent('mouseup', {bubbles: true}));
  expect(picked).toEqual([{anchor: 'exact', range: '35-80', quote: 'O açaí é popular — 東京'}]);
  expect(root.querySelector('p').classList.contains('moderation-passage-highlight')).toBe(true);
});
