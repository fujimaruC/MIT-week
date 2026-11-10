# Deploying AMBA

AMBA has two parts and they deploy differently:

| Part | What it is | Where it runs |
|---|---|---|
| `frontend/` | static SPA (SvelteKit `adapter-static`) | any static host, e.g. Netlify |
| `backend/` | Go server, SSE streams up to 20 s, outbound fetches | a container host (VPS, Fly, Railway, your own machine) |

A static host **cannot** run the Go API. Serverless functions time out well
before a 20 s audit stream, and the API needs outbound network access to the
news endpoint and the LLM gateway. Deploy the two separately.

## Why Netlify

- Netlify Free permits **commercial** use; Vercel Hobby does not.
- Both serve this build equally well. If the project stays strictly
  non-commercial, Vercel Hobby also works (the build output is identical).

## 1. Deploy the frontend to Netlify

`netlify.toml` is already in `frontend/`:

```toml
[build]
  command = "npm run build"
  publish = "build"
```

In the Netlify dashboard:

1. **Add new site → Import an existing project**, pick the repo.
2. **Base directory:** `frontend`
3. **Build command:** `npm run build` (already set by `netlify.toml`)
4. **Publish directory:** `build`
5. Environment variables:

| Variable | Value | Why |
|---|---|---|
| `VITE_API_MODE` | `http` | call the real Go API |
| `VITE_API_BASE_URL` | `https://api.your-domain.example/api/v1` | absolute URL of the Go API |

`VITE_API_BASE_URL` is baked in at build time **and** added to the page CSP's
`connect-src` by `vite.config.ts`, so a wrong value fails loudly instead of
silently blocking requests.

### Preview-only deployment (no backend)

Set `VITE_API_MODE=mock` and leave the URL unset. The app then runs entirely in
the browser and labels every result "Mode pratinjau" with a "CONTOH /
SIMULASI" watermark. This is honest, not a fake demo.

## 2. Deploy the Go API

The image is `backend/Dockerfile` (target `prod`). Required variables:

```bash
ENVIRONMENT=production
REPORT_SIGNING_KEY=<32+ random bytes, base64>
PUBLIC_APP_URL=https://your-site.netlify.app   # CORS origin
LLM_MOCK=false
NINEROUTER_BASE_URL=https://your-llm-gateway-host/v1
NINEROUTER_API_KEY=<key>
NINEROUTER_MODEL=<model>
FACT_RETRIEVAL=true
```

Then set the frontend's `VITE_API_BASE_URL` to that server and redeploy the SPA.

> The API keeps no database and no user content. The only state is an in-memory
> verdict cache, so a restart loses nothing that matters.

## 3. Verify the deployment

```bash
curl https://api.your-domain.example/api/v1/ops/status
```

Check `llm.model_found`, `retrieval.enabled`, and `safe_browsing.placeholder`.
Anything not configured reports `placeholder: true` — that is the intended
behaviour, not a failure.

Then open the site and run one audit. A real result shows a score and no
"Mode pratinjau" banner.

## Known limits of this deployment

- `GOOGLE_SAFE_BROWSING_KEY` is optional; without it the link pillar says so.
- The PWA share target needs HTTPS; Netlify provides it automatically.
- A cold LLM gateway or a dropped gateway call can degrade one audit; the UI shows
  "Sebagian pemeriksaan tidak berjalan" and the next run normally succeeds.
