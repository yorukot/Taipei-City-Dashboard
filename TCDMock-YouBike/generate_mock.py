"""
Generate mock YouBike availability snapshots at 5-min intervals over a day.

Reads station metadata from data/youbike/{taipei,newtaipei}-stations.json
(produced by fetch_youbike_data.py) and writes one BikeAvailability JSON
array per tick to data/youbike/mock/YYYYMMDD-HHMM.json.

Behavior model:
  - Each station is classified by district / name into one of four types:
    business, residential, mixed, tourist.
  - Each type has a signed flow profile across the day (sum of gaussian
    bumps) describing expected change in AvailableRentBikes per 5-min tick.
    Business stations gain bikes during morning rush (people arriving at
    work) and lose them in the evening; residential is the mirror image;
    tourist stations have a broad afternoon arc; mixed has muted peaks.
  - Per-station "intensity" is a clipped gaussian multiplier; flow scales
    with capacity / 20.
  - Initial state at 00:00 set per type (residential ~70% full,
    business ~25%, mixed/tourist ~50%).
  - Gaussian noise added each tick; result clipped to [0, capacity].
  - GeneralBikes / ElectricBikes split per station via fixed e-bike share.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib

import numpy as np

DATA_DIR = pathlib.Path("data/youbike")
STATIONS_FILES = ["taipei-stations.json", "newtaipei-stations.json"]
OUT_DIR = DATA_DIR / "mock"

TICKS_PER_DAY = 288
SECONDS_PER_TICK = 300
TZ = dt.timezone(dt.timedelta(hours=8))

# District centroids (lat, lon) and the behavior type each represents.
# StationAddress doesn't include district info, so we classify by closest
# centroid. Coverage targets the Taipei + New Taipei urban core where
# nearly all YouBike stations sit; fringe townships are absorbed into the
# nearest neighbor (acceptable — they have very few stations).
#
# Types:
#   business    — CBDs / office clusters (Xinyi, Zhongzheng, Songshan, Neihu tech park)
#   residential — bedroom districts; outflow in AM, inflow in PM
#   mixed       — dense areas with both office and residential blocks
#   tourist     — recreational / nightlife (Shilin, Wanhua, riverside towns)
DISTRICTS: tuple[tuple[str, float, float, str], ...] = (
    # Taipei City
    ("中正區", 25.038, 121.518, "business"),
    ("信義區", 25.030, 121.572, "business"),
    ("松山區", 25.058, 121.563, "business"),
    ("內湖區", 25.082, 121.595, "business"),
    ("南港區", 25.054, 121.616, "business"),
    ("大安區", 25.025, 121.543, "mixed"),
    ("中山區", 25.063, 121.534, "mixed"),
    ("大同區", 25.063, 121.514, "mixed"),
    ("文山區", 24.990, 121.570, "residential"),
    ("北投區", 25.130, 121.500, "residential"),
    ("士林區", 25.088, 121.524, "tourist"),
    ("萬華區", 25.034, 121.500, "tourist"),
    # New Taipei City — urban core
    ("板橋區", 25.013, 121.467, "mixed"),
    ("三重區", 25.075, 121.494, "residential"),
    ("中和區", 24.999, 121.500, "residential"),
    ("永和區", 25.007, 121.515, "residential"),
    ("新莊區", 25.043, 121.450, "residential"),
    ("新店區", 24.971, 121.541, "residential"),
    ("土城區", 24.973, 121.443, "residential"),
    ("蘆洲區", 25.087, 121.471, "residential"),
    ("樹林區", 24.991, 121.420, "residential"),
    ("汐止區", 25.063, 121.658, "residential"),
    ("淡水區", 25.169, 121.443, "tourist"),
    ("林口區", 25.077, 121.388, "residential"),
    ("五股區", 25.083, 121.438, "residential"),
    ("泰山區", 25.060, 121.430, "residential"),
    ("八里區", 25.146, 121.402, "tourist"),
    ("三峽區", 24.934, 121.371, "tourist"),
    ("鶯歌區", 24.953, 121.354, "tourist"),
    ("瑞芳區", 25.108, 121.811, "tourist"),
)


def classify_all(stations: list[dict]) -> tuple[list[str], list[str]]:
    """Return (district_name, type) per station via closest-centroid lookup."""
    coords = np.array([
        [(s.get("StationPosition") or {}).get("PositionLat") or 0.0,
         (s.get("StationPosition") or {}).get("PositionLon") or 0.0]
        for s in stations
    ])
    centroids = np.array([[d[1], d[2]] for d in DISTRICTS])
    # Squared euclidean in degrees — fine for ranking at this scale.
    d2 = ((coords[:, None, :] - centroids[None, :, :]) ** 2).sum(axis=2)
    nearest = d2.argmin(axis=1)
    names = [DISTRICTS[i][0] for i in nearest]
    types = [DISTRICTS[i][3] for i in nearest]
    return names, types


def build_profile(kind: str) -> np.ndarray:
    """Return signed expected-delta-per-tick (288,) for a capacity-20 baseline."""
    minutes = np.arange(TICKS_PER_DAY) * 5

    def gauss(center_min: float, sigma_min: float, amplitude: float) -> np.ndarray:
        return amplitude * np.exp(-0.5 * ((minutes - center_min) / sigma_min) ** 2)

    if kind == "business":
        return (
            gauss(8.5 * 60, 45, +0.9)   # morning: bikes arriving (returns)
            + gauss(18.0 * 60, 50, -1.0)  # evening: bikes leaving (rents)
            + gauss(12.5 * 60, 25, -0.3)  # lunch out
            + gauss(13.5 * 60, 25, +0.3)  # lunch back
        )
    if kind == "residential":
        return (
            gauss(8.0 * 60, 45, -1.0)   # morning: bikes leaving
            + gauss(18.5 * 60, 50, +0.9)  # evening: bikes returning
        )
    if kind == "tourist":
        return (
            gauss(13.0 * 60, 90, +0.5)
            + gauss(18.5 * 60, 80, -0.5)
        )
    # mixed
    return (
        gauss(8.5 * 60, 60, +0.35)
        + gauss(8.0 * 60, 60, -0.30)
        + gauss(18.0 * 60, 60, -0.35)
        + gauss(18.5 * 60, 60, +0.30)
    )


def previous_weekday(today: dt.date) -> dt.date:
    d = today - dt.timedelta(days=1)
    while d.weekday() >= 5:
        d -= dt.timedelta(days=1)
    return d


def load_stations() -> list[dict]:
    out: list[dict] = []
    for fn in STATIONS_FILES:
        path = DATA_DIR / fn
        with path.open() as f:
            out.extend(json.load(f))
    return out


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--date", help="YYYY-MM-DD; default = most recent weekday")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    day = dt.date.fromisoformat(args.date) if args.date else previous_weekday(dt.date.today())
    rng = np.random.default_rng(args.seed)

    print(f"Loading stations…")
    stations = [s for s in load_stations() if (s.get("BikesCapacity") or 0) > 0]
    n = len(stations)
    print(f"  {n} stations with capacity > 0")

    districts, types = classify_all(stations)
    type_counts = {t: types.count(t) for t in ("business", "residential", "mixed", "tourist")}
    print(f"  by type: {type_counts}")
    top_districts = sorted({d: districts.count(d) for d in set(districts)}.items(),
                           key=lambda x: -x[1])[:8]
    print(f"  top districts: {top_districts}")

    profiles = {t: build_profile(t) for t in ("business", "residential", "mixed", "tourist")}
    profile_arr = np.stack([profiles[t] for t in types])  # (n, 288)

    capacities = np.array([s["BikesCapacity"] for s in stations], dtype=np.int64)
    intensities = np.clip(rng.normal(1.0, 0.4, size=n), 0.2, 2.5)
    profile_arr = profile_arr * (capacities[:, None] / 20.0) * intensities[:, None]

    is_youbike2 = np.array([s.get("ServiceType") == 2 for s in stations])
    ebike_share = np.where(
        is_youbike2,
        rng.uniform(0.10, 0.30, size=n),
        rng.uniform(0.00, 0.10, size=n),
    )

    init_pct = {"business": 0.25, "residential": 0.70, "mixed": 0.50, "tourist": 0.50}
    rent = np.clip(
        np.round(np.array([init_pct[t] for t in types]) * capacities).astype(np.int64),
        0,
        capacities,
    )

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    midnight = dt.datetime.combine(day, dt.time.min, tzinfo=TZ)

    print(f"Generating {TICKS_PER_DAY} ticks for {day} (seed={args.seed})…")

    for t in range(TICKS_PER_DAY):
        expected = profile_arr[:, t]
        noise = rng.normal(0.0, 0.5 * np.sqrt(np.abs(expected) + 1.0))
        delta = np.round(expected + noise).astype(np.int64)
        rent = np.clip(rent + delta, 0, capacities)

        ts = midnight + dt.timedelta(seconds=t * SECONDS_PER_TICK)
        ts_iso = ts.isoformat(timespec="seconds")

        records = []
        for i, s in enumerate(stations):
            cap = int(capacities[i])
            rec_rent = int(rent[i])
            rec_return = cap - rec_rent
            ebikes = int(round(rec_rent * float(ebike_share[i])))
            generals = rec_rent - ebikes
            records.append({
                "StationUID": s.get("StationUID"),
                "StationID": s.get("StationID"),
                "ServiceStatus": 1,
                "ServiceType": s.get("ServiceType"),
                "AvailableRentBikes": rec_rent,
                "AvailableReturnBikes": rec_return,
                "AvailableRentBikesDetail": {
                    "GeneralBikes": generals,
                    "ElectricBikes": ebikes,
                },
                "SrcUpdateTime": ts_iso,
                "UpdateTime": ts_iso,
            })

        out_path = OUT_DIR / f"{ts.strftime('%Y%m%d-%H%M')}.json"
        with out_path.open("w") as f:
            json.dump(records, f, ensure_ascii=False)

        if t % 36 == 0:
            print(f"  {ts.strftime('%H:%M')}  {out_path.name}")

    print(f"\nDone. {TICKS_PER_DAY} files in {OUT_DIR}")


if __name__ == "__main__":
    main()
