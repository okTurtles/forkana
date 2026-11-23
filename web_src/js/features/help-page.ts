import {registerGlobalEventFunc} from '../modules/observer.ts';

/* #541, the help pages (custom/templates/help/): the reasons' accordion on a phone and the
   content warning example of "What can be reported". */
export function initHelpPage() {
  // on a phone each reason opens and closes on its row (web_src/css/features/help.css)
  registerGlobalEventFunc('click', 'onHelpReasonToggle', (btn: HTMLElement) => {
    setReasonOpen(btn.closest('.help-reason'), btn.getAttribute('aria-expanded') !== 'true');
  });

  // "Show image" reveals the image for this visit only: nothing is remembered, so a reload or a
  // later visit blurs it again. Each button hands the focus to the other, as it hides itself.
  registerGlobalEventFunc('click', 'onHelpContentWarningShow', (btn: HTMLElement) => {
    const warning = btn.closest('.help-cw');
    warning.classList.add('revealed');
    warning.querySelector<HTMLElement>('.help-cw-hide').focus();
  });
  registerGlobalEventFunc('click', 'onHelpContentWarningHide', (btn: HTMLElement) => {
    const warning = btn.closest('.help-cw');
    warning.classList.remove('revealed');
    warning.querySelector<HTMLElement>('.help-cw-warning button').focus();
  });

  // a link to one reason (the report form's, or "Jump to:") opens it on a phone
  const openTarget = () => {
    let id: string;
    try {
      id = decodeURIComponent(window.location.hash.slice(1));
    } catch {
      return; // a malformed hash (e.g. "#%") is the reader's input: it opens nothing
    }
    if (id) setReasonOpen(document.querySelector(`.help-reason[id="${CSS.escape(id)}"]`), true);
  };
  if (document.querySelector('.help-page')) {
    openTarget();
    window.addEventListener('hashchange', openTarget);
  }
}

function setReasonOpen(reason: Element | null, open: boolean) {
  if (!reason) return;
  reason.classList.toggle('open', open);
  reason.querySelector('.help-reason-toggle')?.setAttribute('aria-expanded', String(open));
}
