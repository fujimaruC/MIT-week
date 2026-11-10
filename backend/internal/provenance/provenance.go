// Package provenance reads what an image file says about where it came from
// (PRD "Jejak AI"): Content Credentials (C2PA) manifests, the IPTC digital
// source type in XMP, the generation parameters Stable Diffusion, ComfyUI,
// NovelAI and InvokeAI write into PNG text chunks, and the camera or software
// named in EXIF.
//
// Only metadata blocks are read (JPEG segments before the image data, PNG
// and WebP chunks), never the pixels. Metadata is strong evidence when it is
// there, but WhatsApp, Instagram and most sites strip it, and anyone can
// write it: "no metadata" says nothing, and camera data is a hint, not proof.
package provenance

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"regexp"
	"strings"
)

// Kind is what the metadata says about the image's origin.
type Kind string

const (
	// AIGenerated: made by a generator (AI or another algorithm), not a camera.
	AIGenerated Kind = "AI_GENERATED"
	// AIEdited: a capture edited with generative AI (a composite).
	AIEdited Kind = "AI_EDITED"
	// Camera: the file names a camera, or declares a camera capture, and
	// carries no AI marker. Anyone can write this: it is a hint, not proof.
	Camera Kind = "CAMERA"
	// None: nothing about the origin.
	None Kind = "NONE"
)

// Result is the provenance read from one file.
type Result struct {
	Kind Kind
	// Generator is the AI tool named in the metadata, "" when none is named.
	Generator string
	// Source is the block that decided Kind: c2pa, xmp, png_text or exif
	// ("" for None).
	Source string
	// Camera is "Make Model" from EXIF or XMP, "" when absent.
	Camera string
	// C2PA reports a Content Credentials manifest, with or without AI.
	C2PA bool
}

// AI reports a metadata marker of AI generation or AI editing.
func (r Result) AI() bool { return r.Kind == AIGenerated || r.Kind == AIEdited }

// maxText bounds any one metadata text read, compressed or not.
const maxText = 1 << 20

// Scan reads the metadata blocks of a JPEG, PNG or WebP file.
func Scan(data []byte) Result {
	var b blocks
	switch {
	case len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8:
		b = jpegBlocks(data)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		b = pngBlocks(data)
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		b = webpBlocks(data)
	default:
		return Result{Kind: None}
	}
	return b.judge()
}

// blocks are the metadata blocks found in a file.
type blocks struct {
	c2pa []byte
	xmp  []string
	exif [][]byte
	text map[string]string // PNG text chunks, keyword -> value
}

func (b *blocks) addText(key, value string) {
	if b.text == nil {
		b.text = map[string]string{}
	}
	if len(value) > maxText {
		value = value[:maxText]
	}
	b.text[key] = value
	if key == "XML:com.adobe.xmp" {
		b.xmp = append(b.xmp, value)
	}
}

// judge applies the evidence in order of strength: an AI marker in any
// block, then a capture declared in Content Credentials or XMP, then camera
// make and model.
func (b blocks) judge() Result {
	r := Result{Kind: None, C2PA: len(b.c2pa) > 0}

	ex := exifInfo{}
	for _, raw := range b.exif {
		ex.merge(parseEXIF(raw))
	}
	xmpCam := exifInfo{}
	for _, x := range b.xmp {
		xmpCam.merge(exifInfo{make: xmpValue(xmpMake, x), model: xmpValue(xmpModel, x)})
	}
	pngCam := exifInfo{make: b.text["exif:Make"], model: b.text["exif:Model"]}
	camSource := ""
	switch {
	case ex.camera() != "":
		r.Camera, camSource = ex.camera(), "exif"
	case xmpCam.camera() != "":
		r.Camera, camSource = xmpCam.camera(), "xmp"
	case pngCam.camera() != "":
		r.Camera, camSource = pngCam.camera(), "png_text"
	}

	ai := func(kind Kind, source, generator string) Result {
		r.Kind, r.Source, r.Generator = kind, source, generator
		return r
	}
	capture := ""
	// 1. Content Credentials: the digital source type of the actions, and the
	// generator named in the manifest.
	if len(b.c2pa) > 0 {
		switch kind := sourceType(string(b.c2pa)); kind {
		case AIGenerated, AIEdited:
			return ai(kind, "c2pa", generatorIn(string(b.c2pa)))
		case Camera:
			capture = "c2pa"
		}
	}
	// 2. XMP: IPTC DigitalSourceType, or a generator as the creator tool.
	for _, x := range b.xmp {
		switch kind := sourceType(x); kind {
		case AIGenerated, AIEdited:
			return ai(kind, "xmp", generatorIn(xmpValue(creatorTool, x)+" "+x))
		case Camera:
			if capture == "" {
				capture = "xmp"
			}
		}
		if g := generatorIn(xmpValue(creatorTool, x)); g != "" {
			return ai(AIGenerated, "xmp", g)
		}
	}
	// 3. PNG text chunks written by image generators.
	if g := pngGenerator(b.text); g != "" {
		return ai(AIGenerated, "png_text", g)
	}
	// 4. EXIF: a generator as the software, or generation parameters in the
	// user comment or description.
	if g := generatorIn(ex.software); g != "" {
		return ai(AIGenerated, "exif", g)
	}
	if sdParams(ex.comment) || sdParams(ex.description) {
		return ai(AIGenerated, "exif", "Stable Diffusion")
	}
	switch {
	case capture != "":
		r.Kind, r.Source = Camera, capture
	case r.Camera != "":
		r.Kind, r.Source = Camera, camSource
	}
	return r
}

