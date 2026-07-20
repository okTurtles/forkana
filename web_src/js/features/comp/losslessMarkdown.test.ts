import {installLosslessMarkdownTracker, type LosslessEditor} from './losslessMarkdown.ts';

// Fake mirroring the verified Toast UI 3.2.2 behaviors the tracker depends on:
// - WYSIWYG getMarkdown() serializes lossily (here: escapes `[` and `_`);
// - markdown-mode getMarkdown() returns the raw text;
// - changeMode sets the mode BEFORE converting, the conversion overwrites the markdown
//   document with the lossy serialization and fires `change`, THEN `changeMode` is emitted.
class FakeEditor implements LosslessEditor {
  mode: 'markdown' | 'wysiwyg';
  mdText = '';
  wwSource = '';
  private handlers: Record<string, Array<(...args: unknown[]) => void>> = {};

  constructor(mode: 'markdown' | 'wysiwyg' = 'wysiwyg') {
    this.mode = mode;
  }

  serialize(s: string): string {
    return s.replaceAll('[', '\\[').replaceAll('_', '\\_');
  }

  on(event: string, handler: (...args: unknown[]) => void) {
    (this.handlers[event] ??= []).push(handler);
  }

  private emit(event: string, ...args: unknown[]) {
    for (const h of this.handlers[event] ?? []) h(...args);
  }

  isMarkdownMode() {
    return this.mode === 'markdown';
  }

  getMarkdown() {
    return this.isMarkdownMode() ? this.mdText : this.serialize(this.wwSource);
  }

  setMarkdown(markdown: string, _cursorToEnd?: boolean) {
    this.mdText = markdown;
    if (!this.isMarkdownMode()) this.wwSource = markdown;
    this.emit('change');
  }

  changeMode(mode: 'markdown' | 'wysiwyg') {
    if (this.mode === mode) return;
    this.mode = mode; // the core sets the mode before converting
    if (mode === 'wysiwyg') {
      this.wwSource = this.mdText;
    } else {
      this.mdText = this.serialize(this.wwSource); // lossy overwrite of the md document
    }
    this.emit('change');
    this.emit('changeMode', mode);
  }

  // user edits
  typeMarkdown(text: string) {
    this.mdText = text;
    this.emit('change');
  }

  typeWysiwyg(text: string) {
    this.wwSource = text;
    this.emit('change');
  }
}

const GNARLY = '# Title\n\n[Research program](./Research_program "Research program")\n\nsnake_case_words and *literal* chars';

function setup(mode: 'markdown' | 'wysiwyg' = 'wysiwyg', initial = GNARLY) {
  const fake = new FakeEditor(mode);
  const textarea = document.createElement('textarea');
  textarea.value = initial;
  // editor as consumers see it: tracker overrides getMarkdown/setMarkdown on this object
  const editor = fake as LosslessEditor & FakeEditor;
  installLosslessMarkdownTracker(editor, textarea);
  return {fake, textarea, editor};
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

test('untouched document in WYSIWYG mode round-trips byte-identical', () => {
  const {fake, textarea, editor} = setup();
  expect(fake.serialize(fake.wwSource)).not.toBe(GNARLY); // sanity: the serializer IS lossy
  expect(editor.getMarkdown()).toBe(GNARLY);
  expect(textarea.value).toBe(GNARLY);
});

test('switching Visual→Source without edits restores the pristine source', async () => {
  const {fake, textarea, editor} = setup();
  fake.changeMode('markdown');
  await flush();
  expect(fake.mdText).toBe(GNARLY); // the editor document itself is restored
  expect(editor.getMarkdown()).toBe(GNARLY);
  expect(textarea.value).toBe(GNARLY);
});

test('mode thrashing without edits stays pristine', async () => {
  const {fake, textarea, editor} = setup();
  for (let i = 0; i < 3; i++) {
    fake.changeMode('markdown');
    await flush();
    fake.changeMode('wysiwyg');
  }
  fake.changeMode('markdown');
  await flush();
  expect(editor.getMarkdown()).toBe(GNARLY);
  expect(textarea.value).toBe(GNARLY);
});

test('edits made in Source mode are kept verbatim', async () => {
  const {fake, textarea, editor} = setup();
  fake.changeMode('markdown');
  await flush();
  const edited = `${GNARLY}\n\nappended line`;
  fake.typeMarkdown(edited);
  expect(editor.getMarkdown()).toBe(edited);
  expect(textarea.value).toBe(edited);
  // and they survive an untouched Visual round-trip
  fake.changeMode('wysiwyg');
  fake.changeMode('markdown');
  await flush();
  expect(editor.getMarkdown()).toBe(edited);
  expect(fake.mdText).toBe(edited);
});

test('a genuine Visual edit adopts the serialized form', async () => {
  const {fake, textarea, editor} = setup();
  fake.typeWysiwyg('Hello [world]');
  const serialized = fake.serialize('Hello [world]');
  expect(editor.getMarkdown()).toBe(serialized);
  expect(textarea.value).toBe(serialized);
  fake.changeMode('markdown');
  await flush();
  expect(editor.getMarkdown()).toBe(serialized);
  expect(fake.mdText).toBe(serialized);
});

test('a Visual edit that is fully undone still restores the pristine source', async () => {
  const {fake, editor} = setup();
  fake.typeWysiwyg('changed');
  fake.typeWysiwyg(GNARLY); // back to the baseline content
  fake.changeMode('markdown');
  await flush();
  expect(editor.getMarkdown()).toBe(GNARLY);
  expect(fake.mdText).toBe(GNARLY);
});

test('external setMarkdown becomes the new pristine source (lossless in WYSIWYG mode)', () => {
  const {textarea, editor} = setup();
  editor.setMarkdown('New [text] with_underscores');
  expect(editor.getMarkdown()).toBe('New [text] with_underscores');
  expect(textarea.value).toBe('New [text] with_underscores');
});

test('markdown-only editing is verbatim from the start', () => {
  const {fake, textarea, editor} = setup('markdown');
  expect(editor.getMarkdown()).toBe(GNARLY);
  fake.typeMarkdown('plain **edit**');
  expect(editor.getMarkdown()).toBe('plain **edit**');
  expect(textarea.value).toBe('plain **edit**');
});

test('installation does not rewrite the textarea at load time', () => {
  const {textarea} = setup();
  expect(textarea.value).toBe(GNARLY); // not the serialized form
});

test('empty initial content stays empty', () => {
  const {textarea, editor} = setup('wysiwyg', '');
  expect(editor.getMarkdown()).toBe('');
  expect(textarea.value).toBe('');
});

test('comparisons run through the patched getMarkdown (widget stripping applied)', async () => {
  const fake = new FakeEditor('wysiwyg');
  // simulate installBase64WidgetPatch being installed first
  const original = fake.getMarkdown.bind(fake);
  fake.getMarkdown = () => original().replace(/\$\$widget\d+\s([\s\S]*?)\$\$/g, '$1');
  const textarea = document.createElement('textarea');
  textarea.value = GNARLY;
  installLosslessMarkdownTracker(fake, textarea);
  // WYSIWYG serialization containing a widget placeholder must not read as a user edit
  fake.wwSource = GNARLY; // untouched
  fake.changeMode('markdown');
  await flush();
  expect(fake.getMarkdown()).toBe(GNARLY);
});
