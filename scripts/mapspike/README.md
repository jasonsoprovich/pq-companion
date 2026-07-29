# Map pipeline spike

Prototype for the maps feature (`docs/maps-feasibility.md` §5b). Python, not
production — the real pipeline is a Go build-time tool. Kept as a working
reference for the port.

Requires a TAKP client; paths point at `/Volumes/T7/EQ/TAKPv22` (external drive).

    wld.py         PFS (.s3d) + WLD container parsing
    mesh.py        0x36 mesh fragment -> vertices + triangles
    floorplan.py   walkable-surface boundary extraction (v1, unwelded)
    floorplan3.py  + vertex welding, polyline simplify, corrected transform
    contour.py     Z-slice elevation contours (outdoor terrain)
    silhouette.py  walkable-area silhouette (corridor/cave zones)

    python3 floorplan3.py unrest akheva
    python3 contour.py gfaydark 30
    python3 silhouette.py akheva necropolis

Key facts (full detail in the feasibility doc):

- Name-table CRC is NOT constant — TAKPv22 ships both 0xFFFFFFFF and
  0x61580AC9. Identify the name table structurally.
- Transform: game = (mesh_Y, mesh_X, mesh_Z); map_f1 = -mesh_Y, map_f2 = -mesh_X.
- Vertex welding is mandatory (meshes arrive as ~3200 separate index spaces).
- Three techniques selected per zone by grid occupancy / Z structure.