// sourceType reads an IPTC digital source type from XMP or a C2PA manifest.
// The codes are URIs such as
// http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia.
// algorithmicMedia (made by software without training data) is reported with
// generated media: either way the image is not a camera capture.
func sourceType(s string) Kind {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "digitalsourcetype/compositewithtrainedalgorithmicmedia"),
		strings.Contains(lower, "digitalsourcetype/compositesynthetic"):
		return AIEdited
	case strings.Contains(lower, "digitalsourcetype/trainedalgorithmicmedia"),
		strings.Contains(lower, "digitalsourcetype/algorithmicmedia"):
		return AIGenerated
	case strings.Contains(lower, "digitalsourcetype/digitalcapture"),
		strings.Contains(lower, "digitalsourcetype/computationalcapture"):
		return Camera
	}
	return None
}

// generators maps a lower-case marker to the name shown to the user. Only
// generative tools are listed: "Photoshop" or a phone's camera app are not.
var generators = []struct{ marker, name string }{
	{"chatgpt", "OpenAI"}, {"openai", "OpenAI"}, {"dall-e", "OpenAI"}, {"dall·e", "OpenAI"}, {"dalle", "OpenAI"},
	{"firefly", "Adobe Firefly"},
	{"bing image creator", "Microsoft"}, {"microsoft designer", "Microsoft"}, {"copilot", "Microsoft"},
	{"made with google ai", "Google"}, {"edited with google ai", "Google"}, {"google imagen", "Google"}, {"gemini", "Google"},
	{"meta ai", "Meta"}, {"imagined with ai", "Meta"},
	{"midjourney", "Midjourney"},
	{"stable diffusion", "Stable Diffusion"}, {"stability ai", "Stable Diffusion"}, {"stablediffusion", "Stable Diffusion"},
	{"comfyui", "ComfyUI"}, {"novelai", "NovelAI"}, {"invokeai", "InvokeAI"}, {"fooocus", "Fooocus"},
	{"leonardo.ai", "Leonardo"}, {"ideogram", "Ideogram"}, {"runwayml", "Runway"},
	{"flux.1", "FLUX"}, {"black forest labs", "FLUX"}, {"grok", "Grok"},
	// Video generators.
	{"openai sora", "OpenAI Sora"}, {"with sora", "OpenAI Sora"}, {"dengan sora", "OpenAI Sora"}, {"pakai sora", "OpenAI Sora"},
	{"#sora", "OpenAI Sora"}, {"sora ai", "OpenAI Sora"}, {"sora 2", "OpenAI Sora"},
	{"veo 3", "Google Veo"}, {"veo3", "Google Veo"}, {"veo 2", "Google Veo"}, {"google veo", "Google Veo"},
	{"kling", "Kling"}, {"hailuo", "Hailuo"}, {"haiper", "Haiper"}, {"pixverse", "PixVerse"},
	{"seedance", "Seedance"}, {"hunyuan", "Hunyuan"}, {"heygen", "HeyGen"}, {"synthesia", "Synthesia"},
	{"dream machine", "Luma"}, {"lumalabs", "Luma"}, {"luma ai", "Luma"}, {"pika labs", "Pika"}, {"pikalabs", "Pika"},
	{"runway gen", "Runway"}, {"vidu ai", "Vidu"},
	{"doubao", "Doubao"}, {"豆包", "Doubao"}, {"jimeng", "Jimeng"}, {"即梦", "Jimeng"},
}

// GeneratorIn names the generative AI tool a text mentions (a creator tool,
// a watermark), or "" when it names none.
func GeneratorIn(s string) string { return generatorIn(s) }

// generatorIn names the first known generator mentioned in s.
func generatorIn(s string) string {
	lower := strings.ToLower(s)
	for _, g := range generators {
		if strings.Contains(lower, g.marker) {
			return g.name
		}
	}
	return ""
}

