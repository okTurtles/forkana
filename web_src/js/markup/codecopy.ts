import {svg} from '../svg.ts';
import {queryElems} from '../utils/dom.ts';

export function makeCodeCopyButton(): HTMLButtonElement {
  const button = document.createElement('button');
  button.classList.add('code-copy', 'ui', 'button');
  button.innerHTML = svg('octicon-copy');
  return button;
}

// we only want to use `.code-block-container` if it exists, no matter `.code-block` exists or not.
export function findCodeCopyButtonContainer(el: Element): HTMLElement {
  return el.closest<HTMLElement>('.code-block-container') ?? el.closest<HTMLElement>('.code-block');
}

// remove final trailing newline introduced during HTML rendering
export function normalizeCodeCopyText(text: string): string {
  return text.replace(/\r?\n$/, '');
}

export function initMarkupCodeCopy(elMarkup: HTMLElement): void {
  // .markup .code-block code
  queryElems(elMarkup, '.code-block code', (el) => {
    if (!el.textContent) return;
    const btnContainer = findCodeCopyButtonContainer(el);
    // this can run again for content the observer visits twice; the mermaid renderer moves the
    // button into its own block inside the container, so search the whole container
    if (btnContainer.querySelector('.code-copy')) return;
    const btn = makeCodeCopyButton();
    btn.setAttribute('data-clipboard-text', normalizeCodeCopyText(el.textContent));
    btnContainer.append(btn);
  });
}
