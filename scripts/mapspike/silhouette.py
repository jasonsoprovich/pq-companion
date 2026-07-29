"""
Silhouette layer — flattened outline of the walkable area.

Brewall's hardest zones (akheva, 768 segments) are not floor plans at all: they
are a single simplified outline of where a player can walk, with vertical
structure discarded. That reads far better at a glance than detailed geometry.
This produces the equivalent from client meshes, to ship as the DEFAULT map
view with per-Z-band detail available underneath as a toggle.

Method: rasterize walkable faces into an occupancy grid, morphologically close
it to merge adjacent slabs and swallow gaps, trace the region boundary with
marching squares, then simplify with Douglas-Peucker.
"""
import math, sys
from PIL import Image, ImageDraw, ImageFilter
from mesh import zone_triangles

def to_map(v):
    return (-v[1], -v[0], v[2])

# marching-squares case table over edge midpoints  T(op) R(ight) B(ottom) L(eft)
_MS = {
    1:[('L','B')], 2:[('B','R')], 3:[('L','R')], 4:[('T','R')],
    5:[('L','T'),('B','R')], 6:[('T','B')], 7:[('L','T')], 8:[('L','T')],
    9:[('T','B')], 10:[('L','B'),('T','R')], 11:[('T','R')], 12:[('L','R')],
    13:[('B','R')], 14:[('L','B')],
}

def walkable_faces(zone, up_thresh=0.55):
    verts, tris = zone_triangles(f'/Volumes/T7/EQ/TAKPv22/{zone}.s3d')
    V = [to_map(v) for v in verts]
    out = []
    for pf, a, b, c in tris:
        if pf & 0x10:                      # non-collidable
            continue
        A, B, C = V[a], V[b], V[c]
        ux, uy, uz = B[0]-A[0], B[1]-A[1], B[2]-A[2]
        vx, vy, vz = C[0]-A[0], C[1]-A[1], C[2]-A[2]
        nx, ny, nz = uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
        L = math.sqrt(nx*nx + ny*ny + nz*nz)
        if L < 1e-9 or abs(nz/L) < up_thresh:
            continue
        out.append((A, B, C))
    return out

def rasterize(faces, target=1400, min_cell=1.0, close_units=2.0):
    xs = [p[0] for f in faces for p in f]
    ys = [p[1] for f in faces for p in f]
    x0, x1, y0, y1 = min(xs), max(xs), min(ys), max(ys)
    span = max(x1-x0, y1-y0)
    cell = max(min_cell, span/target)
    # Pad by more than the dilation radius, or the grown region hits the image
    # border, gets clipped, and marching squares emits open contours that run
    # off the edge instead of closing.
    reps = min(10, int(close_units/cell))
    pad = reps + 3
    W = int((x1-x0)/cell) + 2*pad
    H = int((y1-y0)/cell) + 2*pad
    img = Image.new('L', (W, H), 0)
    d = ImageDraw.Draw(img)
    for A, B, C in faces:
        d.polygon([((A[0]-x0)/cell+pad, (A[1]-y0)/cell+pad),
                   ((B[0]-x0)/cell+pad, (B[1]-y0)/cell+pad),
                   ((C[0]-x0)/cell+pad, (C[1]-y0)/cell+pad)], fill=255)
    for _ in range(reps): img = img.filter(ImageFilter.MaxFilter(3))
    for _ in range(reps): img = img.filter(ImageFilter.MinFilter(3))
    return img, (x0, y0, cell, pad)

def march(img, origin):
    x0, y0, cell, pad = origin
    W, H = img.size
    px = img.load()
    segs = []
    def wx(gx): return x0 + (gx-pad)*cell
    def wy(gy): return y0 + (gy-pad)*cell
    for y in range(H-1):
        for x in range(W-1):
            tl = 1 if px[x,   y  ] else 0
            tr = 1 if px[x+1, y  ] else 0
            br = 1 if px[x+1, y+1] else 0
            bl = 1 if px[x,   y+1] else 0
            idx = tl*8 + tr*4 + br*2 + bl
            if idx == 0 or idx == 15: continue
            pts = {'T': (wx(x+0.5), wy(y)), 'R': (wx(x+1), wy(y+0.5)),
                   'B': (wx(x+0.5), wy(y+1)), 'L': (wx(x), wy(y+0.5))}
            for a, b in _MS[idx]:
                segs.append((pts[a], pts[b]))
    return segs

def chain(segs, tol=1e-6):
    from collections import defaultdict
    key = lambda p: (round(p[0], 3), round(p[1], 3))
    adj = defaultdict(list)
    for a, b in segs:
        adj[key(a)].append((key(b), b)); adj[key(b)].append((key(a), a))
    seen = set(); chains = []
    for a, b in segs:
        e = (key(a), key(b))
        if e in seen or (e[1], e[0]) in seen: continue
        seen.add(e); ch = [a, b]
        prev, cur = key(a), key(b)
        while True:
            nxts = [t for t in adj[cur] if t[0] != prev]
            if len(nxts) != 1: break
            nk, npt = nxts[0]
            ee = (cur, nk)
            if ee in seen or (nk, cur) in seen: break
            seen.add(ee); ch.append(npt); prev, cur = cur, nk
        chains.append(ch)
    return chains

def rdp(pts, eps):
    if len(pts) < 3: return pts
    x0, y0 = pts[0][:2]; x1, y1 = pts[-1][:2]
    dx, dy = x1-x0, y1-y0
    n = math.hypot(dx, dy)
    best, bi = -1, 0
    for i in range(1, len(pts)-1):
        px, py = pts[i][:2]
        d = abs(dy*px - dx*py + x1*y0 - y1*x0)/n if n > 1e-9 else math.hypot(px-x0, py-y0)
        if d > best: best, bi = d, i
    if best <= eps:
        return [pts[0], pts[-1]]
    return rdp(pts[:bi+1], eps)[:-1] + rdp(pts[bi:], eps)

def silhouette(zone, target=1400, close_units=2.0, rdp_eps=1.5):
    faces = walkable_faces(zone)
    img, origin = rasterize(faces, target=target, close_units=close_units)
    segs = march(img, origin)
    out = []
    for ch in chain(segs):
        s = rdp(ch, rdp_eps)
        for i in range(len(s)-1):
            out.append(((s[i][0], s[i][1], 0.0), (s[i+1][0], s[i+1][1], 0.0)))
    return out, len(faces), img.size

if __name__ == '__main__':
    from floorplan import svg, brewall_segs
    sys.setrecursionlimit(100000)
    for zone in sys.argv[1:]:
        segs, nf, size = silhouette(zone)
        b = brewall_segs(zone)
        print(f"{zone:11} walkable_faces={nf:>7,}  grid={size[0]}x{size[1]:<5}  "
              f"silhouette={len(segs):>6,}   brewall={len(b):>6,}")
        svg(segs, f'sil_{zone}.svg', stroke="#a78bfa", sw=1.6)
