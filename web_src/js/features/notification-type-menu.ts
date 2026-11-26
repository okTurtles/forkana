/* The Notifications "Type" menu (custom/templates/user/notification/notification_div.tmpl), a
   <details>: it closes on a click outside it and on Escape, as a dropdown does. The listeners
   are on the document because htmx re-renders #notification_div. */
export function initNotificationTypeMenu() {
  if (!document.querySelector('.notifications-page')) return;
  document.addEventListener('click', (e) => {
    for (const menu of document.querySelectorAll<HTMLDetailsElement>('.notifications-type-menu[open]')) {
      if (!menu.contains(e.target as Node)) menu.open = false;
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    const menu = document.querySelector<HTMLDetailsElement>('.notifications-type-menu[open]');
    if (!menu) return;
    menu.open = false;
    menu.querySelector('summary').focus();
  });
}
