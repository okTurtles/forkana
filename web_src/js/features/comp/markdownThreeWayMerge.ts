// Line-level three-way merge used to keep a Visual (WYSIWYG) edit from rewriting the whole
// article (issue #262).
//
// Toast UI's WYSIWYG -> markdown serializer is lossy: it backslash-escapes markdown
// punctuation in every text node, rewrites list bullets, collapses runs of spaces, etc.
// Committing that serialization turns reference links (`[text][1]`), link reference
// definitions (`[1]: url`) and other constructs into literal text, so they stop rendering
// as hyperlinks. The lossless tracker (losslessMarkdown.ts) already avoids the serialization
// entirely while the user has not edited in Visual mode. This module handles the other case:
// the user *did* edit, so the serialization must be honored — but only for the lines they
// actually touched.
//
// The merge is a classic diff3 shape:
//   base   = the serialization captured when Visual mode was entered
//   ours   = the pristine markdown source (what the user's file actually contains)
//   theirs = the serialization after the user's Visual edit
//
// ALIGNMENT is done on a *normalized* form of each line (see normalizeLine) that undoes the
// serializer's cosmetic damage. That is what makes the merge possible at all: `ours` and
// `base` differ on nearly every line in raw bytes, but are equal once normalized, so a raw
// diff3 would report a conflict everywhere. Normalization is deliberately permissive, and
// therefore NOT semantics-preserving: it strips backslash escapes, collapses space runs and
// canonicalizes list markers, so `\*literal\*` and `*italic*`, or `\# text` and `# text`,
// or an indented code line and a paragraph, all normalize to the same string even though
// they render completely differently.
//
// SUBSTITUTION RULE: because of that, normalization-equality is used only to decide which
// lines *correspond*, never to decide that a pristine line may replace an editor line. A
// pristine `ours` line is substituted for a `theirs` line only when that `theirs` line is
// BYTE-IDENTICAL to the `base` line it aligned to. That is positive evidence that the user
// did not touch the line: it is exactly what Toast UI itself produced when Visual mode was
// entered, and `base` is by construction the serialization of `ours`, so the pristine line is
// the very text that produced it. Every other line is emitted exactly as the editor produced
// it. This is a per-line decision; refusing to substitute costs one line its original
// spelling, never the whole document.
//
// This is what makes an edit that consists only of adding or removing markdown syntax safe.
// A user who selects `\*emphasis\*` and makes it italic produces `*emphasis*`, which is
// normalization-equal to the pristine line but not byte-equal to the baseline, so the
// merge keeps the user's version instead of silently reverting it. Note that comparing
// against `base` is what carries the signal: (ours, theirs) alone cannot tell "the user
// removed an escape" from "the serializer removed an escape", and the serializer does both
// (it drops `\[`/`\]` escapes from some paragraphs, and re-indents nested list items from
// two spaces to four).
//
// SAFETY INVARIANT: the output has exactly `theirs`' lines, in order, and every line is
// either `theirs`' own bytes or a pristine line whose baseline the user left untouched.
// mergeVisualEdit re-checks precisely that before returning — re-deriving each substitution
// from `base` rather than re-asserting a normalized comparison — and falls back if it does
// not hold, so a bug here degrades to the old "use the serialization" behavior instead of
// corrupting an article.

// Backslash escapes the serializer adds (`\[`, `\_`, `\.`, ...). CommonMark only honors a
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
// degrade to "almost everything comes from theirs" — which is the old behavior, but reached
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
  if (cached) {
    // Re-insert to refresh recency. `theirs` is a brand new string on every keystroke, so
    // with plain insertion-order eviction the churn would push out the two entries the
    // cache exists for: after a handful of keystrokes `ours` and `base` were evicted and
    // re-normalized every time (measured: 6.8ms vs 3.1ms per merge on a 2000-line article).
    normalizedCache.delete(text);
    normalizedCache.set(text, cached);
    return cached;
  }
  const normalized = lines.map((line) => normalizeLine(line));
  if (normalizedCache.size >= NORMALIZED_CACHE_CAPACITY) {
    const oldest = normalizedCache.keys().next();
    if (!oldest.done) normalizedCache.delete(oldest.value);
  }
  normalizedCache.set(text, normalized);
  return normalized;
}

// The base<->ours alignment is a pure function of the two texts, and neither of them changes
// while the user types: only `theirs` does. Caching it matters most when the alignment fails
// to peel (a baseline that does not correspond to the source), where the full O(n*m) DP would
// otherwise be rebuilt and thrown away on every keystroke (measured: 16ms per keystroke on a
// 1400-line article). Keyed on both texts in full, so a Visual->Source->Visual round trip --
// which produces a new baseline, and possibly a new source — simply misses the cache; an
// entry can never be served for a different pair of inputs.
const ALIGNMENT_CACHE_CAPACITY = 4;
type AlignmentEntry = {ours: string, base: string, matches: Array<[number, number]> | null};
const alignmentCache: AlignmentEntry[] = [];

