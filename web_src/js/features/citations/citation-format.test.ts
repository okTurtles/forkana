import {
  classifyCitation,
  detectSourceType,
  escapeMarkdownText,
  formatCitation,
  formatCitationDate,
  isNewsHost,
  missingRequiredFields,
  normalizeDOI,
  normalizeISBN,
} from './citation-format.ts';

const today = '1 October 2026';

test('detectSourceType', () => {
  expect(detectSourceType('10.1126/science.167.3918.449')).toBe('journal');
  expect(detectSourceType('https://doi.org/10.1126/science.167.3918.449')).toBe('journal');
  expect(detectSourceType('978-0-670-81446-6')).toBe('book');
  expect(detectSourceType('https://www.nytimes.com/2023/08/23/world/asia/india-moon-landing.html')).toBe('news');
  expect(detectSourceType('https://edition.cnn.com/x')).toBe('news');
  expect(detectSourceType('https://nssdc.gsfc.nasa.gov/nmc/spacecraft/display.action?id=1959-014A')).toBe('website');
  expect(detectSourceType('Apollo')).toBeNull();
});

test('normalizeDOI and normalizeISBN', () => {
  expect(normalizeDOI('doi:10.1038/nature12373.')).toBe('10.1038/nature12373');
  expect(normalizeDOI('https://example.com/10.1038/x')).toBe('');
  expect(normalizeISBN('ISBN 978-0-670-81446-6')).toBe('9780670814466');
  expect(normalizeISBN('0-670-81446-6')).toBe('0670814466');
  expect(normalizeISBN('978-0-670-81446-7')).toBe('');
  expect(isNewsHost('notnytimes.com')).toBe(false);
});

test('formatCitationDate', () => {
  expect(formatCitationDate('2023-08-23')).toBe('23 August 2023');
  expect(formatCitationDate('2023-08-23T13:05:41Z')).toBe('23 August 2023');
  expect(formatCitationDate('2024-01')).toBe('January 2024');
  expect(formatCitationDate('Spring 1970')).toBe('Spring 1970');
});

test('formatCitation: the forms of the design', () => {
  // "Add citation – Website" preview (Figma 6597:44669)
  expect(formatCitation('website', {
    url: 'https://nssdc.gsfc.nasa.gov/nmc/spacecraft/display.action?id=1959-014A',
    title: 'Luna 2',
    site: 'NASA–NSSDC',
  }, today)).toBe('["Luna 2"](https://nssdc.gsfc.nasa.gov/nmc/spacecraft/display.action?id=1959-014A). *NASA–NSSDC*. Retrieved 1 October 2026.');

  // News (6597:44847)
  expect(formatCitation('news', {
    url: 'https://www.nytimes.com/2023/08/23/world/asia/india-moon-landing.html',
    title: 'India Moon Landing: In Latest Moon Race, India Lands First in Southern Polar Region',
    site: 'The New York Times',
    published: '2023-08-23',
    authors: 'Kumar, Hari; Travelli, Alex; Mashal, Mujib; Chang, Kenneth',
  }, '25 August 2023')).toBe('Kumar, Hari; Travelli, Alex; Mashal, Mujib; Chang, Kenneth (23 August 2023). ["India Moon Landing: In Latest Moon Race, India Lands First in Southern Polar Region"](https://www.nytimes.com/2023/08/23/world/asia/india-moon-landing.html). *The New York Times*. Retrieved 25 August 2023.');

  // News without an author keeps the date after the publication
  expect(formatCitation('news', {
    url: 'https://apnews.com/x', title: "Chinese spacecraft makes first landing on moon's far side", site: 'AP News', published: '2019-01-03',
  }, '3 January 2019')).toBe('["Chinese spacecraft makes first landing on moon\'s far side"](https://apnews.com/x). *AP News*. 3 January 2019. Retrieved 3 January 2019.');

  // Journal (6597:45025)
  expect(formatCitation('journal', {
    doi: '10.1126/science.167.3918.449',
    title: 'Preliminary examination of lunar samples from Apollo 11',
    authors: 'Lunar Sample Preliminary Examination Team',
    journal: 'Science',
    published: '1970',
    volumeIssue: '167 / 3918',
    pages: '449-458',
  }, today)).toBe('Lunar Sample Preliminary Examination Team (1970). "Preliminary examination of lunar samples from Apollo 11". *Science*. 167 (3918): 449–458. doi:[10.1126/science.167.3918.449](https://doi.org/10.1126/science.167.3918.449).');

  // Book (6597:45203)
  expect(formatCitation('book', {
    isbn: '978-0-670-81446-6', title: 'A Man on the Moon', authors: 'Chaikin, Andrew', published: '1994', publisher: 'Viking', edition: '1st',
  }, today)).toBe('Chaikin, Andrew (1994). *A Man on the Moon* (1st ed.). Viking. ISBN 978-0-670-81446-6.');

  // Other (6597:45381), with where in the source
  expect(formatCitation('other', {
    citation: 'Armstrong, Neil. Oral history interview, NASA Johnson Space Center Oral History Project, Houston, 19 September 2001.',
    location: 'p. 42',
  }, today)).toBe('Armstrong, Neil. Oral history interview, NASA Johnson Space Center Oral History Project, Houston, 19 September 2001. p. 42.');
});

