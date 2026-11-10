# STATUS: AMBA v2

Untuk anggota tim. Tanggal: 10 November 2026 (batas lomba 23:59 WITA).
Sumber kebenaran tetap `instruction/TECH_SPEC_AMBA.md`; kalau berbeda dengan
dokumen ini, spesifikasi teknis yang menang. Alamat publik:
https://amba.madewadigital.com

## Sudah selesai

- Cabang `amba-v2` menyatukan dua jalur kerja: detektor video dan gateway LLM
  dari cabang kami, serta tambahan Fujimaru (`ops/status`, pagefetch, statistik).
- Tautan video TikTok, YouTube Shorts, Instagram, Facebook, dan X: `yt-dlp` +
  `ffmpeg`, 6 bingkai dan 60 detik suara, dianalisis model.
- Pengakuan pembuat (`creator_disclosure`): hashtag dan frasa AI dari pembuat.
- Pencarian jejak di web lewat Tavily, dibatasi 26 situs berita dan cek fakta
  (`web_evidence`, paling banyak 5).
- Cek fakta dengan pencocokan topik bahasa Indonesia (`SameTopic`).
- Gambar: cek buatan AI, teks ditimpa (ELA), keamanan tautan dengan penanda
  domain tiruan (`LOOKALIKE_BRAND`).
- Validator tautan satu pintu (`security.ValidateInputURL`) dan ekstraksi tautan
  dari teks bagikan di browser.
- Header nginx: CSP `frame-ancestors 'none'` dan HSTS, termasuk di `/_app/`.
- Fitur yang dihapus: ukuran teks, mode Jelajah/Esensial, bagikan laporan,
  Balas sopan (`/reply`), kartu bukti.
- Ambang skor: video AI 55, manipulasi 70, konteks 85; gambar AI 80.

## Pemeriksaan terakhir

Diulang pada kepala `v2-docs` (sama dengan `amba-v2` 36345a1) untuk dokumen ini:

- `go test -count=1 ./...` di `backend/`: seluruh 21 paket bertes lulus.

Hasil yang dilaporkan pekerja backend dan frontend sebelum digabung (belum
diulang di sini): gofmt dan `go vet` bersih; `go test -race` di docker
golang:1.24 lulus; typecheck, lint, Vitest, dan build frontend lulus;
`npm audit --omit=dev` 0 kerentanan; `docker compose build` berhasil.

## Batas yang diketahui

- Akurasi 90 persen **belum diklaim**. Set uji 33 klip berlabel, tiga putaran:
  berita asli ditandai 1 dari 33 bacaan, klip AI 16 dari 30 bacaan (53 persen),
  hoaks 7 dari 36. Pencarian web dan cek fakta belum terukur di set itu.
- Pengakuan pembuat yang kuat sendirian sudah menghasilkan skor minimal 85.
- `/audit/url` menolak port selain 80 dan 443, dan karakter berbahaya di path.
- `govulncheck`: sisa temuan ada di pustaka standar Go 1.24 dan paket `x/*`
  yang butuh Go 1.25 atau lebih baru. Naik toolchain setelah lomba.
- Hasil lama di cache browser tidak punya kolom video baru; skema
  menganggapnya opsional.
- Playwright e2e dijalankan pekerja frontend (20 lulus, 2 hanya pada percobaan
  ulang karena rate limit); tidak diulang setelah penggabungan akhir.
- Pencarian web memakai satu kredit Tavily per video.

## Menjalankan

```bash
docker compose up --build     # web http://localhost, API http://localhost:8080
```

Detail ada di `README.md`, `docs/DOCKER.md`, dan `docs/DEPLOY.md`.
