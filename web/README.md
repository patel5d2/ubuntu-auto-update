# Web (React + Vite + TypeScript)

Operator dashboard for Ubuntu Auto-Update. Styled exclusively with
[Pico CSS](https://picocss.com/) loaded via CDN — no Tailwind, no PostCSS,
no design-system layer.

## Layout

```
src/
  App.tsx                 Top-level <Router> + protected/public route split
  api.ts                  Single fetch wrapper (apiGet/apiPost/apiLogin/...)
  types.ts                Shared TS types mirroring backend/pkg/models
  components/
    ProtectedRoute.tsx    Redirect-to-/login guard
  pages/
    LoginPage.tsx
    HostList.tsx
    HostDetail.tsx
    ExecuteScript.tsx
```

## Running locally

```bash
npm install
npm run dev               # Vite on :5173, proxies /api → VITE_PROXY_TARGET
```

Set `VITE_PROXY_TARGET=http://localhost:8080` (or wherever your backend
listens) before `npm run dev` if it differs from the default.

## Building

```bash
npm run build             # tsc + vite build → dist/
npm run preview           # serves dist/ on :4173 to sanity-check the bundle
```

## Conventions

- **Styling**: Pico classes only. If you find yourself reaching for Tailwind
  utilities, add real Pico-friendly CSS instead — the previous Tailwind
  experiment was deleted because it was never wired up.
- **Types**: shared types live in `src/types.ts` and mirror
  `backend/pkg/models`. Don't redeclare `Host` per page.
- **Forms**: read values via `new FormData(event.currentTarget)`, not
  `form.elements.namedItem(...) as HTMLInputElement`.
- **Feedback**: never `alert()` — use an inline `<aside role="status">` /
  `<aside role="alert">` near the form.
- **API calls**: only through `src/api.ts`; never call `fetch` directly
  from a component (it skips the 401-redirect logic).
