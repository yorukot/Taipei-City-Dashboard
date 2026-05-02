# Self-Hosting a Taipei Transit Trip Planner

A docker-compose tutorial for setting up **OpenTripPlanner** (multi-modal routing) and **Nominatim** (address geocoding) with Taiwan/Taipei data. End state: a local stack you can hit at `http://localhost:8080` for routing and `http://localhost:8081` for geocoding, ready to drive the Python script from earlier.

---

## 1. Architecture

```
                          ┌──────────────────┐
   address string  ─────► │   Nominatim      │ ──► lat/lon
                          │  (port 8081)     │
                          └──────────────────┘
                                   │
                                   ▼
                          ┌──────────────────┐
   from + to + time ────► │ OpenTripPlanner  │ ──► itineraries
                          │  (port 8080)     │
                          └──────────────────┘
                                   ▲
                ┌──────────────────┴──────────────────┐
                │                                     │
        Taiwan OSM (Geofabrik)              GTFS feeds (TDX)
        ~308 MB .pbf                        TRTC / TPC / NTPC / TRA / THSR
```

Two long-running services (Nominatim, OTP), plus two one-shot jobs that do data prep:

- **`gtfs-fetch`** — pulls the latest GTFS zips from TDX into `./data/otp/`
- **`otp-build`** — runs OTP with `--build --save` to produce `graph.obj`, then exits

The two long-running services don't restart unless you rebuild data. You can re-run the prep jobs on a cron (weekly is plenty for static GTFS).

## 2. Prerequisites

- Docker + Compose v2 (Docker Desktop or any modern Linux host).
- **RAM**: ~10 GB free during builds. Nominatim wants ~4 GB to import Taiwan; OTP wants ~4 GB to build the graph and ~2 GB to serve it. After the imports complete you can drop OTP's heap to 2 GB.
- **Disk**: ~15 GB. Nominatim's Postgres ends up around 8 GB for Taiwan; OSM + GTFS + OTP graph is another 2–3 GB.
- A **TDX account**. Register at <https://tdx.transportdata.tw/>, wait for approval, then create an API key under the member center. You get a Client ID and Client Secret. The free tier is generous (50 req/sec/key).

## 3. Project layout

```
taipei-otp/
├── docker-compose.yml
├── .env                       # TDX credentials (gitignored)
├── scripts/
│   └── fetch_tdx_gtfs.py
├── data/
│   ├── otp/                   # OSM + GTFS + graph.obj live here
│   │   └── (filled in below)
│   └── nominatim/
│       └── (postgres data, auto-created)
└── config/
    └── otp/
        ├── build-config.json
        └── router-config.json
```

Create the directories now:

```bash
mkdir -p taipei-otp/{scripts,data/otp,data/nominatim,config/otp}
cd taipei-otp
```

## 4. TDX credentials

