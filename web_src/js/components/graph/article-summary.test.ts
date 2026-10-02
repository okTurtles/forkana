import {extractArticleSummary} from './article-summary.ts';

describe('extractArticleSummary', () => {
  test('strips front matter, skips heading and fence, unwraps links and emphasis', () => {
    const md = '---\ntitle: Physics\n---\n\n# Physics\n\n```math\nE=mc^2\n```\n\n**Physics** is the study of [matter](https://x.example).';
    expect(extractArticleSummary(md)).toBe('Physics is the study of matter.');
  });

  test('typical wiki2md article: front matter, title heading, first paragraph', () => {
    const md = '---\ntitle: Mars\nsource: wikipedia\n---\n# Mars\n\nMars is the fourth planet from the Sun.\n\nSecond paragraph.';
    expect(extractArticleSummary(md)).toBe('Mars is the fourth planet from the Sun.');
  });

  test('joins hard-wrapped lines of one paragraph and stops at the blank line', () => {
    const md = 'First line\nsecond line.\n\nNot included.';
    expect(extractArticleSummary(md)).toBe('First line second line.');
  });

  test('skips images, tables and HTML before the paragraph', () => {
    const md = '![logo](x.png)\n\n| a | b |\n|---|---|\n\n<div>x</div>\n\nThe text.';
    expect(extractArticleSummary(md)).toBe('The text.');
  });

  test('CRLF and BOM input', () => {
    const md = '﻿---\r\ntitle: X\r\n---\r\n\r\nHello world.\r\n';
    expect(extractArticleSummary(md)).toBe('Hello world.');
  });

  test('a leading thematic break is not mistaken for the summary', () => {
    expect(extractArticleSummary('---\n\nReal text.')).toBe('Real text.');
    expect(extractArticleSummary('* * *\n\nReal text.')).toBe('Real text.');
  });

  test('unclosed front matter does not leak the delimiter', () => {
    const md = '---\ntitle: X\n';
    expect(extractArticleSummary(md)).toBe('title: X');
  });

  test('empty and whitespace-only input', () => {
    expect(extractArticleSummary('')).toBe('');
    expect(extractArticleSummary('\n\n  \n')).toBe('');
  });

  test('clamps to 400 characters with an ellipsis', () => {
    const long = 'word '.repeat(200).trim();
    const out = extractArticleSummary(long);
    expect(out.length).toBeLessThanOrEqual(400);
    expect(out.endsWith('…')).toBe(true);
  });
});
