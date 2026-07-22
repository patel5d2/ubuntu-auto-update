import '@testing-library/jest-dom/vitest';

// vitest's jsdom env doesn't expose localStorage (it needs a document origin
// jsdom isn't given here), yet app code reads it during render (api.ts
// currentRole/canDoOperator). Provide a minimal in-memory Storage so any
// render path that touches it works, not just the tests that mock around it.
if (typeof globalThis.localStorage === 'undefined') {
  const store = new Map<string, string>();
  globalThis.localStorage = {
    getItem: (k) => (store.has(k) ? store.get(k)! : null),
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => [...store.keys()][i] ?? null,
    get length() { return store.size; },
  } as Storage;
}
