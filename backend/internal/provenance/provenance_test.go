package provenance

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

// The builders below write the same structures real files carry. The reader
// was also checked by hand against real files: a C2PA test image from
// contentauth/c2pa-rs (algorithmicMedia), its edited copy (c2pa.opened, no
// AI), three ComfyUI outputs (FLUX, SDXL) and a Canon photo whose EXIF
// ImageMagick copied into PNG text.

const (
	iptcTrained   = "http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia"
	iptcComposite = "http://cv.iptc.org/newscodes/digitalsourcetype/compositeWithTrainedAlgorithmicMedia"
	iptcAlgo      = "http://cv.iptc.org/newscodes/digitalsourcetype/algorithmicMedia"
	iptcCapture   = "http://cv.iptc.org/newscodes/digitalsourcetype/digitalCapture"
)

func seg(marker byte, payload []byte) []byte {
	n := len(payload) + 2
	return append([]byte{0xFF, marker, byte(n >> 8), byte(n)}, payload...)
}

func jpegFile(segments ...[]byte) []byte {
	b := []byte{0xFF, 0xD8}
	b = append(b, seg(0xE0, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00"))...)
	for _, s := range segments {
		b = append(b, s...)
	}
	// Start of scan, a few bytes of entropy-coded data, end of image. Nothing
	// after the scan marker may be read as metadata.
	b = append(b, 0xFF, 0xDA, 0x00, 0x02, 'c', '2', 'p', 'a', 0xFF, 0xD9)
	return b
}

func xmpSeg(xmp string) []byte {
	return seg(0xE1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), xmp...))
}

func exifSeg(tiff []byte) []byte { return seg(0xE1, append([]byte("Exif\x00\x00"), tiff...)) }

// jumbfSeg is one APP11 segment of a C2PA manifest store (JPEG-XT box
// header, then JUMBF boxes whose CBOR carries the manifest text).
func jumbfSeg(manifest string) []byte {
	return seg(0xEB, []byte("JP\x02\x11\x00\x00\x00\x01\x00\x00\x00\x40jumb\x00\x00\x00\x28jumdc2pa\x00\x11\x00\x10"+manifest))
}

func chunk(typ string, data []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	b = append(b, typ...)
	b = append(b, data...)
	return append(b, 0, 0, 0, 0) // CRC is not checked
}

func pngFile(chunks ...[]byte) []byte {
	b := []byte("\x89PNG\r\n\x1a\n")
	b = append(b, chunk("IHDR", make([]byte, 13))...)
	for _, c := range chunks {
		b = append(b, c...)
	}
	b = append(b, chunk("IDAT", []byte("pixels"))...)
	b = append(b, chunk("IEND", nil)...)
	// Anything after IEND is ignored.
	return append(b, chunk("tEXt", []byte("parameters\x00Steps: 20, Sampler: Euler"))...)
}

func tEXt(k, v string) []byte { return chunk("tEXt", []byte(k+"\x00"+v)) }

func deflate(s string) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Bytes()
}

func zTXt(k, v string) []byte {
	return chunk("zTXt", append([]byte(k+"\x00\x00"), deflate(v)...))
}

func iTXt(k, v string, compressed bool) []byte {
	head := []byte(k + "\x00")
	text := []byte(v)
	if compressed {
		head = append(head, 1, 0)
		text = deflate(v)
	} else {
		head = append(head, 0, 0)
	}
	head = append(head, "en\x00\x00"...)
	return chunk("iTXt", append(head, text...))
}

