# AMBA Docker Guide

Panduan containerization AMBA untuk tim lintas platform (Linux, macOS Apple Silicon / Intel, Windows WSL2).

## Struktur File Docker

```
AMBA/
├── Makefile                     # Shortcut perintah cepat tim (make up, make dev, make down)
├── start.bat                    # Windows: setara `make up` (Docker Desktop)
├── start-dev.bat                # Windows: setara `make dev` (hot-reload)
├── stop.bat                     # Windows: setara `make down`
├── docker-compose.yml           # Stack demo / prod-like (Go static binary + Nginx SPA proxy)
├── docker-compose.dev.yml       # Stack dev (Go Air hot-reload + SvelteKit Vite dev server)
├── .dockerignore                # Root exclude list (git, test artifacts, logs)
├── .env.example                 # Contoh variabel lingkungan container
├── backend/
│   ├── Dockerfile               # Multi-stage: base -> dev -> build -> prod
│   ├── .dockerignore            # Exclude backend (bin, tmp, test files)
│   └── .air.toml                # Konfigurasi Air hot-reload (tmp di /tmp/air)
└── frontend/
    ├── Dockerfile               # Multi-stage: base -> dev -> builder -> prod
    ├── .dockerignore            # Exclude frontend (node_modules, build, .svelte-kit)
    └── nginx.conf               # Nginx reverse proxy + SSE unbuffered stream + SPA fallback
```

---

## 1. Quickstart (Demo / Production-like)

Stack all-in-one instan dengan zero configuration:

```bash
# Menjalankan backend dan frontend siap pakai
docker compose up --build

# Buka di browser:
# Web: http://localhost
# API health check: http://localhost/healthz atau http://localhost:8080/healthz
```

Frontend di-serve oleh Nginx Alpine (~25MB), yang sekaligus melakukan reverse proxy ke Go backend (`/api/` -> `backend:8080`). Semua panggilan API bersifat same-origin sehingga tidak ada masalah CORS maupun Content Security Policy (CSP) saat diakses via IP LAN (misal pengujian di smartphone).

---

## 2. Development Mode (Hot-Reload)

Untuk pengembangan aktif dengan live reload pada backend maupun frontend:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
# Atau:
make dev
```

- **Backend**: Menggunakan [Air](https://github.com/air-verse/air). Setiap perubahan file `.go` akan memicu kompilasi ulang otomatis dalam hitungan detik. Direktori binary sementara diatur ke `/tmp/air` agar tidak mengotori direktori host.
- **Frontend**: Menggunakan Vite dev server dengan HMR (Hot Module Replacement) pada port `5173`.
  - Menggunakan anonymous volume `/app/node_modules` agar binary host tidak menimpa binary Linux di dalam container.
  - Mendukung inotify polling via `CHOKIDAR_USEPOLLING=true` untuk kelancaran file watching di WSL2 dan macOS VirtioFS.

---

## 3. Rebuild & Update Codebase

Saat kode atau dependensi diperbarui:

```bash
# Rebuild saat ada perubahan kode
docker compose up --build

# Rebuild hanya satu service (misal hanya backend)
docker compose build backend
docker compose up -d backend

# Rebuild hanya frontend
docker compose build frontend
docker compose up -d frontend

# Rebuild bersih dan reset volume dev jika package.json/go.mod diubah
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build -V
```

### Optimasi Cache (BuildKit)
- `go mod download` dan `npm ci` dijalankan terpisah sebelum penyalinan source code.
- BuildKit cache mounts (`/go/pkg/mod`, `/root/.cache/go-build`, dan `/root/.npm`) menjaga file dependensi tetap tersimpan antar-build meskipun layer source code invalid. Rebuild kode memakan waktu hanya ~2-5 detik.

---

## 4. Konfigurasi Port & Lingkungan

Jika port 80 terpakai di perangkat Anda, ubah di `.env`:

```env
FRONTEND_PORT=3000
BACKEND_PORT=8080
```

Untuk memakai model sungguhan, matikan mock dan isi kunci OpenRouter di `.env`
(alamat gateway, model gratis, dan `LLM_EXTRA_BODY` sudah terisi di `.env.example`):
```env
LLM_MOCK=false
NINEROUTER_API_KEY=sk-or-...
```

Atau ke LLM gateway lokal (misal router kompatibel OpenAI di host machine), dengan `LLM_EXTRA_BODY` dikosongkan:
```env
LLM_MOCK=false
NINEROUTER_BASE_URL=http://host.docker.internal:20128/v1
NINEROUTER_API_KEY=your_key_here
NINEROUTER_MODEL=gemini-3.8-flash
LLM_EXTRA_BODY=
```
`extra_hosts: ["host.docker.internal:host-gateway"]` telah dikonfigurasi secara default sehingga container dapat berkomunikasi langsung ke host machine.

Layanan opsional lain cukup diisi di `.env` (daftar lengkap ada di `.env.example`):
```env
GOOGLE_API_KEY=...            # Fact Check Tools + Safe Browsing
SIGHTENGINE_API_USER=...      # optional AI-image detector (both or neither)
SIGHTENGINE_API_SECRET=...
TELEGRAM_BOT_TOKEN=...        # bot berjalan dalam mode polling di Docker lokal
```

Stack Docker ini untuk lokal dan LAN. Untuk produksi publik (Vercel + Fly.io/Railway), ikuti [DEPLOY.md](DEPLOY.md).

---

## 5. Perintah Makefile

Tersedia target Make untuk kemudahan tim:

| Perintah | Deskripsi |
|---|---|
| `make up` | Jalankan stack demo / production-like |
| `make dev` | Jalankan stack development dengan hot-reload |
| `make down` | Hentikan container |
| `make clean` | Hentikan container dan hapus volume dev |
| `make build` | Rebuild semua image |
| `make logs` | Pantau log semua container |
| `make ps` | Cek status container |
| `make restart` | Restart container |
| `make test` | Jalankan unit test lokal (Go + Vitest) |
