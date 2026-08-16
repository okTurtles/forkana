// Line-level three-way merge used to keep a Visual (WYSIWYG) edit from rewriting the whole
// article (issue #262).
//
// Toast UI's WYSIWYG -> markdown serializer is lossy: it backslash-escapes markdown
// punctuation in every text node, rewrites list bullets, collapses runs of spaces, etc.
// Committing that serialization turns reference links (`[text][1]`), link reference
// definitions (`[1]: url`) and other constructs into literal text, so they stop rendering
// as hyperlinks. The lossless tracker (losslessMarkdown.ts) already avoids the serialization
// entirely while the user has not edited in Visual mode. This module handles the other case:
// the user *did* edit, so the serialization must be honoured -- but only for the lines they
// actually touched.
//
// The merge is a classic diff3 shape:
//   base   = the serialization captured when Visual mode was entered
//   ours   = the pristine markdown source (what the user's file actually contains)
//   theirs = the serialization after the user's Visual edit
//
// Everything is compared on a *normalized* form of each line (see normalizeLine) that undoes
// the serializer's cosmetic damage. That is what makes the merge possible at all: `ours` and
// `base` differ on nearly every line in raw bytes, but are equal once normalized, so a raw
// diff3 would report a conflict everywhere.
//
// SAFETY INVARIANT: every line emitted by the merge is normalization-equal to the `theirs`
// line at the same position, and the emitted lines are exactly `theirs`' lines, in order.
// In other words the result always says the same thing as the document the user is looking
// at in the editor; the merge only ever chooses a *spelling* for a line (the user's original
// bytes instead of the re-escaped ones). mergeVisualEdit re-checks this invariant before
// returning and falls back if it does not hold, so a bug here degrades to the old
// "use the serialization" behaviour instead of corrupting an article.

