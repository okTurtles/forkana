import {hideElem, showElem} from '../utils/dom.ts';

const invalidClass = 'home-search-input-invalid';

// The landing page search field (custom/templates/home.tmpl) submits to the explore page, where an
// empty keyword just lists everything. Coming from the landing page that is never what was meant,
// so a blank search is rejected here with an inline message instead of being sent off.
export function initHomeSearch() {
  const searchForm = document.querySelector<HTMLFormElement>('#home-search-form');
  if (!searchForm) return;

  const searchInput = searchForm.querySelector<HTMLInputElement>('input[name="q"]');
  const errorMsg = document.querySelector<HTMLElement>('#home-search-error');
  if (!searchInput || !errorMsg) return;

  const clearError = () => {
    searchInput.classList.remove(invalidClass);
    searchInput.removeAttribute('aria-invalid');
    hideElem(errorMsg);
  };

  searchForm.addEventListener('submit', (e) => {
    if (searchInput.value.trim()) {
      clearError();
      return;
    }
    e.preventDefault();
    searchInput.classList.add(invalidClass);
    searchInput.setAttribute('aria-invalid', 'true');
    showElem(errorMsg);
    searchInput.focus();
  });

  // Typing (or clearing the field) dismisses the message again, so it only ever describes the last
  // submit attempt.
  searchInput.addEventListener('input', clearError);
}
