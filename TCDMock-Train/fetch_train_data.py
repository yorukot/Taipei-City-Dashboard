"""
Fetch TDX TRA (Taiwan Railways Administration) skeleton data:
  - Station master list           → data/train/stations.json
  - General per-train metadata    → data/train/general-train-info.json
  - DailyTimetable for a date     → data/train/timetable-{YYYYMMDD}.json

The DailyTimetable supplies the per-train StopTimes (StopSequence,
StationID, ArrivalTime, DepartureTime) that generate_mock.py walks to
synthesize live delay data.

Endpoints (per swagger.json):
  GET /v2/Rail/TRA/Station
  GET /v2/Rail/TRA/GeneralTrainInfo
  GET /v2/Rail/TRA/DailyTimetable/TrainDate/{TrainDate}
"""

from __future__ import annotations

import argparse
import datetime as dt
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

PAGE_SIZE = 1000
OUT_DIR = pathlib.Path("data/train")


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


def paged_get(path: str, token: str) -> Iterator[dict]:
    """OData $top/$skip pagination for a TDX endpoint."""
    skip = 0
    while True:
        r = requests.get(
            f"{TDX_API}{path}",
            params={"$format": "JSON", "$top": PAGE_SIZE, "$skip": skip},
            headers={
                "Authorization": f"Bearer {token}",
                "Accept-Encoding": "gzip",
            },
            timeout=120,
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


def previous_weekday(today: dt.date) -> dt.date:
    d = today - dt.timedelta(days=1)
    while d.weekday() >= 5:
        d -= dt.timedelta(days=1)
    return d


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--date", help="YYYY-MM-DD; default = most recent past weekday")
    args = parser.parse_args()

    day = dt.date.fromisoformat(args.date) if args.date else previous_weekday(dt.date.today())
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    token = get_token()
    print(f"Got TDX token ({len(token)} chars)\n")

    # 1. Stations
    stations = list(paged_get("/v2/Rail/TRA/Station", token))
    (OUT_DIR / "stations.json").write_text(json.dumps(stations, ensure_ascii=False, indent=2))
    print(f"  ✓ stations.json  ({len(stations)} stations)")
    if stations:
        s = stations[0]
        print(f"      e.g. {s.get('StationID')}  {s.get('StationName',{}).get('Zh_tw')}")

    # 2. General train info (train-class, direction, terminus, etc.)
    general = list(paged_get("/v2/Rail/TRA/GeneralTrainInfo", token))
    (OUT_DIR / "general-train-info.json").write_text(
        json.dumps(general, ensure_ascii=False, indent=2)
    )
    print(f"  ✓ general-train-info.json  ({len(general)} train records)")

    # 3. Daily timetable for the chosen date
    path = f"/v2/Rail/TRA/DailyTimetable/TrainDate/{day.isoformat()}"
    timetable = list(paged_get(path, token))
    out = OUT_DIR / f"timetable-{day.strftime('%Y%m%d')}.json"
    out.write_text(json.dumps(timetable, ensure_ascii=False, indent=2))
    print(f"  ✓ {out.name}  ({len(timetable)} trains for {day})")
    if timetable:
        t = timetable[0]
        info = t.get("DailyTrainInfo") or {}
        stops = t.get("StopTimes") or []
        print(f"      e.g. TrainNo={info.get('TrainNo')}  "
              f"type={info.get('TrainTypeName',{}).get('Zh_tw')}  "
              f"{len(stops)} stops")


if __name__ == "__main__":
    main()
