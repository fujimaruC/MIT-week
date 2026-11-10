"""List OpenRouter's free models that can take an image and return a JSON
schema, the two things AMBA's analysis call needs.

    python tools/eval/free_models.py            # live list from openrouter.ai
    python tools/eval/free_models.py --file models.json

The list changes often; check it before choosing NINEROUTER_MODEL. No key is
needed to read the list.
"""
import argparse
import datetime
import json
import urllib.request


def fetch():
    req = urllib.request.Request('https://openrouter.ai/api/v1/models', headers={'User-Agent': 'amba-eval'})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def candidates(doc):
    out = []
    for m in doc.get('data', []):
        pricing = m.get('pricing') or {}
        free = m.get('id', '').endswith(':free') or (str(pricing.get('prompt')) in ('0', '0.0') and str(pricing.get('completion')) in ('0', '0.0'))
        modalities = (m.get('architecture') or {}).get('input_modalities') or []
        params = m.get('supported_parameters') or []
        if not free or 'image' not in modalities:
            continue
        out.append({
            'id': m['id'],
            'context': m.get('context_length') or 0,
            'structured': 'structured_outputs' in params,
            'response_format': 'response_format' in params,
            'created': datetime.datetime.fromtimestamp(m.get('created', 0), datetime.timezone.utc).date().isoformat(),
            'ts': m.get('created', 0),
        })
    # Models that honour a strict JSON schema first, then the newest.
    out.sort(key=lambda m: (not m['structured'], not m['response_format'], -m['ts']))
    return out


if __name__ == '__main__':
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--file', help='read a saved /models response instead of fetching it')
    a = ap.parse_args()
    doc = json.load(open(a.file, encoding='utf-8')) if a.file else fetch()
    rows = candidates(doc)
    print(f"{'model':58s} {'json_schema':11s} {'resp_fmt':8s} {'context':>9s} created")
    for m in rows:
        print(f"{m['id']:58s} {('yes' if m['structured'] else 'no'):11s} {('yes' if m['response_format'] else 'no'):8s} {m['context']:9d} {m['created']}")
    print(f"\n{len(rows)} free models take images; prefer json_schema=yes (AMBA sends a strict schema).")
