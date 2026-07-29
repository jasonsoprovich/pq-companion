import math, sys
from collections import defaultdict
from mesh import zone_triangles

def to_map(v):            # mesh -> brewall/map space: f1 = mesh_Y, f2 = -mesh_X
    return (v[1], -v[0], v[2])

def floorplan(zone, up_thresh=0.6):
    verts, tris = zone_triangles(f'/Volumes/T7/EQ/TAKPv22/{zone}.s3d')
    V=[to_map(v) for v in verts]
    edges=defaultdict(int)
    kept=0
    for pf,a,b,c in tris:
        if pf & 0x10:            # flag 16 = non-collidable / permeable
            continue
        A,B,C=V[a],V[b],V[c]
        ux,uy,uz=B[0]-A[0],B[1]-A[1],B[2]-A[2]
        vx,vy,vz=C[0]-A[0],C[1]-A[1],C[2]-A[2]
        nx,ny,nz=uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
        L=math.sqrt(nx*nx+ny*ny+nz*nz)
        if L<1e-9: continue
        if abs(nz/L) < up_thresh:      # not floor-ish -> skip
            continue
        kept+=1
        for i,j in ((a,b),(b,c),(c,a)):
            k=(min(i,j),max(i,j))
            edges[k]+=1
    # boundary = edges used by exactly one floor triangle
    bnd=[(V[i],V[j]) for (i,j),n in edges.items() if n==1]
    return V,tris,kept,bnd

def svg(segs, path, w=1100, stroke="#7dd3fc", bg="#0b1220", sw=1.0):
    xs=[p[0] for s in segs for p in s]; ys=[p[1] for s in segs for p in s]
    if not xs: open(path,'w').write("<svg/>"); return
    x0,x1,y0,y1=min(xs),max(xs),min(ys),max(ys)
    sc=(w-40)/max(x1-x0,y1-y0); h=int((y1-y0)*sc)+40
    out=[f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" style="background:{bg}">',
         f'<g stroke="{stroke}" stroke-width="{sw}" fill="none" stroke-linecap="round">']
    for (a,b) in segs:
        out.append(f'<line x1="{(a[0]-x0)*sc+20:.1f}" y1="{(y1-a[1])*sc+20:.1f}" '
                   f'x2="{(b[0]-x0)*sc+20:.1f}" y2="{(y1-b[1])*sc+20:.1f}"/>')
    out+=['</g></svg>']
    open(path,'w').write('\n'.join(out))

def brewall_segs(zone):
    segs=[]
    for ln in open(f'/tmp/qmaps/{zone}.txt',errors='replace'):
        if ln.startswith('L'):
            p=[float(v) for v in ln[1:].split(',')[:6]]
            segs.append(((p[0],p[1],p[2]),(p[3],p[4],p[5])))
    return segs

if __name__=='__main__':
    for zone in sys.argv[1:]:
        V,tris,kept,bnd = floorplan(zone)
        b=brewall_segs(zone)
        print(f"{zone:10} tris={len(tris):>7,}  floor_tris={kept:>7,}  our_boundary_segs={len(bnd):>7,}   brewall_segs={len(b):>7,}")
        svg(bnd, f'out_{zone}_ours.svg')
        svg(b,   f'out_{zone}_brewall.svg', stroke="#f5a623")
