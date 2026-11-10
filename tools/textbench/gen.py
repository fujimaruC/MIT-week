"""Build the Timpa Teks benchmark from real camera photos.

For each photo: a legitimate news graphic (photo + caption bar with a
headline, a subhead and a price line, saved by a "publisher", then
recompressed by a "messenger", sometimes downscaled) and a hoax made from that
circulated graphic (the price line covered with a patch and retyped, saved by
an "editor", forwarded once more about half of the time). meta.json holds the
pixel boxes of every line and of the retyped price.

    git clone --depth 1 https://github.com/ianare/exif-samples.git /tmp/exif-samples
    python3 tools/textbench/gen.py /tmp/exif-samples/jpg /tmp/textbench 7
    python3 tools/textbench/gen.py /tmp/exif-samples/jpg /tmp/textbench_holdout 23
    cd backend && TEXTBENCH=/tmp/textbench:/tmp/textbench_holdout \\
        go test ./internal/forensics -run TestTextBenchmark -v

Needs Pillow and the DejaVu, Liberation or Poppins fonts.
"""
import glob
import io
import json
import os
import random
import sys

from PIL import Image, ImageDraw, ImageFont

SRC, OUT = sys.argv[1], sys.argv[2]
random.seed(int(sys.argv[3]) if len(sys.argv) > 3 else 7)
os.makedirs(OUT, exist_ok=True)
FONTS = [f for f in ['/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf',
                     '/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf',
                     '/usr/share/fonts/truetype/google-fonts/Poppins-Bold.ttf'] if os.path.exists(f)]
HEADLINES = ['HARGA BERAS NAIK', 'BANSOS CAIR HARI INI', 'JALAN TOL DITUTUP', 'PEMILU DIUNDUR', 'GEMPA SUSULAN',
             'SEKOLAH LIBUR', 'TARIF LISTRIK TURUN', 'BBM GRATIS', 'VAKSIN WAJIB', 'BANJIR SETINGGI ATAP']
PHOTOS = ['exif-org/*.jpg', 'gps/*.jpg', 'orientation/landscape_1.jpg', 'orientation/portrait_1.jpg',
          'tests/45-gps_ifd.jpg', 'tests/22-canon_tags.jpg', 'tests/11-tests.jpg', 'mobile/jolla.jpg',
          'hdr/iphone_hdr_NO.jpg', 'tests/36-memory_error.jpg', 'tests/87_OSError.jpg', 'Reconyx_HC500_Hyperfire.jpg']


def jpeg(im, q):
    b = io.BytesIO()
    im.save(b, 'JPEG', quality=q)
    b.seek(0)
    return Image.open(b).convert('RGB'), b.getvalue()


bases = [p for pat in PHOTOS for p in sorted(glob.glob(os.path.join(SRC, pat)))]
meta = []
for i, path in enumerate(bases):
    photo = Image.open(path).convert('RGB')
    s = 1280 / max(photo.size)  # a phone photo as posted
    if s < 1:
        photo = photo.resize((round(photo.width * s), round(photo.height * s)), Image.LANCZOS)
    photo, _ = jpeg(photo, random.choice([88, 92, 95]))
    w, h = photo.size
    font_path = random.choice(FONTS)
    size = max(18, w // 18)
    fh, fs = ImageFont.truetype(font_path, size), ImageFont.truetype(font_path, int(size * 0.7))
    dark = random.random() < 0.6
    bar = (20, 30, 60) if dark else (245, 245, 240)
    ink = (255, 255, 255) if dark else (20, 20, 20)
    bar_top = int(h * 0.62)
    g = photo.copy()
    d = ImageDraw.Draw(g)
    d.rectangle([0, bar_top, w, h], fill=bar)
    x0 = int(w * 0.05)
    head, sub, old = random.choice(HEADLINES), 'Menurut sumber resmi pemerintah', 'Rp 12.000 per kg'
    y1 = bar_top + int(size * 0.4)
    y2 = y1 + int(size * 1.3)
    y3 = y2 + int(size * 1.1)
    d.text((x0, y1), head, font=fh, fill=ink)
    d.text((x0, y2), sub, font=fs, fill=ink)
    d.text((x0, y3), old, font=fh, fill=ink)
    boxes = {'headline': d.textbbox((x0, y1), head, font=fh), 'subhead': d.textbbox((x0, y2), sub, font=fs),
             'price': d.textbbox((x0, y3), old, font=fh)}
    q1, q2 = random.choice([80, 88, 95]), random.choice([75, 80, 85])  # publisher, messenger
    g1, _ = jpeg(g, q1)
    resize = random.random() < 0.3
    if resize:
        g1 = g1.resize((round(w * 0.8), round(h * 0.8)), Image.LANCZOS)
        boxes = {k: tuple(round(v * 0.8) for v in b) for k, b in boxes.items()}
        w, h = g1.size
    legit, legit_bytes = jpeg(g1, q2)
    hx = legit.copy()
    dh = ImageDraw.Draw(hx)
    px0, py0, px1, py1 = boxes['price']
    dh.rectangle([px0 - 3, py0 - 3, px1 + 3, py1 + 3], fill=bar)
    fnew = ImageFont.truetype(font_path, round(size * (0.8 if resize else 1.0)))
    new = random.choice(['Rp 2.000 per kg', 'Rp 50.000 per kg', 'Rp 500 per kg'])
    at = (px0, py0 - fnew.getbbox(new)[1])
    dh.text(at, new, font=fnew, fill=ink)
    newbox = dh.textbbox(at, new, font=fnew)
    q3 = random.choice([85, 92, 95])  # editor app
    hoax, hoax_bytes = jpeg(hx, q3)
    q4 = None
    if random.random() < 0.5:  # forwarded again
        q4 = random.choice([75, 80])
        hoax, hoax_bytes = jpeg(hoax, q4)
    name = f'{i:02d}'
    open(os.path.join(OUT, name + '_legit.jpg'), 'wb').write(legit_bytes)
    open(os.path.join(OUT, name + '_hoax.jpg'), 'wb').write(hoax_bytes)
    meta.append({'id': name, 'base': os.path.relpath(path, SRC), 'size': [w, h], 'q': [q1, q2, q3, q4],
                 'resize': resize, 'dark': dark, 'boxes': {k: list(v) for k, v in boxes.items()}, 'newbox': list(newbox)})
json.dump(meta, open(os.path.join(OUT, 'meta.json'), 'w'), indent=1)
print(len(meta), 'pairs')
