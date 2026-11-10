import {registerGlobalEventFunc} from '../modules/observer.ts';

// the phone layout of the Article view (web_src/css/features/article-view.css)
const phoneQuery = '(max-width: 767.98px)';

/* #165: the Article view's Watch button (custom/templates/repo/watch_button.tmpl). On a
   wider screen it toggles the watch; on a phone, where it is an icon, it opens the "Watch
   menu" (figma 7368:416644), a sheet whose choices toggle it. Each answer replaces the whole
   control, so the menu is closed with it. */
export function initArticleWatchMenu() {
  registerGlobalEventFunc('click', 'onArticleWatchClick', (btn: HTMLElement, e: Event) => {
    if (!window.matchMedia(phoneQuery).matches) return;
    const menu = btn.closest('.article-watch-control')?.querySelector<HTMLDialogElement>('dialog.article-watch-menu');
    if (!menu) return;
    e.preventDefault();
    menu.showModal();
  });

  registerGlobalEventFunc('click', 'onArticleWatchMenuClose', (btn: HTMLElement) => {
    btn.closest<HTMLDialogElement>('dialog')?.close();
  });

  // a press outside the sheet (on the dialog's backdrop, which is the dialog itself) closes it
  document.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;
    if (target instanceof HTMLDialogElement && target.matches('.article-watch-menu') && target.open) {
      target.close();
    }
  });

  // The answer replaces the control, focused button included: when the focus was in it (on
  // Watch, or on a choice of the menu), give it back to the new Watch button rather than
  // letting it fall to the page.
  let refocusWatch = false;
  document.addEventListener('htmx:beforeRequest', (e) => {
    const elt = (e as CustomEvent<{elt?: Element}>).detail?.elt;
    refocusWatch = Boolean(elt?.closest('.article-watch-control') && document.activeElement?.closest('.article-watch-control'));
  });
  document.addEventListener('htmx:afterSettle', () => {
    if (!refocusWatch) return;
    refocusWatch = false;
    document.querySelector<HTMLElement>('.article-watch-control [data-role="article-watch"]')?.focus();
  });
}
