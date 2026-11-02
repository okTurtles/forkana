import {isRelativeImageSrc, resolveRelativeSrc} from './editorImageSrc.ts';

const RAW = 'http://localhost:3000/user2/repo1/raw/branch/main/README.md';

test('isRelativeImageSrc', () => {
  for (const src of ['pic.png', './img/a.png', '../b.png', 'images/a b.png']) {
    expect(isRelativeImageSrc(src)).toBe(true);
  }
  for (const src of ['', '/assets/a.png', '//host/a.png', '#x', 'https://host/a.png', 'http://host/a.png', 'data:image/png;base64,AA==', 'blob:http://host/1']) {
    expect(isRelativeImageSrc(src)).toBe(false);
  }
});

test('resolveRelativeSrc', () => {
  expect(resolveRelativeSrc('pic.png', RAW)).toBe('http://localhost:3000/user2/repo1/raw/branch/main/pic.png');
  expect(resolveRelativeSrc('./img/a.png', RAW)).toBe('http://localhost:3000/user2/repo1/raw/branch/main/img/a.png');
  expect(resolveRelativeSrc('https://host/a.png', RAW)).toBe('https://host/a.png');
  expect(resolveRelativeSrc('/assets/a.png', RAW)).toBe('/assets/a.png');
  expect(resolveRelativeSrc('pic.png', '')).toBe('pic.png');
});
