/* The Toast UI editor's own texts (toolbar tooltips, the Markdown/WYSIWYG tabs, its
   popups) come from its i18n files, not from Gitea's locale: this loads the one of the
   page's language, so the editor speaks it too (#138). Languages Toast UI doesn't ship
   (el, fa, ga, hu, id, is, lv, si, sk) stay in English; zh-HK uses zh-TW, pt-PT pt-BR. */

const loaders: Record<string, () => Promise<unknown>> = {
  'cs-CZ': () => import('@toast-ui/editor/dist/i18n/cs-cz'),
  'de-DE': () => import('@toast-ui/editor/dist/i18n/de-de'),
  'es-ES': () => import('@toast-ui/editor/dist/i18n/es-es'),
  'fi-FI': () => import('@toast-ui/editor/dist/i18n/fi-fi'),
  'fr-FR': () => import('@toast-ui/editor/dist/i18n/fr-fr'),
  'it-IT': () => import('@toast-ui/editor/dist/i18n/it-it'),
  'ja-JP': () => import('@toast-ui/editor/dist/i18n/ja-jp'),
  'ko-KR': () => import('@toast-ui/editor/dist/i18n/ko-kr'),
  'nl-NL': () => import('@toast-ui/editor/dist/i18n/nl-nl'),
  'pl-PL': () => import('@toast-ui/editor/dist/i18n/pl-pl'),
  'pt-BR': () => import('@toast-ui/editor/dist/i18n/pt-br'),
  'ru-RU': () => import('@toast-ui/editor/dist/i18n/ru-ru'),
  'sv-SE': () => import('@toast-ui/editor/dist/i18n/sv-se'),
  'tr-TR': () => import('@toast-ui/editor/dist/i18n/tr-tr'),
  'uk-UA': () => import('@toast-ui/editor/dist/i18n/uk-ua'),
  'zh-CN': () => import('@toast-ui/editor/dist/i18n/zh-cn'),
  'zh-TW': () => import('@toast-ui/editor/dist/i18n/zh-tw'),
};

const aliases: Record<string, string> = {'zh-HK': 'zh-TW', 'pt-PT': 'pt-BR'};

/* The Toast UI language for the page (its `language` option), its texts loaded; 'en-US'
   when Toast UI has none, or they fail to load. */
export async function toastEditorLanguage(lang = document.documentElement.lang): Promise<string> {
  const code = aliases[lang] ?? lang;
  const load = loaders[code];
  if (!load) return 'en-US';
  try {
    await load();
    return code;
  } catch {
    return 'en-US';
  }
}

/* Toast UI's mode tabs, Markdown then WYSIWYG whatever the language, renamed Source
   editor and Visual editor in the page's language. */
export function renameModeTabs(container: Element) {
  const tabs = container.querySelectorAll('.toastui-editor-mode-switch .tab-item');
  if (tabs.length !== 2) return;
  const {i18n} = window.config;
  tabs[0].textContent = i18n.editor_source_mode || 'Source editor';
  tabs[1].textContent = i18n.editor_visual_mode || 'Visual editor';
}
