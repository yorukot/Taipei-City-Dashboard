"""
Fetch TDX YouBike station metadata for Taipei City and New Taipei City.

Endpoint:  GET /v2/Bike/Station/City/{City}
Swagger:   ./swagger.json (City enum names verified there)

Output:    data/youbike/{city_lower}-stations.json
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
OUT_DIR = pathlib.Path("data/youbike")


def get_token() -> str:
    """OAuth2 client-credentials grant. Token lasts 24h."""
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
    """Yield every BikeStation record for `city`, paginating with $skip."""
    skip = 0
    while True:
        r = requests.get(
            f"{TDX_API}/v2/Bike/Station/City/{city}",
            params={
                "$format": "JSON",
                "$top": PAGE_SIZE,
                "$skip": skip,
            },
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
        time.sleep(0.2)  # well under the 50/sec/key cap


def summarize(stations: list[dict]) -> None:
    if not stations:
        return
    s = stations[0]
    pos = s.get("StationPosition") or {}
    name = (s.get("StationName") or {}).get("Zh_tw") or "?"
    cap = s.get("BikesCapacity")
    print(
        f"      e.g. {s.get('StationUID'):<14s}  "
        f"{name:<24s}  "
        f"({pos.get('PositionLat'):.4f}, {pos.get('PositionLon'):.4f})  "
        f"capacity={cap}"
    )


def main() -> None:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    token = get_token()
    print(f"Got TDX token ({len(token)} chars)\n")

    for city in CITIES:
        stations = list(fetch_stations(city, token))
        out = OUT_DIR / f"{city.lower()}-stations.json"
        out.write_text(json.dumps(stations, ensure_ascii=False, indent=2))
        print(f"  ✓ {city:10s} → {out.name}  ({len(stations)} stations)")
        summarize(stations)


if __name__ == "__main__":
    main()