// Backslash escapes the serializer adds (`\[`, `\_`, `\.`, ...). CommonMark only honours a
// backslash before ASCII punctuation, which is exactly the set the serializer uses.
const ESCAPED_PUNCTUATION_RE = /\\([!"#$%&'()*+,\-./:;<=>?@[\]^_`{|}~\\])/g;
const UNORDERED_BULLET_RE = /^(\s*)[*+-](\s)/;
const ORDERED_MARKER_RE = /^(\s*\d{1,9})[.)](\s)/;
const SPACE_RUN_RE = /[ \t]+/g;
// Setext underlines (`====`) and table delimiter rows (`| --- |`) only have to be *long
// enough*, so the serializer freely rewrites their length. Detect a line made purely of rule
// characters and collapse each run to one, which makes `====` and `===` compare equal without
// letting a rule line ever match anything else.
const RULE_LINE_RE = /^[\s|:=-]+$/;
const RULE_RUN_RE = /(=+|-+)/g;

// Product of the two line counts above which the O(n*m) LCS is not worth running. A 1500x1500
// document is ~2.2M cells (~9MB as Uint32Array) and still completes in a few milliseconds;
// beyond that we fall back rather than block the UI thread on a keystroke.
const LCS_CELL_LIMIT = 4_000_000;

// Below this fraction of `base` lines recovering a matching `ours` line, we assume
// normalizeLine does not model whatever this document/serializer combination is doing and
// refuse to merge. Without it, a document the normalizer does not understand would silently
// degrade to "almost everything comes from theirs" — which is the old behaviour, but reached
// by accident rather than by decision.
const MIN_BASE_COVERAGE = 0.5;

// Reduces a line to the form both the pristine source and the serialization agree on.
// Deliberately lossy: it exists only for comparison, never for output.
export function normalizeLine(line: string): string {
  const unescaped = line.replace(ESCAPED_PUNCTUATION_RE, '$1');
  if (RULE_LINE_RE.test(unescaped)) {
    return unescaped.replace(RULE_RUN_RE, (run) => run[0]).replace(SPACE_RUN_RE, ' ').trimEnd();
  }
  return unescaped
    .replace(UNORDERED_BULLET_RE, '$1-$2')
    .replace(ORDERED_MARKER_RE, '$1.$2')
    .replace(SPACE_RUN_RE, ' ')
    .trimEnd();
}

// The merge runs on every keystroke in Visual mode, and two of its three inputs (the pristine
// source and the entry baseline) do not change while the user types. Memoizing their
// normalization keeps per-keystroke work proportional to the *edited* document only.
// Capacity 6 = three documents, with room for one mode switch in flight.
const NORMALIZED_CACHE_CAPACITY = 6;
const normalizedCache = new Map<string, string[]>();

function normalizeAll(text: string, lines: string[]): string[] {
  const cached = normalizedCache.get(text);
  if (cached) return cached;
  const normalized = lines.map((line) => normalizeLine(line));
  if (normalizedCache.size >= NORMALIZED_CACHE_CAPACITY) {
    const oldest = normalizedCache.keys().next();
    if (!oldest.done) normalizedCache.delete(oldest.value);
  }
  normalizedCache.set(text, normalized);
  return normalized;
}

// Longest common subsequence over two line arrays, returned as matched index pairs in
// increasing order. Returns null when the problem is too large to solve eagerly.
// Common prefixes/suffixes are peeled off first, which is what makes the typical case
// (one character typed in one paragraph) essentially free.
export function lcsMatches(a: string[], b: string[]): Array<[number, number]> | null {
  let prefix = 0;
  while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix]) prefix++;
  let suffix = 0;
  while (
    suffix < a.length - prefix &&
    suffix < b.length - prefix &&
    a[a.length - 1 - suffix] === b[b.length - 1 - suffix]
  ) suffix++;

  const aMid = a.slice(prefix, a.length - suffix);
  const bMid = b.slice(prefix, b.length - suffix);
  const n = aMid.length;
  const m = bMid.length;
  if (n * m > LCS_CELL_LIMIT) return null;

  const matches: Array<[number, number]> = [];
  for (let i = 0; i < prefix; i++) matches.push([i, i]);

  if (n && m) {
    // dp[i][j] = LCS length of aMid[i..] and bMid[j..], stored row-major with a sentinel row.
    const width = m + 1;
    const dp = new Uint32Array((n + 1) * width);
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        dp[i * width + j] = aMid[i] === bMid[j] ?
          dp[(i + 1) * width + j + 1] + 1 :
          Math.max(dp[(i + 1) * width + j], dp[i * width + j + 1]);
      }
    }
    let i = 0;
    let j = 0;
    while (i < n && j < m) {
      if (aMid[i] === bMid[j]) {
        matches.push([prefix + i, prefix + j]);
        i++;
        j++;
      } else if (dp[(i + 1) * width + j] >= dp[i * width + j + 1]) {
        i++;
      } else {
        j++;
      }
    }
  }

  for (let k = suffix; k > 0; k--) matches.push([a.length - k, b.length - k]);
  return matches;
}

export type MergeStats = {
  // Why the merge refused, for tests and debugging. Empty when the merge succeeded.
  fallbackReason?: 'too-large' | 'base-unrecognized' | 'invariant-violated';
  // How many of `theirs`' lines were served from the pristine source byte-for-byte.
  preservedLines?: number;
  totalLines?: number;
};

/**
 * Merges a Visual-editor edit back onto the pristine markdown source.
 *
 * @returns the merged markdown, or null when the merge cannot be performed confidently —
 * in which case the caller must fall back to using `theirs` verbatim (the pre-#262 behaviour).
 */
