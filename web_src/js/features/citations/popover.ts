// A popover anchored at the editor's caret, for the link and cite tools (issue #399). On a
// phone it is a full-screen sheet instead (Figma M3, M5: 6597:45437, 6597:45453).

export type Popover = {
  el: HTMLElement,
  close: () => void,
  reposition: (rect: DOMRect) => void,
};

let current: Popover | null = null;

export function closePopover(): void {
  current?.close();
}

export function isPhoneLayout(): boolean {
  return window.matchMedia('(max-width: 767.98px)').matches;
}

// openPopover shows el under rect. onClose runs once, whichever way it closes: Escape, a
// click outside, or close(). When the popover took the focus, closing it gives the focus back
// to where it was. A popover holding its own fields (or a phone's sheet) is a dialog.
export function openPopover(el: HTMLElement, rect: DOMRect, onClose?: () => void): Popover {
  closePopover();
  el.classList.add('citation-popover');
  const sheet = isPhoneLayout();
  if (sheet) {
    el.classList.add('citation-popover-sheet');
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
  } else if (el.querySelector('input, button') && !el.hasAttribute('role')) {
    el.setAttribute('role', 'dialog');
  }
  const previousFocus = document.activeElement as HTMLElement | null;
  document.body.append(el);

  let popover: Popover = null;
  const onDocumentPointer = (e: PointerEvent) => {
    if (!el.contains(e.target as Node)) popover.close();
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && !e.defaultPrevented) {
      e.preventDefault();
      popover.close();
    }
  };
  // a click that opened the popover must not close it
  const listenTimer = setTimeout(() => document.addEventListener('pointerdown', onDocumentPointer, true), 0);
  popover = {
    el,
    close: () => {
      if (current !== popover) return;
      current = null;
      clearTimeout(listenTimer);
      const hadFocus = el.contains(document.activeElement);
      el.remove();
      document.removeEventListener('pointerdown', onDocumentPointer, true);
      document.removeEventListener('keydown', onKey);
      onClose?.();
      if (hadFocus && document.activeElement === document.body && previousFocus?.isConnected) previousFocus.focus();
    },
    reposition: (r: DOMRect) => {
      if (el.classList.contains('citation-popover-sheet')) return;
      const width = el.offsetWidth;
      const viewport = document.documentElement.clientWidth;
      const left = Math.max(8, Math.min(r.left, viewport - width - 8));
      el.style.left = `${left + window.scrollX}px`;
      el.style.top = `${r.bottom + window.scrollY + 6}px`;
    },
  };
  current = popover;
  popover.reposition(rect);
  // Escape closes it wherever the focus is (the "[[" list keeps the focus in the editor)
  document.addEventListener('keydown', onKey);
  return popover;
}
