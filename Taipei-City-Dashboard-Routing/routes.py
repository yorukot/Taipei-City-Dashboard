"""
Sample: plan a public-transit route in the Taipei area using a self-hosted
OpenTripPlanner (OTP) instance, with Nominatim for geocoding.

Prerequisites
-------------
- An OTP 2.x server with a graph built from:
    * Taiwan OSM extract (e.g. from Geofabrik)
    * GTFS feeds from TDX  (https://tdx.transportdata.tw/)
      — at minimum: TRTC metro, TPC + NTPC buses; optionally TRA / THSR / YouBike.
- A Nominatim server (or the public one, for testing only).

OTP 2.7+ removed the legacy REST `/plan` endpoint; this script uses the
GTFS GraphQL API (`/otp/gtfs/v1`) with a `planConnection` query.
"""

from __future__ import annotations

import datetime as dt
import logging
import os
from dataclasses import dataclass

import requests

log = logging.getLogger("taipei-routing.geocode")

# --- Configuration ----------------------------------------------------------

OTP_GRAPHQL = os.getenv("OTP_GRAPHQL", "http://localhost:8080/otp/gtfs/v1")
NOMINATIM_BASE = os.getenv("NOMINATIM_BASE", "http://localhost:8081")
USER_AGENT = "taipei-transit-demo/0.1"

ORIGIN_ADDR = "世界大樓"
DEST_ADDR = "400, 忠孝東路四段"

LOCAL_TZ = dt.timezone(dt.timedelta(hours=8))  # Asia/Taipei

# Bias geocoding to Taipei City + inner New Taipei, matching the OTP
# graph's transit coverage. Without this, common street names like 興隆路
# resolve to other counties (Yunlin, Taoyuan, …) and routing returns no
# itineraries. Note: OSM/Nominatim coverage of Taiwan street names is
# patchy — landmark names (台北車站, 台北101, 西門町) work; many street
# and MRT-station queries return nothing inside this box.
TAIPEI_VIEWBOX = "121.40,25.20,121.75,24.90"  # left,top,right,bottom


# --- Geocoding --------------------------------------------------------------


@dataclass
class Place:
    label: str
    lat: float
    lon: float


def geocode(address: str) -> Place:
    resp = requests.get(
        f"{NOMINATIM_BASE}/search",
        params={
            "q": address,
            "format": "json",
            "limit": 1,
            "countrycodes": "tw",
            "accept-language": "zh-TW",
            "viewbox": TAIPEI_VIEWBOX,
            "bounded": 1,
        },
        headers={"User-Agent": USER_AGENT},
        timeout=10,
    )
    resp.raise_for_status()
    results = resp.json()
    if not results:
        log.info("geocode miss: %r", address)
        raise ValueError(f"No geocoding result for: {address}")
    top = results[0]
    log.info(
        "geocoded %r -> (%s, %s) %r (%d candidates)",
        address,
        top["lat"],
        top["lon"],
        top.get("display_name", ""),
        len(results),
    )
    return Place(label=address, lat=float(top["lat"]), lon=float(top["lon"]))


# --- Routing ----------------------------------------------------------------


PLAN_QUERY = """
query Plan(
  $from: PlanLabeledLocationInput!
  $to: PlanLabeledLocationInput!
  $when: PlanDateTimeInput
  $first: Int
) {
  planConnection(
    origin: $from
    destination: $to
    dateTime: $when
    first: $first
    modes: {
      direct: [WALK, BICYCLE_RENTAL]
      transit: {
        access: [WALK, BICYCLE_RENTAL]
        egress: [WALK, BICYCLE_RENTAL]
        transit: [
          { mode: BUS }
          { mode: SUBWAY }
          { mode: RAIL }
          { mode: TRAM }
        ]
      }
    }
  ) {
    edges {
      node {
        start
        end
        duration
        walkDistance
        numberOfTransfers
        legs {
          mode
          duration
          distance
          start { scheduledTime }
          end { scheduledTime }
          from {
            name
            vehicleRentalStation {
              stationId
              name
              lat
              lon
              rentalNetwork { networkId }
            }
          }
          to {
            name
            vehicleRentalStation {
              stationId
              name
              lat
              lon
              rentalNetwork { networkId }
            }
          }
          route { shortName longName }
          trip { tripHeadsign }
        }
      }
    }
  }
}
""".strip()


def plan_trip(origin: Place, dest: Place, depart_at: dt.datetime) -> dict:
    if depart_at.tzinfo is None:
        depart_at = depart_at.replace(tzinfo=LOCAL_TZ)
    variables = {
        "from": {
            "label": origin.label,
            "location": {
                "coordinate": {"latitude": origin.lat, "longitude": origin.lon}
            },
        },
        "to": {
            "label": dest.label,
            "location": {"coordinate": {"latitude": dest.lat, "longitude": dest.lon}},
        },
        "when": {"earliestDeparture": depart_at.isoformat()},
        "first": 3,
    }
    resp = requests.post(
        OTP_GRAPHQL,
        json={"query": PLAN_QUERY, "variables": variables},
        headers={"Accept-Language": "zh-TW"},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()


# --- Pretty printing --------------------------------------------------------


def fmt_iso(ts: str) -> str:
    return dt.datetime.fromisoformat(ts).astimezone(LOCAL_TZ).strftime("%H:%M")


def print_itinerary(it: dict, idx: int) -> None:
    mins = it["duration"] // 60
    print(
        f"\n=== Option {idx}  |  {mins} min  |  "
        f"walk {round(it['walkDistance'])} m  |  "
        f"transfers {it.get('numberOfTransfers', 0)} ==="
    )
    for leg in it["legs"]:
        start = fmt_iso(leg["start"]["scheduledTime"])
        end = fmt_iso(leg["end"]["scheduledTime"])
        mode = leg["mode"]
        from_rental = (leg.get("from") or {}).get("vehicleRentalStation")
        to_rental = (leg.get("to") or {}).get("vehicleRentalStation")
        if leg.get("route"):
            route = leg["route"].get("shortName") or leg["route"].get("longName") or ""
            head = (leg.get("trip") or {}).get("tripHeadsign", "")
            print(f"  {start}–{end}  {mode:6s} {route}  → {head}")
            print(f"            board : {leg['from']['name']}")
            print(f"            alight: {leg['to']['name']}")
        else:
            dist = round(leg["distance"])
            print(
                f"  {start}–{end}  {mode:6s} {dist:>5d} m  "
                f"({leg['from']['name']} → {leg['to']['name']})"
            )
        if from_rental:
            print(f"            pickup: {from_rental['name']} ({from_rental['stationId']})")
        if to_rental:
            print(f"            return: {to_rental['name']} ({to_rental['stationId']})")


# --- Main -------------------------------------------------------------------


def main() -> None:
    origin = geocode(ORIGIN_ADDR)
    dest = geocode(DEST_ADDR)
    print(f"Origin : {origin.lat:.5f}, {origin.lon:.5f}  ({origin.label})")
    print(f"Dest   : {dest.lat:.5f}, {dest.lon:.5f}  ({dest.label})")

    depart = dt.datetime.now(LOCAL_TZ).replace(second=0, microsecond=0)
    plan = plan_trip(origin, dest, depart)

    if plan.get("errors"):
        raise SystemExit(f"OTP error: {plan['errors']}")

    edges = plan["data"]["planConnection"]["edges"]
    if not edges:
        raise SystemExit("No itineraries found.")
    edges.sort(key=lambda e: e["node"]["end"])
    for i, edge in enumerate(edges, 1):
        print_itinerary(edge["node"], i)


if __name__ == "__main__":
    main()
