/**
 * Conflict review interactivity for the conflict resolution page.
 *
 * This module:
 * 1. Groups consecutive conflict lines into "conflict wrappers"
 * 2. Adds navigation headers (N of M conflicts, Prev/Next)
 * 3. Adds Keep this / Use this buttons
 * 4. Adds a comment form with Resolve button per conflict
 * 5. Tracks resolution state and enables Submit when all resolved
 * 6. Provides a fold/unfold toggle for context lines
 */

import {POST} from '../modules/fetch.ts';
import {initToastCommentEditor, getToastCommentEditor, EventEditorContentChanged} from './comp/ToastCommentEditor.ts';

export async function initConflictReview() {
  const container = document.querySelector('#diff-file-boxes');
  if (!container) return;

  // Process each diff-file-box (one per conflicted file)
  const fileBoxes = container.querySelectorAll<HTMLElement>('.diff-file-box');
  const allConflictWrappers: HTMLElement[] = [];

  for (const fileBox of fileBoxes) {
    const table = fileBox.querySelector<HTMLTableElement>('table.chroma');
    if (!table) continue;

    const wrappers = await buildConflictWrappers(table);
    // Per-file index matches the backend's extractConflictGroups output, which is
    // 0..N-1 within each file. The global data-conflict-index set by numberConflicts
    // is used only for cross-file navigation.
    for (const [fileIndex, wrapper] of wrappers.entries()) {
      wrapper.setAttribute('data-file-conflict-index', String(fileIndex));
    }
    allConflictWrappers.push(...wrappers);
  }

  if (allConflictWrappers.length === 0) return;

  // Number conflicts globally and add headers
  numberConflicts(allConflictWrappers);

  // Setup fold/unfold toggle
  initFoldToggle();

  // Setup submit button tracking
  initSubmitTracking();
}

/**
 * Groups consecutive `conflict-line` rows in the table, wraps them in a
 * container `<tr>`, and adds a navigation header and action buttons.
 */
