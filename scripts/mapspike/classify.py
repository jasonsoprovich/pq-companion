"""
Classify every Quarm zone by which extraction technique suits it.

Emits one row per zone with the metrics behind the decision, so the thresholds
stay auditable rather than hidden in a heuristic. See docs/maps-feasibility.md
section 5b.5.

    python3 classify.py            # all zones in quarm.db that have client files
    python3 classify.py unrest akheva
"""
import math, os, sqlite3, sys, csv, traceback
from collections import Counter

from mesh import zone_triangles
from silhouette import walkable_faces, rasterize
from floorplan3 import build

CLIENT = '/Volumes/T7/EQ/TAKPv22'
DB = '/Users/jasonsoprovich/repos/github.com/jasonsoprovich/pq-companion/backend/data/quarm.db'

OCC_THRESHOLD = 0.60      # below -> walkable area is sparse -> silhouette works
GAP_THRESHOLD = 0.25      # fraction of empty Z bins -> discrete floors present
BD_THRESHOLD  = 0.50      # unshared-edge density; below -> continuous terrain


def zone_list():
    db = sqlite3.connect(DB)
    names = [r[0] for r in db.execute("select distinct short_name from zone order by short_name")]
    return [z for z in names if os.path.exists(f'{CLIENT}/{z}.s3d')]


def z_structure(faces, binsize=20.0):
    """Return (z_span, gap_ratio). gap_ratio = fraction of bins between min and
    max Z that hold < 5% of the peak bin. Discrete floors leave empty air
    between them; ramps and terraces do not."""
    zs = [p[2] for f in faces for p in f]
    if not zs:
        return 0.0, 0.0
    lo, hi = min(zs), max(zs)
    if hi - lo < binsize:
        return hi - lo, 0.0
    h = Counter(int((v - lo) // binsize) for v in zs)
    nbins = int((hi - lo) // binsize) + 1
    peak = max(h.values())
    empty = sum(1 for b in range(nbins) if h.get(b, 0) < peak * 0.05)
    return hi - lo, empty / nbins


def classify(zone):
    verts, tris = zone_triangles(f'{CLIENT}/{zone}.s3d')
    faces = walkable_faces(zone)
    if not faces:
        return dict(zone=zone, tris=len(tris), faces=0, occupancy=0.0,
                    z_span=0.0, gap_ratio=0.0, bnd_density=0.0, technique='EMPTY')
    img, _ = rasterize(faces)
    occ = sum(1 for p in img.get_flattened_data() if p) / (img.size[0] * img.size[1])
    span, gap = z_structure(faces)

    # Boundary density = unshared floor edges per floor face. This measures the
    # actual failure mode for terrain: a continuous ground mesh shares every
    # interior edge, so boundary extraction yields almost nothing and contours
    # are the only workable technique. Discrete floor slabs score ~0.7-0.8.
    _, _, _, bnd = build(zone)
    bd = len(bnd) / len(faces)

    if bd < BD_THRESHOLD:
        tech = 'contours'          # continuous terrain surface
    elif occ < OCC_THRESHOLD:
        tech = 'silhouette'        # sparse walkable area -> corridors/caves
    else:
        tech = 'boundary'          # discrete floor slabs
    return dict(zone=zone, tris=len(tris), faces=len(faces), occupancy=occ,
                z_span=span, gap_ratio=gap, bnd_density=bd, technique=tech)


if __name__ == '__main__':
    zones = sys.argv[1:] or zone_list()
    rows, failed = [], []
    print(f"{'zone':<16}{'tris':>9}{'faces':>9}{'occ%':>7}{'z_span':>9}{'bnd_d':>7}  technique",
          flush=True)
    for z in zones:
        try:
            r = classify(z)
        except Exception as e:
            failed.append((z, f'{type(e).__name__}: {e}'))
            print(f"{z:<16}{'':>9}{'':>9}{'':>7}{'':>9}{'':>7}  FAILED  {type(e).__name__}",
                  flush=True)
            continue
        rows.append(r)
        print(f"{r['zone']:<16}{r['tris']:>9,}{r['faces']:>9,}{100*r['occupancy']:>6.1f}%"
              f"{r['z_span']:>9.0f}{r['bnd_density']:>7.2f}  {r['technique']}", flush=True)

    with open('zone_classification.csv', 'w', newline='') as fh:
        w = csv.DictWriter(fh, fieldnames=['zone','tris','faces','occupancy','z_span','gap_ratio','bnd_density','technique'])
        w.writeheader(); w.writerows(rows)

    print(f"\n=== {len(rows)} classified, {len(failed)} failed ===")
    for t, n in Counter(r['technique'] for r in rows).most_common():
        print(f"   {t:<12} {n:>4}  ({100*n/len(rows):.0f}%)")
    if failed:
        print("\nfailures:")
        for z, e in failed:
            print(f"   {z:<16} {e}")
