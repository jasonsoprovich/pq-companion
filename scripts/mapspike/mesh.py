import struct, sys
from wld import read_pfs, parse_wld, sname

def parse_mesh(data, off, size, wld_ver):
    o = off
    flags, matref, animref, f3, f4 = struct.unpack_from('<5i', data, o); o+=20
    cx, cy, cz = struct.unpack_from('<3f', data, o); o+=12
    o += 12  # params2[3]
    o += 4   # max_distance
    o += 24  # min/max xyz
    (vc, tcc, nc, cc, pc, vpc, ptc, vtc, size9, scale) = struct.unpack_from('<10h', data, o); o+=20
    inv = 1.0 / (1 << scale)
    verts=[]
    for i in range(vc):
        x,y,z = struct.unpack_from('<3h', data, o); o+=6
        verts.append((cx + x*inv, cy + y*inv, cz + z*inv))
    o += tcc * (8 if wld_ver == 0x1000C800 else 4)   # tex coords
    o += nc * 3                                       # normals
    o += cc * 4                                       # colors
    tris=[]
    for i in range(pc):
        pf, i1, i2, i3 = struct.unpack_from('<4H', data, o); o+=8
        tris.append((pf, i1, i2, i3))
    return verts, tris

def zone_triangles(s3d_path, wld_name=None):
    files = read_pfs(s3d_path)
    base = wld_name or (s3d_path.split('/')[-1].replace('.s3d','') + '.wld')
    ver, strings, frags, data = parse_wld(files[base])
    allv=[]; allt=[]
    for ftype, off, size, nameref in frags:
        if ftype != 0x36: continue
        try: v, t = parse_mesh(data, off, size, ver)
        except Exception: continue
        b = len(allv); allv.extend(v)
        for pf,i1,i2,i3 in t:
            allt.append((pf, b+i1, b+i2, b+i3))
    return allv, allt

if __name__=='__main__':
    p = sys.argv[1]
    v,t = zone_triangles(p)
    print(f"{p}:  vertices={len(v):,}  triangles={len(t):,}")
    xs=[a[0] for a in v]; ys=[a[1] for a in v]; zs=[a[2] for a in v]
    print(f"  X {min(xs):9.1f} .. {max(xs):9.1f}")
    print(f"  Y {min(ys):9.1f} .. {max(ys):9.1f}")
    print(f"  Z {min(zs):9.1f} .. {max(zs):9.1f}")
    # 'permeable'/solid flag distribution
    pf={}
    for f,_,_,_ in t: pf[f]=pf.get(f,0)+1
    print("  polygon flag values:", dict(sorted(pf.items(), key=lambda kv:-kv[1])[:6]))