async function buildConflictWrappers(table: HTMLTableElement): Promise<HTMLElement[]> {
  const rows = Array.from(table.querySelectorAll<HTMLTableRowElement>('tbody tr, tr'));
  const conflictGroups: HTMLTableRowElement[][] = [];
  let currentGroup: HTMLTableRowElement[] = [];

  for (const row of rows) {
    if (row.getAttribute('data-line-type') === 'conflict') {
      currentGroup.push(row);
    } else {
      if (currentGroup.length > 0) {
        conflictGroups.push(currentGroup);
        currentGroup = [];
      }
    }
  }
  if (currentGroup.length > 0) {
    conflictGroups.push(currentGroup);
  }

  const wrappers: HTMLElement[] = [];
  const editorTemplate = document.querySelector<HTMLTemplateElement>('#issue-comment-editor-template');

  for (const group of conflictGroups) {
    const firstRow = group[0];
    const parentNode = firstRow.parentNode;

    // Create wrapper container row
    const wrapperRow = document.createElement('tr');
    wrapperRow.className = 'conflict-wrapper-row';

    // Insert wrapper into the table tree BEFORE we detach the conflict lines
    if (parentNode) {
      parentNode.insertBefore(wrapperRow, firstRow);
    }

    const wrapperCell = document.createElement('td');
    wrapperCell.colSpan = 6;
    wrapperCell.className = 'conflict-wrapper-cell';
    wrapperRow.append(wrapperCell);

    // Create the conflict wrapper div
    const wrapper = document.createElement('div');
    wrapper.className = 'conflict-wrapper';
    wrapper.setAttribute('data-resolved', 'false');
    wrapperCell.append(wrapper);

    // Add conflict header
    const header = document.createElement('div');
    header.className = 'conflict-header';
    header.innerHTML = `
      <span class="conflict-counter"></span>
      <span class="conflict-nav">
        <a href="#" class="conflict-nav-prev">&lsaquo; Prev</a>
        <a href="#" class="conflict-nav-next">Next &rsaquo;</a>
      </span>
    `;
    wrapper.append(header);

    // Create an inner table for the conflict lines
    const innerTable = document.createElement('table');
    innerTable.className = 'chroma conflict-inner-table';

    // Add colgroup matching the visible columns (4 columns: num, code, num, code)
    // The type marker columns are hidden via display: none, so they are skipped in fixed layout
    // eslint-disable-next-line github/unescaped-html-literal
    innerTable.innerHTML = `<colgroup>
      <col width="50"><col width="50%">
      <col width="50"><col width="50%">
    </colgroup>`;

    const innerTbody = document.createElement('tbody');
    innerTable.append(innerTbody);

    // Extract base and head text before moving rows (rows stay as the same DOM nodes)
    const baseLines: string[] = [];
    const headLines: string[] = [];
    for (const row of group) {
      const oldLineNum = parseInt(row.querySelector('.lines-num-old')?.getAttribute('data-line-num') ?? '0');
      const newLineNum = parseInt(row.querySelector('.lines-num-new')?.getAttribute('data-line-num') ?? '0');
      if (oldLineNum > 0) baseLines.push(row.querySelector('.lines-code-old .code-inner')?.textContent ?? '');
      if (newLineNum > 0) headLines.push(row.querySelector('.lines-code-new .code-inner')?.textContent ?? '');
    }
    const baseText = baseLines.join('\n');
    const headText = headLines.join('\n');

    // Move conflict lines into the inner table
    for (const row of group) {
      innerTbody.append(row);
    }

    // SVG icon for circle-down-arrow (stroked, purple outline, transparent fill)
    // eslint-disable-next-line github/unescaped-html-literal
    const arrowIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" width="14" height="14" class="conflict-choice-icon"><circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M8 4.5v5m0 0L5.5 7M8 9.5L10.5 7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>`;

    // Add Keep this / Use this buttons as table row
    const buttonsRow = document.createElement('tr');
    buttonsRow.className = 'conflict-buttons-row';
    buttonsRow.innerHTML = `
      <td class="lines-num lines-num-old conflict-btn-gutter"></td>
      <td class="lines-type-marker"></td>
      <td class="lines-code lines-code-old conflict-btn-cell">
        <button type="button" class="conflict-choice-btn conflict-keep-btn ui button tiny">
          ${arrowIcon} Keep this
        </button>
      </td>
      <td class="lines-num lines-num-new add-code conflict-btn-gutter"></td>
      <td class="lines-type-marker"></td>
      <td class="lines-code lines-code-new conflict-btn-cell">
        <button type="button" class="conflict-choice-btn conflict-use-btn ui button tiny">
          ${arrowIcon} Use this
        </button>
      </td>
    `;
    innerTbody.append(buttonsRow);

    wrapper.append(innerTable);

    // Add comment section with combo-markdown-editor from template
    const commentSection = document.createElement('div');
    commentSection.className = 'conflict-comment-section';

    if (editorTemplate) {
      // Clone the editor template content
      const editorContent = document.createElement('div');
      editorContent.className = 'conflict-comment-form';
      editorContent.append(editorTemplate.content.cloneNode(true));

      // Replace the default buttons (Cancel/Save) with our Resolve button
      const buttonContainer = editorContent.querySelector('.field.flex-text-block');
      if (buttonContainer) {
        buttonContainer.innerHTML = `
          <button type="button" class="ui tiny primary button conflict-resolve-btn" disabled>Resolve</button>
        `;
      }

      commentSection.append(editorContent);

      // Initialize the toast comment editor
      const toastEditorContainer = commentSection.querySelector<HTMLElement>('.toast-comment-editor');
      if (toastEditorContainer) {
        try {
          await initToastCommentEditor(toastEditorContainer);
        } catch (err) {
          console.error('failed to init conflict editor', err);
        }
      }
    }

    wrapper.append(commentSection);

    // Setup event listeners for this wrapper
    setupWrapperEvents(wrapper, baseText, headText);

    wrappers.push(wrapper);
  }

  return wrappers;
}