// The returned array is shared with the cache and must be treated as read-only.
function alignBaseToOurs(ours: string, base: string, normBase: string[], normOurs: string[]): Array<[number, number]> | null {
  const hit = alignmentCache.findIndex((entry) => entry.ours === ours && entry.base === base);
  if (hit >= 0) {
    const [entry] = alignmentCache.splice(hit, 1);
    alignmentCache.push(entry); // most recently used
    return entry.matches;
  }
  const matches = lcsMatches(normBase, normOurs);
  alignmentCache.push({ours, base, matches});
  if (alignmentCache.length > ALIGNMENT_CACHE_CAPACITY) alignmentCache.shift();
  return matches;
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
 * in which case the caller must fall back to using `theirs` verbatim (the pre-#262 behavior).
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
  // Memoized: both inputs are fixed for as long as the user stays in Visual mode.
  const baseToOurs = alignBaseToOurs(ours, base, normBase, normOurs);
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
  // original bytes — but only if it is byte-identical to that baseline line (the SUBSTITUTION
  // RULE at the top of the file). A line the user retyped, re-emphasized or re-indented is not
  // byte-identical to the baseline and keeps the editor's spelling.
  const merged: string[] = new Array<string>(theirLines.length);
  // Provenance: the `base` line each substitution was justified by, or -1 for "emitted
  // verbatim from theirs". The invariant re-check below re-derives the substitution from it.
  const baseOfMerged: number[] = new Array<number>(theirLines.length).fill(-1);
  const usedOurLines = new Set<number>();
  for (let theirIdx = 0; theirIdx < theirLines.length; theirIdx++) {
    const baseIdx = baseIndexOfTheirs.get(theirIdx);
    if (baseIdx === undefined || theirLines[theirIdx] !== baseLines[baseIdx]) continue;
    const ourIdx = ourIndexOfBase.get(baseIdx);
    if (ourIdx === undefined || usedOurLines.has(ourIdx)) continue;
    merged[theirIdx] = ourLines[ourIdx];
    baseOfMerged[theirIdx] = baseIdx;
    usedOurLines.add(ourIdx);
  }

  // Pass 2 — moved lines. An LCS is monotonic, so a block the user dragged elsewhere in the
  // Visual editor falls out of the alignment and would be re-emitted in its escaped form. A
  // still-unassigned `theirs` line that is byte-identical to a baseline line nobody has
  // claimed is the same untouched content in a new place, so reuse the original bytes. The
  // match is on baseline bytes, not on the normalized form: a line the user moved AND changed
  // (`\*emphasis\*` deleted and retyped as `*emphasis*` elsewhere) is not byte-identical to
  // any baseline line and so keeps the editor's spelling.
  const unclaimedBaseByText = new Map<string, number[]>();
  for (let baseIdx = 0; baseIdx < baseLines.length; baseIdx++) {
    const ourIdx = ourIndexOfBase.get(baseIdx);
    if (ourIdx === undefined || usedOurLines.has(ourIdx)) continue;
    const bucket = unclaimedBaseByText.get(baseLines[baseIdx]);
    if (bucket) bucket.push(baseIdx); else unclaimedBaseByText.set(baseLines[baseIdx], [baseIdx]);
  }
  let preserved = 0;
  for (let theirIdx = 0; theirIdx < theirLines.length; theirIdx++) {
    if (merged[theirIdx] === undefined) {
      const bucket = unclaimedBaseByText.get(theirLines[theirIdx]);
      let ourIdx: number | undefined;
      let baseIdx: number | undefined;
      while (bucket?.length) {
        const candidateBase = bucket.shift();
        const candidate = ourIndexOfBase.get(candidateBase);
        if (candidate !== undefined && !usedOurLines.has(candidate)) {
          ourIdx = candidate;
          baseIdx = candidateBase;
          break;
        }
      }
      // Genuinely new or genuinely changed: emit exactly what the editor produced.
      merged[theirIdx] = ourIdx === undefined ? theirLines[theirIdx] : ourLines[ourIdx];
      if (ourIdx !== undefined) {
        baseOfMerged[theirIdx] = baseIdx;
        usedOurLines.add(ourIdx);
      }
    }
    if (merged[theirIdx] !== theirLines[theirIdx]) preserved++;
  }

  // Defensive re-check of the safety invariant described at the top of the file.
  if (merged.length !== theirLines.length) {
    stats.fallbackReason = 'invariant-violated';
    return null;
  }
  for (let i = 0; i < merged.length; i++) {
    const baseIdx = baseOfMerged[i];
    if (baseIdx === -1) {
      // Nothing was substituted here, so the editor's line must have survived untouched.
      if (merged[i] !== theirLines[i]) {
        stats.fallbackReason = 'invariant-violated';
        return null;
      }
      continue;
    }
    // Re-derive the substitution from `base` instead of re-asserting a normalized comparison:
    // the user's line must be byte-identical to the baseline line that justified the
    // substitution, and the emitted line must be exactly the pristine line that baseline was
    // produced from. A normalized comparison here would accept the very confusions the
    // SUBSTITUTION RULE exists to reject, which is why it is not used.
    const ourIdx = ourIndexOfBase.get(baseIdx);
    if (theirLines[i] !== baseLines[baseIdx] || ourIdx === undefined || merged[i] !== ourLines[ourIdx]) {
      stats.fallbackReason = 'invariant-violated';
      return null;
    }
  }

  stats.preservedLines = preserved;
  stats.totalLines = merged.length;
  return merged.join('\n');
}