var (
	creatorTool = regexp.MustCompile(`(?is)CreatorTool\s*(?:=\s*["']([^"']*)["']|>\s*([^<]*)<)`)
	xmpMake     = regexp.MustCompile(`(?is)tiff:Make\s*(?:=\s*["']([^"']*)["']|>\s*([^<]*)<)`)
	xmpModel    = regexp.MustCompile(`(?is)tiff:Model\s*(?:=\s*["']([^"']*)["']|>\s*([^<]*)<)`)
)

func xmpValue(rx *regexp.Regexp, x string) string {
	m := rx.FindStringSubmatch(x)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1] + m[2])
}

// pngGenerator recognises the text chunks image generators write.
func pngGenerator(text map[string]string) string {
	if len(text) == 0 {
		return ""
	}
	if p := text["parameters"]; sdParams(p) {
		if strings.Contains(strings.ToLower(p), "fooocus") {
			return "Fooocus"
		}
		return "Stable Diffusion"
	}
	for _, key := range []string{"prompt", "workflow"} {
		if v := text[key]; strings.Contains(v, `"class_type"`) || strings.Contains(v, `"nodes"`) && strings.Contains(v, `"links"`) {
			return "ComfyUI"
		}
	}
	if _, ok := text["invokeai_metadata"]; ok {
		return "InvokeAI"
	}
	if _, ok := text["sd-metadata"]; ok {
		return "InvokeAI"
	}
	for _, key := range []string{"Software", "exif:Software"} {
		if g := generatorIn(text[key]); g != "" {
			return g
		}
	}
	if c := text["Comment"]; strings.Contains(c, `"steps"`) && strings.Contains(c, `"sampler"`) {
		return "NovelAI"
	}
	if d := text["Description"]; strings.Contains(d, "Job ID:") {
		return "Midjourney"
	}
	return ""
}

// sdParams recognises the generation parameters Stable Diffusion front ends
// write ("Steps: 20, Sampler: Euler a, CFG scale: 7, Seed: ...").
func sdParams(s string) bool {
	return strings.Contains(s, "Steps:") && (strings.Contains(s, "Sampler:") || strings.Contains(s, "CFG scale:") || strings.Contains(s, "Seed:"))
}

// ---------------------------------------------------------------- formats

// jpegBlocks walks the JPEG segments up to the start of the image data.
func jpegBlocks(d []byte) blocks {
	var b blocks
	i := 2
	for i+4 <= len(d) {
		if d[i] != 0xFF {
			break
		}
		marker := d[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan or end of image
			break
		}
		n := int(binary.BigEndian.Uint16(d[i+2 : i+4]))
		if n < 2 || i+2+n > len(d) {
			break
		}
		payload := d[i+4 : i+2+n]
		switch marker {
		case 0xE1:
			switch {
			case bytes.HasPrefix(payload, []byte("Exif\x00\x00")):
				b.exif = append(b.exif, payload[6:])
			case bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xap/1.0/\x00")):
				b.xmp = append(b.xmp, string(payload[29:]))
			}
		case 0xEB: // APP11: JUMBF boxes, which is where C2PA lives in JPEG
			if bytes.Contains(payload, []byte("jumb")) || bytes.Contains(payload, []byte("c2pa")) {
				b.c2pa = append(b.c2pa, payload...)
			}
		}
		i += 2 + n
	}
	return b
}

// pngBlocks walks the PNG chunks.
func pngBlocks(d []byte) blocks {
	var b blocks
	i := 8
	for i+12 <= len(d) {
		n := int(binary.BigEndian.Uint32(d[i : i+4]))
		typ := string(d[i+4 : i+8])
		if n < 0 || i+12+n > len(d) {
			break
		}
		data := d[i+8 : i+8+n]
		switch typ {
		case "tEXt":
			if k, v, ok := bytes.Cut(data, []byte{0}); ok {
				b.addText(string(k), string(v))
			}
		case "zTXt":
			if k, rest, ok := bytes.Cut(data, []byte{0}); ok && len(rest) > 1 {
				if v, err := inflate(rest[1:]); err == nil {
					b.addText(string(k), v)
				}
			}
		case "iTXt":
			b.addITXt(data)
		case "eXIf":
			b.exif = append(b.exif, data)
		case "caBX":
			b.c2pa = append(b.c2pa, data...)
		case "IEND":
			return b
		}
		i += 12 + n
	}
	return b
}

// addITXt reads keyword, compression flag, method, language and translated
// keyword, then the (possibly compressed) text.
func (b *blocks) addITXt(data []byte) {
	k, rest, ok := bytes.Cut(data, []byte{0})
	if !ok || len(rest) < 2 {
		return
	}
	compressed := rest[0] == 1
	rest = rest[2:]
	_, rest, ok = bytes.Cut(rest, []byte{0}) // language
	if !ok {
		return
	}
	_, text, ok := bytes.Cut(rest, []byte{0}) // translated keyword
	if !ok {
		return
	}
	value := string(text)
	if compressed {
		v, err := inflate(text)
		if err != nil {
			return
		}
		value = v
	}
	b.addText(string(k), value)
}