Create `.env` (don't commit this):

```bash
# .env
TDX_CLIENT_ID=your_client_id_here
TDX_CLIENT_SECRET=your_client_secret_here
NOMINATIM_PASSWORD=$(openssl rand -hex 16)
```

TDX uses OAuth2 client-credentials. The token endpoint is:
`https://tdx.transportdata.tw/auth/realms/TDXConnect/protocol/openid-connect/token`

Tokens last 24 hours. Don't request a fresh one per call — the token endpoint is rate-limited to 20 requests/min/IP.

## 5. The GTFS fetch script

TDX exposes per-operator static GTFS feeds. Save this as `scripts/fetch_tdx_gtfs.py`:

```python
"""
Download TDX GTFS feeds for the Taipei region and place them under
./data/otp/ so OTP can pick them up.

OTP requires GTFS files to:
  - have ".zip" extension
  - contain the literal substring "gtfs" in the filename
"""
from __future__ import annotations

import os
import pathlib
import sys
import time
import requests

TDX_TOKEN_URL = (
    "https://tdx.transportdata.tw/auth/realms/TDXConnect"
    "/protocol/openid-connect/token"
)
TDX_API = "https://tdx.transportdata.tw/api/basic"

# Operators relevant to Taipei multi-modal routing.
# Add or remove as you like — every entry becomes a separate GTFS zip.
OPERATORS = {
    "trtc":  "TRTC",   # Taipei Metro
    "tpc":   "TPC",    # Taipei City buses (gov't operator)
    "ntpc":  "NTPC",   # New Taipei City buses
    "tra":   "TRA",    # Taiwan Railways (optional but very useful)
    "thsr":  "THSR",   # Taiwan High Speed Rail (optional)
    # "krtc":  "KRTC", # Kaohsiung Metro — uncomment for nationwide
}

OUT_DIR = pathlib.Path("data/otp")


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


def download_operator(op_code: str, out_path: pathlib.Path, token: str) -> None:
    # TDX GTFS Static API (the "GTFS服務" beta). The exact path occasionally
    # shifts between v2 and v3 — check the swagger if this 404s:
    # https://tdx.transportdata.tw/api-service/swagger
    url = f"{TDX_API}/v2/Gtfs/Schedule/Operator/{op_code}"
    r = requests.get(
        url,
        headers={
            "Authorization": f"Bearer {token}",
            "Accept-Encoding": "gzip",
        },
        timeout=120,
        stream=True,
    )
    r.raise_for_status()
    out_path.write_bytes(r.content)
    size_mb = out_path.stat().st_size / 1024 / 1024
    print(f"  ✓ {op_code:6s} → {out_path.name}  ({size_mb:.1f} MB)")


def main() -> None:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    token = get_token()
    print(f"Got TDX access token ({len(token)} chars)")

    for label, code in OPERATORS.items():
        out = OUT_DIR / f"{label}-gtfs.zip"
        try:
            download_operator(code, out, token)
        except requests.HTTPError as e:
            print(f"  ✗ {code}: {e}", file=sys.stderr)
        time.sleep(0.5)  # be polite

    print(f"\nDone. Files in {OUT_DIR.resolve()}:")
    for p in sorted(OUT_DIR.glob("*-gtfs.zip")):
        print(f"  - {p.name}")


if __name__ == "__main__":
    main()
```

Run it once locally to verify your credentials work:

```bash
pip install requests
set -a && . ./.env && set +a   # load .env into the shell
python scripts/fetch_tdx_gtfs.py
```

You should end up with `data/otp/trtc-gtfs.zip`, `tpc-gtfs.zip`, etc.

> **If the URL 404s:** TDX has been rolling out a v3 GTFS API alongside v2. Open the swagger at <https://tdx.transportdata.tw/api-service/swagger>, search for "Gtfs", and copy the current path into the script. The OAuth flow is the same.

## 6. Download the OSM extract

```bash
curl -L https://download.geofabrik.de/asia/taiwan-latest.osm.pbf \
     -o data/otp/taiwan.osm.pbf
```

About 308 MB. Update this monthly if you care about new roads.

## 7. OTP configuration

`config/otp/build-config.json` — minimal but explicit about feed IDs:

```json
{
  "osmDefaults": { "timeZone": "Asia/Taipei" },
  "transitFeeds": [
    { "type": "gtfs", "feedId": "trtc",  "source": "trtc-gtfs.zip" },
    { "type": "gtfs", "feedId": "tpc",   "source": "tpc-gtfs.zip"  },
    { "type": "gtfs", "feedId": "ntpc",  "source": "ntpc-gtfs.zip" },
    { "type": "gtfs", "feedId": "tra",   "source": "tra-gtfs.zip"  },
    { "type": "gtfs", "feedId": "thsr",  "source": "thsr-gtfs.zip" }
  ],
  "osm": [
    { "source": "taiwan.osm.pbf" }
  ]
}
```

Distinct `feedId`s are critical — TDX operator feeds reuse `agency_id` and `route_id` namespaces, and without unique feed IDs OTP will silently merge unrelated routes.

`config/otp/router-config.json` — keep it simple for now:

```json
{
  "routingDefaults": {
    "numItineraries": 5,
    "walkSpeed": 1.3,
    "transferSlack": 120,
    "locale": "zh-TW"
  },
  "timetableUpdates": { "maxSnapshotFrequency": "PT1S" },
  "updaters": []
}
```

You can wire up GTFS-Realtime (TPE bus arrivals etc.) into `updaters` later — TDX has those too.

## 8. The docker-compose.yml

```yaml
services:
  # ─── data prep (one-shot) ──────────────────────────────────────
  gtfs-fetch:
    image: python:3.12-slim
    profiles: ["build"]
    working_dir: /work
    volumes:
      - ./:/work
    environment:
      - TDX_CLIENT_ID
      - TDX_CLIENT_SECRET
    command: >
      bash -c "pip install --quiet requests &&
               python scripts/fetch_tdx_gtfs.py"

  otp-build:
    image: docker.io/opentripplanner/opentripplanner:latest
    profiles: ["build"]
    environment:
      JAVA_TOOL_OPTIONS: "-Xmx6g"
    volumes:
      - ./data/otp:/var/opentripplanner
      - ./config/otp/build-config.json:/var/opentripplanner/build-config.json:ro
      - ./config/otp/router-config.json:/var/opentripplanner/router-config.json:ro
    command: ["--build", "--save"]
    depends_on:
      gtfs-fetch:
        condition: service_completed_successfully

  # ─── long-running services ─────────────────────────────────────
  otp:
    image: docker.io/opentripplanner/opentripplanner:latest
    restart: unless-stopped
    environment:
      JAVA_TOOL_OPTIONS: "-Xmx3g"
    ports:
      - "8080:8080"
    volumes:
      - ./data/otp:/var/opentripplanner
      - ./config/otp/router-config.json:/var/opentripplanner/router-config.json:ro
    command: ["--load", "--serve"]

  nominatim:
    image: mediagis/nominatim:5.1
    restart: unless-stopped
    ports:
      - "8081:8080"
    environment:
      PBF_PATH: /nominatim/data/taiwan.osm.pbf
      NOMINATIM_PASSWORD: ${NOMINATIM_PASSWORD}
      IMPORT_STYLE: address      # smaller index than 'full', plenty for geocoding
      REPLICATION_URL: https://download.geofabrik.de/asia/taiwan-updates/
      THREADS: 4
    volumes:
      - ./data/otp/taiwan.osm.pbf:/nominatim/data/taiwan.osm.pbf:ro
      - nominatim-pgdata:/var/lib/postgresql/16/main
      - nominatim-flatnode:/nominatim/flatnode
    shm_size: 1gb

volumes:
  nominatim-pgdata:
  nominatim-flatnode:
```

A few things to know about this file:

- The `profiles: ["build"]` keeps `gtfs-fetch` and `otp-build` out of the default `up` — they only run when you ask. This is intentional: rebuilding the OTP graph takes 5–10 minutes and you don't want it to happen on every restart.
- Nominatim re-uses the same `taiwan.osm.pbf` file that OTP uses. One download, two consumers.
- The Nominatim image runs Postgres inside the container; the `nominatim-pgdata` named volume keeps the imported database across restarts so the multi-hour import only happens once.

## 9. The build phase

The first time, run the build profile to fetch GTFS and produce the OTP graph:

```bash
docker compose --profile build up --abort-on-container-exit
```

Watch the logs. `gtfs-fetch` finishes in seconds. `otp-build` will spend a few minutes parsing the OSM file and linking transit stops to the street graph. When you see it write `graph.obj` and exit cleanly, the build is done. You can verify:

```bash
ls -lh data/otp/graph.obj
# -rw-r--r-- 1 you you 1.2G ...
```

In parallel, kick off Nominatim — its first start triggers a one-time OSM import:

```bash
docker compose up -d nominatim
docker compose logs -f nominatim
```

For Taiwan, the import takes roughly 30–60 minutes on a decent machine. You'll see progress through `osm2pgsql`, then rank-by-rank indexing. When the logs settle into Apache request lines, it's ready. Test it:

```bash
curl 'http://localhost:8081/search?q=台北101&format=json&countrycodes=tw' | jq '.[0]'
```

You should get back a lat/lon for Taipei 101.

## 10. Bring up OTP

```bash
docker compose up -d otp
docker compose logs -f otp
```

Wait for `Grizzly server running` — that's the all-clear. The OTP debug UI lives at <http://localhost:8080/>; right-click on the map twice to set origin and destination.

## 11. Wire up the Python script

Point the earlier script's endpoints at the local stack:

```python
OTP_BASE = "http://localhost:8080/otp/routers/default"
NOMINATIM_BASE = "http://localhost:8081"
USER_AGENT = "taipei-transit-demo/0.1 (local)"
```

> **OTP version note:** the legacy REST `/plan` endpoint exists in OTP 2.x but is being phased out in favor of the GTFS GraphQL API at `/otp/gtfs/v1`. If `/plan` 404s on a future image, switch the script to POST a GraphQL `plan` query — the parameters map 1-to-1.

Run it:

```bash
python plan_taipei_route.py
```

…and you should get itineraries from Banqiao 文化路 to Wenshan 興隆路 using TRTC + buses.

## 12. Keeping it fresh

Static GTFS feeds change roughly weekly as agencies tweak schedules. A simple weekly refresh cron:

```bash
# every Monday 04:00, redownload GTFS and rebuild graph
0 4 * * 1 cd /opt/taipei-otp && \
  docker compose --profile build run --rm gtfs-fetch && \
  docker compose --profile build run --rm otp-build && \
  docker compose restart otp
```

OSM data refreshes itself: Nominatim's `REPLICATION_URL` env var causes it to apply daily diffs from Geofabrik in the background. For OTP, re-download `taiwan.osm.pbf` along with the GTFS rebuild if you care about new streets.

## 13. Common gotchas

- **OTP build OOMs.** Bump `JAVA_TOOL_OPTIONS=-Xmx8g` on the `otp-build` service and make sure Docker Desktop has at least 10 GB allocated.
- **Nominatim returns nothing for `巷` / `弄` addresses.** OSM's coverage of Taiwanese alley/lane geometries is patchy. For production, layer Pelias on top with the 內政部 (TGOS) address dataset — much higher hit rate. Nominatim alone is fine for street-level testing.
- **OTP says "no itinerary" for a route you know works.** Almost always a date/time issue: GTFS calendars have explicit service date ranges, and TDX feeds typically only cover ~30 days forward. Make sure your query's `date` is within that window. Also check that all `feedId`s in `build-config.json` are unique — see §7.
- **`Failed to parse GTFS` during build.** Some TDX operator zips occasionally ship with a stray BOM in `agency.txt` or with `stop_times.txt` rows that violate strict GTFS validation. Run the file through MobilityData's `gtfs-validator` to find the bad row, then either patch the zip or open a ticket with TDX.
- **Slow startup after a reboot.** OTP loading `graph.obj` from disk is single-threaded and takes 30–60 seconds for a country-sized graph. Normal.

## 14. Where to go from here

- **Real-time bus arrivals.** Add a `gtfs-rt` updater in `router-config.json` pointing at TDX's GTFS-Realtime feeds (Taipei + New Taipei publish trip-updates and vehicle-positions).
- **YouBike.** A static GBFS feed for YouBike 2.0 is wired up. The build flow:

  ```bash
  uv run python scripts/build_youbike_gbfs.py        # writes data/gbfs/youbike/*.json
  docker compose up -d gbfs                          # nginx sidecar on host :8082
  docker compose restart otp                         # OTP picks up the updater
  ```

  The script reads cached TDX BikeStation JSON from `../TCDMock-YouBike/data/youbike/` if present, otherwise OAuth2-fetches it. `station_status.json` is a fixed snapshot (every station gets half its capacity in bikes), so OTP treats every station as always rentable — fine for routing demos, not for live availability. Re-run the script quarterly to pick up new/closed stations. Live availability would require periodically rewriting `station_status.json` from `/v2/Bike/Availability/City/{City}`.

  Note: the FastAPI wrapper at `routes.py` still only forwards transit modes — to surface bike-rental itineraries via `POST /plan`, extend its GraphQL `modes` block with `BICYCLE_RENT` and `allowedVehicleRentalNetworks: ["youbike"]`. Until then, exercise bike-rental routing via the OTP debug UI at <http://localhost:8080/>.
- **A real frontend.** OTP's debug UI is functional but ugly. The community standard is [`digitransit-ui`](https://github.com/HSLdevcom/digitransit-ui), which is what the Helsinki regional transport authority runs in production. Or, if you want the whole Google-Maps-clone experience without assembling it yourself, point [Headway](https://github.com/headwaymaps/headway) at this OTP+Nominatim pair.
- **Tile server.** For your own basemap, run [`tileserver-gl`](https://github.com/maptiler/tileserver-gl) with an OpenMapTiles vector tile build of the same `taiwan.osm.pbf`.
