"""
Visualize a single train's scheduled-vs-actual progression across its stops.

Reads:
  - the timetable file written by fetch_train_data.py (for scheduled times)
  - the per-tick snapshots written by generate_mock.py (for the simulated
    DelayTime at each stop on this train's run)

Renders a Marey-style two-line chart: scheduled arrival time vs simulated
actual arrival time across the stop sequence, with the gap between them
shaded as accumulated delay. Saves to data/train/mock/viz/{TrainNo}.png.

Usage:
  python visualize_mock.py                       # random train
  python visualize_mock.py --trainno 1001
  python visualize_mock.py --bucket delay-prone  # constrain random pick
  python visualize_mock.py --seed 7
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

DATA_DIR = pathlib.Path("data/train")
OUT_DIR = DATA_DIR / "mock"
VIZ_DIR = OUT_DIR / "viz"


def latest_timetable() -> pathlib.Path:
    candidates = sorted(DATA_DIR.glob("timetable-*.json"))
    if not candidates:
        raise SystemExit("no timetable-*.json found in data/train/")
    return candidates[-1]


def parse_hm(s: str) -> int:
    parts = s.split(":")
    return int(parts[0]) * 60 + int(parts[1])


def collect_max_delays() -> dict[str, int]:
    """Scan all snapshots; return TrainNo → max DelayTime seen anywhere."""
    out: dict[str, int] = {}
    for f in sorted(OUT_DIR.glob("*.json")):
        for r in json.load(f.open()):
            tn = r["TrainNo"]
            d = r["DelayTime"]
            if tn not in out or d > out[tn]:
                out[tn] = d
    return out


def collect_train_delays(trainno: str) -> dict[str, int]:
    """Scan all snapshots for `trainno`; return StationID → DelayTime.

    Each snapshot's record references the train's *next* station. The same
    station may appear across multiple ticks while the train approaches/dwells;
    we keep the *max* delay seen for that station, which is the cumulative
    value at the moment the train passes it.
    """
    out: dict[str, int] = {}
    for f in sorted(OUT_DIR.glob("*.json")):
        for r in json.load(f.open()):
            if r["TrainNo"] == trainno:
                sid = r["StationID"]
                d = r["DelayTime"]
                if sid not in out or d > out[sid]:
                    out[sid] = d
                break
    return out


def bucket_of(max_delay: int) -> str:
    if max_delay <= 2:
        return "near-clean"
    if max_delay <= 6:
        return "mild"
    return "delay-prone"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--trainno", help="TrainNo to plot; default = random")
    parser.add_argument("--bucket", choices=("delay-prone", "mild", "near-clean"),
                        help="Constrain random pick to this delay bucket")
    parser.add_argument("--seed", type=int, default=None)
    args = parser.parse_args()

    timetable_path = latest_timetable()
    print(f"Loading {timetable_path.name}…")
    timetable = json.loads(timetable_path.read_text())
    by_trainno = {(t.get("DailyTrainInfo") or {}).get("TrainNo"): t for t in timetable}

    print(f"Scanning snapshots for active trains…")
    max_delays = collect_max_delays()
    print(f"  {len(max_delays)} trains were active at some point")

    # Resolve target train
    if args.trainno:
        if args.trainno not in by_trainno:
            raise SystemExit(f"TrainNo {args.trainno} not in timetable")
        trainno = args.trainno
    else:
        rng = random.Random(args.seed)
        candidates = list(max_delays.keys())
        if args.bucket:
            candidates = [tn for tn in candidates if bucket_of(max_delays[tn]) == args.bucket]
        # Bias toward longer trips so plots have more shape
        candidates = [tn for tn in candidates
                      if len((by_trainno.get(tn) or {}).get("StopTimes") or []) >= 8]
        if not candidates:
            raise SystemExit(f"no candidate trains for bucket={args.bucket}")
        trainno = rng.choice(candidates)

    entry = by_trainno[trainno]
    info = entry["DailyTrainInfo"]
    stops = entry["StopTimes"]
    train_type = (info.get("TrainTypeName") or {}).get("Zh_tw") or "?"
    starting = (info.get("StartingStationName") or {}).get("Zh_tw") or "?"
    ending = (info.get("EndingStationName") or {}).get("Zh_tw") or "?"
    direction = info.get("Direction")

    delay_by_sid = collect_train_delays(trainno)
    if not delay_by_sid:
        raise SystemExit(f"train {trainno} did not appear in any snapshot — was it active today?")

    # Build x/y arrays. Use scheduled arrival as the y axis and stop index as x.
    # Forward-fill missing delays: stops between two ticks may never appear as
    # the "next station" in a snapshot, so we inherit the previous stop's delay
    # rather than dropping to 0 (which would lie about the train recovering).
    seq_idx, station_names, sched_arr, actual_arr, delays = [], [], [], [], []
    base_day = dt.date.today()
    prev_arr_min = -1
    last_known_delay = 0
    for s in stops:
        sid = s["StationID"]
        sname = (s.get("StationName") or {}).get("Zh_tw") or sid
        arr_min = parse_hm(s["ArrivalTime"])
        if prev_arr_min >= 0 and arr_min < prev_arr_min - 60:
            break  # midnight wrap; matches generator's truncation
        prev_arr_min = arr_min
        if sid in delay_by_sid:
            last_known_delay = delay_by_sid[sid]
        d = last_known_delay
        sched_dt = dt.datetime.combine(base_day, dt.time.min) + dt.timedelta(minutes=arr_min)
        actual_dt = sched_dt + dt.timedelta(minutes=d)
        seq_idx.append(s["StopSequence"])
        station_names.append(sname)
        sched_arr.append(sched_dt)
        actual_arr.append(actual_dt)
        delays.append(d)

    max_d = max(delays)
    bucket = bucket_of(max_d)

    fig, (ax, ax2) = plt.subplots(
        2, 1, figsize=(max(12, len(station_names) * 0.45), 7),
        gridspec_kw={"height_ratios": [3, 1]}, sharex=True,
    )

    x = list(range(len(station_names)))
    ax.plot(x, sched_arr, marker="o", linewidth=1.5, color="#1e3a8a",
            label="Scheduled arrival", zorder=3)
    ax.plot(x, actual_arr, marker="o", linewidth=1.5, color="#dc2626",
            label="Simulated actual arrival", zorder=3)
    ax.fill_between(x, sched_arr, actual_arr, color="#dc2626", alpha=0.18,
                    label="Accumulated delay")

    ax.yaxis.set_major_formatter(mdates.DateFormatter("%H:%M"))
    ax.set_ylabel("Time")
    ax.grid(alpha=0.3)
    ax.legend(loc="upper left", framealpha=0.9)

    bars = ax2.bar(x, delays, color=["#10b981" if d <= 2 else "#f59e0b" if d <= 6 else "#dc2626" for d in delays])
    ax2.set_ylabel("Delay (min)")
    ax2.set_ylim(0, max(5, max_d + 2))
    ax2.grid(alpha=0.3, axis="y")
    ax2.set_xticks(x)
    ax2.set_xticklabels(station_names, rotation=70, ha="right", fontsize=8)
    ax2.set_xlabel("Stop")
    for xi, d in zip(x, delays):
        if d > 0:
            ax2.text(xi, d + 0.3, str(d), ha="center", va="bottom", fontsize=7)

    title = (f"TrainNo {trainno}  •  {train_type}  •  {starting} → {ending}"
             f"  •  Direction={direction}  •  bucket≈{bucket}  •  max delay={max_d} min")
    fig.suptitle(title, fontsize=11)

    VIZ_DIR.mkdir(parents=True, exist_ok=True)
    out_path = VIZ_DIR / f"{trainno}.png"
    fig.tight_layout()
    fig.savefig(out_path, dpi=120)
    print(f"  ✓ wrote {out_path}")
    print(f"    {trainno} ({train_type})  {starting} → {ending}  bucket≈{bucket}  max_delay={max_d}")


if __name__ == "__main__":
    main()
