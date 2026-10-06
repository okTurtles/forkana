import {nextTick, ref, watch} from 'vue';
import {initRepoBubbleView} from './repo-bubble-view.ts';
import {initArticleEditor} from './article-editor.ts';
import {initArticleSettings} from './article-settings.ts';
import {GET} from '../modules/fetch.ts';
import {BUBBLE_VISIBLE_EVENT} from '../components/graph/graph-viewport.ts';
import {
  BUBBLE_OPEN_ARTICLE_EVENT, BUBBLE_SELECTED_EVENT, SELECTION_PARAM, SELECTION_UPDATED_EVENT,
  clearLegacyStoredSelection, matchesSelection, normalizeSelection, resolveSelection,
  selectionFromParam, setCurrentSelection, withSelectionParam,
  type RepoSelection,
} from '../modules/repo-selection.ts';
import {parseSubjectLocation, pickInitialSelection, selectionFromHistoryState, withCarriedQuery, type HistoryState, type ViewKey} from './repo-history-state.ts';
import {COMPARE_MODE_TOGGLE_EVENT, requestCompareMode, takeCompareRequestFromUrl} from '../modules/compare-mode-request.ts';

function buildSubjectUrl(base: string, view?: ViewKey): string {
  if (!view) return base;
  const url = new URL(base, window.location.origin);
  if (view === 'bubble') {
    url.searchParams.delete('view');
  } else {
    url.searchParams.set('view', view);
  }
  return url.pathname + url.search;
}

function buildSubjectUrlWithMode(base: string, view: ViewKey, mode?: string) {
  const url = new URL(buildSubjectUrl(base, view), window.location.origin);
  if (mode && mode !== 'read') url.searchParams.set('mode', mode);
  return url.pathname + url.search;
}

function buildArticleUrl(appSubUrl: string, articleBase: string, selection: RepoSelection, mode?: string) {
  // The server-built link already carries the article index when the owner holds several
  // articles for the subject, so it is preferred over the plain subject url, which always
  // resolves to the owner's current article. A selection without a link (one the page
  // could not complete) is the current article at the plain subject url, and an archived
  // one is addressed by its permanent repository url.
  let path = selection.link;
  if (!path && selection.archived) {
    path = `${appSubUrl.replace(/\/+$/, '')}/${encodeURIComponent(selection.owner)}/${encodeURIComponent(selection.repo)}`;
  }
  path ||= `${articleBase.replace(/\/+$/, '')}/${encodeURIComponent(selection.subject || selection.repo)}/${encodeURIComponent(selection.owner)}`;
  const url = new URL(path, window.location.origin);
  url.searchParams.set('view', 'article');
  if (mode && mode !== 'read') url.searchParams.set('mode', mode);
  return url.pathname + url.search;
}

// "/subject/{subject}/{owner}" and "/subject/{subject}/{owner}/{index}" are article urls;
// "/subject/{subject}" alone is the subject page, whose view is in the query.
// Do two url paths name the same page? Percent-encoding and trailing slashes aside.
function samePath(a: string, b: string): boolean {
  try {
    return decodeURIComponent(a).replace(/\/+$/, '') === decodeURIComponent(b).replace(/\/+$/, '');
  } catch {
    return a === b;
  }
}

// The state a history entry of the subject page records.
function historyStateFor(view: ViewKey, mode: string, selection: RepoSelection | null): HistoryState {
  return {
    view,
    mode,
    owner: selection?.owner ?? null,
    subject: selection?.subject ?? null,
    repo: selection?.repo ?? null,
    archived: selection?.archived === true,
    link: selection?.link ?? null,
  };
}

function selectionFromElement(el: Element): RepoSelection | null {
  const owner = el.getAttribute('data-owner') || '';
  const subject = el.getAttribute('data-subject') || '';
  const repo = el.getAttribute('data-repo') || subject;
  if (!owner || !repo) return null;
  return {
    owner,
    subject: subject || null,
    repo,
    archived: el.getAttribute('data-archived') === 'true',
    link: el.getAttribute('data-article-link') || null,
  };
}

