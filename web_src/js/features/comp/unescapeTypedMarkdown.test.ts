import {unescapeLine, unescapeTypedMarkdown} from './unescapeTypedMarkdown.ts';

test('unescapeLine removes serializer escapes before ASCII punctuation', () => {
  expect(unescapeLine(String.raw`\# Heading`)).toBe('# Heading');
  expect(unescapeLine(String.raw`\*\*bold\*\* and \_italic\_`)).toBe('**bold** and _italic_');
  expect(unescapeLine(String.raw`\[text](url) and \[ref]\[1]`)).toBe('[text](url) and [ref][1]');
  expect(unescapeLine(String.raw`\> quote`)).toBe('> quote');
  expect(unescapeLine(String.raw`\| a \| b \|`)).toBe('| a | b |');
});

test('unescapeLine leaves non-punctuation backslashes alone', () => {
  expect(unescapeLine(String.raw`C:\dir\name`)).toBe(String.raw`C:\dir\name`);
  expect(unescapeLine('trailing backslash \\')).toBe('trailing backslash \\');
});

test('unescapeLine collapses an escaped backslash to one backslash', () => {
  expect(unescapeLine(String.raw`a \\ b`)).toBe(String.raw`a \ b`);
});

test('unescapeLine keeps inline code spans verbatim', () => {
  expect(unescapeLine('use \\*this\\* and `a\\*b`')).toBe('use *this* and `a\\*b`');
  // double-backtick span
  expect(unescapeLine('pre \\_x\\_ ``a\\_`b`` post \\_y\\_')).toBe('pre _x_ ``a\\_`b`` post _y_');
});

test('unescapeLine unescapes after an unmatched backtick run', () => {
  expect(unescapeLine('\\```mermaid')).toBe('```mermaid');
});

test('a typed mermaid fence becomes a real fence (#367)', () => {
  // The serializer escapes every backtick of a typed fence line (reEscapePairedChars), so
  // the real #367 shape is `\`\`\`mermaid`. A partially escaped opener is handled too.
  const escaped = ['\\`\\`\\`mermaid', 'flowchart TD', ' Start --> Stop', '\\`\\`\\`'].join('\n');
  expect(unescapeTypedMarkdown(escaped)).toBe(['```mermaid', 'flowchart TD', ' Start --> Stop', '```'].join('\n'));
  expect(unescapeTypedMarkdown('\\```mermaid\ngraph LR\n\\```')).toBe('```mermaid\ngraph LR\n```');
});

test('a fully escaped fence becomes a real fence; non-punctuation backslashes in its body survive', () => {
  const escaped = ['\\`\\`\\`js', String.raw`const re = /\d+/;`, '\\`\\`\\`'].join('\n');
  expect(unescapeTypedMarkdown(escaped)).toBe(['```js', String.raw`const re = /\d+/;`, '```'].join('\n'));
});

test('a fence typed as text has serializer escapes removed from its body too', () => {
  // #322/#367 regression: the body of a typed fence was serialized as escaped *paragraphs*
  // (there was no fence at serialization time), so its escapes are artifacts.
  const escaped = ['\\`\\`\\`js', String.raw`const my\_var = a \* b;`, '\\`\\`\\`'].join('\n');
  expect(unescapeTypedMarkdown(escaped)).toBe(['```js', 'const my_var = a * b;', '```'].join('\n'));
});

test('pristine lines inside a typed fence are still left verbatim', () => {
  const doc = ['\\```', String.raw`pristine \_bytes\_`, '\\```'].join('\n');
  expect(unescapeTypedMarkdown(doc, [true, false, true]))
    .toBe(['```', String.raw`pristine \_bytes\_`, '```'].join('\n'));
});

test('content of an already-real fence is never unescaped', () => {
  const doc = ['```', String.raw`printf("a \n b");`, String.raw`\# not a heading, just code`, '```'].join('\n');
  expect(unescapeTypedMarkdown(doc)).toBe(doc);
});

test('tilde fences are recognized too, and a typed tilde fence body is unescaped', () => {
  const escaped = ['\\~~~', String.raw`typed \_body\_ text`, '\\~~~'].join('\n');
  expect(unescapeTypedMarkdown(escaped)).toBe(['~~~', 'typed _body_ text', '~~~'].join('\n'));
});

test('escaped angle brackets are never unescaped (no raw-HTML promotion)', () => {
  expect(unescapeLine(String.raw`a \<br> tag and \*bold\*`)).toBe(String.raw`a \<br> tag and *bold*`);
});

test('adoptedLines protects pristine lines from any change', () => {
  const doc = [
    String.raw`pristine \*kept escaped\*`,
    String.raw`typed \*\*bold\*\*`,
  ].join('\n');
  expect(unescapeTypedMarkdown(doc, [false, true])).toBe([
    String.raw`pristine \*kept escaped\*`,
    'typed **bold**',
  ].join('\n'));
});

test('a pristine fence still protects adopted lines inside it', () => {
  const doc = ['```', String.raw`adopted \d line inside code`, '```'].join('\n');
  expect(unescapeTypedMarkdown(doc, [false, true, false])).toBe(doc);
});

test('an adopted escaped line closes a fence the adopted edit opened', () => {
  const doc = ['\\```mermaid', 'graph LR', '\\```', String.raw`after the fence \_italic\_`].join('\n');
  expect(unescapeTypedMarkdown(doc, [true, true, true, true]))
    .toBe(['```mermaid', 'graph LR', '```', 'after the fence _italic_'].join('\n'));
});

test('an unclosed fence protects the rest of the document', () => {
  const doc = ['```', String.raw`\# still code`].join('\n');
  expect(unescapeTypedMarkdown(doc)).toBe(doc);
});

test('a backtick-fence info string containing a backtick is inline code, not a fence', () => {
  const doc = ['``` `x` ```', String.raw`\*text\*`].join('\n');
  expect(unescapeTypedMarkdown(doc)).toBe(['``` `x` ```', '*text*'].join('\n'));
});

test('empty input round-trips', () => {
  expect(unescapeTypedMarkdown('')).toBe('');
});
