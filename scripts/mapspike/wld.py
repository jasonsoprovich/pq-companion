import struct, zlib, os
HASH_KEY = bytes([0x95,0x3A,0xC5,0x2A,0x95,0x7A,0x95,0x6A])

def read_pfs(path):
    d=open(path,'rb').read()
    dir_off,magic,ver=struct.unpack_from('<I4sI',d,0)
    assert magic==b'PFS ', magic
    count,=struct.unpack_from('<I',d,dir_off)
    ents=[struct.unpack_from('<III',d,dir_off+4+i*12) for i in range(count)]
    def inflate(off,size):
        out=bytearray(); p=off
        while len(out)<size:
            dl,il=struct.unpack_from('<II',d,p); p+=8
            out+=zlib.decompress(d[p:p+dl]); p+=dl
        return bytes(out)
    # The name-table entry is NOT identified by a single CRC constant: TAKPv22
    # ships both 0xFFFFFFFF (unrest, qeynos2, gfaydark) and 0x61580AC9 (akheva)
    # in the same install. Identify it structurally instead — it is the entry
    # that decodes as a name list of exactly len(ents)-1 entries.
    def try_names(off,size):
        try: raw=inflate(off,size)
        except Exception: return None
        if len(raw)<8: return None
        n,=struct.unpack_from('<I',raw,0)
        if not (0 < n < 100000): return None
        p=4; out=[]
        try:
            for _ in range(n):
                ln,=struct.unpack_from('<I',raw,p); p+=4
                if not (0 < ln < 1024) or p+ln > len(raw): return None
                out.append(raw[p:p+ln-1].decode('latin-1')); p+=ln
        except Exception: return None
        return out if len(out)==len(ents)-1 else None

    nt=None; names=None
    for e in ents:
        got=try_names(e[1],e[2])
        if got is not None:
            nt, names = e, got; break
    if names is None:
        raise ValueError('no name table found')
    files=sorted([e for e in ents if e is not nt],key=lambda e:e[1])
    return {nm:inflate(o,s) for nm,(c,o,s) in zip(names,files)}

def parse_wld(data):
    magic,ver,frag_count,h3,h4,hash_size,h6 = struct.unpack_from('<7I',data,0)
    assert magic==0x54503D02, hex(magic)
    p=28
    enc=data[p:p+hash_size]; p+=hash_size
    strings=bytes(b ^ HASH_KEY[i%8] for i,b in enumerate(enc))
    frags=[]
    for i in range(frag_count):
        size,ftype = struct.unpack_from('<II',data,p)
        nameref, = struct.unpack_from('<i',data,p+8)
        frags.append((ftype, p+12, size-4, nameref))   # payload start, payload len
        p += 8 + size
    return ver, strings, frags, data

def sname(strings, ref):
    if ref >= 0: return None
    off = -ref
    if off >= len(strings): return None
    e = strings.find(b'\0', off)
    return strings[off:e].decode('latin-1')

if __name__=='__main__':
    import sys
    files = read_pfs(sys.argv[1])
    for nm in sorted(files):
        if not nm.lower().endswith('.wld'): continue
        ver,strings,frags,_ = parse_wld(files[nm])
        counts={}
        for ftype,_,_,_ in frags: counts[ftype]=counts.get(ftype,0)+1
        print(f"\n=== {nm}  wld_ver=0x{ver:08X}  fragments={len(frags)} ===")
        for t,c in sorted(counts.items(), key=lambda kv:-kv[1]):
            print(f"   0x{t:02X} ({t:>3}) x {c}")
