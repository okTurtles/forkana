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
import {showErrorToast} from '../modules/toast.ts';
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
 *
 * IMPORTANT — grouping contract: this function groups rows by consecutive
 * `data-line-type="conflict"` DOM rows, which must stay 1-to-1 with the
 * conflictGroup objects produced by extractConflictGroups in
 * routers/web/repo/pull.go. Any change to how the template renders conflict
 * rows (conflicts_section_split.tmpl) or how the Go backend groups diff lines
 * must be reflected in both places simultaneously.
 */
async function buildConflictWrappers(table: HTMLTableElement): Promise<HTMLElement[]> {
  const rows = Array.from(table.querySelectorAll<HTMLTableRowElement>('tbody tr, tr'));
  const fileDeleteChoice = table.closest<HTMLElement>('.diff-file-box')?.getAttribute('data-file-delete-choice') ?? 'none';
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

    // eslint-disable-next-line github/unescaped-html-literal
    innerTable.innerHTML = `<colgroup>
      <col width="50"><col class="col-type-marker" width="10"><col>
      <col width="50"><col class="col-type-marker" width="10"><col>
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

    // Inject side labels into the first code cells (shown on mobile as pills)
    const leftHeaderText = table.querySelector<HTMLElement>('.diff-split-header-left')?.textContent?.trim() ?? '';
    const rightHeaderText = table.querySelector<HTMLElement>('.diff-split-header-right')?.textContent?.trim() ?? '';
    const firstOldCode = innerTbody.querySelector<HTMLElement>('.lines-code-old');
    const firstNewCode = innerTbody.querySelector<HTMLElement>('.lines-code-new');
    if (firstOldCode && leftHeaderText) {
      const label = document.createElement('div');
      label.className = 'conflict-side-label conflict-side-label-old';
      label.textContent = leftHeaderText;
      firstOldCode.prepend(label);
    }
    if (firstNewCode && rightHeaderText) {
      const label = document.createElement('div');
      label.className = 'conflict-side-label conflict-side-label-new';
      label.textContent = rightHeaderText;
      firstNewCode.prepend(label);
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
    setupWrapperEvents(wrapper, baseText, headText, fileDeleteChoice);

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
 * Builds a read-only "resolved" table view that shows the base content on the
 * left (del background) and the resolved text on the right (add background).
 * Includes an Edit footer link for going back to edit mode.
 */
function buildResolvedView(wrapper: HTMLElement, resolvedText: string): HTMLElement {
  // Extract base line numbers and text from the original conflict rows
  const innerTable = wrapper.querySelector<HTMLElement>('.conflict-inner-table');
  const baseLineNums: number[] = [];
  const baseTexts: string[] = [];
  let headStartLineNum = 0;

  if (innerTable) {
    for (const row of innerTable.querySelectorAll<HTMLTableRowElement>('tr[data-line-type="conflict"]')) {
      const oldNum = parseInt(row.querySelector<HTMLElement>('.lines-num-old')?.getAttribute('data-line-num') ?? '0');
      if (oldNum > 0) {
        baseLineNums.push(oldNum);
        baseTexts.push(row.querySelector('.lines-code-old .code-inner')?.textContent ?? '');
      }
      if (headStartLineNum === 0) {
        const newNum = parseInt(row.querySelector<HTMLElement>('.lines-num-new')?.getAttribute('data-line-num') ?? '0');
        if (newNum > 0) headStartLineNum = newNum;
      }
    }
  }
  if (headStartLineNum === 0) headStartLineNum = baseLineNums[0] ?? 1;

  const trimmed = resolvedText.replace(/\n+$/, '');
  const resolvedLines = trimmed.length > 0 ? trimmed.split('\n') : [];

  const container = document.createElement('div');
  container.className = 'conflict-resolved-view';

  const table = document.createElement('table');
  table.className = 'chroma conflict-inner-table';
  // eslint-disable-next-line github/unescaped-html-literal
  table.innerHTML = `<colgroup><col width="50"><col class="col-type-marker" width="10"><col><col width="50"><col class="col-type-marker" width="10"><col></colgroup>`;

  const tbody = document.createElement('tbody');
  const maxRows = Math.max(baseTexts.length, resolvedLines.length);

  for (let i = 0; i < maxRows; i++) {
    const row = document.createElement('tr');

    // Left: base line
    const leftNum = document.createElement('td');
    leftNum.className = 'lines-num lines-num-old';
    if (i < baseLineNums.length) {
      leftNum.setAttribute('data-line-num', String(baseLineNums[i]));
      leftNum.textContent = String(baseLineNums[i]);
    }

    const leftMarker = document.createElement('td');
    leftMarker.className = 'lines-type-marker';

    const leftCode = document.createElement('td');
    leftCode.className = 'lines-code lines-code-old resolved-base';
    const leftInner = document.createElement('code');
    leftInner.className = 'code-inner';
    leftInner.textContent = i < baseTexts.length ? baseTexts[i] : '';
    leftCode.append(leftInner);

    row.append(leftNum, leftMarker, leftCode);

    // Right: resolved line (only styled green when there is actual content)
    const hasRight = i < resolvedLines.length;
    const rightNum = document.createElement('td');
    rightNum.className = hasRight ? 'lines-num lines-num-new add-code' : 'lines-num lines-num-new';
    if (hasRight) {
      rightNum.setAttribute('data-line-num', String(headStartLineNum + i));
      rightNum.textContent = String(headStartLineNum + i);
    }

    const rightMarker = document.createElement('td');
    rightMarker.className = 'lines-type-marker';

    const rightCode = document.createElement('td');
    rightCode.className = hasRight ? 'lines-code lines-code-new resolved-result' : 'lines-code lines-code-new';
    const rightInner = document.createElement('code');
    rightInner.className = 'code-inner';
    rightInner.textContent = hasRight ? resolvedLines[i] : '';
    rightCode.append(rightInner);

    row.append(rightNum, rightMarker, rightCode);
    tbody.append(row);
  }

  table.append(tbody);
  container.append(table);

  // Edit footer
  const footer = document.createElement('div');
  footer.className = 'conflict-resolved-footer';
  // eslint-disable-next-line github/unescaped-html-literal
  footer.innerHTML = `<button type="button" class="conflict-edit-btn"><svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" width="12" height="12"><path d="M11.013 1.427a1.75 1.75 0 0 1 2.474 0l1.086 1.086a1.75 1.75 0 0 1 0 2.474l-8.61 8.61c-.21.21-.47.364-.756.445l-3.251.93a.75.75 0 0 1-.927-.928l.929-3.25c.081-.286.235-.547.445-.758l8.61-8.61Zm1.414 1.06a.25.25 0 0 0-.354 0L10.811 3.75l1.439 1.44 1.263-1.263a.25.25 0 0 0 0-.354ZM11.189 6.25 9.75 4.81l-6.286 6.287a.25.25 0 0 0-.064.108l-.558 1.953 1.953-.558a.25.25 0 0 0 .108-.064Z"></path></svg> Edit</button>`;
  container.append(footer);

  return container;
}

