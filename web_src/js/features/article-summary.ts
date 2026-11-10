// The article's Summary field (#475, shared/repo/summary_field.tmpl): required, so an article
// is never saved without one. The field carries the "required" attribute, which the browser
// checks on a native submit (the first-article editor); a summary of blanks only is made
// invalid too, and the Edit tab, whose buttons submit the form by script, asks
// checkArticleSummary() before submitting.

const summarySelector = 'textarea.article-summary-box[name="summary"]';

function updateValidity(field: HTMLTextAreaElement) {
  const blank = field.value.trim() === '';
  field.setCustomValidity(blank ? (field.getAttribute('data-required-message') || '') : '');
  if (!blank) field.removeAttribute('aria-invalid');
}

export function initArticleSummary() {
  for (const field of document.querySelectorAll<HTMLTextAreaElement>(summarySelector)) {
    if (field.hasAttribute('data-article-summary-initialized')) continue;
    field.setAttribute('data-article-summary-initialized', '');
    updateValidity(field);
    field.addEventListener('input', () => updateValidity(field));
    // a native submit that the browser blocks: mark the field as the one to fix
    field.addEventListener('invalid', () => field.setAttribute('aria-invalid', 'true'));
  }
}

// checkArticleSummary reports whether the summary can be submitted. When it cannot, the field
// is marked, focused and the browser shows why. Without a summary field there is nothing to check.
export function checkArticleSummary(): boolean {
  const field = document.querySelector<HTMLTextAreaElement>(summarySelector);
  if (!field) return true;
  updateValidity(field);
  if (field.checkValidity()) return true;
  field.setAttribute('aria-invalid', 'true');
  field.focus();
  field.reportValidity();
  return false;
}
