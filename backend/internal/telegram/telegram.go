// Package telegram is AMBA's Telegram entry point (PRD P2): it exists for
// people whose phone has no Web Share Target (iOS) and who already forward
// suspicious messages in chat apps. It runs the same pipeline and scoring as
// the web app and answers with a short verdict and a signed report link.
//
// Privacy rules match the API: message content, chat ids and user names are
// never logged or stored; the per-chat rate limiter keys on a salted hash.
package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/pipeline"
	"github.com/amba/amba/backend/internal/security"
)

// AuditFunc runs one audit and returns the signed Dossier.
type AuditFunc func(ctx context.Context, req pipeline.Request) models.Dossier

// Bot handles Telegram updates.
type Bot struct {
	Token        string
	Secret       string
	APIBase      string // https://api.telegram.org, overridable in tests
	PublicAppURL string
	Audit        AuditFunc
	HTTP         *http.Client
	Timeout      time.Duration
	MaxUploadMB  int64
	Metrics      interface{ Inc(string, ...string) }

	sem   chan struct{}
	salt  []byte
	mu    sync.Mutex
	chats map[string]*rate.Limiter
}

// New builds a bot. Audits per chat are limited to 6 per minute (burst 3)
// and at most 8 run at once across all chats.
func New(token, secret, publicAppURL string, audit AuditFunc) *Bot {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	return &Bot{
		Token:        token,
		Secret:       secret,
		APIBase:      "https://api.telegram.org",
		PublicAppURL: strings.TrimRight(publicAppURL, "/"),
		Audit:        audit,
		HTTP:         &http.Client{Timeout: 35 * time.Second},
		Timeout:      12 * time.Second,
		MaxUploadMB:  10,
		sem:          make(chan struct{}, 8),
		salt:         salt,
		chats:        map[string]*rate.Limiter{},
	}
}

// Update is the subset of a Telegram update AMBA reads.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is the subset of a Telegram message AMBA reads.
type Message struct {
	MessageID int64       `json:"message_id"`
	Chat      Chat        `json:"chat"`
	Text      string      `json:"text"`
	Caption   string      `json:"caption"`
	Photo     []PhotoSize `json:"photo"`
	Document  *Document   `json:"document"`
}

// Chat identifies where to answer.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// PhotoSize is one rendition of a photo.
type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// Document is a file sent "as file" (uncompressed screenshot).
type Document struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// WebhookHandler accepts updates pushed by Telegram. It answers 200 at once
// and audits in the background, because Telegram retries slow webhooks.
func (b *Bot) WebhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if b.Secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(b.Secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var u Update
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&u); err != nil {
			w.WriteHeader(http.StatusOK) // a malformed update is not worth a retry
			return
		}
		w.WriteHeader(http.StatusOK)
		go b.Handle(context.Background(), u)
	})
}

// Poll runs a getUpdates long-poll loop until ctx ends (local development,
// or a host without a public URL). It deletes any webhook first, because
// Telegram refuses getUpdates while one is set.
func (b *Bot) Poll(ctx context.Context) {
	_ = b.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil)
	var offset int64
	for ctx.Err() == nil {
		var updates []Update
		err := b.call(ctx, "getUpdates", map[string]any{
			"offset": offset, "timeout": 25, "allowed_updates": []string{"message"},
		}, &updates)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			go b.Handle(ctx, u)
		}
	}
}

// SetWebhook registers the webhook URL with Telegram.
func (b *Bot) SetWebhook(ctx context.Context, webhookURL string) error {
	return b.call(ctx, "setWebhook", map[string]any{
		"url":             webhookURL,
		"secret_token":    b.Secret,
		"allowed_updates": []string{"message"},
		"max_connections": 20,
	}, nil)
}