/**
 * Updates the counter label in every conflict wrapper header to reflect
 * the current resolved/unresolved state.
 */
function updateAllCounters() {
  const allWrappers = Array.from(document.querySelectorAll<HTMLElement>('.conflict-wrapper'));
  const total = allWrappers.length;
  const unresolvedCount = allWrappers.filter((w) => w.getAttribute('data-resolved') !== 'true').length;

  for (const [index, wrapper] of allWrappers.entries()) {
    const counter = wrapper.querySelector('.conflict-counter');
    if (!counter) continue;
    if (wrapper.getAttribute('data-resolved') === 'true') {
      const rest = unresolvedCount === 0 ? 'Resolved' : `Resolved · ${unresolvedCount} more ${unresolvedCount === 1 ? 'conflict' : 'conflicts'}`;
      // eslint-disable-next-line github/unescaped-html-literal
      counter.innerHTML = `<span class="conflict-resolved-check">✓</span> ${rest}`;
    } else {
      counter.textContent = `${index + 1} of ${total} conflicts`;
    }
  }
}

/**
 * Sets up Keep/Use/Resolve event listeners for a single conflict wrapper.
 * Keep this → pre-fills editor with base text; Use this → pre-fills with head text.
 * The editor content is what gets submitted as the resolved version.
 */