func riff(typ string, data []byte) []byte {
	b := append([]byte(typ), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	b = append(b, data...)
	if len(data)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func webpFile(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	body = append(body, riff("VP8X", make([]byte, 10))...)
	for _, c := range chunks {
		body = append(body, c...)
	}
	return append(append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...), body...)
}

type tag struct {
	id, typ uint16
	value   []byte
}

func asciiTag(id uint16, s string) tag { return tag{id, 2, []byte(s + "\x00")} }

// tiffData builds a TIFF structure with IFD0 and, when sub is not empty, an
// EXIF sub-IFD that IFD0 points to.
func tiffData(bo binary.ByteOrder, ifd0, sub []tag) []byte {
	if len(sub) > 0 {
		ifd0 = append(ifd0, tag{0x8769, 4, nil})
	}
	size := func(n int) int { return 2 + 12*n + 4 }
	offSub := 8 + size(len(ifd0))
	dataOff := offSub
	if len(sub) > 0 {
		dataOff += size(len(sub))
	}
	out := make([]byte, dataOff)
	if bo == binary.LittleEndian {
		copy(out, "II")
	} else {
		copy(out, "MM")
	}
	bo.PutUint16(out[2:], 42)
	bo.PutUint32(out[4:], 8)
	var data []byte
	write := func(at int, tags []tag) {
		bo.PutUint16(out[at:], uint16(len(tags)))
		for i, t := range tags {
			e := at + 2 + 12*i
			v := t.value
			if t.id == 0x8769 {
				v = make([]byte, 4)
				bo.PutUint32(v, uint32(offSub))
			}
			count := len(v)
			if t.typ == 4 {
				count = len(v) / 4
			}
			bo.PutUint16(out[e:], t.id)
			bo.PutUint16(out[e+2:], t.typ)
			bo.PutUint32(out[e+4:], uint32(count))
			if len(v) <= 4 {
				copy(out[e+8:], v)
			} else {
				bo.PutUint32(out[e+8:], uint32(dataOff+len(data)))
				data = append(data, v...)
			}
		}
	}
	write(8, ifd0)
	if len(sub) > 0 {
		write(offSub, sub)
	}
	return append(out, data...)
}

func xmpPacket(inner string) string {
	return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:tiff="http://ns.adobe.com/tiff/1.0/" xmlns:Iptc4xmpExt="http://iptc.org/std/Iptc4xmpExt/2008-02-29/" ` +
		inner + `</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
}

func TestScan(t *testing.T) {
	sdParams := "a photo of a flooded street, cinematic\nNegative prompt: blurry\nSteps: 30, Sampler: DPM++ 2M Karras, CFG scale: 7, Seed: 1234, Size: 1024x1024, Model: sd_xl_base_1.0"
	comfyPrompt := `{"3": {"inputs": {"seed": 1, "steps": 20, "cfg": 8, "sampler_name": "euler"}, "class_type": "KSampler"}, "4": {"inputs": {"ckpt_name": "flux1-dev.safetensors"}, "class_type": "CheckpointLoaderSimple"}}`

	cases := []struct {
		name string
		file []byte
		want Result
	}{
		{
			name: "C2PA from ChatGPT: trained algorithmic media",
			file: jpegFile(jumbfSeg(`c2pa.claimclaim_generatorChatGPTc2pa.actionsactionc2pa.createddigitalSourceType` + iptcTrained + `softwareAgentGPT-4o`)),
			want: Result{Kind: AIGenerated, Source: "c2pa", Generator: "OpenAI", C2PA: true},
		},
		{
			name: "C2PA composite: a photo edited with generative fill",
			file: jpegFile(jumbfSeg(`claim_generatorAdobe_Photoshop/25.0 c2pa.openedc2pa.placeddigitalSourceType` + iptcComposite + `Adobe Firefly`)),
			want: Result{Kind: AIEdited, Source: "c2pa", Generator: "Adobe Firefly", C2PA: true},
		},
		{
			name: "C2PA plain algorithmic media (the c2pa-rs test image)",
			file: jpegFile(jumbfSeg(`claim_generatormake_test_images/0.33 c2pa.createddigitalSourceType` + iptcAlgo)),
			want: Result{Kind: AIGenerated, Source: "c2pa", C2PA: true},
		},
		{
			name: "C2PA without AI: opened and edited",
			file: jpegFile(jumbfSeg(`claim_generatormake_test_images/0.33 c2pa.openedc2pa.color_adjustments`)),
			want: Result{Kind: None, C2PA: true},
		},
		{
			name: "C2PA camera capture",
			file: jpegFile(jumbfSeg(`claim_generatorLeica c2pa.createddigitalSourceType` + iptcCapture)),
			want: Result{Kind: Camera, Source: "c2pa", C2PA: true},
		},
		{
			name: "XMP IPTC digital source type",
			file: jpegFile(xmpSeg(xmpPacket(`Iptc4xmpExt:DigitalSourceType="` + iptcTrained + `" xmp:CreatorTool="Microsoft Designer">`))),
			want: Result{Kind: AIGenerated, Source: "xmp", Generator: "Microsoft"},
		},
		{
			name: "XMP creator tool names a generator",
			file: jpegFile(xmpSeg(xmpPacket(`xmp:CreatorTool="Adobe Firefly">`))),
			want: Result{Kind: AIGenerated, Source: "xmp", Generator: "Adobe Firefly"},
		},
		{
			name: "XMP creator tool as an element",
			file: jpegFile(xmpSeg(xmpPacket(`><xmp:CreatorTool>Midjourney v6</xmp:CreatorTool>`))),
			want: Result{Kind: AIGenerated, Source: "xmp", Generator: "Midjourney"},
		},
		{
			name: "XMP from an ordinary editor is not AI",
			file: jpegFile(xmpSeg(xmpPacket(`xmp:CreatorTool="Adobe Photoshop 25.0 (Windows)" tiff:Make="NIKON CORPORATION" tiff:Model="NIKON D750">`))),
			want: Result{Kind: Camera, Source: "xmp", Camera: "NIKON CORPORATION NIKON D750"},
		},
		{
			name: "a word that merely contains a generator's name is not one",
			file: jpegFile(xmpSeg(xmpPacket(`xmp:CreatorTool="Editor de imagen 2.1">`))),
			want: Result{Kind: None},
		},
		{
			name: "EXIF camera, little endian",
			file: jpegFile(exifSeg(tiffData(binary.LittleEndian, []tag{asciiTag(0x010F, "Canon"), asciiTag(0x0110, "Canon EOS 80D"), asciiTag(0x0131, "Adobe Photoshop Lightroom")}, nil))),
			want: Result{Kind: Camera, Source: "exif", Camera: "Canon EOS 80D"},
		},
		{
			name: "EXIF camera, big endian",
			file: jpegFile(exifSeg(tiffData(binary.BigEndian, []tag{asciiTag(0x010F, "Apple"), asciiTag(0x0110, "iPhone 15")}, nil))),
			want: Result{Kind: Camera, Source: "exif", Camera: "Apple iPhone 15"},
		},
		{
			name: "EXIF software names a generator",
			file: jpegFile(exifSeg(tiffData(binary.LittleEndian, []tag{asciiTag(0x0131, "NovelAI")}, nil))),
			want: Result{Kind: AIGenerated, Source: "exif", Generator: "NovelAI"},
		},
		{
			name: "EXIF user comment carries Stable Diffusion parameters (UTF-16)",
			file: jpegFile(exifSeg(tiffData(binary.BigEndian, nil, []tag{{0x9286, 7, append([]byte("UNICODE\x00"), utf16be(sdParams)...)}}))),
			want: Result{Kind: AIGenerated, Source: "exif", Generator: "Stable Diffusion"},
		},
		{
			name: "an AI marker wins over camera data, which is still reported",
			file: jpegFile(
				exifSeg(tiffData(binary.LittleEndian, []tag{asciiTag(0x010F, "SONY"), asciiTag(0x0110, "ILCE-7M3")}, nil)),
				xmpSeg(xmpPacket(`Iptc4xmpExt:DigitalSourceType="`+iptcComposite+`">`)),
			),
			want: Result{Kind: AIEdited, Source: "xmp", Camera: "SONY ILCE-7M3"},
		},
		{
			name: "PNG Stable Diffusion parameters",
			file: pngFile(tEXt("parameters", sdParams)),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "Stable Diffusion"},
		},
		{
			name: "PNG Fooocus parameters",
			file: pngFile(tEXt("parameters", sdParams+", Version: Fooocus v2.5.0")),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "Fooocus"},
		},
		{
			name: "PNG ComfyUI prompt graph",
			file: pngFile(tEXt("prompt", comfyPrompt)),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "ComfyUI"},
		},
		{
			name: "PNG compressed text (zTXt)",
			file: pngFile(zTXt("parameters", sdParams)),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "Stable Diffusion"},
		},
		{
			name: "PNG compressed international text carrying XMP",
			file: pngFile(iTXt("XML:com.adobe.xmp", xmpPacket(`Iptc4xmpExt:DigitalSourceType="`+iptcTrained+`">`), true)),
			want: Result{Kind: AIGenerated, Source: "xmp"},
		},
		{
			name: "PNG NovelAI",
			file: pngFile(tEXt("Software", "NovelAI"), tEXt("Comment", `{"steps": 28, "sampler": "k_euler"}`)),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "NovelAI"},
		},
		{
			name: "PNG Midjourney description",
			file: pngFile(tEXt("Description", "a cat astronaut --ar 16:9 Job ID: 1234abcd")),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "Midjourney"},
		},
		{
			name: "PNG InvokeAI",
			file: pngFile(tEXt("invokeai_metadata", `{"model": "sdxl"}`)),
			want: Result{Kind: AIGenerated, Source: "png_text", Generator: "InvokeAI"},
		},
		{
			name: "PNG with camera EXIF copied into text by ImageMagick",
			file: pngFile(tEXt("exif:Make", "Canon"), tEXt("exif:Model", "Canon EOS REBEL T3"), tEXt("date:create", "2024-01-01")),
			want: Result{Kind: Camera, Source: "png_text", Camera: "Canon EOS REBEL T3"},
		},
		{
			name: "PNG C2PA chunk",
			file: pngFile(chunk("caBX", []byte("jumbc2pa claim_generator DALL-E digitalSourceType "+iptcTrained))),
			want: Result{Kind: AIGenerated, Source: "c2pa", Generator: "OpenAI", C2PA: true},
		},
		{
			name: "PNG ordinary screenshot",
			file: pngFile(tEXt("Software", "gnome-screenshot"), iTXt("XML:com.adobe.xmp", xmpPacket(`xmp:CreatorTool="Snipping Tool">`), false)),
			want: Result{Kind: None},
		},
		{
			name: "WebP with C2PA, XMP and EXIF chunks",
			file: webpFile(
				riff("EXIF", tiffData(binary.LittleEndian, []tag{asciiTag(0x010F, "Google"), asciiTag(0x0110, "Pixel 9")}, nil)),
				riff("XMP ", []byte(xmpPacket(`xmp:CreatorTool="Gemini">`))),
				riff("VP8 ", []byte("pixels")),
			),
			want: Result{Kind: AIGenerated, Source: "xmp", Generator: "Google", Camera: "Google Pixel 9"},
		},
		{
			name: "WebP C2PA",
			file: webpFile(riff("C2PA", []byte("jumbc2pa claim_generator ChatGPT digitalSourceType "+iptcTrained)), riff("VP8L", []byte("px"))),
			want: Result{Kind: AIGenerated, Source: "c2pa", Generator: "OpenAI", C2PA: true},
		},
		{
			name: "JPEG with no metadata (what WhatsApp sends)",
			file: jpegFile(),
			want: Result{Kind: None},
		},
		{
			name: "not an image",
			file: []byte("GIF89a parameters Steps: 20, Sampler: Euler"),
			want: Result{Kind: None},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Scan(tc.file); got != tc.want {
				t.Fatalf("Scan = %+v\nwant  %+v", got, tc.want)
			}
		})
	}
}

