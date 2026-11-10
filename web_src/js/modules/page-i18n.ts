/* Strings a script gets from the server's locale (#138): its template puts them, keyed by
   their locale key, as JSON in a data-i18n attribute (as shared/repo/bubble.tmpl and
   repo/diff/conflicts_box.tmpl do). The script lists the keys with their English
   fallback, used where the attribute is missing (unit tests); each such list has a test
   that compares it with custom en-US and the template. */

/* Fills Go-style verbs: %s / %d in order, %[n]s / %[n]d by position. */
export function formatMessage(format: string, args: Array<string | number>): string {
  let next = 0;
  return format.replace(/%(?:\[(\d+)\])?([sd%])/g, (_m, index: string | undefined, verb: string) => {
    if (verb === '%') return '%';
    const arg = index ? args[Number(index) - 1] : args[next++];
    return arg === undefined ? '' : String(arg);
  });
}

/* Whether a count takes the singular, by the page language's plural rules. */
export function isSingular(count: number): boolean {
  try {
    return new Intl.PluralRules(document.documentElement.lang || 'en').select(count) === 'one';
  } catch {
    return count === 1; // an unknown language tag: the English rule
  }
}

export type PageMessages<K extends string> = {
  t: (key: K, ...args: Array<string | number>) => string,
  tn: (count: number, key1: K, keyN: K, ...args: Array<string | number>) => string,
  reset: () => void,
};

/* t() and tn() over the data-i18n JSON of the element `selector` finds, read once. */
export function pageMessages<K extends string>(selector: string, fallback: Record<K, string>): PageMessages<K> {
  let translated: Partial<Record<K, string>> | null = null;
  const messages = (): Partial<Record<K, string>> => {
    if (translated) return translated;
    const json = document.querySelector(selector)?.getAttribute('data-i18n');
    try {
      translated = json ? JSON.parse(json) : {};
    } catch {
      translated = {};
    }
    return translated;
  };
  const t = (key: K, ...args: Array<string | number>) => formatMessage(messages()[key] ?? fallback[key], args);
  return {
    t,
    tn: (count, key1, keyN, ...args) => t(isSingular(count) ? key1 : keyN, ...args),
    reset: () => { translated = null },
  };
}
