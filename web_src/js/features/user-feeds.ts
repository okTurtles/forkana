// Filter panel on the /feeds page: toggle the checkbox dropdown and reload
// the page with the selected filters applied.
export function initUserFeedsFilter(): void {
  const toggle = document.querySelector('#feeds-filter-toggle');
  const panel = document.querySelector('#feeds-filter-panel');
  if (!toggle || !panel) return;

  toggle.addEventListener('click', (e: Event) => {
    e.stopPropagation();
    const isHidden = panel.classList.toggle('tw-hidden');
    toggle.setAttribute('aria-expanded', String(!isHidden));
  });

  document.addEventListener('click', (e: MouseEvent) => {
    if (!panel.classList.contains('tw-hidden') && !panel.contains(e.target as Node) && !toggle.contains(e.target as Node)) {
      panel.classList.add('tw-hidden');
      toggle.setAttribute('aria-expanded', 'false');
    }
  });

  for (const cb of panel.querySelectorAll<HTMLInputElement>('.feeds-filter-checkbox')) {
    cb.addEventListener('change', () => {
      const params = new URLSearchParams(window.location.search);
      params.delete('filter');
      params.delete('page');
      for (const checked of panel.querySelectorAll<HTMLInputElement>('.feeds-filter-checkbox:checked')) {
        params.append('filter', checked.value);
      }
      window.location.href = `${window.location.pathname}?${params.toString()}`;
    });
  }
}
