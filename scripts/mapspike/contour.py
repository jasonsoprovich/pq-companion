import math, sys
from mesh import zone_triangles
from floorplan import svg
def to_map(v): return (-v[1], -v[0], v[2])

def contours(zone, interval=25.0, up_thresh=0.35):
    verts,tris = zone_triangles(f'/Volumes/T7/EQ/TAKPv22/{zone}.s3d')
    V=[to_map(v) for v in verts]
    segs=[]
    zs=[v[2] for v in V]; zlo,zhi=min(zs),max(zs)
    levels=[zlo+interval*i for i in range(int((zhi-zlo)/interval)+1)]
    for pf,a,b,c in tris:
        if pf & 0x10: continue
        A,B,C=V[a],V[b],V[c]
        # only terrain-ish (roughly upward) faces
        ux,uy,uz=B[0]-A[0],B[1]-A[1],B[2]-A[2]
        vx,vy,vz=C[0]-A[0],C[1]-A[1],C[2]-A[2]
        nz=ux*vy-uy*vx
        L=math.sqrt((uy*vz-uz*vy)**2+(uz*vx-ux*vz)**2+nz*nz)
        if L<1e-9 or abs(nz/L)<up_thresh: continue
        tzs=[A[2],B[2],C[2]]; tlo,thi=min(tzs),max(tzs)
        for k in levels:
            if not (tlo<=k<=thi): continue
            pts=[]
            for P,Q in ((A,B),(B,C),(C,A)):
                if (P[2]-k)*(Q[2]-k) < 0:
                    t=(k-P[2])/(Q[2]-P[2])
                    pts.append((P[0]+t*(Q[0]-P[0]), P[1]+t*(Q[1]-P[1]), k))
            if len(pts)==2: segs.append((pts[0],pts[1]))
    return segs, levels

if __name__=='__main__':
    zone=sys.argv[1]; iv=float(sys.argv[2]) if len(sys.argv)>2 else 25.0
    s,lv = contours(zone, iv)
    print(f"{zone}: {len(s):,} contour segments across {len(lv)} levels (interval {iv})")
    svg(s, f'ct_{zone}.svg', stroke="#5eead4", sw=0.7)
