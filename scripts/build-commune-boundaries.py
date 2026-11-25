#!/usr/bin/env python3
"""Convert the official DPA 2023 commune shapefile to private WGS84 GeoJSON.

Usage: python3 scripts/build-commune-boundaries.py SOURCE.zip OUTPUT.bin.gz
Source: https://www.geoportal.cl/geoportal/catalog/download/912598ad-ac92-35f6-8045-098f214bd9c2
License metadata: https://datos.gob.cl/dataset/categoria-geoespacial-limites-y-fronteras/resource/0cc095c1-80f0-4d31-9aad-4dcffce3d957 (CC BY)
The DPA 2023 metadata says 1:50,000 and excludes Antártica. It requires
attribution and contains additional DIFROL conditions for published maps.
This conversion is for server side point validation, not public map rendering.
"""

import csv
import gzip
import json
import pathlib
import struct
import sys
import zipfile


def dbf_codes(data):
    count = struct.unpack_from("<I", data, 4)[0]
    header = struct.unpack_from("<H", data, 8)[0]
    length = struct.unpack_from("<H", data, 10)[0]
    fields = []
    offset = 1
    for pos in range(32, header - 1, 32):
        descriptor = data[pos:pos + 32]
        if descriptor[0] == 13:
            break
        fields.append((descriptor[:11].split(b"\0")[0].decode("ascii"), offset, descriptor[16]))
        offset += descriptor[16]
    cut = next((field for field in fields if field[0] == "CUT_COM"), None)
    if cut is None:
        raise ValueError("CUT_COM absent from official DBF")
    return [data[header + i * length + cut[1]:header + i * length + cut[1] + cut[2]].strip().decode("ascii").zfill(5) for i in range(count)]


def shapes(data):
    pos = 100
    while pos < len(data):
        record_length = struct.unpack_from(">I", data, pos + 4)[0] * 2
        start = pos + 8
        shape_type = struct.unpack_from("<I", data, start)[0]
        if shape_type != 5:
            raise ValueError(f"expected Polygon shape, got {shape_type}")
        parts_count, points_count = struct.unpack_from("<II", data, start + 36)
        parts = struct.unpack_from("<" + "I" * parts_count, data, start + 44)
        points_start = start + 44 + 4 * parts_count
        polygons = []
        for i, first in enumerate(parts):
            last = parts[i + 1] if i + 1 < parts_count else points_count
            ring = [list(struct.unpack_from("<dd", data, points_start + 16 * j)) for j in range(first, last)]
            if len(ring) < 4 or ring[0] != ring[-1]:
                raise ValueError("open or degenerate official ring")
            area = sum(a[0] * b[1] - b[0] * a[1] for a, b in zip(ring, ring[1:]))
            if area >= 0:
                raise ValueError("unexpected counterclockwise ring; inspect holes before conversion")
            polygons.append([ring])
        yield polygons
        pos = start + record_length
    if pos != len(data):
        raise ValueError("trailing or truncated shape bytes")


def main():
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    source, output = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    with zipfile.ZipFile(source) as archive:
        projection = archive.read("COMUNAS/COMUNAS_v1.prj").decode("ascii")
        if 'GEOGCS["GCS_SIRGAS-Chile"' not in projection or 'UNIT["Degree"' not in projection:
            raise ValueError("unexpected projection; WGS84 conversion required")
        codes = dbf_codes(archive.read("COMUNAS/COMUNAS_v1.dbf"))
        geometries = list(shapes(archive.read("COMUNAS/COMUNAS_v1.shp")))
    expected = {row["commune_code"] for row in csv.DictReader(open(pathlib.Path(__file__).resolve().parents[1] / "db/reference/dpa_cl_cut_2018.csv", encoding="utf8"))}
    if len(codes) != 345 or len(set(codes)) != 345 or len(geometries) != len(codes):
        raise ValueError("DPA 2023 expected 345 unique commune geometries")
    missing, additional = expected - set(codes), set(codes) - expected
    if missing or additional:
        raise ValueError(f"CUT 2018 mismatch: missing={sorted(missing)} additional={sorted(additional)}")
    output.parent.mkdir(parents=True, exist_ok=True)
    if output.name.endswith(".bin.gz"):
        with open(output, "wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", compresslevel=6, mtime=0, filename="") as target:
            target.write(b"EGCUT001" + struct.pack("<I", len(codes)))
            for code, polygons in zip(codes, geometries):
                target.write(code.encode("ascii") + struct.pack("<I", len(polygons)))
                for polygon in polygons:
                    target.write(struct.pack("<I", len(polygon)))
                    for ring in polygon:
                        target.write(struct.pack("<I", len(ring)))
                        for coordinate in ring:
                            target.write(struct.pack("<dd", *coordinate))
    else:
        with gzip.open(output, "wt", encoding="utf8", compresslevel=6) as target:
            target.write('{"type":"FeatureCollection","features":[')
            for i, (code, polygons) in enumerate(zip(codes, geometries)):
                if i:
                    target.write(",")
                json.dump({"type": "Feature", "properties": {"comuna_code": code}, "geometry": {"type": "MultiPolygon", "coordinates": polygons}}, target, separators=(",", ":"))
            target.write("]}")
    print(f"wrote {len(codes)} communes; complete existing CUT catalog; {output.stat().st_size} bytes")


if __name__ == "__main__":
    main()
