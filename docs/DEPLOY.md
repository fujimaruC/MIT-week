# Deploy AMBA ke produksi

Panduan ini untuk satu host Docker biasa (VPS, mini PC, server rumah) yang
dipublikasikan lewat **Cloudflare Tunnel**. Tidak perlu membuka port masuk di
router atau firewall. Alamat publik yang dipakai: **https://amba.madewadigital.com**.

Hosting statis terpisah (Netlify) dibahas di
[`DEPLOY-NETLIFY.md`](DEPLOY-NETLIFY.md): isinya cara memasang hanya frontend di
Netlify dengan API Go di server lain, plus catatan pencarian berita tanpa kunci.

```
Pengguna ── HTTPS ──► Cloudflare ── tunnel ──► cloudflared (di host)
                                                  │ http://localhost:80
                                                  ▼
                                   Docker Compose di host
                                   ├─ frontend (nginx: SPA + proxy /api/)
                                   └─ backend  (Go API, yt-dlp, ffmpeg)
                                          ├─► LLM gateway (kompatibel OpenAI)
                                          ├─► Tavily
                                          └─► Safe Browsing, Fact Check Tools, RDAP (opsional)
```

Browser hanya bicara ke satu origin. nginx menyajikan SPA dan meneruskan
`/api/` ke Go tanpa buffering, jadi aliran SSE (`stage`, `partial`, `verdict`)
sampai ke browser secara progresif.

## 1. Prasyarat

- Host Linux (atau Windows dengan Docker Desktop) dengan Docker dan Docker
  Compose v2. Kebutuhan sedang: 1 vCPU dan 1 GB RAM cukup untuk demo; analisis
  video menjalankan `yt-dlp` dan `ffmpeg` di dalam kontainer API.
- Domain di Cloudflare dan akun Cloudflare.
- Kunci layanan (lihat bagian 2). Simpan hanya di `.env` di host, jangan di git.

## 2. Kunci dan konfigurasi

Salin contoh lalu isi:

```bash
git clone <alamat-repo> amba && cd amba
cp .env.example .env
```

Isi minimal untuk produksi:

| Variabel | Wajib | Keterangan |
|---|---|---|
| `ENVIRONMENT` | ya | `production`. Server menolak boot bila syarat di bawah tidak terpenuhi. |
| `PUBLIC_APP_URL` | ya | `https://amba.madewadigital.com`, tanpa garis miring di akhir. Menjadi origin CORS. |
| `LLM_MOCK` | ya | `false`. Model tiruan mengarang temuan; produksi menolaknya. |
| `REPORT_SIGNING_KEY` | ya | Minimal 32 byte acak: `openssl rand -base64 48`. Mengganti kunci membatalkan token lama. |
| `NINEROUTER_API_KEY`, `NINEROUTER_BASE_URL`, `NINEROUTER_MODEL` | ya | Gateway LLM utama yang kompatibel OpenAI. Nama variabelnya lama; isinya bisa router mana pun. Model harus menerima gambar dan keluaran `json_schema`. |
| `LLM_FALLBACK_BASE_URL`, `LLM_FALLBACK_API_KEY`, `LLM_FALLBACK_MODEL` | disarankan | Gateway cadangan (misalnya OpenRouter). Isi ketiganya atau kosongkan semua. |
| `TAVILY_API_KEY` | disarankan | Mengaktifkan pencarian jejak web untuk video dan cek fakta tanpa kunci Google. |
| `GOOGLE_API_KEY` | opsional | Fact Check Tools dan Safe Browsing. |
| `VIDEO_TIMEOUT_MS` | tidak | Bawaan `60000`: anggaran satu audit video (unduh, bingkai, model). |
| `HARD_TIMEOUT_MS` | tidak | Bawaan `20000`: batas audit gambar atau tautan biasa. |

Daftar lengkap beserta komentar ada di [`.env.example`](../.env.example).
Variabel `NINEROUTER_REASONING_EFFORT` (bawaan `low`) dikirim sebagai
`reasoning_effort`; kalau gateway Anda menolaknya, set `LLM_EXTRA_BODY` dengan
kolom `reasoning` sendiri dan variabel itu dilewati.

Kebijakan pemutus sirkuit gateway (hanya aktif bila gateway cadangan diisi):
bila gateway utama gagal karena tidak tersambung, ditolak (401, 403, 429),
error server (5xx), atau terlalu lambat dalam anggaran 15 detik ke atas, ia
dilewati selama 60 detik dan permintaan langsung ke cadangan. Jawaban rusak
atau 4xx lain dianggap masalah permintaan itu saja.

## 3. Jalankan stack

```bash
docker compose up -d --build
docker compose ps
curl -s http://localhost/healthz        # {"status":"ok"}
```

Compose membaca `.env`. Setiap `docker compose up` memakai nilai dari `.env`
itu; nilai bawaan di `docker-compose.yml` adalah mode mock tanpa kunci, jadi
bila `.env` hilang aplikasi diam-diam turun ke mock. Periksa bahwa log boot
menyebut `readiness` dengan model terjangkau:

```bash
docker compose logs backend | grep readiness
```

Port 8080 ikut diterbitkan oleh Compose. Di host produksi, batasi ke localhost
(ubah pemetaan `ports` backend menjadi `127.0.0.1:8080:8080`) atau blokir lewat
firewall, supaya satu-satunya jalan masuk publik adalah tunnel ke nginx.

