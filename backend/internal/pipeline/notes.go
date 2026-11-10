package pipeline

import (
	"strings"

	"github.com/amba/amba/backend/internal/models"
)

// Plain-language notes for the verdict. They say what was looked at and what
// was found, never that something "could not be checked" (Sir, 10 Nov: every
// finished audit ends in a clear conclusion).

// simpleMax is the contract's limit for explanation_simple.
const simpleMax = 140

// VideoNote says what a video link was judged on when its footage was not
// read. It names the basis, not the failure, and lists only what really ran:
// an X post behind a login wall has no title or preview to cite.
func VideoNote(res Result) string {
	if res.Video == nil || res.Video.Status == models.VideoOK {
		return ""
	}
	var used []string
	if res.Metadata != nil && res.Metadata.Status == models.MetadataOK {
		used = append(used, "judul dan keterangan")
	}
	if res.Pillars.AI.Available {
		used = append(used, "gambar pratinjau")
	}
	if res.Pillars.Fact.Available {
		used = append(used, "pencarian cek fakta")
	}
	if res.Pillars.Link.Available {
		used = append(used, "alamat tautan")
	}
	switch n := len(used); n {
	case 0:
		return ""
	case 1:
		return "Penilaian memakai " + used[0] + "."
	case 2:
		return "Penilaian memakai " + used[0] + " dan " + used[1] + "."
	default:
		return "Penilaian memakai " + strings.Join(used[:n-1], ", ") + ", dan " + used[n-1] + "."
	}
}

// FactNote says what the fact check found, in one sentence; "" when no claim
// was checked.
func FactNote(res Result) string {
	f := res.Pillars.Fact
	if !f.Available {
		if res.InputType == models.InputFile {
			return "Gambar ini tidak memuat klaim yang bisa dicek."
		}
		return "Tidak ada klaim yang bisa dicek."
	}
	// Same-topic articles that settle nothing are not shown on the result page,
	// so the sentence does not mention them either.
	switch f.Status {
	case models.FactContradicts:
		return "Ada artikel cek fakta yang membantah klaim ini."
	case models.FactSupports:
		return "Artikel terpercaya sejalan dengan klaim ini."
	default:
		return "Belum ada artikel cek fakta yang membantah atau membenarkan klaim ini."
	}
}

// safeSimple is the short verdict line for SAFE: what was checked and found
// nothing, plus the one note that tells how strong that is, within the
// contract's 140 characters.
func safeSimple(res Result) string {
	var found []string
	if res.Pillars.AI.Available {
		found = append(found, "tanda AI")
	}
	if res.Pillars.ELA.Available {
		found = append(found, "suntingan")
	}
	if res.Pillars.Fact.Available {
		found = append(found, "bantahan cek fakta")
	}
	if res.Pillars.Link.Available {
		found = append(found, "tanda bahaya pada tautan")
	}
	lead := "Tidak ditemukan tanda bahaya."
	switch len(found) {
	case 0:
	case 1:
		lead = "Tidak ditemukan " + found[0] + "."
	default:
		lead = "Tidak ditemukan " + strings.Join(found[:len(found)-1], ", ") + " atau " + found[len(found)-1] + "."
	}
	note := VideoNote(res)
	if note == "" {
		note = FactNote(res)
	}
	out := lead
	if note != "" && len([]rune(lead+" "+note)) <= simpleMax {
		out = lead + " " + note
	}
	return truncateRunes(out, simpleMax)
}