func TestAI(t *testing.T) {
	for kind, want := range map[Kind]bool{AIGenerated: true, AIEdited: true, Camera: false, None: false} {
		if got := (Result{Kind: kind}).AI(); got != want {
			t.Errorf("%s.AI() = %v, want %v", kind, got, want)
		}
	}
}

// Every prefix of every sample must scan without panicking: uploads are
// untrusted and may be truncated anywhere.
func TestScanTruncated(t *testing.T) {
	samples := [][]byte{
		jpegFile(jumbfSeg("c2pa digitalSourceType "+iptcTrained), xmpSeg(xmpPacket(`xmp:CreatorTool="Firefly">`)),
			exifSeg(tiffData(binary.BigEndian, []tag{asciiTag(0x010F, "Canon")}, []tag{{0x9286, 7, []byte("ASCII\x00\x00\x00Steps: 20, Seed: 1")}}))),
		pngFile(tEXt("parameters", "Steps: 20, Seed: 1"), zTXt("prompt", `{"class_type": "KSampler"}`), iTXt("XML:com.adobe.xmp", "<x/>", true)),
		webpFile(riff("EXIF", tiffData(binary.LittleEndian, []tag{asciiTag(0x0110, "X")}, nil)), riff("XMP ", []byte("<x/>"))),
	}
	for _, s := range samples {
		for i := range s {
			_ = Scan(s[:i])
		}
	}
}

// A hostile EXIF block may point anywhere; the reader must stay inside it.
func TestScanHostileEXIF(t *testing.T) {
	tiff := tiffData(binary.LittleEndian, []tag{asciiTag(0x010F, "Canon"), asciiTag(0x0110, "Canon EOS 80D")}, nil)
	for i := 4; i+4 <= len(tiff); i++ {
		bad := bytes.Clone(tiff)
		binary.LittleEndian.PutUint32(bad[i:], 0xFFFFFFF0)
		_ = Scan(jpegFile(exifSeg(bad)))
	}
}

func FuzzScan(f *testing.F) {
	f.Add(jpegFile(jumbfSeg("c2pa digitalSourceType " + iptcTrained)))
	f.Add(pngFile(zTXt("parameters", "Steps: 20, Seed: 1")))
	f.Add(webpFile(riff("XMP ", []byte("<x/>"))))
	f.Fuzz(func(t *testing.T, data []byte) {
		r := Scan(data)
		if r.Kind == "" {
			t.Fatal("empty kind")
		}
	})
}

func utf16be(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}
