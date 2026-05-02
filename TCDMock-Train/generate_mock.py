"""
Generate mock TRA LiveBoard snapshots at 5-min intervals over a day.

Reads the timetable produced by fetch_train_data.py and synthesizes per-train
delay accumulation, then emits one RailLiveBoard-shaped JSON file per tick to
data/train/mock/HHMM.json.

Delay model — per-train rate is sampled from a three-bucket mixture:
  20% delay-prone    rate ~ N(0.15, 0.08)  clipped [0.00, 0.30]
  50% mild           rate ~ N(0.05, 0.02)  clipped [0.00, 0.10]
  30% near-clean     rate ~ N(0.03, 0.015) clipped [0.00, 0.07]

Cumulative delay for stop k:
  segment_min  = scheduled_arr[k] - scheduled_dep[k-1]
  seg_delay    = N(rate * segment_min, 0.4)
  cum[k]       = clip(cum[k-1] + seg_delay, 0, 30 min)

Per-tick snapshot: emit a LiveBoard record for every train that, at this
tick, has departed its origin (with delay) but has not yet reached terminus.
The record references the train's *next* station.

No cancellations — SuspendedFlag is always 0.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import pathlib
from typing import Any

import numpy as np

DATA_DIR = pathlib.Path("data/train")
OUT_DIR = DATA_DIR / "mock"

TICKS_PER_DAY = 288
SECONDS_PER_TICK = 300
TZ = dt.timezone(dt.timedelta(hours=8))
MAX_CUM_DELAY_MIN = 30.0
SEG_NOISE_SIGMA = 0.4

BUCKETS = (
    # (cumulative threshold, mean, sigma, clip_max, label)
    (0.20, 0.15, 0.08,  0.30, "delay-prone"),
    (0.70, 0.05, 0.02,  0.10, "mild"),
    (1.00, 0.03, 0.015, 0.07, "near-clean"),
)


def parse_hm(s: str) -> int:
    """Parse 'HH:MM' or 'HH:MM:SS' to minutes since midnight."""
    parts = s.split(":")
    return int(parts[0]) * 60 + int(parts[1])


def sample_rate(rng: np.random.Generator) -> tuple[float, str]:
    u = float(rng.random())
    cum = 0.0
    for thresh, mean, sigma, clip_max, label in BUCKETS:
        if u < thresh:
            r = float(np.clip(rng.normal(mean, sigma), 0.0, clip_max))
            return r, label
        cum = thresh
    # unreachable
    return 0.0, "near-clean"


def find_latest_train_info(timetable: list[dict]) -> dict[str, dict]:
    """Last-write-wins map of TrainNo → latest DailyTrainInfo (for de-dup)."""
    out: dict[str, dict] = {}
    for t in timetable:
        info = t.get("DailyTrainInfo") or {}
        tn = info.get("TrainNo")
        if tn:
            out[tn] = info
    return out


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--date", help="YYYY-MM-DD; defaults to the timetable file's TrainDate")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    if args.date:
        day = dt.date.fromisoformat(args.date)
        timetable_path = DATA_DIR / f"timetable-{day.strftime('%Y%m%d')}.json"
    else:
        # Pick the most recent timetable file in data/train/.
        candidates = sorted(DATA_DIR.glob("timetable-*.json"))
        if not candidates:
            raise SystemExit("no timetable-*.json found in data/train/ — run fetch_train_data.py first")
        timetable_path = candidates[-1]
        day = dt.date.fromisoformat(timetable_path.stem.split("-")[1][:4]
                                    + "-" + timetable_path.stem.split("-")[1][4:6]
                                    + "-" + timetable_path.stem.split("-")[1][6:])

    print(f"Loading timetable {timetable_path.name} (day = {day})…")
    timetable = json.loads(timetable_path.read_text())
    print(f"  {len(timetable)} train records")

    rng = np.random.default_rng(args.seed)

    # Per-train state: rate, bucket, list of stops with cumulative delays.
    bucket_counts: dict[str, int] = {}
    train_state: list[dict[str, Any]] = []

    for entry in timetable:
        info = entry.get("DailyTrainInfo") or {}
        stops = entry.get("StopTimes") or []
        if len(stops) < 2:
            continue

        rate, bucket = sample_rate(rng)
        bucket_counts[bucket] = bucket_counts.get(bucket, 0) + 1

        # Parse all stop times to minutes-since-midnight.
        sched: list[dict[str, Any]] = []
        prev_dep = None
        cum = 0.0
        for s in stops:
            arr_min = parse_hm(s["ArrivalTime"])
            dep_min = parse_hm(s["DepartureTime"])
            # Heuristic: if a stop appears to go backwards in time (e.g.
            # midnight wrap), drop everything from here on — we only
            # simulate within one day and TRA cross-midnight runs are rare.
            if prev_dep is not None and arr_min < prev_dep - 60:
                break
            seg_min = 0 if prev_dep is None else max(0, arr_min - prev_dep)
            seg_delay = float(rng.normal(rate * seg_min, SEG_NOISE_SIGMA))
            cum = float(np.clip(cum + seg_delay, 0.0, MAX_CUM_DELAY_MIN))
            sched.append({
                "StopSequence": s["StopSequence"],
                "StationID": s["StationID"],
                "StationName": s["StationName"],
                "ScheduledArrivalTime": s["ArrivalTime"],
                "ScheduledDepartureTime": s["DepartureTime"],
                "arr_min": arr_min,
                "dep_min": dep_min,
                "cum_delay_min": cum,
            })
            prev_dep = dep_min

        if len(sched) < 2:
            continue

        train_state.append({
            "info": info,
            "rate": rate,
            "bucket": bucket,
            "sched": sched,
        })

    print(f"  per-train delay buckets: {bucket_counts}")
    print(f"  modeled trains: {len(train_state)}")

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    midnight = dt.datetime.combine(day, dt.time.min, tzinfo=TZ)

    print(f"\nGenerating {TICKS_PER_DAY} ticks…")

    written = 0
    for t in range(TICKS_PER_DAY):
        tick_min = t * 5  # minutes since midnight
        ts = midnight + dt.timedelta(seconds=t * SECONDS_PER_TICK)
        ts_iso = ts.isoformat(timespec="seconds")

        records = []
        for tr in train_state:
            sched = tr["sched"]
            info = tr["info"]
            origin = sched[0]
            terminus = sched[-1]

            # Train hasn't departed origin (incl. delay) yet?
            if tick_min < origin["dep_min"] + origin["cum_delay_min"]:
                continue
            # Train has reached terminus (incl. delay)?
            if tick_min >= terminus["arr_min"] + terminus["cum_delay_min"]:
                continue

            # Find the next station (smallest k where actual_arr[k] > tick_min).
            next_idx = None
            for i in range(1, len(sched)):
                if sched[i]["arr_min"] + sched[i]["cum_delay_min"] > tick_min:
                    next_idx = i
                    break
            if next_idx is None:
                continue
            stop = sched[next_idx]

            records.append({
                "StationID": stop["StationID"],
                "StationName": stop["StationName"],
                "TrainNo": info.get("TrainNo"),
                "Direction": info.get("Direction"),
                "TrainTypeID": info.get("TrainTypeID"),
                "TrainTypeCode": info.get("TrainTypeCode"),
                "TrainTypeName": info.get("TrainTypeName"),
                "TripLine": info.get("TripLine"),
                "EndingStationID": info.get("EndingStationID"),
                "EndingStationName": info.get("EndingStationName"),
                "ScheduledArrivalTime": stop["ScheduledArrivalTime"],
                "ScheduledDepartureTime": stop["ScheduledDepartureTime"],
                "DelayTime": int(round(stop["cum_delay_min"])),
                "SuspendedFlag": 0,
                "SrcUpdateTime": ts_iso,
                "UpdateTime": ts_iso,
            })

        out_path = OUT_DIR / f"{ts.strftime('%H%M')}.json"
        with out_path.open("w") as f:
            json.dump(records, f, ensure_ascii=False)
        written += 1

        if t % 36 == 0:
            print(f"  {ts.strftime('%H:%M')}  {len(records):>4d} active trains  → {out_path.name}")

    print(f"\nDone. {written} files in {OUT_DIR}")


if __name__ == "__main__":
    main()