export function initRepoHistory() {
  const root = document.querySelector<HTMLElement>('#repo-history-app');
  if (!root) return;

  // The selection is no longer kept in localStorage (#402): drop what earlier versions left.
  clearLegacyStoredSelection();
  // a Compare press made on a page without the bubble view, which sent the reader here
  takeCompareRequestFromUrl();

  const appSubUrl = window.config.appSubUrl || '';
  const subjectUrl = root.getAttribute('data-subject-url') || window.location.pathname;
  const bubbleUrl = root.getAttribute('data-bubble-url') || buildSubjectUrl(subjectUrl, 'bubble');
  const tableUrl = root.getAttribute('data-table-url') || buildSubjectUrl(subjectUrl, 'table');
  const articleBase = root.getAttribute('data-article-base') || `${appSubUrl}/subject`;
  const articleCanonical = root.getAttribute('data-article-canonical') || '';

  const bubbleSection = root.querySelector<HTMLElement>('[data-view="bubble"]');
  const tableSection = root.querySelector<HTMLElement>('[data-view="table"]');
  const articleSection = root.querySelector<HTMLElement>('[data-view="article"]');

  const table = root.querySelector<HTMLTableElement>('#articles-table');

  const navEl = document.querySelector('#subject-view-tabs');

  const initialView = (root.getAttribute('data-initial-view') as ViewKey) || 'bubble';
  const initialOwner = root.getAttribute('data-initial-owner');
  const initialRepo = root.getAttribute('data-initial-repo');
  const initialSubject = root.getAttribute('data-initial-subject');
  const initialMode = root.getAttribute('data-initial-mode');
  const initialArchived = root.getAttribute('data-initial-archived') === 'true';
  const initialLink = root.getAttribute('data-initial-link') || '';

  // Every article of the subject: the table has one row per bubble (#405).
  const candidates: RepoSelection[] = [];
  for (const row of root.querySelectorAll('#articles-table tr.article-row')) {
    const sel = selectionFromElement(row);
    if (sel) candidates.push(sel);
  }

  // The article the server rendered into the article section, if it rendered one. It is
  // the selection when the page opens on the Article view (see pickInitialSelection).
  const renderedArticle: RepoSelection | null = root.getAttribute('data-initial-article') === 'true' && initialOwner && (initialRepo || initialSubject) ?
    normalizeSelection({
      owner: initialOwner,
      repo: initialRepo || initialSubject,
      subject: initialSubject,
      archived: initialArchived,
      link: initialLink || null,
    }) :
    null;

  // Was the page opened on an article url (the vanity "/subject/{subject}/{owner}[/{n}]" or
  // the permanent repository url) rather than on the subject url itself?
  const openedOnArticleUrl = !samePath(window.location.pathname, new URL(subjectUrl, window.location.origin).pathname);
  const initialState = window.history.state as HistoryState | null;
  const initialSelection = pickInitialSelection({
    // On the Article view the server has chosen the article (the one an article url names,
    // the one "selected=" names, or a subject's only one) and rendered it: it is the
    // selection, whatever else says otherwise. This also makes the article shown after a
    // fork redirect the selected one (#177).
    serverArticle: initialView === 'article' ? renderedArticle : undefined,
    historySelection: selectionFromHistoryState(initialState),
    urlSelection: selectionFromParam(new URL(window.location.href).searchParams.get(SELECTION_PARAM)),
    candidates,
  });
  setCurrentSelection(initialSelection);

  // The article may have been served from its permanent repository URL, which resolves to
  // that exact repository. Keep using it for that article so navigating between modes
  // never falls back to the vanity URL of another repository of the subject.
  function articleUrlFor(selection: RepoSelection, mode?: string) {
    if (!articleCanonical || !matchesSelection(renderedArticle, selection)) {
      return buildArticleUrl(appSubUrl, articleBase, selection, mode);
    }
    const url = new URL(articleCanonical, window.location.origin);
    url.searchParams.set('view', 'article');
    if (mode && mode !== 'read') url.searchParams.set('mode', mode);
    return url.pathname + url.search;
  }

  const activeView = ref<ViewKey>(initialView);
  const articleMode = ref<string>(initialMode || 'read');
  const selectedRepo = ref<RepoSelection | null>(initialSelection);
  const isLoading = ref(false);
  const loadError = ref('');
  let articleRequestToken = 0;
  // the article (and mode) the article section currently holds, so switching back to it
  // shows it again instead of fetching it anew
  let loadedArticle: RepoSelection | null = renderedArticle;
  let loadedMode = initialMode || 'read';

  // whether the bubble view has been mounted (nothing watches it)
  let bubbleMounted = false;

  let tableBound = false;
  let loaderEl: HTMLElement | null = null;
  let errorEl: HTMLElement | null = null;
  let errorTextEl: HTMLElement | null = null;
  let articleTabs: HTMLElement | null = null;
  let articleEmptyEl: HTMLElement | null = null;
  let articleContentEl: HTMLElement | null = null;
  const archivedNoticeEl = document.querySelector<HTMLElement>('#article-archived-notice');
  // the notice is hidden outside the article view, so the archived state is read from its text
  let isArchivedArticle = Boolean(archivedNoticeText(archivedNoticeEl));

  function archivedNoticeText(el: HTMLElement | null): string {
    return el?.querySelector('[data-role="article-archived-text"]')?.textContent.trim() || '';
  }

  function collectArticleRefs() {
    if (!articleSection) return;
    loaderEl = articleSection.querySelector('[data-role="article-loader"]');
    errorEl = articleSection.querySelector('[data-role="article-error"]');
    errorTextEl = articleSection.querySelector('[data-role="article-error-text"]');
    articleTabs = articleSection.querySelector('#article-tabs');
    articleEmptyEl = articleSection.querySelector('[data-role="article-empty"]');
    articleContentEl = articleSection.querySelector('[data-role="article-content"]');
  }

  function toggleHidden(el: Element | null, hidden: boolean) {
    if (!el) return;
    if (hidden) el.setAttribute('hidden', '');
    else el.removeAttribute('hidden');
  }

  function showArticleEmpty() {
    toggleHidden(articleEmptyEl, false);
    toggleHidden(articleContentEl, true);
  }

  function showArticleContent() {
    toggleHidden(articleEmptyEl, true);
    toggleHidden(articleContentEl, false);
  }

  // The archived notice lives above the article section so it stays visible across
  // all article modes, so it has to be updated separately when a new article is loaded.
  // An incoming article without archival metadata clears the banner of the previous one.
  function syncArchivedNotice(doc: Document) {
    if (!archivedNoticeEl) return;
    const incoming = doc.querySelector<HTMLElement>('#article-archived-notice');
    const incomingEl = incoming?.querySelector('[data-role="article-archived-text"]');
    isArchivedArticle = Boolean(archivedNoticeText(incoming));
    const currentEl = archivedNoticeEl.querySelector('[data-role="article-archived-text"]');
    if (currentEl) {
      // the notice holds an <absolute-date> element, so the node is replaced rather than
      // its text: a textContent copy would leave the ISO fallback instead of the localised date
      if (incomingEl) currentEl.replaceWith(document.importNode(incomingEl, true));
      else currentEl.textContent = '';
    }
    updateArchivedNoticeVisibility();
  }

  // The transfer notice sits next to the archived notice, outside the swapped article
  // section, and is only rendered for the recipient of a pending transfer. It therefore
  // has to be inserted, replaced or removed whenever another article is loaded.
  function syncTransferNotice(doc: Document) {
    const current = document.querySelector('#article-transfer-notice');
    const incoming = doc.querySelector('#article-transfer-notice');
    if (!incoming) {
      current?.remove();
      return;
    }
    const incomingNode = document.importNode(incoming, true);
    if (current) current.replaceWith(incomingNode);
    else archivedNoticeEl?.after(incomingNode);
  }

  function updateArchivedNoticeVisibility() {
    if (!archivedNoticeEl) return;
    // the notice is a flex container, so it has to be hidden by class rather than by attribute
    // and it only describes the article on screen: the one loaded into the article section
    const showsLoaded = activeView.value === 'article' && Boolean(selectedRepo.value) && matchesSelection(loadedArticle, selectedRepo.value);
    archivedNoticeEl.classList.toggle('tw-hidden', !isArchivedArticle || !showsLoaded);
  }

  function syncNavActive() {
    if (!navEl) return;
    for (const anchor of navEl.querySelectorAll<HTMLAnchorElement>('a[data-view]')) {
      anchor.classList.toggle('active', anchor.getAttribute('data-view') === activeView.value);
    }
  }

  function urlFor(view: ViewKey, mode: string, selection: RepoSelection | null): string {
    if (view === 'article') {
      return selection ? articleUrlFor(selection, mode) : buildSubjectUrlWithMode(subjectUrl, 'article', mode);
    }
    // the rest of the current query (the Table view's sort, say) goes along
    return withSelectionParam(withCarriedQuery(view === 'table' ? tableUrl : bubbleUrl, window.location.search), selection);
  }

  // The view tabs are links: keep their targets on the selection, so opening one in a new
  // tab (or with JavaScript unavailable to intercept it) lands on the same article.
  function syncNavLinks() {
    if (navEl) {
      for (const anchor of navEl.querySelectorAll<HTMLAnchorElement>('a[data-view]')) {
        const view = anchor.getAttribute('data-view') as ViewKey;
        if (view) anchor.setAttribute('href', urlFor(view, 'read', selectedRepo.value));
      }
    }
    // the Table view's Sort menu reloads the page, so its links carry the selection too
    for (const anchor of root.querySelectorAll<HTMLAnchorElement>('a.history-table-sort')) {
      const href = anchor.getAttribute('href');
      if (href) anchor.setAttribute('href', withSelectionParam(href, selectedRepo.value));
    }
  }

  function writeHistory(view: ViewKey, mode: string, selection: RepoSelection | null, how: 'push' | 'replace') {
    const state = historyStateFor(view, mode, selection);
    const url = urlFor(view, mode, selection);
    if (how === 'replace') {
      window.history.replaceState(state, '', url);
    } else {
      window.history.pushState(state, '', url);
    }
  }

  function updateSectionVisibility() {
    toggleHidden(bubbleSection, activeView.value !== 'bubble');
    toggleHidden(tableSection, activeView.value !== 'table');
    toggleHidden(articleSection, activeView.value !== 'article');
    updateArchivedNoticeVisibility();
  }

  function updateCheckboxes() {
    if (!table) return;
    const selection = selectedRepo.value;
    for (const checkbox of table.querySelectorAll<HTMLInputElement>('tbody .row-check')) {
      const row = checkbox.closest<HTMLTableRowElement>('tr.article-row');
      if (!row) continue;
      checkbox.checked = matchesSelection(selection, selectionFromElement(row));
    }
  }

  function updateArticleStatus() {
    if (loaderEl) toggleHidden(loaderEl, !isLoading.value);
    if (errorEl) {
      const showError = !isLoading.value && Boolean(loadError.value);
      toggleHidden(errorEl, !showError);
      if (errorTextEl) errorTextEl.textContent = loadError.value;
    }
  }

  // THE one place the selection changes. `history` says what happens to the browser
  // history: 'replace' records it in the current entry (a selection made inside a view,
  // so Back/Forward and a reload restore it), 'none' leaves the history alone (the caller
  // is restoring an entry, or is about to push a new one).
  function setSelection(next: RepoSelection | null | undefined, history: 'replace' | 'none') {
    const raw = normalizeSelection(next);
    const normalized = resolveSelection(raw, candidates) ?? raw;
    const changed = !(selectedRepo.value === null && normalized === null) && !matchesSelection(selectedRepo.value, normalized);
    if (!changed) return; // every entry this page wrote already records the current selection
    selectedRepo.value = normalized;
    setCurrentSelection(normalized);
    window.dispatchEvent(new CustomEvent(SELECTION_UPDATED_EVENT, {detail: normalized}));
    if (history === 'replace') writeHistory(activeView.value, articleMode.value, normalized, 'replace');
  }

  // A subject with a single article has nothing to choose between: that article is the
  // one the Article view shows (#405 item 4), as the Bubble view already shows it open.
  function defaultSelection(): RepoSelection | null {
    return candidates.length === 1 ? normalizeSelection(candidates[0]) : null;
  }

  async function ensureBubbleView() {
    if (!bubbleMounted) {
      /* Claim the mount BEFORE the await. Two callers race on the first switch
         to bubble — switchView() and the activeView watcher — and with the
         flag set after the await both got through the guard. */
      bubbleMounted = true;
      await nextTick();
      initRepoBubbleView();
    }
    /* #348: tell the graph to measure the box it is actually drawn in. Dispatched on
       EVERY call, not just the first: it is the backstop for resizes that happened
       while the table view was showing. */
    await nextTick();
    window.dispatchEvent(new CustomEvent(BUBBLE_VISIBLE_EVENT));
  }

  function openArticleFrom(el: Element) {
    const selection = selectionFromElement(el);
    if (!selection) return;
    // the clicked row becomes the selection of the table entry as well, so Back returns
    // to the table with that row checked
    setSelection(selection, 'replace');
    switchView('article', {mode: 'read', pushState: true});
  }

  function bindTableInteractions() {
    if (tableBound || !table) return;

    table.addEventListener('click', (event) => {
      const target = event.target as HTMLElement;
      if (!target) return;

      if (target.closest('.row-toggle')) {
        event.preventDefault();
        const btn = target.closest<HTMLButtonElement>('.row-toggle');
        if (!btn) return;
        const targetId = btn.getAttribute('data-target');
        if (!targetId) return;
        const detailRow = document.querySelector<HTMLElement>(`#${targetId}`);
        if (!detailRow) return;
        detailRow.classList.toggle('tw-hidden');
        const down = btn.querySelector<HTMLElement>('.icon-down');
        const up = btn.querySelector<HTMLElement>('.icon-up');
        const isHidden = detailRow.classList.contains('tw-hidden');
        if (down && up) {
          down.classList.toggle('tw-hidden', !isHidden);
          up.classList.toggle('tw-hidden', isHidden);
        }
        return;
      }

      const goTo = target.closest('.go-to-article');
      if (goTo) {
        event.preventDefault();
        openArticleFrom(goTo);
        return;
      }

      const row = target.closest<HTMLTableRowElement>('tr.article-row');
      if (!row) return;
      if (target.closest('input') || target.closest('label')) return;
      if (target.closest('.ui.checkbox')) return;
      openArticleFrom(row);
    });

    table.addEventListener('change', (event) => {
      const target = event.target as HTMLInputElement;
      if (!target || target.type !== 'checkbox' || !target.classList.contains('row-check')) return;
      const row = target.closest<HTMLTableRowElement>('tr.article-row');
      const selection = row ? selectionFromElement(row) : null;
      if (!selection) return;
      if (target.checked) {
        setSelection(selection, 'replace');
      } else if (matchesSelection(selectedRepo.value, selection)) {
        setSelection(null, 'replace');
      }
      // one checkbox at most: the others follow the selection
      updateCheckboxes();
    });

    tableBound = true;

    const $ = (window as unknown as {$?: any}).$;
    if ($ && typeof $.fn?.dropdown === 'function') {
      $('.ui.dropdown').dropdown();
    }
  }

  function bindArticleTabs() {
    if (!articleTabs) return;
    for (const anchor of articleTabs.querySelectorAll<HTMLAnchorElement>('a[data-article-tab]')) {
      anchor.addEventListener('click', (event) => {
        if (!selectedRepo.value) return;
        event.preventDefault();
        const tab = anchor.getAttribute('data-article-tab') || 'read';
        switchView('article', {mode: tab, pushState: true});
      });
    }
  }

  async function loadArticleContent(selection: RepoSelection, mode: string, pushState: boolean) {
    const currentToken = ++articleRequestToken;
    isLoading.value = true;
    loadError.value = '';
    updateArticleStatus();
    showArticleContent();
    const url = articleUrlFor(selection, mode);
    try {
      const response = await GET(url);
      if (!response.ok) throw new Error(`Failed with status ${response.status}`);
      const html = await response.text();
      if (articleRequestToken !== currentToken) return;
      const parser = new DOMParser();
      const doc = parser.parseFromString(html, 'text/html');
      /* A page whose rendering failed half-way still answers 200: the server appends
         its error page to what it had written. Such a page, or one without the
         article section, is an error, never markup to put into this page. */
      const newSection = doc.querySelector('.history-view-section--article');
      if (!newSection || doc.querySelector('.status-page-500') || doc.querySelectorAll('title').length > 1) {
        throw new Error('The article view could not be rendered');
      }
      syncArchivedNotice(doc);
      syncTransferNotice(doc);
      if (articleSection) {
        articleSection.innerHTML = newSection.innerHTML;
        collectArticleRefs();
        showArticleContent();
        const newMode = articleSection.querySelector<HTMLElement>('#article-view-root')?.getAttribute('data-article-mode');
        articleMode.value = newMode || mode;
        loadedArticle = selection;
        loadedMode = articleMode.value;
        bindArticleTabs();
        if (articleMode.value === 'edit') {
          initArticleEditor();
        } else if (articleMode.value === 'settings') {
          initArticleSettings();
        }
      }
      isLoading.value = false;
      updateArticleStatus();
      updateArchivedNoticeVisibility();
      if (pushState) writeHistory('article', articleMode.value, selection, 'push');
    } catch (err) {
      if (articleRequestToken !== currentToken) return;
      console.error('Failed to load article view', err);
      isLoading.value = false;
      loadError.value = 'Unable to load article view';
      updateArticleStatus();
    }
  }

  async function switchView(view: ViewKey, options: {mode?: string, pushState?: boolean} = {}) {
    const nextMode = (options.mode ?? articleMode.value) || 'read';
    // a pending article fetch must not land on (and push over) the view switched to now
    if (view !== 'article') articleRequestToken++;

    activeView.value = view;
    articleMode.value = nextMode;

    if (view === 'bubble') {
      if (options.pushState) writeHistory('bubble', articleMode.value, selectedRepo.value, 'push');
      await ensureBubbleView();
      return;
    }

    if (view === 'table') {
      bindTableInteractions();
      updateCheckboxes();
      if (options.pushState) writeHistory('table', articleMode.value, selectedRepo.value, 'push');
      return;
    }

    if (!selectedRepo.value) {
      const fallback = defaultSelection();
      if (fallback) setSelection(fallback, 'none');
    }
    const selection = selectedRepo.value;
    if (!selection) {
      articleRequestToken++;
      isLoading.value = false;
      loadError.value = '';
      if (options.pushState) writeHistory('article', articleMode.value, null, 'push');
      showArticleEmpty();
      updateArticleStatus();
      updateArchivedNoticeVisibility();
      return;
    }

    if (matchesSelection(loadedArticle, selection) && loadedMode === articleMode.value && !loadError.value) {
      // the section already holds this article: show it again instead of fetching it
      articleRequestToken++;
      isLoading.value = false;
      showArticleContent();
      updateArticleStatus();
      updateArchivedNoticeVisibility();
      if (options.pushState) writeHistory('article', articleMode.value, selection, 'push');
      return;
    }

    await loadArticleContent(selection, articleMode.value, options.pushState ?? false);
  }

  function handleBubbleSelection(event: Event) {
    setSelection((event as CustomEvent<RepoSelection | null>).detail, 'replace');
  }

  function handleBubbleOpenArticle(event: Event) {
    const detail = normalizeSelection((event as CustomEvent<RepoSelection | null>).detail);
    if (!detail) return;
    setSelection(detail, 'replace');
    switchView('article', {mode: 'read', pushState: true});
  }

  function isPlainClick(event: MouseEvent) {
    return !(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0);
  }

  function handleNavClick(event: MouseEvent) {
    const anchor = (event.target as HTMLElement).closest<HTMLAnchorElement>('a[data-view]');
    if (!anchor || !isPlainClick(event)) return;
    const view = anchor.getAttribute('data-view') as ViewKey;
    if (!view) return;
    event.preventDefault();
    switchView(view, {mode: view === 'article' ? 'read' : undefined, pushState: true});
  }

  // "Back to bubble view" inside the (possibly reloaded) article section
  function handleArticleSectionClick(event: MouseEvent) {
    const anchor = (event.target as HTMLElement).closest<HTMLAnchorElement>('a[data-role="back-to-bubble"]');
    if (!anchor || !isPlainClick(event)) return;
    event.preventDefault();
    switchView('bubble', {pushState: true});
  }

  // An entry without a state of ours (one created by an in-page anchor, say) is read
  // from its url, which carries everything the state would.
  function stateFromLocation(): HistoryState {
    const loc = parseSubjectLocation(window.location.href, appSubUrl);
    const pathname = window.location.pathname;
    let selection: RepoSelection | null;
    if (loc.owner && loc.repo) {
      // the permanent article url /{owner}/{repo} (an archived article's, say): that
      // article, or one only known by its url
      const named: RepoSelection = {owner: loc.owner, repo: loc.repo, subject: null};
      selection = candidates.find((c) => matchesSelection(c, named)) ??
        (matchesSelection(renderedArticle, named) ? renderedArticle : null) ??
        normalizeSelection({owner: loc.owner, repo: loc.repo, subject: renderedArticle?.subject ?? loc.repo, archived: true, link: pathname});
    } else if (loc.owner) {
      const linksHere = (s: RepoSelection | null) => Boolean(s?.link) && samePath(new URL(s.link, window.location.origin).pathname, pathname);
      // An article url no row links to (say, one from before the owner's articles were
      // renumbered): keep the url itself as the link, so its article index is not lost.
      selection = candidates.find(linksHere) ??
        (linksHere(renderedArticle) ? renderedArticle : null) ??
        normalizeSelection({owner: loc.owner, repo: loc.subject, subject: loc.subject, link: pathname});
    } else {
      selection = resolveSelection(selectionFromParam(new URL(window.location.href).searchParams.get(SELECTION_PARAM)), candidates);
    }
    return historyStateFor(loc.view, loc.mode, selection);
  }

  function handlePopState(event: PopStateEvent) {
    const state = (event.state as HistoryState | null)?.view ? event.state as HistoryState : stateFromLocation();
    setSelection(selectionFromHistoryState(state), 'none');
    switchView(state.view || 'bubble', {mode: state.mode || 'read', pushState: false});
  }

  watch(activeView, () => {
    updateSectionVisibility();
    syncNavActive();
    if (activeView.value === 'bubble') ensureBubbleView();
    if (activeView.value === 'table') bindTableInteractions();
  }, {immediate: true});

  watch(selectedRepo, () => {
    updateCheckboxes();
    updateArchivedNoticeVisibility();
    syncNavLinks();
  }, {immediate: true});

  watch([isLoading, loadError], () => {
    updateArticleStatus();
  }, {immediate: true});

  // The article section as the page was served: it holds the rendered article only when
  // that article is the selection; otherwise it says nothing is selected, or the selected
  // article is fetched into it.
  collectArticleRefs();
  bindArticleTabs();
  // On the Article view the selection is the rendered article (or none), so nothing has to
  // be fetched for the first view (#405).
  if (activeView.value === 'article') {
    if (!selectedRepo.value) {
      showArticleEmpty();
    } else {
      showArticleContent();
      if (articleMode.value === 'edit') initArticleEditor();
    }
  } else if (!matchesSelection(renderedArticle, selectedRepo.value)) {
    showArticleEmpty();
  }
  updateArchivedNoticeVisibility();

  // Record the selection in the entry the page was opened on, so Back/Forward to it and a
  // reload restore it. An article url is left as it is (it names its article); a subject
  // url gets the selection parameter, keeping whatever else its query holds.
  const entryUrl = openedOnArticleUrl ?
    window.location.pathname + window.location.search :
    withSelectionParam(window.location.pathname + window.location.search, selectedRepo.value);
  window.history.replaceState(historyStateFor(activeView.value, articleMode.value, selectedRepo.value), '', entryUrl + window.location.hash);

  window.addEventListener(BUBBLE_SELECTED_EVENT, handleBubbleSelection as EventListener);
  window.addEventListener(BUBBLE_OPEN_ARTICLE_EVENT, handleBubbleOpenArticle as EventListener);
  // Compare mode lives in the bubble view: pressing Compare on another view goes there.
  // A graph already mounted (hidden) handles the press itself; one that is not yet gets
  // it as a request when it mounts.
  window.addEventListener(COMPARE_MODE_TOGGLE_EVENT, () => {
    if (activeView.value === 'bubble') return;
    if (!bubbleMounted) requestCompareMode();
    switchView('bubble', {pushState: true});
  });
  if (navEl) navEl.addEventListener('click', handleNavClick as EventListener);
  articleSection?.addEventListener('click', handleArticleSectionClick);
  window.addEventListener('popstate', handlePopState);
}