/**
 * Numbers all conflict wrappers and sets the counter text.
 */
function numberConflicts(wrappers: HTMLElement[]) {
  const total = wrappers.length;
  for (const [index, wrapper] of wrappers.entries()) {
    const counter = wrapper.querySelector('.conflict-counter');
    if (counter) {
      counter.textContent = `${index + 1} of ${total} conflicts`;
    }
    wrapper.setAttribute('data-conflict-index', String(index));

    // Disable Prev on first conflict, Next on last conflict
    const prevLink = wrapper.querySelector<HTMLAnchorElement>('.conflict-nav-prev');
    const nextLink = wrapper.querySelector<HTMLAnchorElement>('.conflict-nav-next');
    if (index === 0 && prevLink) {
      prevLink.classList.add('disabled');
    }
    if (index === total - 1 && nextLink) {
      nextLink.classList.add('disabled');
    }
  }
}

/**
 * Sets up Keep/Use/Resolve event listeners for a single conflict wrapper.
 * Keep this → pre-fills editor with base text; Use this → pre-fills with head text.
 * The editor content is what gets submitted as the resolved version.
 */
function setupWrapperEvents(wrapper: HTMLElement, baseText: string, headText: string) {
  const keepBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-keep-btn');
  const useBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-use-btn');
  const resolveBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-resolve-btn');
  const toastContainer = wrapper.querySelector<HTMLElement>('.toast-comment-editor');

  const fillEditor = (text: string) => {
    const editor = getToastCommentEditor(toastContainer);
    if (editor) editor.value(text);
    if (resolveBtn) resolveBtn.disabled = !text.trim();
  };

  keepBtn?.addEventListener('click', () => {
    keepBtn.classList.add('selected');
    useBtn?.classList.remove('selected');
    wrapper.setAttribute('data-choice', 'keep');
    fillEditor(baseText);
  });

  useBtn?.addEventListener('click', () => {
    useBtn.classList.add('selected');
    keepBtn?.classList.remove('selected');
    wrapper.setAttribute('data-choice', 'use');
    fillEditor(headText);
  });

  // Also enable/disable Resolve when the user edits the editor directly
  if (toastContainer) {
    toastContainer.addEventListener(EventEditorContentChanged, () => {
      const editor = getToastCommentEditor(toastContainer);
      if (resolveBtn) resolveBtn.disabled = !editor?.value().trim();
    });
  }

  // Resolve button
  resolveBtn?.addEventListener('click', () => {
    const isResolved = wrapper.getAttribute('data-resolved') === 'true';

    if (isResolved) {
      wrapper.setAttribute('data-resolved', 'false');
      wrapper.classList.remove('resolved');
      if (keepBtn) keepBtn.disabled = false;
      if (useBtn) useBtn.disabled = false;
      resolveBtn.textContent = 'Resolve';
      resolveBtn.classList.remove('disabled');
    } else {
      wrapper.setAttribute('data-resolved', 'true');
      wrapper.classList.add('resolved');
      if (keepBtn) keepBtn.disabled = true;
      if (useBtn) useBtn.disabled = true;
      resolveBtn.textContent = '✓ Resolved';
    }

    checkAllResolved();
  });

  // Navigation
  const prevLink = wrapper.querySelector<HTMLAnchorElement>('.conflict-nav-prev');
  const nextLink = wrapper.querySelector<HTMLAnchorElement>('.conflict-nav-next');

  prevLink?.addEventListener('click', (e) => {
    e.preventDefault();
    navigateConflict(wrapper, -1);
  });

  nextLink?.addEventListener('click', (e) => {
    e.preventDefault();
    navigateConflict(wrapper, 1);
  });
}

/**
 * Navigate to the previous or next conflict wrapper.
 */
