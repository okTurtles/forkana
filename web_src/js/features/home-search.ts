import {GET} from '../modules/fetch.ts';
import {createElementFromHTML, hideElem, showElem} from '../utils/dom.ts';
import {html, htmlEscape, htmlRaw} from '../utils/html.ts';
import {pathEscapeSegments} from '../utils/url.ts';

const {appSubUrl} = window.config;

// The landing page search field (custom/templates/home.tmpl) suggests the subjects matching what
// has been typed so far, so that an existing subject can be opened without a full search first.
const inputSelector = '#home-search-input';
const suggestionsSelector = '#home-search-suggestions';

// How long the typing has to pause before the suggestions are fetched.
const debounceMs = 200;

// Renders a subject name with the part matching the keyword in bold, like the search fields of
// the Figma design do.
export function highlightKeyword(name: string, keyword: string): string {
  const start = keyword ? name.toLowerCase().indexOf(keyword.toLowerCase()) : -1;
  if (start === -1) return htmlEscape(name);
  const end = start + keyword.length;
  return html`${name.slice(0, start)}<b>${name.slice(start, end)}</b>${name.slice(end)}`;
}

class HomeSearch {
  private readonly input: HTMLInputElement;
  private readonly suggestions: HTMLElement;
  private debounceTimer: number | null = null;
  private abortController: AbortController | null = null;
  // Index of the suggestion the keyboard is on, -1 while the typed keyword itself is "selected".
  private activeIndex: number = -1;

  constructor(input: HTMLInputElement, suggestions: HTMLElement) {
    this.input = input;
    this.suggestions = suggestions;
  }

  init(): void {
    this.input.addEventListener('input', () => this.scheduleSearch());
    // Coming back to a field that still holds a keyword should offer the suggestions again.
    this.input.addEventListener('focus', () => this.scheduleSearch());
    this.input.addEventListener('keydown', (e: KeyboardEvent) => this.onKeyDown(e));

    // Clicking anywhere else closes the dropdown, but the click on a suggestion itself has to go
    // through first, so listen for the bubbled event instead of the field losing focus.
    document.addEventListener('click', (e: MouseEvent) => {
      if (!(e.target instanceof Node) || (!this.input.contains(e.target) && !this.suggestions.contains(e.target))) {
        this.close();
      }
    });
  }

  private scheduleSearch(): void {
    if (this.debounceTimer !== null) clearTimeout(this.debounceTimer);
    const keyword = this.input.value.trim();
    if (!keyword) {
      this.close();
      return;
    }
    this.debounceTimer = window.setTimeout(() => this.search(keyword), debounceMs);
  }

  private async search(keyword: string): Promise<void> {
    // Only the most recent keystroke matters, an earlier request that is still on its way would
    // otherwise overwrite its suggestions.
    this.abortController?.abort();
    this.abortController = new AbortController();
    let names: string[];
    try {
      const response = await GET(`${appSubUrl}/explore/subjects/suggestions?q=${encodeURIComponent(keyword)}`, {signal: this.abortController.signal});
      if (!response.ok) throw new Error(`subject suggestions failed with status ${response.status}`);
      names = (await response.json()).subjects ?? [];
    } catch (e) {
      if ((e as Error).name === 'AbortError') return;
      // A failed lookup shouldn't get in the way of the search itself, which is a plain form
      // submit that doesn't need the dropdown.
      console.error(e);
      this.close();
      return;
    }
    // The keyword may have changed again while the request was in flight.
    if (keyword !== this.input.value.trim()) return;
    this.render(names, keyword);
  }

  private render(names: string[], keyword: string): void {
    this.suggestions.replaceChildren(...names.map((name, i) => createElementFromHTML(
      html`<a id="home-search-suggestion-${i}" class="home-search-suggestion" role="option" aria-selected="false" href="${appSubUrl}/subject/${htmlRaw(pathEscapeSegments(name))}">${htmlRaw(highlightKeyword(name, keyword))}</a>`,
    )));
    this.input.removeAttribute('aria-activedescendant');
    this.activeIndex = -1;
    if (!names.length) {
      this.close();
      return;
    }
    showElem(this.suggestions);
    this.input.setAttribute('aria-expanded', 'true');
  }

  private close(): void {
    if (this.debounceTimer !== null) clearTimeout(this.debounceTimer);
    this.debounceTimer = null;
    hideElem(this.suggestions);
    this.input.setAttribute('aria-expanded', 'false');
    this.activeIndex = -1;
  }

  private items(): HTMLAnchorElement[] {
    return Array.from(this.suggestions.querySelectorAll<HTMLAnchorElement>('.home-search-suggestion'));
  }

  private onKeyDown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      this.close();
      return;
    }
    const items = this.items();
    if (!items.length || this.suggestions.classList.contains('tw-hidden')) return;

    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault(); // the arrow keys would otherwise jump to either end of the keyword
      const step = e.key === 'ArrowDown' ? 1 : -1;
      // Stepping past either end goes back to the keyword the user typed, like a browser's own
      // address bar does.
      this.setActive(this.activeIndex + step >= items.length || this.activeIndex + step < -1 ? -1 : this.activeIndex + step);
    } else if (e.key === 'Enter' && this.activeIndex !== -1) {
      e.preventDefault(); // opening the highlighted subject instead of submitting the search
      items[this.activeIndex].click();
    }
  }

  private setActive(index: number): void {
    const items = this.items();
    for (const [i, item] of items.entries()) {
      item.classList.toggle('active', i === index);
      item.setAttribute('aria-selected', String(i === index));
    }
    this.activeIndex = index;
    if (index === -1) {
      this.input.removeAttribute('aria-activedescendant');
    } else {
      this.input.setAttribute('aria-activedescendant', items[index].id);
    }
  }
}

export function initHomeSearch(): void {
  const input = document.querySelector<HTMLInputElement>(inputSelector);
  const suggestions = document.querySelector<HTMLElement>(suggestionsSelector);
  if (!input || !suggestions) return;
  new HomeSearch(input, suggestions).init();
}