// Handle answers one update.
func (b *Bot) Handle(ctx context.Context, u Update) {
	m := u.Message
	if m == nil {
		return
	}
	text := strings.TrimSpace(m.Text)
	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text, " ", 2)
		command := strings.SplitN(parts[0], "@", 2)[0] // "/cek@AmbaBot" -> "/cek"
		switch command {
		case "/start", "/help":
			b.inc("command")
			b.send(ctx, m.Chat.ID, m.MessageID, welcome)
			return
		case "/cek":
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				b.send(ctx, m.Chat.ID, m.MessageID, "Tulis /cek lalu tempel tautannya, atau kirim gambarnya langsung.")
				return
			}
			text = strings.TrimSpace(parts[1])
		default:
			b.send(ctx, m.Chat.ID, m.MessageID, "Perintah tidak dikenal. Kirim tautan atau gambar yang ingin diperiksa.")
			return
		}
	}

	if !b.allow(m.Chat.ID) {
		b.inc("rate_limited")
		b.send(ctx, m.Chat.ID, m.MessageID, "Terlalu banyak pemeriksaan dari obrolan ini. Coba lagi sebentar lagi, ya.")
		return
	}
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
	default:
		b.send(ctx, m.Chat.ID, m.MessageID, "AMBA sedang sibuk. Coba kirim lagi sebentar lagi, ya.")
		return
	}

	req, problem := b.request(ctx, m, text)
	if problem != "" {
		b.send(ctx, m.Chat.ID, m.MessageID, problem)
		return
	}
	b.inc(strings.ToLower(string(req.InputType)))
	b.typing(ctx, m.Chat.ID)

	actx, cancel := context.WithTimeout(ctx, b.Timeout)
	defer cancel()
	d := b.Audit(actx, req)
	b.send(ctx, m.Chat.ID, m.MessageID, b.Format(d))
}

// request turns a message into a pipeline request, or a user-facing reason
// why it cannot be checked.
func (b *Bot) request(ctx context.Context, m *Message, text string) (pipeline.Request, string) {
	fileID := ""
	switch {
	case len(m.Photo) > 0:
		largest := m.Photo[0]
		for _, p := range m.Photo {
			if p.Width*p.Height > largest.Width*largest.Height {
				largest = p
			}
		}
		fileID = largest.FileID
	case m.Document != nil && strings.HasPrefix(m.Document.MimeType, "image/"):
		fileID = m.Document.FileID
	case m.Document != nil:
		return pipeline.Request{}, "Jenis berkas ini belum didukung. Kirim gambar JPG, PNG, atau WebP."
	}

	if fileID != "" {
		data, format, err := b.download(ctx, fileID)
		if err != nil {
			return pipeline.Request{}, "Gambar tidak bisa dibaca. Pastikan berupa JPG, PNG, atau WebP dan maksimal 10 MB."
		}
		return pipeline.Request{InputType: models.InputFile, Image: data, ImageFormat: format, OCRText: strings.TrimSpace(m.Caption)}, ""
	}

	// AMBA checks links and images only. A message that carries a link is
	// checked by that link; text without one is not something AMBA checks.
	link := pipeline.FirstURL(text)
	if link == "" {
		return pipeline.Request{}, "AMBA memeriksa tautan dan gambar. Kirim tautannya (diawali http:// atau https://) atau gambarnya."
	}
	if len(link) > 2048 {
		return pipeline.Request{}, "Tautannya terlalu panjang untuk diperiksa."
	}
	if _, err := security.ValidateFetchURL(link); err != nil {
		return pipeline.Request{}, "Tautan ini mengarah ke alamat yang bukan situs publik, jadi ditolak demi keamanan."
	}
	return pipeline.Request{InputType: models.InputURL, URL: link}, ""
}

