import {initMarkupReferences} from './references.ts';

function article(): HTMLElement {
  const container = document.createElement('div');
  container.className = 'markup file-view';
  container.innerHTML = `
    <p>Luna 2.<sup class="footnote-ref" id="fnref:1"><a href="#fn:1">[1]</a></sup>
      India.<sup class="footnote-ref" id="fnref:2"><a href="#fn:2">[2]</a></sup></p>
    <div class="footnotes"><h2 class="footnotes-heading">References</h2><ol>
      <li id="fn:1"><p><span class="footnote-backrefs"><a href="#fnref:1">^</a></span> <a href="https://nssdc.gsfc.nasa.gov/x">"Luna 2"</a>. <em>NASA</em>.</p></li>
      <li id="fn:2"><p><span class="footnote-backrefs"><a href="#fnref:2">^</a></span> <a href="https://www.theguardian.com/x">"India lands"</a>. <em>The Guardian</em>.</p></li>
    </ol></div>`;
  document.body.append(container);
  initMarkupReferences(container);
  return container;
}

function filter(container: HTMLElement, label: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('.citation-filter')).find((b) => b.textContent.includes(label));
}

afterEach(() => {
  document.body.innerHTML = '';
});

test('"Go to reference" shows the reference a filter hides', () => {
  const container = article();
  filter(container, 'type_news').click();
  const website = container.querySelector<HTMLLIElement>('#fn\\:1');
  expect(website.hidden).toBe(true);

  container.querySelector<HTMLAnchorElement>('a[href="#fn:1"]').dispatchEvent(new FocusEvent('focus'));
  const card = document.querySelector<HTMLElement>('.citation-hover-card');
  expect(card).not.toBeNull();
  card.querySelector<HTMLAnchorElement>('[data-citation-goto]').click();
  expect(website.hidden).toBe(false);
});

test('Escape closes a hover card wherever the focus is', () => {
  const container = article();
  const marker = container.querySelector<HTMLAnchorElement>('a[href="#fn:2"]');
  marker.dispatchEvent(new FocusEvent('focus'));
  const card = document.querySelector<HTMLElement>('.citation-hover-card');
  expect(card.getAttribute('role')).toBe('tooltip');
  expect(marker.getAttribute('aria-describedby')).toBe(card.id);

  document.body.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}));
  expect(document.querySelector('.citation-hover-card')).toBeNull();
  expect(marker.hasAttribute('aria-describedby')).toBe(false);
});
