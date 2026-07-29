import math, sys
from collections import defaultdict
from mesh import zone_triangles
from floorplan import svg, brewall_segs

def to_map(v): return (-v[1], -v[0], v[2])   # f1=-game_x=-mesh_Y, f2=-game_y=-mesh_X

def build(zone, up_thresh=0.55, weld=0.25):
    verts, tris = zone_triangles(f'/Volumes/T7/EQ/TAKPv22/{zone}.s3d')
    V=[to_map(v) for v in verts]
    # --- weld vertices by quantized position ---
    q={}; remap=[0]*len(V); uniq=[]
    for i,(x,y,z) in enumerate(V):
        k=(round(x/weld),round(y/weld),round(z/weld))
        if k not in q:
            q[k]=len(uniq); uniq.append((x,y,z))
        remap[i]=q[k]
    edges=defaultdict(int); kept=0
    for pf,a,b,c in tris:
        if pf & 0x10: continue
        a,b,c = remap[a],remap[b],remap[c]
        if a==b or b==c or a==c: continue
        A,B,C=uniq[a],uniq[b],uniq[c]
        ux,uy,uz=B[0]-A[0],B[1]-A[1],B[2]-A[2]
        vx,vy,vz=C[0]-A[0],C[1]-A[1],C[2]-A[2]
        nx,ny,nz=uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
        L=math.sqrt(nx*nx+ny*ny+nz*nz)
        if L<1e-9 or abs(nz/L)<up_thresh: continue
        kept+=1
        for i,j in ((a,b),(b,c),(c,a)):
            edges[(min(i,j),max(i,j))]+=1
    bnd=[(uniq[i],uniq[j]) for (i,j),n in edges.items() if n==1]
    return uniq,len(tris),kept,bnd

def simplify(segs, ang_tol=0.03):
    """join boundary segments into polylines, drop near-collinear interior points"""
    adj=defaultdict(list); key=lambda p:(round(p[0],2),round(p[1],2),round(p[2],2))
    for a,b in segs:
        adj[key(a)].append((key(b),a,b)); adj[key(b)].append((key(a),b,a))
    used=set(); out=[]
    for a,b in segs:
        e=(key(a),key(b))
        if e in used or (e[1],e[0]) in used: continue
        chain=[a,b]; used.add(e)
        # walk forward while degree-2
        cur,prev=key(b),key(a)
        while len(adj[cur])==2:
            nxt=[t for t in adj[cur] if t[0]!=prev]
            if not nxt: break
            nk,_,nb = nxt[0]
            ee=(cur,nk)
            if ee in used or (nk,cur) in used: break
            used.add(ee); chain.append(nb); prev,cur = cur,nk
        # collinear decimation
        keep=[chain[0]]
        for i in range(1,len(chain)-1):
            p,c,n = keep[-1],chain[i],chain[i+1]
            v1=(c[0]-p[0],c[1]-p[1]); v2=(n[0]-c[0],n[1]-c[1])
            l1=math.hypot(*v1); l2=math.hypot(*v2)
            if l1<1e-6 or l2<1e-6: continue
            cross=abs(v1[0]*v2[1]-v1[1]*v2[0])/(l1*l2)
            if cross>ang_tol: keep.append(c)
        keep.append(chain[-1])
        for i in range(len(keep)-1): out.append((keep[i],keep[i+1]))
    return out

if __name__=='__main__':
    for zone in sys.argv[1:]:
        uniq,nt,kept,bnd = build(zone)
        s = simplify(bnd)
        b = brewall_segs(zone)
        print(f"{zone:9} tris={nt:>7,} floor={kept:>7,} | raw_bnd={len(bnd):>6,} -> simplified={len(s):>6,} | brewall={len(b):>6,}")
        svg(s, f'v3_{zone}_ours.svg', sw=1.2)
        svg(b, f'v3_{zone}_brewall.svg', stroke="#f5a623", sw=1.2)
