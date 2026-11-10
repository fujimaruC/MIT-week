"""Summarise `go run ./cmd/evalimages` output (a TSV on stdout).

    python tools/eval/analyze.py results.tsv [--positive ai-foto] [--negative asli]

For every label it prints the AI scores; for every label against the
negative label (default "asli") it prints how well the AI score separates
them (AUC: 1.0 perfect, 0.5 a coin toss), and at each threshold how many of
that label are flagged versus how many real photos are flagged by mistake.
It also reports Timpa Teks hits (text evidence >= 70), degraded runs and
latency. Only the standard library is used.
"""
import argparse
import csv
import statistics
import sys
from collections import defaultdict

THRESHOLDS = (40, 50, 60, 70, 80, 90)


def load(path):
    with open(path, newline='', encoding='utf-8') as f:
        return [r for r in csv.DictReader(f, delimiter='\t') if r.get('file') and r.get('ai') is not None]


def auc(pos, neg):
    if not pos or not neg:
        return float('nan')
    wins = sum((p > n) + 0.5 * (p == n) for p in pos for n in neg)
    return wins / (len(pos) * len(neg))


def summarize(rows, negative='asli'):
    scores, degraded, text_hits, counted, seconds = defaultdict(list), defaultdict(int), defaultdict(int), defaultdict(int), []
    for r in rows:
        label = r['label']
        if r.get('degraded') == 'true':
            degraded[label] += 1
            continue
        counted[label] += 1
        try:
            if int(r.get('text_evidence', '-1')) >= 70:
                text_hits[label] += 1
        except ValueError:
            pass
        if r['ai'] not in ('-', ''):
            scores[label].append(int(r['ai']))
        try:
            seconds.append(float(r.get('seconds', 'nan')))
        except ValueError:
            pass
    out = {'labels': {}, 'vs_negative': {}, 'latency': None}
    for label in sorted(set(scores) | set(degraded) | set(counted)):
        v = sorted(scores[label])
        out['labels'][label] = {'n': counted[label], 'degraded': degraded[label], 'median_ai': statistics.median(v) if v else None,
                                'scores': v, 'text_hits': text_hits[label]}
    neg = scores.get(negative, [])
    for label, v in scores.items():
        if label == negative:
            continue
        out['vs_negative'][label] = {
            'auc': auc(v, neg),
            'thresholds': {t: (sum(x >= t for x in v) / len(v), sum(x >= t for x in neg) / len(neg) if neg else float('nan'))
                           for t in THRESHOLDS},
        }
    secs = sorted(s for s in seconds if s == s)
    if secs:
        out['latency'] = (statistics.median(secs), secs[int(len(secs) * 0.9)])
    return out


def report(summary, negative='asli', file=sys.stdout):
    for label, s in summary['labels'].items():
        med = '-' if s['median_ai'] is None else f"{s['median_ai']:.0f}"
        print(f"{label:16s} n={s['n']:3d} degraded={s['degraded']:2d} median_ai={med:>3s} text>=70={s['text_hits']:2d} scores={s['scores']}", file=file)
    for label, s in summary['vs_negative'].items():
        print(f"AI score, {label} vs {negative}: AUC {s['auc']:.3f}", file=file)
        for t, (hit, fp) in s['thresholds'].items():
            print(f"   >= {t}: flags {hit:5.0%} of {label}, {fp:5.0%} of {negative}", file=file)
    if summary['latency']:
        print(f"seconds per image: median {summary['latency'][0]:.1f}, p90 {summary['latency'][1]:.1f}", file=file)


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('tsv')
    ap.add_argument('--negative', default='asli', help='label of the real photos (default asli)')
    a = ap.parse_args()
    report(summarize(load(a.tsv), a.negative), a.negative)