function setupWrapperEvents(wrapper: HTMLElement, baseText: string, headText: string, fileDeleteChoice: string) {
  const keepBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-keep-btn');
  const useBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-use-btn');
  const resolveBtn = wrapper.querySelector<HTMLButtonElement>('.conflict-resolve-btn');
  const toastContainer = wrapper.querySelector<HTMLElement>('.toast-comment-editor');

  const isResolveEnabled = () => Boolean(wrapper.getAttribute('data-choice'));

  const updateDeleteIntent = (choice: string | null, text: string) => {
    const deletesFile = choice !== null && fileDeleteChoice === choice && text.replace(/\n+$/, '') === '';
    wrapper.setAttribute('data-delete-file', String(deletesFile));
  };

  const fillEditor = (choice: string, text: string) => {
    const editor = getToastCommentEditor(toastContainer);
    if (editor) editor.value(text);
    updateDeleteIntent(choice, text);
    if (resolveBtn) resolveBtn.disabled = !isResolveEnabled();
  };

  keepBtn?.addEventListener('click', () => {
    keepBtn.classList.add('selected');
    useBtn?.classList.remove('selected');
    wrapper.setAttribute('data-choice', 'keep');
    fillEditor('keep', baseText);
  });

  useBtn?.addEventListener('click', () => {
    useBtn.classList.add('selected');
    keepBtn?.classList.remove('selected');
    wrapper.setAttribute('data-choice', 'use');
    fillEditor('use', headText);
  });

  // Re-evaluate Resolve when the user edits the editor directly (choice already set)
  if (toastContainer) {
    toastContainer.addEventListener(EventEditorContentChanged, () => {
      const choice = wrapper.getAttribute('data-choice');
      updateDeleteIntent(choice, getToastCommentEditor(toastContainer)?.value() ?? '');
      if (resolveBtn) resolveBtn.disabled = !isResolveEnabled();
    });
  }

  const innerTable = wrapper.querySelector<HTMLElement>('.conflict-inner-table');
  const commentSection = wrapper.querySelector<HTMLElement>('.conflict-comment-section');

  const setResolved = (resolved: boolean) => {
    if (resolved) {
      const editor = getToastCommentEditor(toastContainer);
      const resolvedText = editor?.value() ?? '';

      const resolvedView = buildResolvedView(wrapper, resolvedText);
      resolvedView.querySelector('.conflict-edit-btn')?.addEventListener('click', () => setResolved(false));

      if (innerTable) {
        innerTable.style.display = 'none';
        innerTable.after(resolvedView);
      }
      if (commentSection) commentSection.style.display = 'none';

      wrapper.setAttribute('data-resolved', 'true');
      wrapper.classList.add('resolved');
      if (keepBtn) keepBtn.disabled = true;
      if (useBtn) useBtn.disabled = true;
      if (resolveBtn) resolveBtn.textContent = '✓ Resolved';
    } else {
      wrapper.querySelector('.conflict-resolved-view')?.remove();
      if (innerTable) innerTable.style.display = '';
      if (commentSection) commentSection.style.display = '';

      wrapper.setAttribute('data-resolved', 'false');
      wrapper.classList.remove('resolved');
      if (keepBtn) keepBtn.disabled = false;
      if (useBtn) useBtn.disabled = false;
      if (resolveBtn) {
        resolveBtn.textContent = 'Resolve';
        resolveBtn.disabled = !isResolveEnabled();
      }
    }

    checkAllResolved();
  };

  resolveBtn?.addEventListener('click', () => {
    setResolved(wrapper.getAttribute('data-resolved') !== 'true');
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

  updateAllCounters();
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
  const originalTexts = new Map(Array.from(submitBtns, (b) => [b, b.textContent ?? '']));

  const setButtonsState = (disabled: boolean, text?: string) => {
    for (const btn of submitBtns) {
      btn.disabled = disabled;
      btn.textContent = text !== undefined ? text : (originalTexts.get(btn) ?? '');
    }
  };

  for (const btn of submitBtns) {
    btn.addEventListener('click', async () => {
      if (btn.disabled) return;

      // Collect resolved editor text grouped by file
      const fileMap = new Map<string, Array<{index: number; text: string; deleteFile?: boolean}>>();
      const allWrappers = document.querySelectorAll<HTMLElement>('.conflict-wrapper');
      for (const wrapper of allWrappers) {
        if (wrapper.getAttribute('data-resolved') !== 'true') continue;
        const conflictIndex = parseInt(wrapper.getAttribute('data-file-conflict-index') ?? '0');
        const toastContainer = wrapper.querySelector<HTMLElement>('.toast-comment-editor');
        const text = getToastCommentEditor(toastContainer)?.value() ?? '';
        const deleteFile = wrapper.getAttribute('data-delete-file') === 'true';
        const fileBox = wrapper.closest<HTMLElement>('.diff-file-box');
        if (!fileBox) continue;
        const filePath = fileBox.getAttribute('data-new-filename') ?? '';
        if (!filePath) continue;
        if (!fileMap.has(filePath)) fileMap.set(filePath, []);
        fileMap.get(filePath).push({index: conflictIndex, text, ...(deleteFile ? {deleteFile} : {})});
      }

      const files = Array.from(fileMap, ([path, conflicts]) => ({
        path,
        conflicts: conflicts.sort((a, b) => a.index - b.index),
      }));

      const issueLink = window.location.pathname.replace(/\/conflicts$/, '');
      const diffBoxes = document.querySelector('#diff-file-boxes');
      const baseCommitID = diffBoxes?.getAttribute('data-base-commit-id') ?? '';
      const headCommitID = diffBoxes?.getAttribute('data-head-commit-id') ?? '';
      // Echo back the whitespace mode that was active when the page was rendered
      // so the backend can verify GET/POST consistency.
      const whitespace = diffBoxes?.getAttribute('data-whitespace') ?? '';

      setButtonsState(true, 'Submitting…');

      try {
        const resp = await POST(window.location.pathname, {data: {baseCommitID, headCommitID, whitespace, files}});
        if (resp.ok) {
          window.location.href = issueLink;
        } else {
          let msg: string;
          try {
            msg = await resp.text();
          } catch {
            msg = `HTTP ${resp.status}`;
          }
          setButtonsState(false);
          showErrorToast(`Submit failed: ${msg}. Your resolutions have not been saved — you will need to redo them after reloading.`);
        }
      } catch {
        setButtonsState(false);
        showErrorToast('Submit failed: network error');
      }
    });
  }
}
