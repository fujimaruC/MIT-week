package tactics

import (
	"testing"

	"github.com/amba/amba/backend/internal/models"
)

func tactic(code, phrase string) models.Tactic {
	return models.Tactic{Code: code, Phrase: phrase, Source: "text"}
}

func TestVerifyKeepsOnlyRealSubstrings(t *testing.T) {
	source := "Bantuan Rp 500.000 dibagikan HARI INI SAJA. Jangan sampai ketinggalan!"
	got := Verify([]models.Tactic{
		tactic("URGENCY", "hari ini saja"),
		tactic("REWARD_BAIT", "Bantuan Rp 500.000"),
		tactic("FEAR", "akun Anda diblokir"), // not present
	}, source)

	if len(got) != 2 {
		t.Fatalf("kept %d tactics, want 2: %+v", len(got), got)
	}
	if got[0].Code != "URGENCY" || got[1].Code != "REWARD_BAIT" {
		t.Fatalf("unexpected tactics: %+v", got)
	}
}

func TestVerifyIsCaseAndWhitespaceInsensitive(t *testing.T) {
	source := "Akun   ANDA akan DIBLOKIR."
	got := Verify([]models.Tactic{tactic("FEAR", "akun anda akan diblokir")}, source)
	if len(got) != 1 {
		t.Fatalf("expected a match despite case/whitespace, got %+v", got)
	}
	// The emitted phrase must be the user's own characters, not the candidate's.
	if got[0].Phrase != "Akun   ANDA akan DIBLOKIR" {
		t.Errorf("phrase = %q, want the source spelling", got[0].Phrase)
	}
}

func TestVerifyDropsUnknownCodes(t *testing.T) {
	got := Verify([]models.Tactic{tactic("NOT_A_REAL_CODE", "halo")}, "halo")
	if len(got) != 0 {
		t.Fatalf("unknown code kept: %+v", got)
	}
}

func TestVerifyDropsLongPhrases(t *testing.T) {
	long := ""
	for i := 0; i < 81; i++ {
		long += "a"
	}
	got := Verify([]models.Tactic{tactic("URGENCY", long)}, long)
	if len(got) != 0 {
		t.Fatalf("phrase over 80 chars kept: %+v", got)
	}
}

func TestVerifyCapsAtFive(t *testing.T) {
	source := "satu dua tiga empat lima enam"
	candidates := []models.Tactic{
		tactic("URGENCY", "satu"), tactic("FEAR", "dua"), tactic("SECRECY", "tiga"),
		tactic("OUTRAGE_BAIT", "empat"), tactic("IMPERSONATION", "lima"),
		tactic("REWARD_BAIT", "enam"),
	}
	if got := Verify(candidates, source); len(got) != MaxTactics {
		t.Fatalf("kept %d, want %d", len(got), MaxTactics)
	}
}

func TestVerifyDeduplicates(t *testing.T) {
	got := Verify([]models.Tactic{
		tactic("URGENCY", "sekarang"),
		tactic("URGENCY", "sekarang"),
	}, "verifikasi sekarang juga")
	if len(got) != 1 {
		t.Fatalf("duplicate kept: %+v", got)
	}
}

func TestVerifyEmptySource(t *testing.T) {
	if got := Verify([]models.Tactic{tactic("URGENCY", "x")}, ""); len(got) != 0 {
		t.Fatalf("expected no tactics for empty source, got %+v", got)
	}
}

func TestVerifyRejectsInvalidSourceField(t *testing.T) {
	got := Verify([]models.Tactic{{Code: "URGENCY", Phrase: "halo", Source: "guess"}}, "halo")
	if len(got) != 0 {
		t.Fatalf("invalid source kept: %+v", got)
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"  Hari   INI  ": "hari ini",
		"HARI\tINI":      "hari ini",
		"":               "",
		"Sudah":          "sudah",
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVerifyAcceptsOCRSource(t *testing.T) {
	got := Verify([]models.Tactic{{Code: "URGENCY", Phrase: "hari ini", Source: "ocr"}}, "promo hari ini")
	if len(got) != 1 || got[0].Source != "ocr" {
		t.Fatalf("ocr tactic not kept: %+v", got)
	}
}

// The phrase comes back without the space that preceded it in the message.
func TestPhraseHasNoLeadingSpace(t *testing.T) {
	got := Verify([]models.Tactic{{Code: "URGENCY", Phrase: "segera", Source: "text"}}, "Transfer sekarang, segera ya.")
	if len(got) != 1 || got[0].Phrase != "segera" {
		t.Fatalf("got %+v", got)
	}
}
