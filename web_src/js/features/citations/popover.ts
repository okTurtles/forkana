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

// openPopover shows el under rect. onClose runs once, whichever way it closes: Escape (when
// the popover has the focus), a click outside, or close().
export function openPopover(el: HTMLElement, rect: DOMRect, onClose?: () => void): Popover {
  closePopover();
  el.classList.add('citation-popover');
  if (isPhoneLayout()) el.classList.add('citation-popover-sheet');
  document.body.append(el);

  let popover: Popover = null;
  const onDocumentPointer = (e: PointerEvent) => {
    if (!el.contains(e.target as Node)) popover.close();
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      popover.close();
    }
  };
  popover = {
    el,
    close: () => {
      if (current !== popover) return;
      current = null;
      el.remove();
      document.removeEventListener('pointerdown', onDocumentPointer, true);
      el.removeEventListener('keydown', onKey);
      onClose?.();
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
  // a click that opened the popover must not close it
  setTimeout(() => document.addEventListener('pointerdown', onDocumentPointer, true), 0);
  el.addEventListener('keydown', onKey);
  return popover;
}
