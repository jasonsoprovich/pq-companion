"""Run the shipping Wishlist page offline in a task-owned headless browser.

Requires existing repo node_modules, Node, Python stdlib and Chromium/Edge.
No server, backend, database, native app or network connection is used.
"""
import argparse
import hashlib
import html
import json
from pathlib import Path
import re
import subprocess
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', required=True, type=Path)
parser.add_argument('--browser', required=True, type=Path)
parser.add_argument('--node', default='node')
parser.add_argument('--source', type=Path, help='Optional baseline repository; fixture and dependencies remain from this repo')
args = parser.parse_args()
repo = Path(__file__).resolve().parent.parent
out = args.output.resolve()
if out.exists():
    parser.error('Use a new output directory to preserve prior evidence')
if out == repo or repo in out.parents:
    parser.error('Output must be outside the source repository')
out.mkdir(parents=True)
build_cmd = [args.node, str(repo / 'scripts/build-wishlist-isolation.cjs'), str(out), str((args.source or repo).resolve())]
build = subprocess.run(build_cmd, capture_output=True, cwd=repo, timeout=60)
(out / 'build-stdout.txt').write_bytes(build.stdout)
(out / 'build-stderr.txt').write_bytes(build.stderr)
receipt = {'build_command': build_cmd, 'build_exit': build.returncode}
if build.returncode:
    (out / 'receipt.json').write_text(json.dumps(receipt, indent=2), encoding='utf-8')
    sys.exit(build.returncode)
cmd = [str(args.browser.resolve()), '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update', '--disable-sync', '--metrics-recording-only', '--host-resolver-rules=MAP * ~NOTFOUND', f'--user-data-dir={out / "browser-profile"}', '--dump-dom', '--virtual-time-budget=30000', (out / 'fixture.html').as_uri()]
try:
    browser = subprocess.run(cmd, capture_output=True, timeout=60)
    (out / 'browser-stdout.html').write_bytes(browser.stdout)
    (out / 'browser-stderr.txt').write_bytes(browser.stderr)
    match = re.search(r'<pre id="result">(.*?)</pre>', browser.stdout.decode('utf-8', 'replace'), re.S)
    result = json.loads(html.unescape(match.group(1))) if match else {'status': 'MISSING_RESULT'}
    receipt.update(browser_command=cmd, browser_exit=browser.returncode)
except (subprocess.TimeoutExpired, json.JSONDecodeError) as error:
    result = {'status': 'HARNESS_FAILURE', 'error': str(error)}
    receipt.update(browser_command=cmd, browser_exit=None)
pins = json.loads((out / 'input-hashes.json').read_text(encoding='utf-8'))
receipt['input_drift'] = [p for p, digest in pins.items() if hashlib.sha256(Path(p).read_bytes()).hexdigest() != digest]
receipt['result_status'] = result['status']
receipt['case_count'] = len(result.get('cases', []))
receipt['passed_cases'] = sum(c['pass'] for c in result.get('cases', []))
receipt['assertion_count'] = sum(len(c['assertions']) for c in result.get('cases', []))
(out / 'result.json').write_text(json.dumps(result, indent=2), encoding='utf-8')
(out / 'receipt.json').write_text(json.dumps(receipt, indent=2), encoding='utf-8')
print(json.dumps(receipt, indent=2))
sys.exit(0 if receipt.get('browser_exit') == 0 and result['status'] == 'PASS' and not receipt['input_drift'] else 1)
