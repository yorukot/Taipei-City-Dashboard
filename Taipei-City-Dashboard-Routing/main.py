"""
FastAPI server exposing the Taipei transit-routing pipeline (Nominatim
geocoder + OpenTripPlanner GraphQL) over HTTP.

Endpoints
---------
- POST ``/plan``     Plan a trip between two addresses or coordinates.
- GET  ``/geocode``  Resolve an address to lat/lon (Taipei-biased).
- GET  ``/healthz``  Liveness + upstream reachability check.

Errors
------
All non-2xx responses are RFC 9457 Problem Details with the
``application/problem+json`` content type. Problem ``type`` values are
URNs under the ``urn:problem:taipei-routing:`` namespace.

Run
---
    uv run fastapi dev main.py
    # or
    uv run uvicorn main:app --reload
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import Annotated, Any

import requests
from fastapi import FastAPI, Query, Request
from fastapi.encoders import jsonable_encoder
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field, model_validator
from starlette.exceptions import HTTPException as StarletteHTTPException

import routes as routing

log = logging.getLogger("taipei-routing")

PROBLEM_MEDIA_TYPE = "application/problem+json"
PROBLEM_TYPE_BASE = "urn:problem:taipei-routing"


app = FastAPI(
    title="Taipei Transit Routing",
    description=(
        "Thin HTTP wrapper around a self-hosted Nominatim + OpenTripPlanner "
        "stack covering Taipei City and inner New Taipei. Errors follow "
        "RFC 9457 (Problem Details for HTTP APIs)."
    ),
    version="0.1.0",
)


# ─── RFC 9457 Problem Details ──────────────────────────────────────────────


def problem(
    request: Request,
    *,
    slug: str,
    title: str,
    status: int,
    detail: str | None = None,
    **extensions: Any,
) -> JSONResponse:
    """Build an `application/problem+json` response per RFC 9457."""
    body: dict[str, Any] = {
        "type": f"{PROBLEM_TYPE_BASE}:{slug}",
        "title": title,
        "status": status,
        "instance": str(request.url.path),
    }
    if detail is not None:
        body["detail"] = detail
    body.update(extensions)
    return JSONResponse(status_code=status, content=body, media_type=PROBLEM_MEDIA_TYPE)


# ─── Domain exceptions ─────────────────────────────────────────────────────


class GeocodingNotFound(Exception):
    """Nominatim returned no results inside the Taipei viewbox."""

    def __init__(self, address: str) -> None:
        self.address = address


class UpstreamUnavailable(Exception):
    """Network/timeout error reaching an upstream service."""

    def __init__(self, service: str, detail: str) -> None:
        self.service = service
        self.detail = detail


class UpstreamError(Exception):
    """Upstream service responded with an error (HTTP non-2xx or GraphQL errors)."""

    def __init__(self, service: str, status: int | None, detail: str) -> None:
        self.service = service
        self.upstream_status = status
        self.detail = detail


class NoItinerary(Exception):
    """OTP returned an empty itinerary list."""


# ─── Wrappers around routes.py that translate raw exceptions ───────────────


def _safe_geocode(address: str) -> routing.Place:
    try:
        return routing.geocode(address)
    except ValueError as e:
        raise GeocodingNotFound(address) from e
    except (requests.ConnectionError, requests.Timeout) as e:
        raise UpstreamUnavailable("nominatim", str(e)) from e
    except requests.HTTPError as e:
        status = e.response.status_code if e.response is not None else None
        raise UpstreamError("nominatim", status, str(e)) from e
    except requests.RequestException as e:
        raise UpstreamError("nominatim", None, str(e)) from e


def _safe_plan(
    origin: routing.Place, dest: routing.Place, depart: dt.datetime, first: int
) -> list[dict[str, Any]]:
    try:
        # routes.plan_trip hard-codes first=3; pass through by patching the
        # variables we send. Simplest path is to call the underlying request
        # directly so the API can honor the caller's `first`.
        result = _plan_with_first(origin, dest, depart, first)
    except (requests.ConnectionError, requests.Timeout) as e:
        raise UpstreamUnavailable("opentripplanner", str(e)) from e
    except requests.HTTPError as e:
        status = e.response.status_code if e.response is not None else None
        raise UpstreamError("opentripplanner", status, str(e)) from e
    except requests.RequestException as e:
        raise UpstreamError("opentripplanner", None, str(e)) from e

    if result.get("errors"):
        raise UpstreamError("opentripplanner", 200, str(result["errors"]))
    edges = (result.get("data") or {}).get("planConnection", {}).get("edges") or []
    if not edges:
        raise NoItinerary()
    return [edge["node"] for edge in edges]


def _plan_with_first(
    origin: routing.Place, dest: routing.Place, depart: dt.datetime, first: int
) -> dict[str, Any]:
    """Same as routes.plan_trip but with a configurable ``first`` count."""
    if depart.tzinfo is None:
        depart = depart.replace(tzinfo=routing.LOCAL_TZ)
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
        "when": {"earliestDeparture": depart.isoformat()},
        "first": first,
    }
    resp = requests.post(
        routing.OTP_GRAPHQL,
        json={"query": routing.PLAN_QUERY, "variables": variables},
        headers={"Accept-Language": "zh-TW"},
        timeout=30,
    )
    resp.raise_for_status()
    return resp.json()


# ─── Request / response models ─────────────────────────────────────────────


class LocationInput(BaseModel):
    """Either an ``address`` to geocode, or an explicit ``lat``/``lon`` pair."""

    address: str | None = Field(
        default=None,
        description="Free-text address; geocoded via Nominatim and biased to Taipei.",
        examples=["台北車站"],
    )
    lat: float | None = Field(default=None, ge=-90, le=90)
    lon: float | None = Field(default=None, ge=-180, le=180)
    label: str | None = Field(
        default=None,
        description="Optional display label; defaults to the address or 'origin'/'dest'.",
    )

    @model_validator(mode="after")
    def _exactly_one_form(self) -> "LocationInput":
        has_addr = self.address is not None
        has_coord = self.lat is not None and self.lon is not None
        partial_coord = (self.lat is None) != (self.lon is None)
        if partial_coord:
            raise ValueError("'lat' and 'lon' must be provided together")
        if has_addr == has_coord:
            raise ValueError("provide exactly one of 'address' or ('lat', 'lon')")
        return self

    def resolve(self, default_label: str) -> routing.Place:
        if self.address is not None:
            place = _safe_geocode(self.address)
            if self.label:
                place.label = self.label
            return place
        return routing.Place(
            label=self.label or default_label, lat=float(self.lat), lon=float(self.lon)
        )


class PlanRequest(BaseModel):
    origin: LocationInput
    destination: LocationInput
    departure_time: dt.datetime | None = Field(
        default=None,
        alias="departureTime",
        description=(
            "Departure time as ISO 8601. Defaults to 'now' in Asia/Taipei. "
            "Naive datetimes are interpreted as Asia/Taipei."
        ),
    )
    first: int = Field(
        default=3,
        ge=1,
        le=10,
        description="Maximum number of itineraries to return.",
    )

    model_config = {"populate_by_name": True}


class RentalStationInfo(BaseModel):
    """Bike-share station metadata for a rental pickup or return."""

    station_id: str
    name: str
    lat: float
    lon: float
    network: str


class LegResponse(BaseModel):
    mode: str
    duration_seconds: float
    distance_meters: float
    start_time: dt.datetime
    end_time: dt.datetime
    from_name: str
    to_name: str
    route: str | None = None
    headsign: str | None = None
    pickup_station: RentalStationInfo | None = Field(
        default=None,
        description="Set when the leg starts by picking up a rental vehicle.",
    )
    return_station: RentalStationInfo | None = Field(
        default=None,
        description="Set when the leg ends by returning a rental vehicle.",
    )


class ItineraryResponse(BaseModel):
    duration_seconds: int
    walk_distance_meters: float
    transfers: int
    legs: list[LegResponse]


class PlaceResponse(BaseModel):
    label: str
    lat: float
    lon: float


class PlanResponse(BaseModel):
    origin: PlaceResponse
    destination: PlaceResponse
    departure_time: dt.datetime
    itineraries: list[ItineraryResponse]


def _rental_station(place: dict[str, Any] | None) -> RentalStationInfo | None:
    if not place:
        return None
    s = place.get("vehicleRentalStation")
    if not s:
        return None
    return RentalStationInfo(
        station_id=s["stationId"],
        name=s["name"],
        lat=s["lat"],
        lon=s["lon"],
        network=(s.get("rentalNetwork") or {}).get("networkId", ""),
    )


def _itinerary_to_response(node: dict[str, Any]) -> ItineraryResponse:
    legs: list[LegResponse] = []
    for leg in node["legs"]:
        route_name = None
        if leg.get("route"):
            route_name = leg["route"].get("shortName") or leg["route"].get("longName")
        legs.append(
            LegResponse(
                mode=leg["mode"],
                duration_seconds=leg["duration"],
                distance_meters=leg["distance"],
                start_time=dt.datetime.fromisoformat(leg["start"]["scheduledTime"]),
                end_time=dt.datetime.fromisoformat(leg["end"]["scheduledTime"]),
                from_name=leg["from"]["name"],
                to_name=leg["to"]["name"],
                route=route_name,
                headsign=(leg.get("trip") or {}).get("tripHeadsign"),
                pickup_station=_rental_station(leg.get("from")),
                return_station=_rental_station(leg.get("to")),
            )
        )
    return ItineraryResponse(
        duration_seconds=node["duration"],
        walk_distance_meters=node["walkDistance"],
        transfers=node.get("numberOfTransfers", 0),
        legs=legs,
    )


# ─── Endpoints ─────────────────────────────────────────────────────────────


@app.post(
    "/plan",
    response_model=PlanResponse,
    summary="Plan a public-transit trip",
    responses={
        404: {"description": "Geocoding miss or no itinerary", "content": {PROBLEM_MEDIA_TYPE: {}}},
        422: {"description": "Invalid request body", "content": {PROBLEM_MEDIA_TYPE: {}}},
        502: {"description": "Upstream returned an error", "content": {PROBLEM_MEDIA_TYPE: {}}},
        503: {"description": "Upstream unreachable", "content": {PROBLEM_MEDIA_TYPE: {}}},
    },
)
def plan(req: PlanRequest) -> PlanResponse:
    origin = req.origin.resolve("origin")
    dest = req.destination.resolve("destination")
    depart = req.departure_time or dt.datetime.now(routing.LOCAL_TZ)
    nodes = _safe_plan(origin, dest, depart, req.first)
    return PlanResponse(
        origin=PlaceResponse(label=origin.label, lat=origin.lat, lon=origin.lon),
        destination=PlaceResponse(label=dest.label, lat=dest.lat, lon=dest.lon),
        departure_time=depart if depart.tzinfo else depart.replace(tzinfo=routing.LOCAL_TZ),
        itineraries=[_itinerary_to_response(n) for n in nodes],
    )


@app.get(
    "/geocode",
    response_model=PlaceResponse,
    summary="Geocode an address (Taipei-biased)",
    responses={
        404: {"description": "No match inside the Taipei viewbox", "content": {PROBLEM_MEDIA_TYPE: {}}},
        502: {"content": {PROBLEM_MEDIA_TYPE: {}}},
        503: {"content": {PROBLEM_MEDIA_TYPE: {}}},
    },
)
def geocode(
    q: Annotated[str, Query(description="Address or place name", min_length=1)],
) -> PlaceResponse:
    place = _safe_geocode(q)
    return PlaceResponse(label=place.label, lat=place.lat, lon=place.lon)


@app.get("/healthz", summary="Liveness + upstream reachability")
def healthz() -> dict[str, Any]:
    """Returns 200 always; per-upstream status reflects reachability."""
    return {
        "status": "ok",
        "nominatim": _ping(f"{routing.NOMINATIM_BASE}/status"),
        "opentripplanner": _ping(routing.OTP_GRAPHQL, method="POST", json={"query": "{feeds{feedId}}"}),
    }


def _ping(url: str, method: str = "GET", **kwargs: Any) -> dict[str, Any]:
    try:
        resp = requests.request(method, url, timeout=3, **kwargs)
        return {"reachable": True, "status": resp.status_code}
    except requests.RequestException as e:
        return {"reachable": False, "error": str(e)}


# ─── Exception handlers (all responses are application/problem+json) ───────


@app.exception_handler(GeocodingNotFound)
async def _h_geocoding_not_found(request: Request, exc: GeocodingNotFound) -> JSONResponse:
    return problem(
        request,
        slug="geocoding-not-found",
        title="Address could not be geocoded",
        status=404,
        detail=(
            f"Nominatim returned no results for {exc.address!r} inside the "
            "Taipei viewbox. Try a more specific landmark or pass lat/lon directly."
        ),
        address=exc.address,
    )


@app.exception_handler(NoItinerary)
async def _h_no_itinerary(request: Request, exc: NoItinerary) -> JSONResponse:
    return problem(
        request,
        slug="no-itinerary",
        title="No itinerary found",
        status=404,
        detail=(
            "OpenTripPlanner could not find a transit itinerary for the "
            "requested origin, destination, and departure time. Check that "
            "both points fall inside the GTFS coverage area and that the "
            "departure time is within the active GTFS service window."
        ),
    )


@app.exception_handler(UpstreamUnavailable)
async def _h_upstream_unavailable(request: Request, exc: UpstreamUnavailable) -> JSONResponse:
    log.warning("upstream %s unavailable: %s", exc.service, exc.detail)
    return problem(
        request,
        slug="upstream-unavailable",
        title="Upstream service unavailable",
        status=503,
        detail=f"Could not reach {exc.service}: {exc.detail}",
        service=exc.service,
    )


@app.exception_handler(UpstreamError)
async def _h_upstream_error(request: Request, exc: UpstreamError) -> JSONResponse:
    log.warning("upstream %s error (%s): %s", exc.service, exc.upstream_status, exc.detail)
    return problem(
        request,
        slug="upstream-error",
        title="Upstream service returned an error",
        status=502,
        detail=f"{exc.service} responded with an error: {exc.detail}",
        service=exc.service,
        upstream_status=exc.upstream_status,
    )


@app.exception_handler(RequestValidationError)
async def _h_validation(request: Request, exc: RequestValidationError) -> JSONResponse:
    return problem(
        request,
        slug="validation-error",
        title="Request validation failed",
        status=422,
        detail="One or more fields in the request are invalid.",
        errors=jsonable_encoder(exc.errors()),
    )


@app.exception_handler(StarletteHTTPException)
async def _h_http(request: Request, exc: StarletteHTTPException) -> JSONResponse:
    return problem(
        request,
        slug="http-error",
        title=exc.detail if isinstance(exc.detail, str) else "HTTP error",
        status=exc.status_code,
    )


@app.exception_handler(Exception)
async def _h_unhandled(request: Request, exc: Exception) -> JSONResponse:
    log.exception("unhandled exception")
    return problem(
        request,
        slug="internal-error",
        title="Internal server error",
        status=500,
        detail="An unexpected error occurred. See server logs for details.",
    )
