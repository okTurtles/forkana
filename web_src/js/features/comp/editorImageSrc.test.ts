import {isRelativeImageSrc, resolveRelativeSrc} from './editorImageSrc.ts';

const FILE_URL = 'http://localhost:3000/user2/repo1/media/branch/main/README.md';

test('isRelativeImageSrc', () => {
  for (const src of ['pic.png', './img/a.png', '../b.png', 'images/a b.png']) {
    expect(isRelativeImageSrc(src)).toBe(true);
  }
  for (const src of ['', '/assets/a.png', '//host/a.png', '#x', 'https://host/a.png', 'http://host/a.png', 'data:image/png;base64,AA==', 'blob:http://host/1']) {
    expect(isRelativeImageSrc(src)).toBe(false);
  }
});

test('resolveRelativeSrc', () => {
  expect(resolveRelativeSrc('pic.png', FILE_URL)).toBe('http://localhost:3000/user2/repo1/media/branch/main/pic.png');
  expect(resolveRelativeSrc('./img/a.png', FILE_URL)).toBe('http://localhost:3000/user2/repo1/media/branch/main/img/a.png');
  expect(resolveRelativeSrc('https://host/a.png', FILE_URL)).toBe('https://host/a.png');
  expect(resolveRelativeSrc('/assets/a.png', FILE_URL)).toBe('/assets/a.png');
  expect(resolveRelativeSrc('pic.png', '')).toBe('pic.png');
});
