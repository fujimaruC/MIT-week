"""Compare models on labelled image folders with AMBA's real pipeline.

    python tools/eval/compare.py --models "model/a:free,model/b:free" \\
        --out eval-results ../eval-data/ai-foto ../eval-data/ai-ilustrasi ../eval-data/asli

Run it from the repository root. The OpenRouter key must already be in the
environment as NINEROUTER_API_KEY (never on the command line). For each
model it runs `go run ./cmd/evalimages` (backend/), saves the TSV in --out
and prints one comparison table. --limit N uses the first N images of each
folder, to check a model's stability before spending a full run of the
free daily quota. --mock checks the plumbing without any model.
"""
import argparse
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

sys.path.insert(0, str(pathlib.Path(__file__).parent))
import analyze  # noqa: E402

ROOT = pathlib.Path(__file__).resolve().parents[2]


def subset(folders, limit, tmp):
    if not limit:
        return [str(pathlib.Path(f).resolve()) for f in folders]
    out = []
    for f in folders:
        src = pathlib.Path(f).resolve()
        dst = pathlib.Path(tmp) / src.name
        dst.mkdir(parents=True, exist_ok=True)
        for img in sorted(p for p in src.iterdir() if p.is_file())[:limit]:
            shutil.copy(img, dst / img.name)
        out.append(str(dst))
    return out


def run(model, folders, out_dir, extra, mock, delay):
    env = dict(os.environ)
    env.update({'LLM_MOCK': 'true' if mock else 'false', 'NINEROUTER_MODEL': model, 'ENVIRONMENT': 'development',
                'DOMAIN_AGE_LOOKUP': 'false', 'EVAL_DELAY_MS': str(delay), 'LLM_EXTRA_BODY': extra})
    env.setdefault('NINEROUTER_BASE_URL', 'https://openrouter.ai/api/v1')
    if not mock and not env.get('NINEROUTER_API_KEY'):
        sys.exit('NINEROUTER_API_KEY is not set')
    target = pathlib.Path(out_dir) / (model.replace('/', '__').replace(':', '_') + '.tsv')
    with open(target, 'w', encoding='utf-8') as f:
        proc = subprocess.run(['go', 'run', './cmd/evalimages', *folders], cwd=ROOT / 'backend', env=env, stdout=f, stderr=subprocess.PIPE, text=True)
    for line in proc.stderr.splitlines():
        if not line.startswith('{'):  # JSON log lines carry no content; skip them
            print('   ' + line)
    if proc.returncode != 0:
        print(f'   evalimages exited with {proc.returncode}')
    return target


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--models', required=True, help='comma-separated OpenRouter model ids')
    ap.add_argument('--out', default='eval-results')
    ap.add_argument('--limit', type=int, default=0, help='first N images per folder (0: all)')
    ap.add_argument('--extra', default='{"reasoning":{"enabled":false}}', help='LLM_EXTRA_BODY for every model (no fallbacks: they mix models)')
    ap.add_argument('--delay-ms', type=int, default=3500)
    ap.add_argument('--negative', default='asli')
    ap.add_argument('--mock', action='store_true')
    ap.add_argument('folders', nargs='+')
    a = ap.parse_args()
    json.loads(a.extra)  # fail early on a typo
    os.makedirs(a.out, exist_ok=True)
    table = []
    with tempfile.TemporaryDirectory() as tmp:
        folders = subset(a.folders, a.limit, tmp)
        for model in [m.strip() for m in a.models.split(',') if m.strip()]:
            print(f'== {model}')
            tsv = run(model, folders, a.out, a.extra, a.mock, a.delay_ms)
            s = analyze.summarize(analyze.load(tsv), a.negative)
            analyze.report(s, a.negative)
            degraded = sum(v['degraded'] for v in s['labels'].values())
            total = sum(v['n'] + v['degraded'] for v in s['labels'].values())
            for label, v in s['vs_negative'].items():
                hit80, fp80 = v['thresholds'][80]
                table.append((model, label, v['auc'], hit80, fp80, degraded, total, s['latency'][0] if s['latency'] else float('nan')))
    print('\nmodel                                                     label            AUC   flag@80  false@80  failed  s/img')
    for m, label, auc, hit, fp, deg, total, sec in table:
        print(f'{m:57s} {label:15s} {auc:5.3f}  {hit:6.0%}  {fp:7.0%}  {deg:3d}/{total:<3d} {sec:5.1f}')


if __name__ == '__main__':
    main()
