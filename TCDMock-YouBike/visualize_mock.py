"""
Visualize a random station's mock availability across the day.

Reads the per-tick snapshots in data/youbike/mock/ and the station metadata
from data/youbike/{taipei,newtaipei}-stations.json, picks a station, and
renders AvailableRentBikes (with GeneralBikes / ElectricBikes split) over
the full day. Saves a PNG to data/youbike/mock/viz/{StationUID}.png.

Usage:
    python visualize_mock.py                # random station
    python visualize_mock.py --uid TPE500101001
    python visualize_mock.py --seed 7       # reproducible random pick
    python visualize_mock.py --type business # constrain to a type
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
import random

import matplotlib
matplotlib.use("Agg")
import matplotlib.dates as mdates
import matplotlib.font_manager as fm
import matplotlib.pyplot as plt

# Pick a CJK-capable font installed on the system so station names render.
_CJK_CANDIDATES = (
    "PingFang TC", "Heiti TC", "STHeiti", "Hiragino Sans GB",
    "Noto Sans CJK TC", "Arial Unicode MS",
)
_installed = {f.name for f in fm.fontManager.ttflist}
for _f in _CJK_CANDIDATES:
    if _f in _installed:
        plt.rcParams["font.family"] = [_f, "DejaVu Sans"]
        break
plt.rcParams["axes.unicode_minus"] = False

from generate_mock import DATA_DIR, OUT_DIR, STATIONS_FILES, classify_all, load_stations


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--uid", help="StationUID to plot; default = random")
    parser.add_argument("--type", choices=("business", "residential", "mixed", "tourist"),
                        help="Restrict random pick to this type")
    parser.add_argument("--seed", type=int, default=None)
    args = parser.parse_args()

    stations = [s for s in load_stations() if (s.get("BikesCapacity") or 0) > 0]
    by_uid = {s["StationUID"]: s for s in stations}
    districts, types = classify_all(stations)
    meta_by_uid = {s["StationUID"]: (d, t) for s, d, t in zip(stations, districts, types)}

    if args.uid:
        if args.uid not in by_uid:
            raise SystemExit(f"StationUID {args.uid} not found")
        uid = args.uid
    else:
        rng = random.Random(args.seed)
        candidates = [s["StationUID"] for s, t in zip(stations, types)
                      if args.type is None or t == args.type]
        if not candidates:
            raise SystemExit(f"no stations matched --type={args.type}")
        uid = rng.choice(candidates)

    station = by_uid[uid]
    district, kind = meta_by_uid[uid]
    capacity = station["BikesCapacity"]
    name = (station.get("StationName") or {}).get("Zh_tw") or uid

    files = sorted(OUT_DIR.glob("*.json"))
    if not files:
        raise SystemExit(f"no mock files in {OUT_DIR} — run generate_mock.py first")

    times: list[dt.datetime] = []
    rent: list[int] = []
    general: list[int] = []
    electric: list[int] = []
    for f in files:
        for r in json.load(f.open()):
            if r["StationUID"] == uid:
                # Strip tzinfo so matplotlib displays the wall-clock time
                # (Taipei) rather than auto-converting to UTC.
                times.append(dt.datetime.fromisoformat(r["UpdateTime"]).replace(tzinfo=None))
                rent.append(r["AvailableRentBikes"])
                detail = r.get("AvailableRentBikesDetail") or {}
                general.append(detail.get("GeneralBikes", 0))
                electric.append(detail.get("ElectricBikes", 0))
                break

    fig, ax = plt.subplots(figsize=(12, 5))
    ax.fill_between(times, 0, general, label="General bikes",
                    color="#3b82f6", alpha=0.7, step="post")
    ax.fill_between(times, general, [g + e for g, e in zip(general, electric)],
                    label="Electric bikes", color="#10b981", alpha=0.7, step="post")
    ax.plot(times, rent, color="#1e3a8a", linewidth=1.2, label="Available rent bikes")
    ax.axhline(capacity, color="red", linestyle="--", linewidth=0.8,
               label=f"Capacity ({capacity})")

    ax.set_xlabel("Time")
    ax.set_ylabel("Bikes available")
    ax.set_ylim(0, capacity * 1.1)
    ax.xaxis.set_major_formatter(mdates.DateFormatter("%H:%M"))
    ax.xaxis.set_major_locator(mdates.HourLocator(interval=2))
    ax.grid(alpha=0.3)
    ax.legend(loc="upper right", framealpha=0.9)

    title = f"{name}\n{uid}  •  {district}  •  type={kind}  •  capacity={capacity}  •  {times[0].date()}"
    ax.set_title(title, fontsize=11)

    viz_dir = OUT_DIR / "viz"
    viz_dir.mkdir(parents=True, exist_ok=True)
    out_path = viz_dir / f"{uid}.png"
    fig.tight_layout()
    fig.savefig(out_path, dpi=120)
    print(f"  ✓ wrote {out_path}")
    print(f"    {name}  ({district}, {kind}, cap={capacity})")


if __name__ == "__main__":
    main()
