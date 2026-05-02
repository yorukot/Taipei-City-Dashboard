#!/usr/bin/env python3
"""Add district (行政區) column to bus_stop_tpe using Taipei GIS boundary data.

Steps:
  1. Read Taipei district polygons from Taipei-Range_20220915/G97_A_CADIST_P.shp
  2. Load all stops (stop_uid, position_lon, position_lat) from bus_stop_tpe
  3. Spatial join: find which polygon each stop falls in
  4. ALTER TABLE to add district column if missing, then UPDATE in bulk

Usage (from project root):
    .venv/bin/python3 add_district_to_bus_stops.py
"""

import sys
from pathlib import Path

try:
    import geopandas as gpd
    import psycopg2
    import psycopg2.extras
    from shapely.geometry import Point
except ImportError:
    sys.exit("Missing dependencies — activate the project venv first.")

PROJECT_ROOT = Path(__file__).parent
SHAPEFILE    = PROJECT_ROOT / "Taipei-Range_20220915" / "G97_A_CADIST_P.shp"
ENV_PATH     = PROJECT_ROOT / ".env"


def load_env(path: Path) -> dict:
    env = {}
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, _, v = line.partition("=")
            env[k.strip()] = v.strip()
    return env


def get_conn(env: dict):
    return psycopg2.connect(
        host=env["DB_DASHBOARD_HOST"],
        port=int(env.get("DB_DASHBOARD_PORT", 5432)),
        user=env["DB_DASHBOARD_USER"],
        password=env["DB_DASHBOARD_PASSWORD"],
        dbname=env["DB_DASHBOARD_DBNAME"],
    )


def main():
    env  = load_env(ENV_PATH)
    conn = get_conn(env)

    # --- Load district polygons, reproject to WGS84 for easy point matching ---
    districts = gpd.read_file(SHAPEFILE).to_crs("EPSG:4326")

    # --- Fetch stops from DB ---
    with conn.cursor() as cur:
        cur.execute("SELECT stop_uid, position_lon, position_lat FROM bus_stop_tpe")
        rows = cur.fetchall()

    print(f"Loaded {len(rows):,} stops, {len(districts)} districts.")

    # --- Build GeoDataFrame of stops ---
    stops_gdf = gpd.GeoDataFrame(
        [{"stop_uid": r[0], "geometry": Point(float(r[1]), float(r[2]))} for r in rows],
        crs="EPSG:4326",
    )

    # --- Spatial join ---
    joined = gpd.sjoin(
        stops_gdf,
        districts[["TNAME", "geometry"]],
        how="left",
        predicate="within",
    )

    # TNAME is the district name (e.g. 北投區); NaN for stops outside all polygons
    uid_to_district = dict(zip(joined["stop_uid"], joined["TNAME"]))

    # --- Ensure column exists ---
    with conn.cursor() as cur:
        cur.execute("""
            ALTER TABLE bus_stop_tpe
            ADD COLUMN IF NOT EXISTS district VARCHAR(20)
        """)
    conn.commit()

    # --- Bulk update ---
    updates = [
        (district if isinstance(district, str) else None, uid)
        for uid, district in uid_to_district.items()
    ]

    with conn.cursor() as cur:
        psycopg2.extras.execute_batch(
            cur,
            "UPDATE bus_stop_tpe SET district = %s WHERE stop_uid = %s",
            updates,
            page_size=1000,
        )
    conn.commit()
    conn.close()

    matched   = sum(1 for _, d in uid_to_district.items() if isinstance(d, str))
    unmatched = len(rows) - matched
    print(f"Updated {matched:,} stops with district. {unmatched:,} stops had no match (outside Taipei?).")


if __name__ == "__main__":
    main()