func (b *Bot) download(ctx context.Context, fileID string) ([]byte, string, error) {
	var file struct {
		FilePath string `json:"file_path"`
		FileSize int64  `json:"file_size"`
	}
	if err := b.call(ctx, "getFile", map[string]any{"file_id": fileID}, &file); err != nil {
		return nil, "", err
	}
	max := b.MaxUploadMB << 20
	if file.FilePath == "" || file.FileSize > max {
		return nil, "", errors.New("file unavailable or too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.APIBase+"/file/bot"+b.Token+"/"+file.FilePath, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("download failed")
	}
	data, format, _, err := security.ValidateUpload(resp.Body, max)
	return data, format, err
}

// AISummary says in one plain sentence what the AI pillar found about an
// image: what the image declares, or how likely it looks AI-made.
func AISummary(ai models.AIPillar, subject string) string {
	p := ai.Provenance
	by := ""
	if p.Generator != "" {
		by = " (" + p.Generator + ")"
	}
	made := "dibuat"
	if p.Kind == models.ProvenanceAIEdited {
		made = "diedit"
	}
	var out string
	switch {
	case p.Kind == models.ProvenanceAIGenerated || p.Kind == models.ProvenanceAIEdited:
		switch p.Source {
		case "c2pa":
			out = fmt.Sprintf("Content Credentials (C2PA) di file menyatakan %s %s dengan AI%s.", strings.ToLower(subject), made, by)
		case "watermark":
			out = fmt.Sprintf("Ada label AI yang terlihat pada %s%s.", strings.ToLower(subject), by)
		case "png_text":
			out = fmt.Sprintf("File menyimpan pengaturan pembuat gambar AI%s.", by)
		case "exif":
			out = fmt.Sprintf("Data EXIF di file menyebut pembuat gambar AI%s.", by)
		default:
			out = fmt.Sprintf("Metadata file menyatakan %s %s dengan AI%s.", strings.ToLower(subject), made, by)
		}
	default:
		level := "rendah"
		switch {
		case ai.Score >= 70:
			level = "tinggi"
		case ai.Score >= 40:
			level = "sedang"
		}
		out = fmt.Sprintf("%s kemungkinan dibuat dengan AI: %s (%d/100).", subject, level, ai.Score)
	}
	if ai.Detector != nil {
		out += fmt.Sprintf(" Pemeriksa khusus AI: %d/100.", ai.Detector.Score)
	}
	if p.Kind == models.ProvenanceCamera && p.Camera != "" {
		out += " Data kamera: " + p.Camera + " (petunjuk, bukan bukti)."
	}
	return out
}

// Format renders a Dossier as a plain-text Telegram message.
func (b *Bot) Format(d models.Dossier) string {
	var s strings.Builder
	score := "tanpa skor"
	if d.Verdict.Score != nil {
		score = fmt.Sprintf("skor risiko %d/100", *d.Verdict.Score)
	}
	fmt.Fprintf(&s, "Hasil AMBA: %s (%s)\n%s\n", d.Verdict.ShortTitle, score, d.Verdict.ExplanationSimple)

	if md := d.SourceMetadata; md != nil && md.Status == models.MetadataBlocked {
		s.WriteString("\nPlatform tidak mengizinkan pembacaan otomatis. Kirim tangkapan layarnya agar bisa diperiksa lengkap.\n")
	}
	if ai := d.Pillars.AI; ai.Available {
		subject := "Gambar ini"
		if d.InputType == models.InputURL {
			subject = "Gambar pratinjau tautan"
		}
		s.WriteString("\nJejak AI: " + AISummary(ai, subject) + "\n")
		for i, ind := range ai.Indicators {
			if i == 3 {
				break
			}
			s.WriteString("- " + ind + "\n")
		}
	}
	if ela := d.Pillars.ELA; ela.Available && len(ela.Anomalies) > 0 {
		s.WriteString("\nTeks yang tampak ditempel:\n")
		for i, a := range ela.Anomalies {
			if i == 3 {
				break
			}
			line := a.Reason
			if a.ExtractedText != "" {
				line = fmt.Sprintf("\"%s\": %s", a.ExtractedText, a.Reason)
			}
			if a.Evidence != nil && *a.Evidence >= 50 {
				line += " (kompresinya berbeda dari teks lain di gambar)"
			}
			s.WriteString("- " + line + "\n")
		}
	}
	if d.Pillars.Fact.Available && len(d.Pillars.Fact.Sources) > 0 {
		s.WriteString("\nSumber cek fakta:\n")
		for i, src := range d.Pillars.Fact.Sources {
			if i == 3 {
				break
			}
			fmt.Fprintf(&s, "- %s (%s): %s\n", src.Publisher, matchLabel(src.Match), src.URL)
		}
	}
	if d.Pillars.Link.Available && len(d.Pillars.Link.Reasons) > 0 {
		s.WriteString("\nTanda pada tautan:\n")
		for i, r := range d.Pillars.Link.Reasons {
			if i == 4 {
				break
			}
			s.WriteString("- " + r.Text + "\n")
		}
		if fu := d.Pillars.Link.FinalURL; fu != "" && len(d.Pillars.Link.RedirectChain) > 1 {
			s.WriteString("Tujuan akhir tautan: " + fu + "\n")
		}
	}
	if len(d.Tactics) > 0 {
		s.WriteString("\nPola yang perlu diwaspadai:\n")
		for _, t := range d.Tactics {
			fmt.Fprintf(&s, "- %s: \"%s\"\n", tacticLabels[t.Code], t.Phrase)
		}
	}
	if d.Degraded {
		s.WriteString("\nSebagian pemeriksaan tidak berjalan, jadi hasil ini belum lengkap.\n")
	}
	if d.ShareToken != "" && b.PublicAppURL != "" {
		fmt.Fprintf(&s, "\nLaporan lengkap: %s/hasil#t=%s\n", b.PublicAppURL, d.ShareToken)
	}
	s.WriteString("\nHasil ini indikatif, bukan alat bukti hukum.")
	out := s.String()
	if utf8.RuneCountInString(out) > 4000 {
		out = string([]rune(out)[:3990]) + "…"
	}
	return out
}

var tacticLabels = map[string]string{
	"URGENCY": "Mendesak", "REWARD_BAIT": "Iming-iming hadiah", "FALSE_AUTHORITY": "Mengaku otoritas",
	"FEAR": "Menakut-nakuti", "SECRECY": "Minta dirahasiakan", "OUTRAGE_BAIT": "Memancing emosi",
	"IMPERSONATION": "Menyamar", "FAKE_SOCIAL_PROOF": "Klaim banyak yang percaya",
}

func matchLabel(m models.FactStatus) string {
	switch m {
	case models.FactContradicts:
		return "membantah"
	case models.FactSupports:
		return "mendukung"
	default:
		return "belum terkait langsung"
	}
}

const welcome = `Halo, ini AMBA (Aplikasi Mengenali & Bertindak Aman).

Kirim ke sini tautan atau gambar yang mencurigakan. AMBA memeriksa apakah gambar dibuat dengan AI atau teksnya ditimpa, dan apakah tautan itu asli atau penipuan, lalu menjelaskannya dengan bahasa sederhana.

Privasi: tautan dan gambar yang kamu kirim hanya diproses sesaat dan tidak disimpan. Gambar, klaim, dan tautan dapat dikirim ke layanan pihak ketiga (AI dan Google) untuk dianalisis.

Hasil AMBA bersifat indikatif, bukan alat bukti hukum.`

func (b *Bot) allow(chatID int64) bool {
	sum := sha256.Sum256(append(b.salt, []byte(fmt.Sprint(chatID))...))
	key := hex.EncodeToString(sum[:8])
	b.mu.Lock()
	defer b.mu.Unlock()
	lim, ok := b.chats[key]
	if !ok {
		if len(b.chats) > 10000 {
			b.chats = map[string]*rate.Limiter{} // crude bound; limits reset
		}
		lim = rate.NewLimiter(rate.Every(10*time.Second), 3)
		b.chats[key] = lim
	}
	return lim.Allow()
}

func (b *Bot) typing(ctx context.Context, chatID int64) {
	_ = b.call(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": "typing"}, nil)
}

func (b *Bot) send(ctx context.Context, chatID, replyTo int64, text string) {
	params := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if replyTo != 0 {
		params["reply_parameters"] = map[string]any{"message_id": replyTo, "allow_sending_without_reply": true}
	}
	if err := b.call(ctx, "sendMessage", params, nil); err != nil {
		slog.Warn("telegram send failed") // never log the chat or the text
	}
}

func (b *Bot) inc(kind string) {
	if b.Metrics != nil {
		b.Metrics.Inc("amba_telegram_updates_total", "kind", kind)
	}
}

// call performs a Bot API method and decodes "result" into out.
func (b *Bot) call(ctx context.Context, method string, params map[string]any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.APIBase+"/bot"+url.PathEscape(b.Token)+"/"+method, bytes.NewReader(body))
	if err != nil {
		return errors.New("telegram request could not be built") // never wrap err: it quotes the URL
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return errors.New("telegram unreachable") // the URL holds the token: never wrap err
	}
	defer func() { _ = resp.Body.Close() }()
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return errors.New("telegram returned an unreadable response")
	}
	if !envelope.OK {
		return fmt.Errorf("telegram %s refused", method)
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
