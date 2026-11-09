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
// get. A title that breaks the rule also makes the input invalid, so the browser refuses to
// submit the form and shows the same message (#401). The one exception is the existing subject
// the server prefilled (data-existing-subject): it may predate the rule and is still accepted.
// The server checks the title again either way.
export function initSubjectTitleHint() {
  for (const elHint of document.querySelectorAll<HTMLElement>('[data-subject-title-hint]')) {
    const input = elHint.closest('.field')?.querySelector<HTMLInputElement>('input[name="subject"]');
    if (!input) continue;
    const existing = elHint.getAttribute('data-existing-subject');
    const update = () => {
      let {problem, normalized} = subjectTitleHint(input.value);
      if (existing !== null && input.value.trim() === existing) {
        problem = '';
        normalized = '';
      }
      const message = problem ? elHint.getAttribute(`data-msg-${problem.replaceAll('_', '-')}`) ?? '' : '';
      if (problem) {
        elHint.textContent = message;
      } else if (normalized) {
        elHint.textContent = (elHint.getAttribute('data-msg-normalized') ?? '').replace('%s', () => normalized);
      }
      // a message is required: an empty one would leave the input valid
      input.setCustomValidity(problem ? message || problem : '');
      elHint.classList.toggle('red', Boolean(problem));
      toggleElem(elHint, Boolean(problem || normalized));
    };
    input.addEventListener('input', update);
    update();
  }
}
