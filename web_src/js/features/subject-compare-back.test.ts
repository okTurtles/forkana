import {shouldGoBackInHistory} from './subject-compare-back.ts';

const compareUrl = '/subject/Moon%20Landing/compare/alice...bob?selected=bob%2Fmoon-landing';
const subjectPath = '/subject/Moon%20Landing';

describe('shouldGoBackInHistory', () => {
  test('the Navigation API says the previous entry is the subject page', () => {
    expect(shouldGoBackInHistory({
      currentUrl: compareUrl, subjectPath, canGoBack: true, note: null,
      previousEntryUrl: 'http://localhost:3000/subject/Moon Landing?view=bubble&selected=bob%2Fmoon-landing',
    })).toBe(true);
  });

  test('the Navigation API says the previous entry is another page, whatever the note says', () => {
    expect(shouldGoBackInHistory({
      currentUrl: compareUrl, subjectPath, canGoBack: true, note: {compareUrl, subjectPath},
      previousEntryUrl: 'http://localhost:3000/explore/subjects',
    })).toBe(false);
  });

  test('without the Navigation API, or without a url from it (WebKit), the note left by the graph decides', () => {
    expect(shouldGoBackInHistory({currentUrl: compareUrl, subjectPath, canGoBack: true, note: {compareUrl, subjectPath}, previousEntryUrl: undefined})).toBe(true);
    expect(shouldGoBackInHistory({currentUrl: compareUrl, subjectPath, canGoBack: true, note: {compareUrl, subjectPath}, previousEntryUrl: null})).toBe(true);
    expect(shouldGoBackInHistory({currentUrl: compareUrl, subjectPath, canGoBack: true, note: null, previousEntryUrl: null})).toBe(false);
    expect(shouldGoBackInHistory({currentUrl: compareUrl, subjectPath, canGoBack: true, note: null, previousEntryUrl: undefined})).toBe(false);
    expect(shouldGoBackInHistory({
      currentUrl: compareUrl, subjectPath, canGoBack: true, previousEntryUrl: undefined,
      note: {compareUrl: '/subject/Moon%20Landing/compare/alice...carol', subjectPath},
    })).toBe(false);
  });

  test('a page opened in a fresh tab follows the link', () => {
    expect(shouldGoBackInHistory({currentUrl: compareUrl, subjectPath, canGoBack: false, note: {compareUrl, subjectPath}, previousEntryUrl: undefined})).toBe(false);
  });
});