## 4. Cloudflare Tunnel

1. Pasang `cloudflared` di host dan masuk: `cloudflared tunnel login`.
2. Buat tunnel: `cloudflared tunnel create amba`.
3. Tulis konfigurasi tunnel (`config.yml`) dengan satu aturan ingress:

   ```yaml
   tunnel: <id-tunnel>
   credentials-file: <jalur-ke-berkas-kredensial-tunnel>
   ingress:
     - hostname: amba.madewadigital.com
       service: http://localhost:80
     - service: http_status:404
   ```

4. Arahkan DNS: `cloudflared tunnel route dns amba amba.madewadigital.com`.
5. Jalankan sebagai layanan: `cloudflared service install` (atau jalankan
   `cloudflared tunnel run amba` dari manajer proses Anda).

Berkas kredensial tunnel adalah rahasia: jangan masukkan ke repo.

Catatan SSE: Cloudflare menutup koneksi yang diam lama. API mengirim
komentar `: ping` setiap 10 detik, dan nginx memakai `proxy_buffering off`,
jadi audit video sampai 60 detik tetap mengalir.

## 5. Uji setelah deploy

```bash
APP=https://amba.madewadigital.com

curl -s $APP/healthz
curl -sI $APP | grep -i -E "strict-transport|x-content-type|referrer-policy|content-security|x-frame"

# Audit tautan, aliran SSE: stage, partial, verdict, done
curl -N -X POST $APP/api/v1/audit/url \
  -H "Content-Type: application/json" -H "Accept: text/event-stream" \
  -d '{"url":"https://bri-verifikasi.my.id/login"}'

# Penolakan validator tautan: harus 400 INVALID_URL
curl -s -X POST $APP/api/v1/audit/url -H "Content-Type: application/json" \
  -d '{"url":"javascript:alert(1)"}'

# Audit gambar (JPG, PNG, WebP, maksimal 10 MB)
curl -N -X POST $APP/api/v1/audit/file -H "Accept: text/event-stream" -F "file=@foto.jpg"
```

Lalu coba satu tautan video asli di peramban. `GET /api/v1/ops/status`
menunjukkan integrasi mana yang aktif (gateway, pencarian, Safe Browsing)
tanpa membuka kunci.

## 6. Memperbarui

```bash
git pull
docker compose build
docker compose up -d --force-recreate
```

`yt-dlp` dipatok ke satu versi di `backend/Dockerfile` (dengan checksum).
Platform sering mengubah format, jadi saat unduhan video mulai gagal, naikkan
versi dan checksum di berkas itu lalu build ulang.

## 7. Operasional

- **Log** berformat JSON dan tidak memuat IP, URL, teks, judul, nama berkas,
  maupun kunci. Kegagalan layanan luar hanya mencatat alasan dan kode HTTP.
- **Metrik** anonim (opsional): `METRICS_ADDR=127.0.0.1:9091` membuka teks
  Prometheus di listener internal. Isinya kode aturan, tidak pernah teks atau
  tautan.
- **Rate limit:** 30 permintaan per menit per klien (burst 10) dan 32 audit
  bersamaan. nginx mengisi `X-Real-IP` dari alamat penyambung; di belakang
  tunnel itu alamat `cloudflared`, bukan pengguna. Kalau `TRUSTED_PROXY_CIDRS`
  dan `CLIENT_IP_HEADER` tidak diisi, semua pengguna berbagi satu ember.
  Untuk batas per pengguna, nginx perlu meneruskan `CF-Connecting-IP` sebagai
  `X-Real-IP` dan `TRUSTED_PROXY_CIDRS` diisi rentang jaringan Docker; ini
  belum diatur di repo dan belum diuji pada host produksi.
- **Cache** hasil dan rate limit ada di memori kontainer; restart
  menghapusnya. Tidak ada basis data.
- **Bot Telegram** opsional. `nginx.conf` tidak meneruskan `/telegram/webhook`,
  jadi pakai `TELEGRAM_MODE=polling` atau biarkan bot mati (kosongkan token).
  Jangan menjalankan polling dengan token yang sama di dua tempat.

## 8. Masalah umum

| Gejala | Penyebab dan solusi |
|---|---|
| Boot gagal: `LLM_MOCK: must be false in production` | Set `LLM_MOCK=false` dan isi variabel gateway. |
| Boot gagal: `PUBLIC_APP_URL: must be an https:// origin` | Isi dengan `https://amba.madewadigital.com`. |
| Browser: error CORS | `PUBLIC_APP_URL` atau `CORS_EXTRA_ORIGINS` harus persis sama dengan origin (skema dan host, tanpa `/` di akhir). |
| Semua pengguna kena rate limit bersamaan | Lihat catatan rate limit di bagian 7. |
| Video selalu "tidak tersedia" | Periksa `docker compose logs backend`: video privat atau dikunci login, `yt-dlp` kedaluwarsa, atau mode mock aktif (mock tidak mengunduh video). |
| Log `status=401` atau `403` dari gateway | Kunci salah atau layanan belum diaktifkan untuk kunci itu. |
| Log `status=429` | Kuota gateway habis. Isi gateway cadangan. |
| Hasil selalu "Belum cukup data" | Periksa log `readiness`: model atau kunci pencarian belum siap. |
| Setelah `compose up` aplikasi jadi mode tiruan | `.env` tidak terbaca di direktori itu. Jalankan dari akar repo. |
