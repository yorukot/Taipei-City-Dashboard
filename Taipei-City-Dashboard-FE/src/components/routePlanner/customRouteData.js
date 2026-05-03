// Reactive data for the route planner's "custom route" (自訂路線) phase.
// Metro lines/stations are loaded from the bundled JSON. Bus routes are
// fetched from /route, and bus stops for a route are fetched from
// /route/<id>/stops on demand. Bus stop coordinates are not provided by
// the API, so we generate plausible Taipei-area positions and cache them.

import { reactive, ref } from "vue";
import metroStationsRaw from "../../assets/configs/selectors/metro-stations.json";
import http from "../../router/axios";

const LINE_COLORS = {
	板南線: "#0070BD",
	淡水信義線: "#E3002C",
	中和新蘆線: "#F8B62D",
	松山新店線: "#008659",
	文湖線: "#B57A2C",
	新北投支線: "#E3002C",
	小碧潭支線: "#008659",
};
const BUS_COLOR = "#F8B62D";
const FALLBACK_COLOR = "#5a9cf8";

// A few Taipei boxes (lon/lat) used to scatter bus stops on the map.
const TAIPEI_BOXES = [
	{ latMin: 25.02, latMax: 25.06, lonMin: 121.5, lonMax: 121.55 },
	{ latMin: 25.03, latMax: 25.07, lonMin: 121.54, lonMax: 121.58 },
	{ latMin: 25.04, latMax: 25.08, lonMin: 121.52, lonMax: 121.57 },
	{ latMin: 25.0, latMax: 25.04, lonMin: 121.49, lonMax: 121.54 },
];

const PUNCTUALITY_KEYS = ["on_time", "sometimes_late", "often_late"];

function randomTaipeiCoord() {
	const box = TAIPEI_BOXES[Math.floor(Math.random() * TAIPEI_BOXES.length)];
	const lat = box.latMin + Math.random() * (box.latMax - box.latMin);
	const lon = box.lonMin + Math.random() * (box.lonMax - box.lonMin);
	return [lon, lat];
}

function randomPunctuality() {
	return PUNCTUALITY_KEYS[
		Math.floor(Math.random() * PUNCTUALITY_KEYS.length)
	];
}

// id -> { name, coord:[lon,lat], punctuality }
export const stationRegistry = reactive({});

// [{ id, type, name, color, stationIds, backendId? }]
export const lineRegistry = reactive([]);

const busRoutesLoaded = ref(false);
const busStopsLoaded = reactive({});

function initMetroData() {
	Object.entries(metroStationsRaw).forEach(([lineName, stops]) => {
		const stationIds = [];
		stops.forEach((s) => {
			const id = `mrt_${s.stationId}`;
			if (!stationRegistry[id]) {
				stationRegistry[id] = {
					name: s.name,
					coord: [s.lon, s.lat],
					punctuality: randomPunctuality(),
				};
			}
			stationIds.push(id);
		});
		lineRegistry.push({
			id: `mrt_${lineName}`,
			type: "mrt",
			name: lineName,
			color: LINE_COLORS[lineName] || FALLBACK_COLOR,
			stationIds,
		});
	});
}
initMetroData();

export async function loadBusRoutes() {
	if (busRoutesLoaded.value) return;
	busRoutesLoaded.value = true;
	try {
		const { data } = await http.get("/route");
		const items = data?.data || [];
		items.forEach((r) => {
			const id = `bus_${r.id}`;
			if (lineRegistry.find((l) => l.id === id)) return;
			lineRegistry.push({
				id,
				type: "bus",
				name: r.route_name,
				color: BUS_COLOR,
				stationIds: [],
				backendId: r.id,
			});
		});
	} catch (e) {
		busRoutesLoaded.value = false;
		console.error("loadBusRoutes failed", e);
	}
}

export async function loadBusRouteStops(lineId) {
	if (busStopsLoaded[lineId]) return;
	const line = lineRegistry.find((l) => l.id === lineId);
	if (!line || line.type !== "bus") return;
	busStopsLoaded[lineId] = true;
	try {
		const { data } = await http.get(`/route/${line.backendId}/stops`);
		const stops = data?.data || [];
		const ids = [];
		stops.forEach((s) => {
			const id = `bus_stop_${s.stop_uid}`;
			if (!stationRegistry[id]) {
				stationRegistry[id] = {
					name: s.stop_name,
					coord: randomTaipeiCoord(),
					punctuality: randomPunctuality(),
				};
			}
			ids.push(id);
		});
		line.stationIds = ids;
	} catch (e) {
		busStopsLoaded[lineId] = false;
		console.error("loadBusRouteStops failed", e);
	}
}

export function getCustomLine(id) {
	return lineRegistry.find((l) => l.id === id);
}

export function getCustomStation(id) {
	return stationRegistry[id];
}

export function customLineCoords(lineId, fromStationId, toStationId) {
	const line = getCustomLine(lineId);
	if (!line) return [];
	const i = line.stationIds.indexOf(fromStationId);
	const j = line.stationIds.indexOf(toStationId);
	if (i < 0 || j < 0) return [];
	const slice =
		i <= j
			? line.stationIds.slice(i, j + 1)
			: line.stationIds.slice(j, i + 1).reverse();
	return slice.map((id) => stationRegistry[id]?.coord).filter(Boolean);
}

export function customRouteGeometry(route) {
	if (!route) return { polylines: [], markers: [] };
	const polylines = [];
	const markers = [];
	const seen = new Set();
	const addMarker = (id, name, coord, color) => {
		const key = `${name}|${coord.join(",")}`;
		if (seen.has(key)) return;
		seen.add(key);
		markers.push({ id, name, coord, color, kind: "transit" });
	};
	route.steps.forEach((step, idx) => {
		const coords = customLineCoords(step.lineId, step.from, step.to);
		if (coords.length >= 2) {
			polylines.push({
				id: `transit-${idx}`,
				coords,
				color: step.color,
				dashed: false,
			});
		}
		const a = getCustomStation(step.from);
		const b = getCustomStation(step.to);
		if (a) addMarker(`m-${idx}-a`, a.name, a.coord, step.color);
		if (b) addMarker(`m-${idx}-b`, b.name, b.coord, step.color);
	});
	return { polylines, markers };
}
