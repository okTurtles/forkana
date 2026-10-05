import {toggleElem} from '../utils/dom.ts';

// isNewSubjectTitleValid reports whether a typed title follows the subject title rule, using the
// pattern rendered by the server (modules/subjecttitle.Pattern). An empty value is not flagged.
export function isNewSubjectTitleValid(value: string, pattern: string): boolean {
  if (!value.trim()) return true;
  return new RegExp(`^(?:${pattern})$`, 'u').test(value);
}

// initSubjectTitleHint shows the subject title rule under the subject inputs of the create and
// migrate forms while the typed title breaks it. The hint never blocks submitting: a title that
// breaks the rule is still accepted when it resolves to an existing subject, which only the
// server knows, and the server rejects invalid new subjects with its own error.
export function initSubjectTitleHint() {
  for (const elHint of document.querySelectorAll<HTMLElement>('[data-subject-title-hint]')) {
    const input = elHint.closest('.field')?.querySelector<HTMLInputElement>('input[name="subject"]');
    const pattern = elHint.getAttribute('data-pattern');
    if (!input || !pattern) continue;
    const update = () => toggleElem(elHint, !isNewSubjectTitleValid(input.value, pattern));
    input.addEventListener('input', update);
    update();
  }
}
