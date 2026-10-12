// JavaScript mirror of the subject title rule in modules/subjecttitle/subjecttitle.go, which
// follows Wikipedia's technical restrictions on page titles. The server stays the authority:
// this is only used to explain the rule while typing. Both implementations are tested against
// the shared cases in modules/subjecttitle/testdata/cases.json.

export type SubjectTitleProblem = '' | 'empty' | 'forbidden_char' | 'percent_encoding' | 'html_entity' |
  'tildes' | 'relative_path' | 'leading_colon' | 'too_long';

// MaxBytes in Go: fewer than 256 bytes in UTF-8, as on Wikipedia
export const subjectTitleMaxBytes = 255;

const forbiddenCharRe = /[#<>[\]|{}\u0000-\u001F\u007F\ufffd\ufffe\uffff]/u;
const loneSurrogateRe = /[\uD800-\uDFFF]/u; // with the "u" flag, only unpaired surrogates match
const percentEncodingRe = /%[0-9A-Fa-f]{2}/u;
const htmlEntityRe = /&[A-Za-z0-9\u{80}-\u{10FFFF}]+;|&#[0-9]+;|&#x[0-9A-Fa-f]+;/u;
const spacesRe = /[ _\u00a0\u1680\u180e\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/gu;
const directionalRe = /[\u200e\u200f\u202a-\u202e]/gu;

// normalizeSubjectTitle mirrors subjecttitle.Normalize: NFC, directional marks removed,
// underscores and Unicode spaces turned into single spaces, surrounding spaces stripped and the
// first character uppercased when it has a single-character uppercase form.
export function normalizeSubjectTitle(title: string): string {
  let s = title.normalize('NFC').replace(directionalRe, '').replace(spacesRe, ' ');
  s = s.replace(/^ +| +$/g, '');
  if (s) {
    const first = String.fromCodePoint(s.codePointAt(0));
    const upper = first.toUpperCase();
    if (Array.from(upper).length === 1) s = upper + s.slice(first.length);
  }
  return s.normalize('NFC');
}

function isRelativePath(s: string): boolean {
  return s === '.' || s === '..' || s.startsWith('./') || s.startsWith('../') ||
    s.includes('/./') || s.includes('/../') || s.endsWith('/.') || s.endsWith('/..');
}

// checkSubjectTitle mirrors subjecttitle.Check on an already normalized title.
export function checkSubjectTitle(s: string): SubjectTitleProblem {
  if (s === '') return 'empty';
  if (loneSurrogateRe.test(s)) return 'forbidden_char'; // invalid UTF-8 in Go
  if (htmlEntityRe.test(s)) return 'html_entity';
  if (percentEncodingRe.test(s)) return 'percent_encoding';
  if (forbiddenCharRe.test(s)) return 'forbidden_char';
  if (s.includes('~~~')) return 'tildes';
  if (isRelativePath(s)) return 'relative_path';
  if (s.startsWith(':')) return 'leading_colon';
  if (new TextEncoder().encode(s).length > subjectTitleMaxBytes) return 'too_long';
  return '';
}
