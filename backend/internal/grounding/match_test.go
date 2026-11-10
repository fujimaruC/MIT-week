package grounding

import "testing"

func TestIsFactCheck(t *testing.T) {
	for _, tc := range []struct {
		title, url string
		want       bool
	}{
		{"Video Pasukan PBB", "https://turnbackhoax.id/2026/11/01/salah-video-pasukan-pbb/", true},
		{"[HOAKS] Wapres Gibran Serahkan Surat", "https://www.kompas.com/cekfakta/read/2026/05/11/175600982/x", true},
		{"Hoaks! video Wapres Gibran mengundurkan diri", "https://www.antaranews.com/berita/5258633/hoaks-video-wapres", true},
		{"[HOAKS] Gibran Mengundurkan Diri", "https://www.komdigi.go.id/berita/berita-hoaks/detail/hoaks-gibran", true},
		// Seen as false "contradictions" on 9 Nov:
		{"Harga BBM di SPBU BP AKR Hari Ini, Naik Semua!", "https://www.liputan6.com/bisnis/read/5859493/harga-bbm", false},
		{"CekFakta", "https://cekfakta.com/institusi/19", false},
		{"spbu bp - Kumpulan Berita ANTARA News", "https://www.antaranews.com/tag/spbu-bp/8", false},
	} {
		if got := IsFactCheck(tc.title, tc.url); got != tc.want {
			t.Errorf("IsFactCheck(%q, %q) = %v, want %v", tc.title, tc.url, got, tc.want)
		}
	}
}

func TestSameTopic(t *testing.T) {
	for _, tc := range []struct {
		claim, text string
		want        bool
	}{
		{"Gibran Rakabuming mengundurkan diri dari jabatan Wakil Presiden Republik Indonesia.",
			"[HOAKS] Gibran Mengundurkan Diri sebagai Wakil Presiden Terpilih", true},
		{"Prabowo menang karena adanya Gibran", "Prabowo menang pilpres berkat Gibran, benarkah?", true},
		// Seen as false "contradictions" on 9 Nov:
		{"The Birth of a New Life - Witness the magic of creation!",
			"Cek fakta, bayi WNI baru lahir otomatis masuk peserta BPJS", false},
		{"BP Global Investments Limited melepas seluruh bisnis SPBU di Indonesia",
			"Harga BBM di SPBU BP AKR Hari Ini, Naik Semua!", false},
		{"Akun akan diblokir", "[HOAKS] Akun WhatsApp akan diblokir", true},
		{"", "apa saja", false},
	} {
		if got := SameTopic(tc.claim, tc.text); got != tc.want {
			t.Errorf("SameTopic(%q, %q) = %v, want %v", tc.claim, tc.text, got, tc.want)
		}
	}
}

func TestSameTopicIndonesianMorphology(t *testing.T) {
	for _, tc := range []struct {
		name, claim, text string
		want              bool
	}{
		{"gibran mundur vs mengundurkan diri",
			"Gibran mundur dari wapres", "Wakil Presiden Gibran mengundurkan diri, benarkah?", true},
		{"gibran pengunduran",
			"Wapres Gibran mundur setelah demo", "Hoaks: pengunduran diri Wakil Presiden Gibran Rakabuming", true},
		{"gibran claim long form",
			"Gibran Rakabuming mundur sebagai wakil presiden", "[HOAKS] Video Wapres Gibran Mengundurkan Diri", true},
		{"tewas meninggal",
			"Artis Vidi Aldiano tewas dalam kecelakaan", "Vidi Aldiano meninggal dunia? Ini faktanya", true},
		{"hoax hoaks spelling",
			"Hoax bansos Rp 600 ribu cair lewat WhatsApp", "Hoaks bansos 600 ribu dicairkan lewat WhatsApp", true},
		{"prefix me- pe-",
			"Polisi menangkap pelaku pembunuhan di Bekasi", "Penangkapan pembunuh di Bekasi oleh kepolisian", true},
		{"entity plus one stem",
			"Prabowo meresmikan bendungan", "Presiden Prabowo's bendungan baru diresmikan", true},
		{"different person same verb",
			"Gibran mundur dari wapres", "Menteri Keuangan mengundurkan diri dari kabinet", false},
		{"same person other event",
			"Gibran mundur dari wapres", "Gibran meresmikan sekolah baru di Solo", false},
		{"only a shared generic word",
			"Harga cabai naik di Jakarta", "Harga emas dunia turun hari ini", false},
		{"one shared acronym only",
			"BP melepas bisnis SPBU", "Harga BBM di SPBU Pertamina turun", false},
		{"english unrelated",
			"Tiktok video shows tsunami in Japan", "Fact check: Tokyo marathon winner banned", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameTopic(tc.claim, tc.text); got != tc.want {
				t.Fatalf("SameTopic = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStem(t *testing.T) {
	for in, want := range map[string]string{
		"mengundurkan": "undur", "pengunduran": "undur", "mundur": "undur",
		"wapres": "wapres", "wafat": "tewas", "meninggal": "tewas", "hoax": "hoaks",
		"gibran": "gibran", "kecelakaan": "celaka",
	} {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSharesEntityNeedsTheSubjectsName(t *testing.T) {
	subject := "Apa yang kita sampaikan harus berdasarkan kebenaran dan fakta | Kang Dedi Mulyadi"
	if SharesEntity(subject, "Hoaks! Video Anies jadi pemimpin aksi penolakan hasil Pemilu 2024") {
		t.Error("a debunk about someone else must not count")
	}
	if SharesEntity(subject, "Pemerintah harus antisipasi hoaks diproduksi AI selama Pilkada berdasarkan kebenaran") {
		t.Error("shared everyday words must not count")
	}
	if !SharesEntity(subject, "[SALAH] Video Dedi Mulyadi Minta Maaf ke Presiden") {
		t.Error("the subject's own name must count")
	}
	if !SharesEntity("MBG kok begini amat programnya", "[SALAH] Video Mahasiswa Tolak Program MBG Selama Ramadan") {
		t.Error("an all-caps programme name must count")
	}
	if SharesEntity("detik-detik banjir menerjang desa", "Banjir di Desa") {
		t.Error("a subject that names nobody has no entity to share")
	}
}