func inflate(data []byte) (string, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(io.LimitReader(r, maxText))
	return string(out), err
}

// webpBlocks walks the RIFF chunks of a WebP file.
func webpBlocks(d []byte) blocks {
	var b blocks
	i := 12
	for i+8 <= len(d) {
		typ := string(d[i : i+4])
		n := int(binary.LittleEndian.Uint32(d[i+4 : i+8]))
		if n < 0 || i+8+n > len(d) {
			break
		}
		data := d[i+8 : i+8+n]
		switch typ {
		case "EXIF":
			b.exif = append(b.exif, bytes.TrimPrefix(data, []byte("Exif\x00\x00")))
		case "XMP ":
			b.xmp = append(b.xmp, string(data))
		case "C2PA":
			b.c2pa = append(b.c2pa, data...)
		}
		i += 8 + n + n%2
	}
	return b
}

// ---------------------------------------------------------------- EXIF

type exifInfo struct {
	make, model, software, description, comment string
}

func (e *exifInfo) merge(o exifInfo) {
	if e.make == "" {
		e.make = o.make
	}
	if e.model == "" {
		e.model = o.model
	}
	if e.software == "" {
		e.software = o.software
	}
	if e.description == "" {
		e.description = o.description
	}
	if e.comment == "" {
		e.comment = o.comment
	}
}

func (e exifInfo) camera() string {
	mk, md := strings.TrimSpace(e.make), strings.TrimSpace(e.model)
	if md == "" {
		return mk
	}
	if mk != "" && strings.HasPrefix(strings.ToLower(md), strings.ToLower(mk)) {
		return md // "Canon" + "Canon EOS 80D"
	}
	return strings.TrimSpace(mk + " " + md)
}

// parseEXIF reads IFD0 (make, model, software, description) and the EXIF
// sub-IFD's user comment from a TIFF structure.
func parseEXIF(t []byte) exifInfo {
	var e exifInfo
	if len(t) < 8 {
		return e
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return e
	}
	ifd0 := int(bo.Uint32(t[4:8]))
	sub := 0
	readIFD(t, bo, ifd0, func(tag uint16, typ uint16, count uint32, value []byte) {
		switch tag {
		case 0x010F:
			e.make = ascii(value)
		case 0x0110:
			e.model = ascii(value)
		case 0x0131:
			e.software = ascii(value)
		case 0x010E:
			e.description = ascii(value)
		case 0x8769:
			if len(value) >= 4 {
				sub = int(bo.Uint32(value[:4]))
			}
		}
	})
	if sub > 0 {
		readIFD(t, bo, sub, func(tag uint16, typ uint16, count uint32, value []byte) {
			if tag == 0x9286 {
				e.comment = userComment(value)
			}
		})
	}
	return e
}

// readIFD calls fn with each entry's raw value bytes (inline or pointed to).
func readIFD(t []byte, bo binary.ByteOrder, off int, fn func(tag, typ uint16, count uint32, value []byte)) {
	if off <= 0 || off+2 > len(t) {
		return
	}
	n := int(bo.Uint16(t[off : off+2]))
	if n > 512 {
		return
	}
	size := map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}
	for k := 0; k < n; k++ {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return
		}
		tag := bo.Uint16(t[e : e+2])
		typ := bo.Uint16(t[e+2 : e+4])
		count := bo.Uint32(t[e+4 : e+8])
		unit, ok := size[typ]
		if !ok || count > maxText {
			continue
		}
		total := int(count) * unit
		var value []byte
		if total <= 4 {
			value = t[e+8 : e+8+total]
		} else {
			p := int(bo.Uint32(t[e+8 : e+12]))
			if p < 0 || p+total > len(t) {
				continue
			}
			value = t[p : p+total]
		}
		fn(tag, typ, count, value)
	}
}

func ascii(v []byte) string {
	return strings.TrimSpace(strings.TrimRight(string(v), "\x00"))
}

// userComment drops the 8-byte character code; UTF-16 text keeps its ASCII.
func userComment(v []byte) string {
	if len(v) < 8 {
		return ""
	}
	code, body := string(v[:8]), v[8:]
	if strings.HasPrefix(code, "UNICODE") {
		var b strings.Builder
		for i := 0; i+1 < len(body); i += 2 {
			c := body[i]
			if c == 0 {
				c = body[i+1]
			}
			if c >= 0x20 && c < 0x7f || c == '\n' {
				b.WriteByte(c)
			}
		}
		return b.String()
	}
	return ascii(body)
}