function navigateConflict(currentWrapper: HTMLElement, direction: number) {
  const allWrappers = document.querySelectorAll<HTMLElement>('.conflict-wrapper');
  const currentIndex = parseInt(currentWrapper.getAttribute('data-conflict-index') || '0');
  let targetIndex = currentIndex + direction;

  if (targetIndex < 0) targetIndex = allWrappers.length - 1;
  if (targetIndex >= allWrappers.length) targetIndex = 0;

  const targetWrapper = allWrappers[targetIndex];
  if (targetWrapper) {
    targetWrapper.scrollIntoView({behavior: 'smooth', block: 'center'});
  }
}

/**
 * Check if all conflicts are resolved and enable Submit buttons.
 */
function checkAllResolved() {
  const allWrappers = document.querySelectorAll<HTMLElement>('.conflict-wrapper');
  let allResolved = true;

  for (const wrapper of allWrappers) {
    if (wrapper.getAttribute('data-resolved') !== 'true') {
      allResolved = false;
      break;
    }
  }

  const submitBtns = document.querySelectorAll<HTMLButtonElement>('.conflict-submit-btn');
  for (const btn of submitBtns) {
    btn.disabled = !allResolved;
  }
}

/**
 * Toggle visibility of context (non-conflict) lines.
 */
function initFoldToggle() {
  const toggleBtns = document.querySelectorAll<HTMLButtonElement>('.conflict-fold-toggle-btn');
  let folded = false;

  for (const btn of toggleBtns) {
    btn.addEventListener('click', () => {
      folded = !folded;
      const contextRows = document.querySelectorAll<HTMLTableRowElement>('tr.context-line');
      for (const row of contextRows) {
        row.style.display = folded ? 'none' : '';
      }
      // Update button icon tooltip
      btn.setAttribute('data-tooltip-content', folded ? 'Show context lines' : 'Hide context lines');
    });
  }
}

/**
 * Initialize submit button click handlers.
 * Collects all keep/use choices, POSTs them to the backend, then redirects to the PR page.
 */
function initSubmitTracking() {
  const submitBtns = document.querySelectorAll<HTMLButtonElement>('.conflict-submit-btn');

  const setButtonsState = (disabled: boolean, text?: string) => {
    for (const btn of submitBtns) {
      btn.disabled = disabled;
      if (text !== undefined) btn.textContent = text;
    }
  };

  for (const btn of submitBtns) {
    btn.addEventListener('click', async () => {
      if (btn.disabled) return;

      // Collect resolved editor text grouped by file
      const fileMap = new Map<string, Array<{index: number; text: string}>>();
      const allWrappers = document.querySelectorAll<HTMLElement>('.conflict-wrapper');
      for (const wrapper of allWrappers) {
        const conflictIndex = parseInt(wrapper.getAttribute('data-file-conflict-index') ?? '0');
        const toastContainer = wrapper.querySelector<HTMLElement>('.toast-comment-editor');
        const text = getToastCommentEditor(toastContainer)?.value() ?? '';
        if (!text.trim()) continue;
        const fileBox = wrapper.closest<HTMLElement>('.diff-file-box');
        if (!fileBox) continue;
        const filePath = fileBox.getAttribute('data-new-filename') ?? '';
        if (!filePath) continue;
        if (!fileMap.has(filePath)) fileMap.set(filePath, []);
        fileMap.get(filePath).push({index: conflictIndex, text});
      }

      const files = Array.from(fileMap, ([path, conflicts]) => ({
        path,
        conflicts: conflicts.sort((a, b) => a.index - b.index),
      }));

      const issueLink = window.location.pathname.replace(/\/conflicts$/, '');

      setButtonsState(true, 'Submitting…');

      try {
        const resp = await POST(window.location.pathname, {data: {files}});
        if (resp.ok) {
          window.location.href = issueLink;
        } else {
          let msg: string;
          try {
            msg = await resp.text();
          } catch {
            msg = resp.statusText;
          }
          setButtonsState(false, `Submit failed: ${msg}`);
        }
      } catch {
        setButtonsState(false, 'Submit failed');
      }
    });
  }
}