test('formatCitation escapes Markdown in the fields', () => {
  expect(escapeMarkdownText('a *b* [c]\nd')).toBe('a \\*b\\* \\[c\\] d');
  // "$" is math, "~" strikethrough, "&" an entity (review #27)
  expect(escapeMarkdownText('$5 to $10 ~~ AT&T&copy;')).toBe('\\$5 to \\$10 \\~\\~ AT\\&T\\&copy;');
  // a pasted "ISBN …" is not written twice (review #13); a DOI with parentheses stays one link (#26)
  expect(formatCitation('book', {isbn: 'ISBN: 978-0-670-81446-6', title: 'T', authors: 'A'}, today)).toBe('A. *T*. ISBN 978-0-670-81446-6.');
  expect(formatCitation('journal', {doi: '10.1016/S0140-6736(20)30183-5'}, today)).toBe('doi:[10.1016/S0140-6736(20)30183-5](<https://doi.org/10.1016/S0140-6736(20)30183-5>).');
  expect(formatCitation('website', {url: 'https://example.com/a (b)', title: 'T [x]'}, today))
    .toBe('["T \\[x\\]"](<https://example.com/a (b)>). Retrieved 1 October 2026.');
});

test('formatCitation keeps the edition (review 2 #7)', () => {
  const book = (edition: string) => formatCitation('book', {title: 'T', authors: 'A', edition}, today);
  expect(book('Revised')).toBe('A. *T* (Revised ed.).');
  expect(book('Illustrated')).toBe('A. *T* (Illustrated ed.).');
  expect(book('2nd edition')).toBe('A. *T* (2nd ed.).');
  expect(book('2nd ed.')).toBe('A. *T* (2nd ed.).');
  expect(book('3rd Ed')).toBe('A. *T* (3rd ed.).');
});

test('an "Other" citation does not start a list or a heading (review 2 #19)', () => {
  expect(formatCitation('other', {citation: '1984. Orwell estate letters'}, today)).toBe('1984\\. Orwell estate letters.');
  expect(formatCitation('other', {citation: '- Interview notes'}, today)).toBe('\\- Interview notes.');
  expect(formatCitation('other', {citation: '# 12 Report'}, today)).toBe('\\# 12 Report.');
  expect(formatCitation('other', {citation: '1984 was a year'}, today)).toBe('1984 was a year.');
});

test('missingRequiredFields', () => {
  expect(missingRequiredFields('website', {})).toEqual(['url']);
  expect(missingRequiredFields('news', {url: 'https://apnews.com/x'})).toEqual(['site']);
  expect(missingRequiredFields('book', {title: 'T', authors: 'A'})).toEqual([]);
});

function citationElement(html: string): HTMLElement {
  const p = document.createElement('p');
  p.innerHTML = html;
  return p;
}

test('classifyCitation reads back what formatCitation wrote', () => {
  expect(classifyCitation(citationElement('<a href="https://nssdc.gsfc.nasa.gov/x">"Luna 2"</a>. <em>NASA–NSSDC</em>. Retrieved 1 October 2026.')))
    .toEqual({type: 'website', publisher: 'NASA–NSSDC', sourceURL: 'https://nssdc.gsfc.nasa.gov/x', archived: false});
  expect(classifyCitation(citationElement('<a href="https://www.theguardian.com/x">"India lands"</a>. <em>The Guardian</em>. Archived from <a href="https://web.archive.org/x">the original</a>.')))
    .toEqual({type: 'news', publisher: 'The Guardian', sourceURL: 'https://www.theguardian.com/x', archived: true});
  expect(classifyCitation(citationElement('Team (1970). "Preliminary". <em>Science</em>. 167 (3918): 449–458. doi:<a href="https://doi.org/10.1126/x">10.1126/x</a>.')).type)
    .toBe('journal');
  expect(classifyCitation(citationElement('Chaikin, Andrew (1994). <em>A Man on the Moon</em> (1st ed.). Viking. ISBN 978-0-670-81446-6.')))
    .toEqual({type: 'book', publisher: 'Viking', sourceURL: '', archived: false});
  // the title or an initial is not the publisher (review 2 #8)
  expect(classifyCitation(citationElement('Sagan, C. (1980). <em>Cosmos</em>. ISBN 978-0-394-50294-6.')).publisher).toBe('');
  expect(classifyCitation(citationElement('Kubrick, S. (1964). <em>Dr. Strangelove</em>. ISBN 978-0-394-50294-6.')).publisher).toBe('');
  expect(classifyCitation(citationElement('<a href="https://archive.org/x"><em>Dr. Strangelove</em></a> (2nd ed.). Univ. Press. 1964. ISBN 978-0-394-50294-6.')).publisher)
    .toBe('Univ. Press');
  expect(classifyCitation(citationElement('<em>T</em>. 1964. ISBN 978-0-394-50294-6.')).publisher).toBe('');
  expect(classifyCitation(citationElement('Armstrong, Neil. Oral history interview.')))
    .toEqual({type: 'other', publisher: '', sourceURL: '', archived: false});
});

test('citationPreviewHTML', async () => {
  const {citationPreviewHTML} = await import('./cite-tool.ts');
  expect(citationPreviewHTML('doi:[10.1016/S0140-6736(20)30183-5](<https://doi.org/10.1016/S0140-6736(20)30183-5>).'))
    .toBe('doi:<a class="external-link" href="https://doi.org/10.1016/S0140-6736(20)30183-5" target="_blank" rel="noopener">10.1016/S0140-6736(20)30183-5</a>.');
  expect(citationPreviewHTML('A. <https://x.org/?a=1&b=2>'))
    .toBe('A. <a class="external-link" href="https://x.org/?a=1&amp;b=2" target="_blank" rel="noopener">https://x.org/?a=1&amp;b=2</a>');
  expect(citationPreviewHTML('*Pub* \\*not\\*')).toBe('<em>Pub</em> *not*');
});