export function mergeVisualEdit(
  ours: string,
  base: string,
  theirs: string,
  stats: MergeStats = {},
): string | null {
  if (theirs === base) return ours; // nothing was edited
  if (!base && ours) {
    // No usable base (Visual mode was never entered cleanly): nothing to anchor against.
    stats.fallbackReason = 'base-unrecognized';
    return null;
  }

  // Splitting on '\n' and re-joining is exactly lossless, including the trailing newline
  // (which shows up as a final empty element).
  const ourLines = ours.split('\n');
  const baseLines = base.split('\n');
  const theirLines = theirs.split('\n');

  const normOurs = normalizeAll(ours, ourLines);
  const normBase = normalizeAll(base, baseLines);
  const normTheirs = normalizeAll(theirs, theirLines);

  // base <-> ours: which pristine line does each serialized baseline line correspond to?
  const baseToOurs = lcsMatches(normBase, normOurs);
  if (!baseToOurs) {
    stats.fallbackReason = 'too-large';
    return null;
  }
  // Confidence check: if normalization cannot line up the baseline serialization with the
  // source it was produced from, our model of the serializer is wrong for this document.
  const coverageDenominator = Math.max(baseLines.length, ourLines.length);
  if (coverageDenominator > 0 && baseToOurs.length / coverageDenominator < MIN_BASE_COVERAGE) {
    stats.fallbackReason = 'base-unrecognized';
    return null;
  }
  const ourIndexOfBase = new Map<number, number>();
  for (const [baseIdx, ourIdx] of baseToOurs) ourIndexOfBase.set(baseIdx, ourIdx);

  // base <-> theirs: which lines did the user actually leave alone?
  const baseToTheirs = lcsMatches(normBase, normTheirs);
  if (!baseToTheirs) {
    stats.fallbackReason = 'too-large';
    return null;
  }
  const baseIndexOfTheirs = new Map<number, number>();
  for (const [baseIdx, theirIdx] of baseToTheirs) baseIndexOfTheirs.set(theirIdx, baseIdx);

  // Pass 1 — lines the user left alone, in place. A `theirs` line that the LCS ties back to a
  // `base` line, which in turn ties back to a pristine source line, is emitted with the user's
  // original bytes.
  const merged: string[] = new Array<string>(theirLines.length);
  const usedOurLines = new Set<number>();
  for (let theirIdx = 0; theirIdx < theirLines.length; theirIdx++) {
    const baseIdx = baseIndexOfTheirs.get(theirIdx);
    const ourIdx = baseIdx === undefined ? undefined : ourIndexOfBase.get(baseIdx);
    if (ourIdx === undefined || usedOurLines.has(ourIdx)) continue;
    merged[theirIdx] = ourLines[ourIdx];
    usedOurLines.add(ourIdx);
  }

  // Pass 2 — moved lines. An LCS is monotonic, so a block the user dragged elsewhere in the
  // Visual editor falls out of the alignment and would be re-emitted in its escaped form.
  // Any still-unassigned `theirs` line that is normalization-equal to a source line nobody
  // has claimed yet is the same content in a new place, so reuse the original bytes.
  const unusedOurLinesByNorm = new Map<string, number[]>();
  for (let ourIdx = 0; ourIdx < ourLines.length; ourIdx++) {
    if (usedOurLines.has(ourIdx)) continue;
    const bucket = unusedOurLinesByNorm.get(normOurs[ourIdx]);
    if (bucket) bucket.push(ourIdx); else unusedOurLinesByNorm.set(normOurs[ourIdx], [ourIdx]);
  }
  let preserved = 0;
  for (let theirIdx = 0; theirIdx < theirLines.length; theirIdx++) {
    if (merged[theirIdx] === undefined) {
      const bucket = unusedOurLinesByNorm.get(normTheirs[theirIdx]);
      const ourIdx = bucket?.shift();
      // Genuinely new or genuinely changed: emit exactly what the editor produced.
      merged[theirIdx] = ourIdx === undefined ? theirLines[theirIdx] : ourLines[ourIdx];
    }
    if (merged[theirIdx] !== theirLines[theirIdx]) preserved++;
  }

  // Defensive re-check of the safety invariant described at the top of the file.
  if (merged.length !== theirLines.length) {
    stats.fallbackReason = 'invariant-violated';
    return null;
  }
  for (let i = 0; i < merged.length; i++) {
    if (normalizeLine(merged[i]) !== normTheirs[i]) {
      stats.fallbackReason = 'invariant-violated';
      return null;
    }
  }

  stats.preservedLines = preserved;
  stats.totalLines = merged.length;
  return merged.join('\n');
}
