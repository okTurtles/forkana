// The header's Compare button can be pressed on the Table or Article view, before the
// bubble view (which owns compare mode) has ever been mounted. repo-history.ts then
// switches to the bubble view and leaves the request here; FishboneGraph takes it once
// it listens for the button.
let pending = false;

export function requestCompareMode() {
  pending = true;
}

export function takeCompareModeRequest(): boolean {
  const was = pending;
  pending = false;
  return was;
}
