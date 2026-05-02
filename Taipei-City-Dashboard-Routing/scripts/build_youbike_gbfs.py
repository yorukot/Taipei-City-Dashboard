"""
Build a static GBFS 2.3 feed for YouBike (Taipei + New Taipei) so that
OpenTripPlanner can plan multi-modal trips that include bike rentals.

Reads cached TDX BikeStation JSON from ../TCDMock-YouBike/data/youbike/
when available; otherwise OAuth2 + paginates the TDX API directly.

Output: data/gbfs/youbike/{gbfs,system_information,station_information,
station_status}.json — GBFS 2.3 envelopes (last_updated, ttl, version,
data) with JSON booleans, served by the `gbfs` nginx sidecar at
http://gbfs/youbike/gbfs.json on the compose network.

`station_status.json` is a fixed snapshot: every station gets half its
capacity in bikes (>=1 bike, >=1 dock) so OTP never refuses to plan
because a station looks empty/full.
"""

from __future__ import annotations

import json
import os
import pathlib
import time
from typing import Iterator

import requests
from dotenv import load_dotenv

load_dotenv()

TDX_TOKEN_URL = (
    "https://tdx.transportdata.tw/auth/realms/TDXConnect/protocol/openid-connect/token"
)
TDX_API = "https://tdx.transportdata.tw/api/basic"

CITIES = ["Taipei", "NewTaipei"]
PAGE_SIZE = 1000

ROOT = pathlib.Path(__file__).resolve().parents[1]
GBFS_DIR = ROOT / "data" / "gbfs" / "youbike"
CACHE_DIRS = [
    ROOT.parent / "TCDMock-YouBike" / "data" / "youbike",
]

GBFS_VERSION = "2.3"
LANGUAGE = "zh-TW"
SYSTEM_ID = "youbike"
BASE_URL = "http://gbfs/youbike"


def get_token() -> str:
    cid = os.environ["TDX_CLIENT_ID"]
    sec = os.environ["TDX_CLIENT_SECRET"]
    r = requests.post(
        TDX_TOKEN_URL,
        data={
            "grant_type": "client_credentials",
            "client_id": cid,
            "client_secret": sec,
        },
        timeout=15,
    )
    r.raise_for_status()
    return r.json()["access_token"]


def fetch_stations(city: str, token: str) -> Iterator[dict]:
    skip = 0
    while True:
        r = requests.get(
            f"{TDX_API}/v2/Bike/Station/City/{city}",
            params={"$format": "JSON", "$top": PAGE_SIZE, "$skip": skip},
            headers={
                "Authorization": f"Bearer {token}",
                "Accept-Encoding": "gzip",
            },
            timeout=60,
        )
        r.raise_for_status()
        batch = r.json()
        if not batch:
            return
        yield from batch
        if len(batch) < PAGE_SIZE:
            return
        skip += PAGE_SIZE
        time.sleep(0.2)


def load_cached(city: str) -> list[dict] | None:
    fname = f"{city.lower()}-stations.json"
    for cache_dir in CACHE_DIRS:
        path = cache_dir / fname
        if path.exists():
            return json.loads(path.read_text())
    return None


def collect_stations() -> list[dict]:
    all_stations: list[dict] = []
    token: str | None = None
    for city in CITIES:
        cached = load_cached(city)
        if cached is not None:
            print(f"  [cache] {city}: {len(cached)} stations")
            all_stations.extend(cached)
            continue
        if token is None:
            token = get_token()
            print(f"  Got TDX token ({len(token)} chars)")
        stations = list(fetch_stations(city, token))
        print(f"  [tdx]   {city}: {len(stations)} stations")
        all_stations.extend(stations)
    return all_stations


def is_valid(s: dict) -> bool:
    pos = s.get("StationPosition") or {}
    if pos.get("PositionLat") is None or pos.get("PositionLon") is None:
        return False
    cap = s.get("BikesCapacity")
    if cap is None or cap < 1:
        return False
    if not s.get("StationUID"):
        return False
    return True


def to_station_information(stations: list[dict]) -> list[dict]:
    out = []
    for s in stations:
        pos = s["StationPosition"]
        name = (s.get("StationName") or {}).get("Zh_tw") or s["StationUID"]
        addr = (s.get("StationAddress") or {}).get("Zh_tw")
        entry: dict = {
            "station_id": s["StationUID"],
            "name": name,
            "lat": float(pos["PositionLat"]),
            "lon": float(pos["PositionLon"]),
            "capacity": int(s["BikesCapacity"]),
        }
        if addr:
            entry["address"] = addr
        out.append(entry)
    return out


def to_station_status(stations: list[dict], reported: int) -> list[dict]:
    out = []
    for s in stations:
        cap = int(s["BikesCapacity"])
        bikes = max(1, cap // 2)
        docks = max(1, cap - bikes)
        out.append(
            {
                "station_id": s["StationUID"],
                "num_bikes_available": bikes,
                "num_docks_available": docks,
                "is_installed": True,
                "is_renting": True,
                "is_returning": True,
                "last_reported": reported,
            }
        )
    return out


def envelope(data: dict, now: int) -> dict:
    return {
        "last_updated": now,
        "ttl": 0,
        "version": GBFS_VERSION,
        "data": data,
    }


def write_json(path: pathlib.Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2))


def main() -> None:
    raw = collect_stations()
    seen: dict[str, dict] = {}
    for s in raw:
        uid = s.get("StationUID")
        if not uid or uid in seen:
            continue
        seen[uid] = s
    deduped = list(seen.values())
    valid = [s for s in deduped if is_valid(s)]
    print(
        f"\n  raw={len(raw)}  deduped={len(deduped)}  "
        f"valid={len(valid)}  skipped={len(deduped) - len(valid)}"
    )

    now = int(time.time())

    gbfs_index = {
        "last_updated": now,
        "ttl": 0,
        "version": GBFS_VERSION,
        "data": {
            LANGUAGE: {
                "feeds": [
                    {"name": "system_information", "url": f"{BASE_URL}/system_information.json"},
                    {"name": "station_information", "url": f"{BASE_URL}/station_information.json"},
                    {"name": "station_status", "url": f"{BASE_URL}/station_status.json"},
                ]
            }
        },
    }

    system_info = envelope(
        {
            "system_id": SYSTEM_ID,
            "language": LANGUAGE,
            "name": "YouBike",
            "short_name": "YouBike",
            "operator": "YouBike Co., Ltd.",
            "timezone": "Asia/Taipei",
        },
        now,
    )

    station_info = envelope({"stations": to_station_information(valid)}, now)
    station_status = envelope({"stations": to_station_status(valid, now)}, now)

    write_json(GBFS_DIR / "gbfs.json", gbfs_index)
    write_json(GBFS_DIR / "system_information.json", system_info)
    write_json(GBFS_DIR / "station_information.json", station_info)
    write_json(GBFS_DIR / "station_status.json", station_status)

    print(f"\nWrote 4 GBFS files to {GBFS_DIR.resolve()}")


if __name__ == "__main__":
    main()
