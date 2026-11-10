# Progress

_Terakhir diperbarui: 2026-11-10 (v2)_

Ringkasan status v2. Riwayat keputusan lengkap ada di `docs/DECISIONS.md`;
pembaruan terbaru dokumen ini ada di `STATUS.md`.

## v2, 10 November 2026

| Bagian | Status | Catatan |
|---|---|---|
| Penggabungan cabang kami dengan `main` Fujimaru | Selesai | cabang `amba-v2`; catatan keputusan per berkas ada di riwayat commit |
| Video: TikTok, YouTube Shorts, Instagram, Facebook, X | Selesai | `yt-dlp` + `ffmpeg`, 6 bingkai, 60 detik suara |
| Pengakuan pembuat (`creator_disclosure`) | Selesai | kuat memberi skor minimal 85, lemah +8 |
| Pencarian jejak web (`web_evidence`) | Selesai | Tavily, 26 situs, butuh `TAVILY_API_KEY` |
| Cek fakta, topik bahasa Indonesia | Selesai | `grounding.SameTopic` |
| Validator tautan dan penanda domain tiruan | Selesai | `security.ValidateInputURL`, `LOOKALIKE_BRAND` |
| Header nginx (CSP `frame-ancestors`, HSTS, `/_app/`) | Selesai | `frontend/nginx.conf`, diuji di `nginx.test.ts` |
| Pembersihan frontend | Selesai | ukuran teks, Jelajah/Esensial, bagikan laporan, Balas sopan, kartu bukti dihapus |
| Kalibrasi ambang video | Selesai | 55 / 70 / 85, bukti di `docs/DECISIONS.md` |
| Akurasi 90 persen | **Belum tercapai** | lihat batas di bawah |

## Batas yang diketahui

- Akurasi belum diklaim 90 persen: pada 33 klip berlabel (tiga putaran) berita
  asli ditandai 3 persen, klip AI 53 persen. Pencarian web belum terukur.
- Port selain 80 dan 443 serta tanda petik terenkode di path ditolak.
- Temuan `govulncheck` yang tersisa menunggu Go 1.25 atau lebih baru.
- Hasil lama di cache browser tidak punya `creator_disclosure` dan
  `web_evidence`.
- e2e Playwright tidak diulang setelah penggabungan akhir.

## Cara memeriksa

```bash
cd backend  && gofmt -l . && go vet ./... && go test -race ./...
cd backend  && go run ./cmd/gendemo && git status --porcelain   # harus kosong
cd frontend && npm run typecheck && npm run lint && npm run test
cd frontend && npm run build && npm run size && npm run contrast && npm run headers
cd frontend && npm run test:e2e
```

## Riwayat singkat sebelum v2

- 8 November 2026: semua requirement PRD (P0, P1, P2) terpasang; jalur deploy
  disiapkan (`docs/DEPLOY.md`).
- 9 November 2026: ruang lingkup dipersempit ke tautan dan gambar; Jejak AI,
  Timpa Teks, umur domain, dan aturan penipuan dikalibrasi (lihat
  `docs/DECISIONS.md`).
