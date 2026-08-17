import {mergeVisualEdit, normalizeLine, lcsMatches, type MergeStats} from './markdownThreeWayMerge.ts';

// Stand-in for Toast UI's serializer damage: escape markdown punctuation in every line and
// rewrite `-` bullets to `*`. The real thing is pinned in losslessMarkdown.editor.test.ts.
function serialize(markdown: string): string {
  return markdown.split('\n').map((line) => line
    .replace(/^(\s*)-(\s)/, '$1*$2')
    .replace(/([[\]_.!#+])/g, '\\$1'),
  ).join('\n');
}

const ARTICLE = [
  '# Research program',
  '',
  'See [the program][1] for details.',
  '',
  'A second paragraph.',
  '',
  '- bullet with_underscore',
  '',
  '[1]: https://example.com/ref "Ref title"',
].join('\n');

describe('normalizeLine', () => {
  test('undoes backslash escapes the serializer adds', () => {
    expect(normalizeLine('\\[the program\\]\\[1\\]')).toBe('[the program][1]');
    expect(normalizeLine('snake\\_case\\_word')).toBe('snake_case_word');
    expect(normalizeLine('https://example\\.com/a\\_b')).toBe('https://example.com/a_b');
  });

  test('canonicalizes list markers and whitespace, so bullets and spacing do not fake an edit', () => {
    expect(normalizeLine('* item')).toBe(normalizeLine('- item'));
    expect(normalizeLine('+ item')).toBe(normalizeLine('- item'));
    expect(normalizeLine('1) item')).toBe(normalizeLine('1. item'));
    expect(normalizeLine('a  b   c')).toBe('a b c');
    expect(normalizeLine('trailing   ')).toBe('trailing');
  });

  test('treats setext underlines and table delimiter rows as length-insensitive', () => {
    expect(normalizeLine('======')).toBe(normalizeLine('==='));
    expect(normalizeLine('| --- | --- |')).toBe(normalizeLine('| - | - |'));
    // alignment markers still distinguish delimiter rows
    expect(normalizeLine('| :-- | --: |')).not.toBe(normalizeLine('| --- | --- |'));
    // a rule line never collides with ordinary text
    expect(normalizeLine('---')).not.toBe(normalizeLine('- item'));
  });

  test('keeps genuinely different lines different', () => {
    expect(normalizeLine('hello world')).not.toBe(normalizeLine('hello worlds'));
    expect(normalizeLine('[a](b)')).not.toBe(normalizeLine('[a](c)'));
  });
});

describe('lcsMatches', () => {
  test('matches identical sequences entirely', () => {
    expect(lcsMatches(['a', 'b', 'c'], ['a', 'b', 'c'])).toEqual([[0, 0], [1, 1], [2, 2]]);
  });

  test('skips an inserted element', () => {
    expect(lcsMatches(['a', 'c'], ['a', 'b', 'c'])).toEqual([[0, 0], [1, 2]]);
  });

  test('skips a deleted element', () => {
    expect(lcsMatches(['a', 'b', 'c'], ['a', 'c'])).toEqual([[0, 0], [2, 1]]);
  });

  test('handles no common elements and empty inputs', () => {
    expect(lcsMatches(['a'], ['b'])).toEqual([]);
    expect(lcsMatches([], ['b'])).toEqual([]);
    expect(lcsMatches([], [])).toEqual([]);
  });

  test('refuses inputs that would blow up the O(n*m) table', () => {
    const huge = Array.from({length: 3000}, (_, i) => `line ${i}`);
    const other = Array.from({length: 3000}, (_, i) => `other ${i}`);
    expect(lcsMatches(huge, other)).toBeNull();
  });
});

describe('mergeVisualEdit', () => {
  const base = serialize(ARTICLE);

  test('an unedited serialization yields the pristine source', () => {
    expect(mergeVisualEdit(ARTICLE, base, base)).toBe(ARTICLE);
  });

  test('an edit in the middle only rewrites the edited line', () => {
    const theirs = base.replace('A second paragraph\\.', 'A second paragraph, edited\\.');
    const stats: MergeStats = {};
    const merged = mergeVisualEdit(ARTICLE, base, theirs, stats);
    expect(merged).toBe(ARTICLE.replace('A second paragraph.', 'A second paragraph, edited\\.'));
    // Everything that matters is byte-identical to what the user wrote.
    expect(merged).toContain('See [the program][1] for details.');
    expect(merged).toContain('[1]: https://example.com/ref "Ref title"');
    expect(merged).toContain('- bullet with_underscore');
    expect(stats.fallbackReason).toBeUndefined();
    expect(stats.preservedLines).toBe(4); // the 4 non-blank lines the serializer had mangled
  });

  test('an edit at the very start keeps the rest intact', () => {
    const theirs = base.replace('\\# Research program', '\\# Research program v2');
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    expect(merged).toContain('See [the program][1] for details.');
    // The edited line itself keeps the editor's spelling; that is the point of the merge.
    expect(merged.split('\n')[0]).toBe('\\# Research program v2');
  });

  test('an edit inside the paragraph holding the reference link re-serializes only that line', () => {
    const theirs = base.replace('for details\\.', 'for more details\\.');
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    // That one line legitimately loses its original spelling...
    expect(merged).toContain('\\[the program\\]\\[1\\] for more details\\.');
    // ...but the link *definition* and everything else survive untouched.
    expect(merged).toContain('[1]: https://example.com/ref "Ref title"');
    expect(merged).toContain('- bullet with_underscore');
  });

  test('an added paragraph is inserted without disturbing its neighbours', () => {
    const theirs = `${base}\n\nBrand new \\[paragraph\\]\\.`;
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    expect(merged).toBe(`${ARTICLE}\n\nBrand new \\[paragraph\\]\\.`);
  });

  test('a deleted paragraph removes exactly that paragraph', () => {
    const theirs = base.split('\n').filter((l) => !l.includes('A second paragraph')).join('\n');
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    expect(merged).not.toContain('A second paragraph');
    expect(merged).toContain('See [the program][1] for details.');
    expect(merged).toContain('[1]: https://example.com/ref "Ref title"');
  });

  test('reordered blocks keep their original bytes', () => {
    const lines = base.split('\n');
    // move the "second paragraph" block above the reference-link paragraph
    const theirs = [lines[0], lines[1], lines[4], lines[3], lines[2], ...lines.slice(5)].join('\n');
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    expect(merged).toContain('See [the program][1] for details.');
    expect(merged).toContain('A second paragraph.');
    expect(merged).not.toContain('\\[the program\\]');
  });

  test('a full rewrite degrades to the serialization', () => {
    const theirs = 'Completely \\[different\\] content\\.';
    expect(mergeVisualEdit(ARTICLE, base, theirs)).toBe(theirs);
  });

  test('duplicate lines cannot corrupt the result (ambiguous mapping is still safe)', () => {
    const ours = 'same_line\n\nsame_line\n\nunique_line';
    const b = serialize(ours);
    const theirs = b.replace('unique\\_line', 'changed\\_line');
    const merged = mergeVisualEdit(ours, b, theirs);
    // Whichever of the two identical lines the alignment picks, they are byte-identical.
    expect(merged).toBe('same_line\n\nsame_line\n\nchanged\\_line');
  });

  test('the trailing newline is preserved', () => {
    const ours = 'alpha_one\n\nbeta_two\n';
    const b = serialize(ours);
    const theirs = b.replace('beta\\_two', 'beta\\_two!');
    const merged = mergeVisualEdit(ours, b, theirs);
    expect(merged?.endsWith('\n')).toBe(true);
    expect(merged).toContain('alpha_one');
  });

  test('base64 image widget markdown survives an unrelated edit', () => {
    const img = '![alt](data:image/png;base64,AAAA)';
    const ours = `Intro_text\n\n${img}\n\nOutro [x][1]\n\n[1]: https://e.com/x`;
    const b = serialize(ours);
    const theirs = b.replace('Intro\\_text', 'Intro\\_text edited');
    const merged = mergeVisualEdit(ours, b, theirs);
    expect(merged).toContain(img);
    expect(merged).toContain('Outro [x][1]');
    expect(merged).toContain('[1]: https://e.com/x');
  });

  describe('fallbacks', () => {
    test('reports base-unrecognized when the baseline does not line up with the source', () => {
      const stats: MergeStats = {};
      const unrelatedBase = 'totally\ndifferent\nbaseline\ncontent\nhere\nagain';
      expect(mergeVisualEdit(ARTICLE, unrelatedBase, `${unrelatedBase}\nmore`, stats)).toBeNull();
      expect(stats.fallbackReason).toBe('base-unrecognized');
    });

    test('reports base-unrecognized when there is no baseline at all', () => {
      const stats: MergeStats = {};
      expect(mergeVisualEdit(ARTICLE, '', 'anything', stats)).toBeNull();
      expect(stats.fallbackReason).toBe('base-unrecognized');
    });

    test('reports too-large for documents beyond the LCS budget', () => {
      const stats: MergeStats = {};
      const ours = Array.from({length: 3000}, (_, i) => `line_${i}`).join('\n');
      const b = serialize(ours);
      const theirs = Array.from({length: 3000}, (_, i) => `other_${i}`).join('\n');
      expect(mergeVisualEdit(ours, b, theirs, stats)).toBeNull();
      expect(stats.fallbackReason).toBe('too-large');
    });
  });

  test('the safety invariant holds: the merge only ever respells theirs, never changes it', () => {
    const theirs = base
      .replace('A second paragraph\\.', 'Edited second paragraph\\.')
      .concat('\n\nAppended\\.');
    const merged = mergeVisualEdit(ARTICLE, base, theirs);
    const norm = (s: string) => s.split('\n').map((l) => normalizeLine(l));
    expect(norm(merged)).toEqual(norm(theirs));
  });
});

// The normalization of the two stable inputs, and the base<->ours alignment computed from
// them, are memoized across keystrokes. These tests pin that the caches cannot serve a stale
// or cross-document answer.
describe('memoization', () => {
  test('repeated merges of different documents stay correct (normalization cache)', () => {
    const docs = ['a_one\n\nb_two', 'c_three\n\nd_four', 'e_five\n\nf_six', 'g_seven\n\nh_eight'];
    for (let round = 0; round < 3; round++) {
      for (const ours of docs) {
        const b = serialize(ours);
        const theirs = `${b}\n\nextra\\_line`;
        expect(mergeVisualEdit(ours, b, theirs)).toBe(`${ours}\n\nextra\\_line`);
      }
    }
  });

  // A Visual -> Source -> Visual round trip produces a new baseline for the same source, so
  // the alignment cache must key on both inputs, not just the source.
  test('the same source with a different baseline is re-aligned, not reused', () => {
    const ours = 'x_one\n\ny_two\n\nz_three';
    const baseA = serialize(ours);
    // A baseline whose lines sit at different indices than baseA's.
    const baseB = serialize(`lead_in\n\n${ours}`);

    expect(mergeVisualEdit(ours, baseA, baseA.replace('y\\_two', 'y\\_two!')))
      .toBe('x_one\n\ny\\_two!\n\nz_three');
    expect(mergeVisualEdit(ours, baseB, baseB.replace('y\\_two', 'y\\_two!')))
      .toBe('lead\\_in\n\nx_one\n\ny\\_two!\n\nz_three');
    // ...and back again, in case the second call poisoned the first's entry.
    expect(mergeVisualEdit(ours, baseA, baseA.replace('y\\_two', 'y\\_two?')))
      .toBe('x_one\n\ny\\_two?\n\nz_three');
  });

  test('cycling more source/baseline pairs than the cache holds stays correct', () => {
    const docs = Array.from({length: 8}, (_, i) => `alpha_${i}\n\nbeta_${i}\n\ngamma_${i}`);
    for (let round = 0; round < 3; round++) {
      for (const ours of docs) {
        const b = serialize(ours);
        const theirs = b.replace(/beta\\_(\d)/, 'beta\\_$1 edited');
        expect(mergeVisualEdit(ours, b, theirs)).toBe(ours.replace(/beta_(\d)/, 'beta\\_$1 edited'));
      }
    }
  });
});
