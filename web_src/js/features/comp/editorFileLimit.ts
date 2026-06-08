import {showErrorToast} from '../../modules/toast.ts';

// getMaxAttachmentSize returns the configured upload limit in bytes, or 0 when unlimited.
export function getMaxAttachmentSize(): number {
  const v = window.config?.maxAttachmentSize;
  return typeof v === 'number' && v > 0 ? v : 0;
}

// findOversizedFile returns the first file exceeding the configured limit, or null.
export function findOversizedFile(files: FileList | File[] | null | undefined): File | null {
  const max = getMaxAttachmentSize();
  if (!max || !files) return null;
  for (const file of Array.from(files)) {
    if (file.size > max) return file;
  }
  return null;
}

// showFileTooLargeError shows a localized "file too large" error toast for the named file.
export function showFileTooLargeError(name: string): void {
  const maxMb = Math.floor(getMaxAttachmentSize() / (1024 * 1024));
  const tmpl = window.config.i18n.editor_file_too_large || 'File "%s" exceeds the limit of %d MB and cannot be saved.';
  // Replace %d (a number) before %s so a filename containing "%d" can't be corrupted.
  showErrorToast(tmpl.replace('%d', String(maxMb)).replace('%s', name));
}

// ensureFilesWithinLimit returns true when every file is within the limit. Otherwise it
// shows an error toast naming the offending file and returns false.
export function ensureFilesWithinLimit(files: FileList | File[] | null | undefined): boolean {
  const oversized = findOversizedFile(files);
  if (!oversized) return true;
  showFileTooLargeError(oversized.name);
  return false;
}
