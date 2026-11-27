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
    setRevealed(warning, true);
    warning.querySelector<HTMLElement>('.help-cw-hide').focus();
  });
  registerGlobalEventFunc('click', 'onHelpContentWarningHide', (btn: HTMLElement) => {
    const warning = btn.closest('.help-cw');
    setRevealed(warning, false);
    warning.querySelector<HTMLElement>('.help-cw-show').focus();
  });

  // a link to one reason (the report form's, or "Jump to:") opens it on a phone
  const openTarget = () => {
    let id: string;
    try {
      id = decodeURIComponent(window.location.hash.slice(1));
    } catch {
      return; // a malformed hash (e.g. "#%") is the reader's input: it opens nothing
    }
    if (id) setReasonOpen(document.querySelector(`#${CSS.escape(id)}`)?.closest('.help-reason'), true);
  };
  if (document.querySelector('.help-page')) {
    openTarget();
    window.addEventListener('hashchange', openTarget);
    // Back and Forward may restore this page as it was left (the bfcache, kept on purpose by
    // initPageShowBfcacheReload): blur the images again as the reader leaves, so coming back
    // hides them as a reload does
    window.addEventListener('pagehide', () => {
      for (const warning of document.querySelectorAll('.help-cw.revealed')) setRevealed(warning, false);
    });
  }
}

// the image is hidden from screen readers too while blurred (content_warning_example.tmpl)
function setRevealed(warning: Element, revealed: boolean) {
  warning.classList.toggle('revealed', revealed);
  const image = warning.querySelector('.help-cw-image');
  if (revealed) image.removeAttribute('aria-hidden');
  else image.setAttribute('aria-hidden', 'true'); // not toggleAttribute: an empty aria-hidden is ignored
}

function setReasonOpen(reason: Element | null, open: boolean) {
  if (!reason) return;
  reason.classList.toggle('open', open);
  reason.querySelector('.help-reason-toggle')?.setAttribute('aria-expanded', String(open));
}
