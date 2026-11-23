import {registerGlobalEventFunc} from '../modules/observer.ts';

/* The account settings menu on a phone (custom/templates/user/settings/navbar.tmpl): one row,
   "Settings · Security", that opens and closes the list in place. On a desktop the row is
   hidden and the list always shows (web_src/css/features/user-settings.css). */
export function initUserSettingsMenu() {
  registerGlobalEventFunc('click', 'onSettingsMenuToggle', (btn: HTMLElement) => {
    const open = btn.getAttribute('aria-expanded') !== 'true';
    btn.setAttribute('aria-expanded', String(open));
    btn.closest('.flex-container-nav').classList.toggle('open', open);
  });
}
