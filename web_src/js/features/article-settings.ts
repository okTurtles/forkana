import {fomanticQuery} from '../modules/fomantic/base.ts';
import {addDelegatedEventListener} from '../utils/dom.ts';

// Opens the Article settings modals (transfer/archive/delete). The modal
// content and submission handling are not wired up yet.
export function initArticleSettings(): void {
  if (!document.querySelector('#article-settings-general')) return;

  addDelegatedEventListener(document, 'click', '[data-article-settings-modal]', (el: HTMLElement, e: MouseEvent) => {
    e.preventDefault();
    const modal = document.querySelector<HTMLElement>(el.getAttribute('data-article-settings-modal'));
    if (!modal) return;
    fomanticQuery(modal).modal('show');
  });
}
