# Image-check evaluation

Tools to measure Jejak AI (AI-made or real) and Timpa Teks (pasted text) with
the real pipeline and a real model, and to choose the model.

| Script | What it does |
|---|---|
| `free_models.py` | Lists OpenRouter's free models that take images, those that honour a JSON schema first. |
| `compare.py` | Runs `backend/cmd/evalimages` per model over labelled folders and prints one comparison table. |
| `analyze.py` | Summarises one TSV from `evalimages`: AUC, flags and false alarms per threshold, Timpa Teks hits, latency. |

Folders are labels: `ai-foto` (photorealistic AI), `ai-ilustrasi` (stylised
AI), `asli` (camera photos), `teks-ditempel` / `teks-asli` (Timpa Teks). Use
images **without metadata** (re-encoded, as after WhatsApp): files that still
carry Content Credentials or generator settings are decided without the
model. The OpenRouter key goes in `NINEROUTER_API_KEY`, never on a command
line or in a file in the repository.

Typical run, from the repository root:

```bash
python tools/eval/free_models.py
python tools/eval/compare.py --limit 6 --models "a:free,b:free" ../eval-data/ai-foto ../eval-data/asli
python tools/eval/compare.py --models "a:free" --out eval-results \
  ../eval-data/ai-foto ../eval-data/ai-ilustrasi ../eval-data/asli
python tools/eval/compare.py --models "a:free" --negative teks-asli ../eval-data/teks-ditempel ../eval-data/teks-asli
```

Free models allow about 20 requests a minute (the scripts wait 3.5 s
between images) and, on accounts that never bought credits, a small daily
quota; `--limit` keeps a first comparison cheap. Record the numbers you act
on in `docs/DECISIONS.md`.
