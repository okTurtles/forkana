import {toggleElem} from '../utils/dom.ts';
import {checkSubjectTitle, normalizeSubjectTitle} from './subject-title.ts';

// subjectTitleHint returns what the hint under a subject input says about the typed title:
// the problem when it breaks the subject title rule, or else the normalized title when the
// server would store a new subject under a different spelling ("iPhone" → "IPhone"). An empty
// value is not flagged.
export function subjectTitleHint(value: string): {problem: string, normalized: string} {
  const normalized = normalizeSubjectTitle(value);
  if (!normalized) return {problem: '', normalized: ''};
  const problem = checkSubjectTitle(normalized);
  if (problem) return {problem, normalized: ''};
  return {problem: '', normalized: normalized === value.trim() ? '' : normalized};
}

// initSubjectTitleHint explains the subject title rule under the subject inputs of the create
// and migrate forms while the typed title breaks it, and shows the spelling a new subject would
// get. The hint never blocks submitting: a title that breaks the rule is still accepted when it
// resolves to an existing subject, which only the server knows, and the server rejects invalid
// new subjects with its own error.
export function initSubjectTitleHint() {
  for (const elHint of document.querySelectorAll<HTMLElement>('[data-subject-title-hint]')) {
    const input = elHint.closest('.field')?.querySelector<HTMLInputElement>('input[name="subject"]');
    if (!input) continue;
    const update = () => {
      const {problem, normalized} = subjectTitleHint(input.value);
      if (problem) {
        elHint.textContent = elHint.getAttribute(`data-msg-${problem.replaceAll('_', '-')}`) ?? '';
      } else if (normalized) {
        elHint.textContent = (elHint.getAttribute('data-msg-normalized') ?? '').replace('%s', normalized);
      }
      elHint.classList.toggle('red', Boolean(problem));
      toggleElem(elHint, Boolean(problem || normalized));
    };
    input.addEventListener('input', update);
    update();
  }
}
