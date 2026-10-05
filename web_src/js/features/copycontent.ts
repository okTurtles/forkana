import {clippie} from 'clippie';
import {showTemporaryTooltip} from '../modules/tippy.ts';
import {convertImage} from '../utils.ts';
import {GET} from '../modules/fetch.ts';
import {registerGlobalEventFunc} from '../modules/observer.ts';

const {i18n} = window.config;

export function initCopyContent() {
  registerGlobalEventFunc('click', 'onCopyContentButtonClick', async (btn: HTMLElement) => {
    if (btn.classList.contains('disabled') || btn.classList.contains('is-loading')) return;
    const rawFileLink = btn.getAttribute('data-raw-file-link');

    let content, isRasterImage = false;

    // when "data-raw-link" is present, we perform a fetch. this is either because
    // the text to copy is not in the DOM, or it is an image that should be
    // fetched to copy in full resolution
    if (rawFileLink) {
      btn.classList.add('is-loading', 'loading-icon-2px');
      try {
        const res = await GET(rawFileLink, {credentials: 'include', redirect: 'follow'});
        const contentType = res.headers.get('content-type');

        if (contentType.startsWith('image/') && !contentType.startsWith('image/svg')) {
          isRasterImage = true;
          content = await res.blob();
        } else {
          content = await res.text();
        }
      } catch {
        return showTemporaryTooltip(btn, i18n.copy_error);
      } finally {
        btn.classList.remove('is-loading', 'loading-icon-2px');
      }
    } else { // text, read from DOM
      const lineEls = document.querySelectorAll('.file-view .lines-code');
      content = Array.from(lineEls, (el) => el.textContent).join('');
    }

    // try copy original first, if that fails, and it's an image, convert it to png
    const success = await clippie(content);
    if (success) {
      showTemporaryTooltip(btn, i18n.copy_success);
    } else {
      if (isRasterImage) {
        const success = await clippie(await convertImage(content as Blob, 'image/png'));
        showTemporaryTooltip(btn, success ? i18n.copy_success : i18n.copy_error);
      } else {
        showTemporaryTooltip(btn, i18n.copy_error);
      }
    }
  });
}

export function initCompareModeToggle() {
  /* The bubble view owns compare mode; the button only asks for it. Its look
     follows the state the graph reports below, so the two cannot disagree. */
  registerGlobalEventFunc('click', 'onCompareModeToggle', () => {
    window.dispatchEvent(new CustomEvent('repo:compare-mode-toggle'));
  });

  /* #421 item 1 (figma 641:61763 / 641:61930): while compare mode is on the
     button is filled with the brand colour and reads "Compare on"; on a subject
     with a single article it is shown unavailable, with figma's tooltip (6661:52942). */
  window.addEventListener('repo:compare-mode-state', (event: Event) => {
    const {on, available} = (event as CustomEvent<{on: boolean, available: boolean}>).detail;
    const btn = document.querySelector<HTMLElement>('#compare-mode-button');
    if (!btn) return;
    const label = on ? btn.getAttribute('data-label-on') : btn.getAttribute('data-label-off');
    btn.classList.toggle('primary', on);
    btn.classList.toggle('is-unavailable', !available && !on);
    btn.setAttribute('aria-pressed', on ? 'true' : 'false');
    if (label) {
      btn.setAttribute('aria-label', label);
      const text = btn.querySelector('[data-role="compare-label"]');
      if (text) text.textContent = label;
    }
    if (!available && !on) {
      btn.setAttribute('data-tooltip-content', btn.getAttribute('data-unavailable') || '');
      btn.setAttribute('aria-disabled', 'true');
    } else {
      btn.removeAttribute('data-tooltip-content');
      btn.removeAttribute('aria-disabled');
    }
  });
}
