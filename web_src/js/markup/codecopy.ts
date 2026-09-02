import {svg} from '../svg.ts';
import {queryElems} from '../utils/dom.ts';

export function makeCodeCopyButton(): HTMLButtonElement {
  const button = document.createElement('button');
  button.classList.add('code-copy', 'ui', 'button');
  button.innerHTML = svg('octicon-copy');
  return button;
}

export function initMarkupCodeCopy(elMarkup: HTMLElement): void {
  // .markup .code-block code
  queryElems(elMarkup, '.code-block code', (el) => {
    if (!el.textContent) return;
    // we only want to use `.code-block-container` if it exists, no matter `.code-block` exists or not.
    const btnContainer = el.closest('.code-block-container') ?? el.closest('.code-block');
    // this can run again for content the observer visits twice; the mermaid renderer moves the
    // button into its own block inside the container, so search the whole container
    if (btnContainer.querySelector('.code-copy')) return;
    const btn = makeCodeCopyButton();
    // remove final trailing newline introduced during HTML rendering
    btn.setAttribute('data-clipboard-text', el.textContent.replace(/\r?\n$/, ''));
    btnContainer.append(btn);
  });
}
