<script setup>
import axios from "axios";
import { computed, reactive, ref, watch } from "vue";
import RoutePlannerMap from "../components/routePlanner/RoutePlannerMap.vue";
import {
	PUNCTUALITY,
	defaultEndpoints,
	endpointsGeometry,
	getStation,
	routeGeometry,
} from "../components/routePlanner/mockData.js";
import {
	customRouteGeometry,
	getCustomLine,
	getCustomStation,
	lineRegistry,
	loadBusRouteStops,
	loadBusRoutes,
} from "../components/routePlanner/customRouteData.js";
import {
	getBusStopIndex,
	getBusStopIndexSync,
	pickClosest,
} from "../components/routePlanner/busStopIndex.js";
import {
	metroStationByName,
	normalizeStationName,
} from "../components/routePlanner/metroStationIndex.js";
import http from "../router/axios";

const RELIABILITY_META = {
	green: { label: "可靠" },
	yellow: { label: "中等風險" },
	red: { label: "不可靠" },
	unknown: { label: "資料不足" },
};

const BACKEND_RELIABILITY_MODES = [
	"BUS",
	"RAIL",
	"TRAIN",
	"TRA",
	"YOUBIKE",
	"BICYCLE_RENTAL",
	"BIKE_RENTAL",
];

const ROUTING_API_URL = (
	import.meta.env.VITE_ROUTING_API_URL || "/routing"
).replace(/\/$/, "");

const DEFAULT_STATION_META = {
	label: "站點",
	color: "#8b949e",
	icon: "radio_button_unchecked",
};

const MODE_ICONS = {
	BUS: "directions_bus",
	SUBWAY: "subway",
	RAIL: "directions_railway",
	TRAIN: "directions_railway",
	TRA: "directions_railway",
	BICYCLE_RENTAL: "pedal_bike",
	BIKE_RENTAL: "pedal_bike",
	BICYCLE: "pedal_bike",
	YOUBIKE: "pedal_bike",
};

const MODE_COLORS = {
	BUS: "#f8b62d",
	SUBWAY: "#5a9cf8",
	RAIL: "#d29922",
	TRAIN: "#d29922",
	TRA: "#d29922",
	BICYCLE_RENTAL: "#3fb950",
	BIKE_RENTAL: "#3fb950",
	BICYCLE: "#3fb950",
	YOUBIKE: "#3fb950",
};

const MODE_LABELS = {
	BUS: "公車",
	SUBWAY: "捷運",
	RAIL: "台鐵",
	TRAIN: "台鐵",
	TRA: "台鐵",
	BICYCLE_RENTAL: "YouBike",
	BIKE_RENTAL: "YouBike",
	BICYCLE: "YouBike",
	YOUBIKE: "YouBike",
};

const DEFAULT_ORIGIN = {
	address: "臺北市中正區北平西路3號",
	label: "台北車站",
	lat: 25.04776,
	lon: 121.51706,
};

const DEFAULT_DESTINATION = {
	address: "臺北市信義區信義路五段7號",
	label: "台北101",
	lat: 25.03396,
	lon: 121.56447,
};

// ── Phase state ─────────────────────────────────────────────────────────
// "form" → "list" → "detail"   (when backend planning)
// "form" → "custom"            (when self planning)
const phase = ref("form");

const today = new Date();
const formData = reactive({
	date: today.toISOString().slice(0, 10),
	time: today.toTimeString().slice(0, 5),
	start: DEFAULT_ORIGIN.address,
	end: DEFAULT_DESTINATION.address,
});

const selectedRoute = ref(null);
const plannedRoutes = ref([]);
const hoveredRouteIdx = ref(null);
const ROUTE_BADGE_COLORS = ["#5a9cf8", "#3fb950", "#f8b62d"];
const routeEndpoints = ref({ origin: null, destination: null });
const planningLoading = ref(false);
const planningError = ref("");
const reliabilityLoading = ref(false);
const reliabilityError = ref(false);

// Custom plan: user-built sequence of transit segments.
const customPlan = reactive({ steps: [] });

// In-progress "add node" form for custom planning.
// Times are derived from formData.time + segment durations, not entered by hand.
const newStep = reactive({
	open: false,
	mode: "mrt", // "mrt" | "bus"
	lineId: "",
	from: "",
	to: "",
});

// Mock travel-time per stop (minutes) and transfer buffer between segments.
const PER_STOP_MIN = { mrt: 2, bus: 3 };
const TRANSFER_BUFFER_MIN = 2;

// ── Form actions ────────────────────────────────────────────────────────
async function submitForm() {
	phase.value = "list";
	selectedRoute.value = null;
	plannedRoutes.value = [];
	planningLoading.value = true;
	planningError.value = "";
	reliabilityError.value = false;
	routeEndpoints.value = { origin: null, destination: null };

	try {
		const response = await axios.post(routingApiPath("/plan"), {
			origin: locationPayload(formData.start, DEFAULT_ORIGIN),
			destination: locationPayload(formData.end, DEFAULT_DESTINATION),
			departureTime: toTaipeiIso(formData.date, formData.time),
			first: 3,
		});

		routeEndpoints.value = routeEndpointCoords(response.data);
		await getBusStopIndex().catch(() => null);
		plannedRoutes.value = toPlannedRoutes(response.data);
		if (plannedRoutes.value.length === 0) {
			planningError.value = "查無可用路線";
			return;
		}
		analyzeRouteReliability();
	} catch (err) {
		planningError.value = routePlanningErrorMessage(err);
		plannedRoutes.value = [];
	} finally {
		planningLoading.value = false;
	}
}

function submitCustom() {
	customPlan.steps = [];
	phase.value = "custom";
	loadBusRoutes();
}

function backToForm() {
	phase.value = "form";
	selectedRoute.value = null;
	planningError.value = "";
	reliabilityError.value = false;
	routeEndpoints.value = { origin: null, destination: null };
}

function pickRoute(r) {
	selectedRoute.value = r;
	phase.value = "detail";
}

function backToList() {
	selectedRoute.value = null;
	phase.value = "list";
}

// ── Custom plan actions ─────────────────────────────────────────────────
const availableLines = computed(() =>
	lineRegistry.filter((l) => l.type === newStep.mode),
);

const stationsForNewLine = computed(() => {
	const line = getCustomLine(newStep.lineId);
	if (!line) return [];
	return line.stationIds
		.map((id) => {
			const s = getCustomStation(id);
			return s ? { id, ...s } : null;
		})
		.filter(Boolean);
});

watch(
	() => newStep.lineId,
	(id) => {
		if (!id) return;
		const line = getCustomLine(id);
		if (line?.type === "bus") loadBusRouteStops(id);
	},
);

function openAddNode() {
	newStep.open = true;
	newStep.mode = "mrt";
	newStep.lineId = "";
	newStep.from = "";
	newStep.to = "";
}

function cancelAddNode() {
	newStep.open = false;
}

function commitAddNode() {
	const line = getCustomLine(newStep.lineId);
	if (!line || !newStep.from || !newStep.to) return;
	if (newStep.from === newStep.to) return;
	customPlan.steps.push({
		kind: "transit",
		lineId: line.id,
		lineName: line.name,
		color: line.color,
		modeIcon: line.type === "mrt" ? "subway" : "directions_bus",
		from: newStep.from,
		to: newStep.to,
	});
	newStep.open = false;
}

function removeStep(idx) {
	customPlan.steps.splice(idx, 1);
}

function segmentMinutes(step) {
	const line = getCustomLine(step.lineId);
	if (!line) return 0;
	const i = line.stationIds.indexOf(step.from);
	const j = line.stationIds.indexOf(step.to);
	if (i < 0 || j < 0) return 0;
	return Math.abs(j - i) * (PER_STOP_MIN[line.type] || 2);
}

function addMinutes(hhmm, mins) {
	if (!hhmm) return "";
	const [h, m] = hhmm.split(":").map(Number);
	const total = h * 60 + m + mins;
	const nh = Math.floor(total / 60) % 24;
	const nm = total % 60;
	return `${String(nh).padStart(2, "0")}:${String(nm).padStart(2, "0")}`;
}

// Wrap the custom plan as a route-shaped object, deriving board/alight times
// from formData.time + each segment's estimated duration (+ transfer buffer).
const customRoute = computed(() => {
	if (customPlan.steps.length === 0) return null;
	let cursor = formData.time;
	let total = 0;
	const enriched = customPlan.steps.map((step, idx) => {
		if (idx > 0) {
			cursor = addMinutes(cursor, TRANSFER_BUFFER_MIN);
			total += TRANSFER_BUFFER_MIN;
		}
		const dur = segmentMinutes(step);
		const boardTime = cursor;
		const alightTime = addMinutes(cursor, dur);
		cursor = alightTime;
		total += dur;
		return { ...step, boardTime, alightTime, durationMin: dur };
	});
	return {
		id: "custom",
		summary: "自訂路線",
		boardTime: enriched[0].boardTime,
		alightTime: enriched[enriched.length - 1].alightTime,
		durationMin: total,
		transferCount: Math.max(0, enriched.length - 1),
		steps: enriched,
	};
});

// ── Map geometry per phase ──────────────────────────────────────────────
const mapGeometry = computed(() => {
	if (phase.value === "form") {
		return endpointsGeometry(
			defaultEndpoints.ORIGIN,
			defaultEndpoints.DEST,
		);
	}
	if (phase.value === "list") {
		// Show the endpoints + every route option lightly overlapped, with
		// a numbered badge per route at its midpoint.
		const merged = endpointsGeometry(
			routeEndpoints.value.origin || defaultEndpoints.ORIGIN,
			routeEndpoints.value.destination || defaultEndpoints.DEST,
		);
		plannedRoutes.value.forEach((r, i) => {
			const g = routeGeometry(r);
			const routeColor =
				ROUTE_BADGE_COLORS[i % ROUTE_BADGE_COLORS.length];
			g.polylines.forEach((p) => {
				merged.polylines.push({
					...p,
					id: `r${i}-${p.id}`,
					color: routeColor,
					routeIdx: i,
				});
			});
			const anchor = longestSegmentMidpoint(g.polylines);
			if (anchor) {
				merged.markers.push({
					id: `route-label-${i}`,
					name: String(i + 1),
					coord: anchor,
					color: routeColor,
					kind: "route-label",
					routeIdx: i,
				});
			}
		});
		return merged;
	}
	if (phase.value === "detail" && selectedRoute.value) {
		return routeGeometry(selectedRoute.value);
	}
	if (phase.value === "custom" && customRoute.value) {
		return customRouteGeometry(customRoute.value);
	}
	return endpointsGeometry(null, null);
});

// ── Display helpers ─────────────────────────────────────────────────────
function transitIcons(route) {
	const icons = [];
	route.steps.forEach((s, idx) => {
		if (s.kind === "walk") {
			icons.push({
				key: `w${idx}`,
				icon: "directions_walk",
				color: "#888",
			});
		} else {
			icons.push({
				key: `t${idx}`,
				icon: s.modeIcon,
				color: s.color,
			});
		}
	});
	return icons;
}

function routingApiPath(path) {
	return `${ROUTING_API_URL}${path}`;
}

function locationPayload(value, defaultLocation) {
	const label = value.trim();
	if (label === defaultLocation.address || label === defaultLocation.label) {
		return {
			label,
			lat: defaultLocation.lat,
			lon: defaultLocation.lon,
		};
	}
	return {
		address: label,
		label,
	};
}

function toPlannedRoutes(planPayload) {
	const itineraries = planPayload?.itineraries || [];
	const endpoints = routeEndpointCoords(planPayload);
	return itineraries.map((itinerary, routeIdx) => {
		const steps = (itinerary.legs || []).map((leg, legIdx) =>
			toPlannedRouteStep(leg, legIdx, itinerary.legs || [], endpoints),
		);
		const firstStep = steps[0] || {};
		const lastStep = steps[steps.length - 1] || {};

		return {
			id: `planned_${routeIdx + 1}`,
			summary: routeSummary(steps, routeIdx),
			boardTime: firstStep.boardTime || formData.time,
			alightTime: lastStep.alightTime || formData.time,
			durationMin: Math.max(
				1,
				Math.round((itinerary.duration_seconds || 0) / 60),
			),
			transferCount: itinerary.transfers || 0,
			steps: steps.map((step, stepIdx) => ({
				...step,
				reliabilityId: routeStepReliabilityId(
					`planned_${routeIdx + 1}`,
					stepIdx,
				),
				reliability: unknownReliability(),
			})),
			reliability: unknownReliability(),
		};
	});
}

function toPlannedRouteStep(leg, legIdx, legs, endpoints) {
	const mode = normalizeModeString(leg.mode);
	const from = routeLegPoint(leg, "from", legIdx, legs.length, endpoints);
	const to = routeLegPoint(leg, "to", legIdx, legs.length, endpoints);
	const durationMin = Math.max(
		1,
		Math.round((leg.duration_seconds || 0) / 60),
	);

	if (mode === "WALK") {
		return {
			kind: "walk",
			mode,
			durationMin,
			from,
			to,
			boardTime: timeText(leg.start_time),
			alightTime: timeText(leg.end_time),
		};
	}

	return {
		kind: "transit",
		mode,
		route: leg.route,
		headsign: leg.headsign,
		lineName: leg.route || modeLabel(mode),
		color: modeColor(mode),
		modeIcon: modeIcon(mode),
		boardTime: timeText(leg.start_time),
		alightTime: timeText(leg.end_time),
		durationMin,
		from,
		to,
		pickup_station: leg.pickup_station,
		return_station: leg.return_station,
	};
}

function routeLegPoint(leg, side, legIdx, legCount, endpoints) {
	const name = side === "from" ? leg.from_name : leg.to_name;
	const rentalStation =
		side === "from" ? leg.pickup_station : leg.return_station;
	const metroStation = metroStationByName(name);
	const endpointCoord =
		side === "from" && legIdx === 0
			? endpoints.origin
			: side === "to" && legIdx === legCount - 1
				? endpoints.destination
				: null;
	const interpHint = interpolateCoord(
		endpoints.origin,
		endpoints.destination,
		side === "from" ? legIdx / legCount : (legIdx + 1) / legCount,
	);
	const busStopCoord = busStopCoordByName(name, interpHint);

	return {
		name: name || (side === "from" ? "起點" : "終點"),
		coord:
			rentalStationCoord(rentalStation) ||
			metroStation?.coord ||
			busStopCoord ||
			endpointCoord ||
			interpHint,
	};
}

function busStopCoordByName(name, hintCoord) {
	if (!name) return null;
	const index = getBusStopIndexSync();
	if (!index) return null;
	const entries =
		index.byName.get(name) || index.byName.get(normalizeStationName(name));
	if (!entries) return null;
	return pickClosest(entries, hintCoord)?.coord || null;
}

function routeEndpointCoords(planPayload) {
	return {
		origin: placeCoord(planPayload?.origin),
		destination: placeCoord(planPayload?.destination),
	};
}

function placeCoord(place) {
	if (!Number.isFinite(place?.lon) || !Number.isFinite(place?.lat)) {
		return null;
	}
	return [place.lon, place.lat];
}

// Pick the midpoint of the longest single segment across all polylines.
// Anchors the route badge between two stations rather than on top of one.
function longestSegmentMidpoint(polylines) {
	let bestLen = -1;
	let bestMid = null;
	for (const p of polylines || []) {
		const coords = p.coords || [];
		for (let k = 0; k < coords.length - 1; k++) {
			const [ax, ay] = coords[k];
			const [bx, by] = coords[k + 1];
			const dx = bx - ax;
			const dy = by - ay;
			const len = dx * dx + dy * dy;
			if (len > bestLen) {
				bestLen = len;
				bestMid = [(ax + bx) / 2, (ay + by) / 2];
			}
		}
	}
	return bestMid;
}

function rentalStationCoord(station) {
	if (!Number.isFinite(station?.lon) || !Number.isFinite(station?.lat)) {
		return null;
	}
	return [station.lon, station.lat];
}

function interpolateCoord(origin, destination, ratio) {
	if (!origin || !destination) return null;
	return [
		origin[0] + (destination[0] - origin[0]) * ratio,
		origin[1] + (destination[1] - origin[1]) * ratio,
	];
}

function routeSummary(steps, routeIdx) {
	const transitNames = steps
		.filter((step) => step.kind === "transit")
		.map((step) => step.lineName)
		.filter(Boolean);
	if (transitNames.length) return transitNames.join(" + ");
	return routeIdx === 0 ? "推薦路線" : `建議路線 ${routeIdx + 1}`;
}

function timeText(value) {
	if (!value) return "";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return "";
	return date.toLocaleTimeString("zh-TW", {
		hour: "2-digit",
		minute: "2-digit",
		hour12: false,
		timeZone: "Asia/Taipei",
	});
}

function modeIcon(mode) {
	return MODE_ICONS[mode] || "directions_transit";
}

function modeColor(mode) {
	return MODE_COLORS[mode] || "#8b949e";
}

function modeLabel(mode) {
	return MODE_LABELS[mode] || "大眾運輸";
}

function routePlanningErrorMessage(err) {
	const body = err?.response?.data;
	if (body?.detail) return body.detail;
	if (body?.title) return body.title;
	if (err?.response?.status === 500 && !body) {
		return "路線規劃服務尚未啟動，請確認 routing-api 已在 8000 port 運行";
	}
	if (err?.message) return `路線規劃失敗：${err.message}`;
	return "路線規劃暫時無法取得";
}

async function analyzeRouteReliability() {
	reliabilityLoading.value = true;
	reliabilityError.value = false;
	const legs = plannedRoutes.value.flatMap((route) =>
		route.steps
			.map((step, idx) => toReliabilityLegRequest(route, step, idx))
			.filter((leg) => shouldFetchReliability(leg.mode)),
	);
	try {
		if (legs.length === 0) {
			mergeRouteReliability([]);
			return;
		}
		const response = await http.post("/route/reliability", {
			departure_time: toTaipeiIso(formData.date, formData.time),
			legs,
		});
		const { data: responseBody } = response;
		const { data: reliabilityPayload = {} } = responseBody || {};
		const { legs: legReliabilities = [] } = reliabilityPayload;
		mergeRouteReliability(legReliabilities);
	} catch {
		reliabilityError.value = true;
		plannedRoutes.value = plannedRoutes.value.map((route) => {
			const steps = route.steps.map((step) => ({
				...step,
				reliability:
					localReliabilityForStep(step) ||
					unknownReliability("可靠度分析暫時無法取得"),
			}));
			return {
				...route,
				reliability: aggregateRouteReliability(steps),
				steps,
			};
		});
	} finally {
		reliabilityLoading.value = false;
	}
}

function toReliabilityLegRequest(route, step, idx) {
	const {
		alightTime: stepAlightTime,
		boardTime: stepBoardTime,
		direction,
		durationMin,
		from,
		lineName,
		selectorKey,
		selector_key: selectorKeySnake,
		stopName,
		stop_name: stopNameSnake,
		to,
	} = step;
	const {
		alightTime: routeAlightTime,
		boardTime: routeBoardTime,
		id,
	} = route;
	const fromName = stepPointName(from);
	const toName = stepPointName(to);
	const startTime = stepBoardTime || routeBoardTime || formData.time;
	const endTime = stepAlightTime || routeAlightTime || startTime;

	return {
		id: routeStepReliabilityId(id, idx),
		mode: reliabilityMode(step),
		route_name: routeNameForReliability(step) || lineName || "",
		selector_key: firstNonEmpty(selectorKey, selectorKeySnake),
		from_name: fromName,
		to_name: toName,
		stop_name: firstNonEmpty(stopName, stopNameSnake, fromName),
		direction: normalizeDirection(direction),
		start_time: toTaipeiIso(formData.date, startTime),
		end_time: toTaipeiIso(formData.date, endTime),
		duration_seconds: (durationMin || 0) * 60,
		station_id: trainStationIdForReliability(step),
		station_uid: stationUidForReliability(step),
		train_type_code: trainTypeCodeForReliability(step),
		pickup_station_uid: rentalStationUid(step, "pickup"),
		return_station_uid: rentalStationUid(step, "return"),
		pickup_station_name: rentalStationName(step, "pickup"),
		return_station_name: rentalStationName(step, "return"),
	};
}

function mergeRouteReliability(legReliabilities) {
	const reliabilityById = legReliabilities.reduce((lookup, item) => {
		lookup[item.id] = normalizeReliability(item);
		return lookup;
	}, {});

	plannedRoutes.value = plannedRoutes.value.map((route) => {
		const steps = route.steps.map((step) => ({
			...step,
			reliability:
				localReliabilityForStep(step) ||
				reliabilityById[step.reliabilityId] ||
				unknownReliability("查無此路段可靠度資料"),
		}));
		return {
			...route,
			steps,
			reliability: aggregateRouteReliability(steps),
		};
	});
}

function routeStepReliabilityId(routeId, idx) {
	return `${routeId}-${idx}`;
}

function reliabilityMode(step) {
	const mode = normalizeModeString(step.mode || step.transitMode);
	if (
		mode === "BICYCLE_RENTAL" ||
		mode === "BIKE_RENTAL" ||
		mode === "YOUBIKE" ||
		(mode === "BICYCLE" &&
			(rentalStationForReliability(step, "pickup") ||
				rentalStationForReliability(step, "return")))
	) {
		return "YOUBIKE";
	}
	if (mode) return mode;
	if (step.kind === "walk") return "WALK";
	if (step.modeIcon === "directions_bus") return "BUS";
	if (step.modeIcon === "subway") return "SUBWAY";
	return "RAIL";
}

function shouldFetchReliability(mode) {
	return BACKEND_RELIABILITY_MODES.includes(mode);
}

function localReliabilityForStep(step) {
	const mode = reliabilityMode(step);
	if (mode === "WALK") {
		return {
			status: "unknown",
			label: "不納入",
			reason: "步行路段不納入可靠度分析",
			metrics: [],
			ignored: true,
		};
	}
	if (mode === "SUBWAY") {
		return {
			status: "green",
			label: statusMeta("green").label,
			reason: "捷運原則上不誤點",
			metrics: [],
			ignored: false,
		};
	}
	return null;
}

function stepPointName(point) {
	if (!point) return "";
	if (typeof point === "string") return stationOf(point)?.name || point;
	return point.name || "";
}

function toTaipeiIso(date, hhmm) {
	const safeTime = hhmm?.length === 5 ? `${hhmm}:00` : hhmm || "00:00:00";
	return `${date}T${safeTime}+08:00`;
}

function normalizeReliability(item) {
	const {
		available,
		match_quality: matchQuality,
		metrics = [],
		mode = "",
		reason,
		source,
		status,
	} = item || {};
	if (!available) {
		return unknownReliability(reason || "查無可靠度資料");
	}
	const derivedStatus = RELIABILITY_META[status]
		? status
		: reliabilityStatusFromMetrics(mode, metrics);
	const meta = statusMeta(derivedStatus);
	return {
		status: derivedStatus,
		label: meta.label,
		reason: reason || "查無可靠度資料",
		metrics,
		source,
		matchQuality,
		ignored: false,
	};
}

function unknownReliability(reason = "尚未取得可靠度分析") {
	const { label } = RELIABILITY_META.unknown;
	return {
		status: "unknown",
		label,
		reason,
		metrics: [],
	};
}

function statusMeta(status) {
	return RELIABILITY_META[status] || RELIABILITY_META.unknown;
}

function aggregateRouteReliability(steps) {
	const analyzed = steps.filter((step) => !step.reliability?.ignored);
	if (analyzed.length === 0) {
		return unknownReliability("此路線沒有可分析路段");
	}

	const red = analyzed.find((step) => step.reliability?.status === "red");
	if (red) return routeStatusReliability("red", red.reliability.reason);

	const yellow = analyzed.find(
		(step) => step.reliability?.status === "yellow",
	);
	if (yellow) {
		return routeStatusReliability("yellow", yellow.reliability.reason);
	}

	const missing = analyzed.find(
		(step) => step.reliability?.status === "unknown",
	);
	if (missing) return unknownReliability("部分路段可靠度資料不足");

	return routeStatusReliability("green", "可分析路段皆為低風險");
}

function routeStatusReliability(status, reason) {
	const { label } = statusMeta(status);
	return {
		status,
		label,
		reason,
		metrics: [],
	};
}

function reliabilityStatusFromMetrics(mode, metrics) {
	const normalizedMode = normalizeModeString(mode);
	const lookup = metricLookup(metrics);
	if (normalizedMode === "BUS") return busReliabilityStatus(lookup);
	if (["RAIL", "TRAIN", "TRA"].includes(normalizedMode)) {
		return railReliabilityStatus(lookup);
	}
	if (
		["YOUBIKE", "BICYCLE_RENTAL", "BIKE_RENTAL", "BICYCLE"].includes(
			normalizedMode,
		)
	) {
		return youBikeReliabilityStatus(lookup);
	}
	return "unknown";
}

function busReliabilityStatus(metrics) {
	const sampleCount = metricNumber(metrics, "sample_count");
	if (sampleCount !== null && sampleCount < 3) return "unknown";

	const avgError = metricNumber(metrics, "avg_abs_arrival_error_minutes");
	if (avgError !== null) {
		return statusFromLowerIsBetter(avgError, 5, 10);
	}

	const onTimeCount = metricNumber(metrics, "on_time_count");
	if (onTimeCount !== null && sampleCount) {
		return statusFromHigherIsBetter(
			(onTimeCount / sampleCount) * 100,
			80,
			60,
		);
	}
	return "unknown";
}

function railReliabilityStatus(metrics) {
	const sampleCount = metricNumber(metrics, "sample_count");
	if (sampleCount !== null && sampleCount < 3) return "unknown";

	const onTimeRate = metricNumber(metrics, "on_time_rate");
	if (onTimeRate !== null) {
		return statusFromHigherIsBetter(onTimeRate, 80, 60);
	}
	return "unknown";
}

function youBikeReliabilityStatus(metrics) {
	const values = [
		metricNumber(metrics, "avg_available_rent_bikes"),
		metricNumber(metrics, "avg_available_return_bikes"),
	].filter((value) => value !== null);
	if (values.length === 0) return "unknown";
	return statusFromHigherIsBetter(Math.min(...values), 5, 2);
}

function statusFromLowerIsBetter(value, greenMax, yellowMax) {
	if (value <= greenMax) return "green";
	if (value <= yellowMax) return "yellow";
	return "red";
}

function statusFromHigherIsBetter(value, greenMin, yellowMin) {
	if (value >= greenMin) return "green";
	if (value >= yellowMin) return "yellow";
	return "red";
}

function metricLookup(metrics) {
	return metrics.reduce((lookup, metric) => {
		lookup[metric.key] = metric;
		return lookup;
	}, {});
}

function metricNumber(metrics, key) {
	const metric = metrics[key];
	if (!metric) return null;
	const value = Number(metric.value);
	return Number.isFinite(value) ? value : null;
}

function routeNameForReliability(step) {
	const { route } = step;
	if (typeof route === "string") return route;
	if (route) {
		return firstNonEmpty(
			route.shortName,
			route.short_name,
			route.longName,
			route.long_name,
			route.name,
		);
	}
	return firstNonEmpty(
		step.routeName,
		step.route_name,
		step.lineName,
		step.line_name,
	);
}

function trainStationIdForReliability(step) {
	return firstNonEmpty(
		step.stationId,
		step.station_id,
		step.fromStationId,
		step.from_station_id,
	);
}

function stationUidForReliability(step) {
	return firstNonEmpty(
		step.stationUid,
		step.station_uid,
		step.fromStationUid,
		step.from_station_uid,
	);
}

function trainTypeCodeForReliability(step) {
	const trainTypeCode = firstNonEmpty(
		step.trainTypeCode,
		step.train_type_code,
	);
	if (["RAIL", "TRAIN", "TRA"].includes(reliabilityMode(step))) {
		return trainTypeCode || "all";
	}
	return trainTypeCode;
}

function rentalStationUid(step, side) {
	const station = rentalStationForReliability(step, side);
	if (!station) return "";
	if (typeof station === "string") return normalizeRentalStationUid(station);
	return normalizeRentalStationUid(
		firstNonEmpty(
			station.station_id,
			station.stationId,
			station.station_uid,
			station.stationUid,
			station.uid,
			station.id,
		),
	);
}

function normalizeRentalStationUid(uid) {
	return String(uid || "")
		.trim()
		.replace(/^youbike:/i, "");
}

function rentalStationName(step, side) {
	const station = rentalStationForReliability(step, side);
	if (!station || typeof station === "string") return "";
	return firstNonEmpty(
		station.name,
		station.station_name,
		station.stationName,
	);
}

function rentalStationForReliability(step, side) {
	if (side === "pickup") {
		return (
			step.pickup_station ||
			step.pickupStation ||
			step.pickupRentalStation ||
			step.fromRentalStation
		);
	}
	return (
		step.return_station ||
		step.returnStation ||
		step.returnRentalStation ||
		step.toRentalStation
	);
}

function normalizeDirection(direction) {
	if (direction === undefined || direction === null || direction === "") {
		return undefined;
	}
	const value = Number(direction);
	return Number.isInteger(value) ? value : undefined;
}

function normalizeModeString(mode) {
	return String(mode || "")
		.trim()
		.toUpperCase()
		.replaceAll("-", "_");
}

function firstNonEmpty(...values) {
	for (const value of values) {
		if (value === undefined || value === null) continue;
		const normalized = String(value).trim();
		if (normalized) return normalized;
	}
	return "";
}

function reliabilityOf(item) {
	return item?.reliability || unknownReliability();
}

function reliabilityMetricText(reliability) {
	if (!reliability?.metrics?.length) return "";
	return reliability.metrics
		.slice(0, 2)
		.map((metric) => `${metric.label} ${metric.value}${metric.unit}`)
		.join(" · ");
}

function stationOf(id) {
	if (!id) return { name: "", coord: null, punctuality: "" };
	if (typeof id === "object") return id;
	return (
		getCustomStation(id) ||
		getStation(id) || { name: id, coord: null, punctuality: "" }
	);
}

function punctualityOf(id) {
	const s = stationOf(id);
	if (!s) return null;
	return PUNCTUALITY[s.punctuality] || DEFAULT_STATION_META;
}

function showStationDetail(stationId) {
	const s = stationOf(stationId);
	if (!s) return;
	const p = punctualityOf(stationId);
	const coordText = s.coord ? s.coord.join(", ") : "尚無座標";
	alert(`${s.name}\n座標：${coordText}\n準點狀況：${p.label}`);
}
</script>

<template>
	<div class="rp">
		<!-- LEFT PANEL ────────────────────────────────────────────── -->
		<aside class="rp-panel">
			<!-- Phase: form -->
			<section v-if="phase === 'form'" class="rp-section">
				<header class="rp-header">
					<h2>規劃路線</h2>
					<p>輸入起終點與出發時間，立即取得多條建議路線</p>
				</header>

				<label class="rp-field">
					<span class="rp-field-label">出發日期</span>
					<input v-model="formData.date" type="date" />
				</label>
				<label class="rp-field">
					<span class="rp-field-label">出發時間</span>
					<input v-model="formData.time" type="time" />
				</label>

				<label class="rp-field">
					<span class="rp-field-label">
						<span class="rp-pin rp-pin--start" />
						起點
					</span>
					<input
						v-model="formData.start"
						type="text"
						placeholder="輸入起點地址或地名"
					/>
				</label>
				<label class="rp-field">
					<span class="rp-field-label">
						<span class="rp-pin rp-pin--end" />
						終點
					</span>
					<input
						v-model="formData.end"
						type="text"
						placeholder="輸入終點地址或地名"
					/>
				</label>

				<button
					class="rp-btn rp-btn--primary"
					:disabled="
						planningLoading || !formData.start || !formData.end
					"
					@click="submitForm"
				>
					繼續
					<span class="rp-icon">arrow_forward</span>
				</button>

				<div class="rp-or">
					<span>或</span>
				</div>

				<button
					class="rp-btn"
					:disabled="!formData.start || !formData.end"
					@click="submitCustom"
				>
					<span class="rp-icon">tune</span>
					自訂路線
				</button>
			</section>

			<!-- Phase: list (backend results) -->
			<section v-else-if="phase === 'list'" class="rp-section">
				<header class="rp-header rp-header--with-back">
					<button class="rp-back" @click="backToForm">
						<span class="rp-icon">arrow_back</span>
					</button>
					<div>
						<h2>建議路線</h2>
						<p class="rp-sub">
							{{ formData.start }} → {{ formData.end }} ·
							{{ formData.date }} {{ formData.time }} 出發
						</p>
						<p v-if="planningLoading" class="rp-sub">
							路線規劃中...
						</p>
						<p v-else-if="planningError" class="rp-sub">
							{{ planningError }}
						</p>
						<p v-else-if="reliabilityLoading" class="rp-sub">
							可靠度分析中...
						</p>
						<p v-else-if="reliabilityError" class="rp-sub">
							可靠度分析暫時無法取得
						</p>
					</div>
				</header>

				<ul v-if="plannedRoutes.length" class="rp-routes">
					<li
						v-for="(r, i) in plannedRoutes"
						:key="r.id"
						class="rp-route"
						:class="{
							'rp-route--hovered': hoveredRouteIdx === i,
							'rp-route--dimmed':
								hoveredRouteIdx !== null &&
								hoveredRouteIdx !== i,
						}"
						@click="pickRoute(r)"
						@mouseenter="hoveredRouteIdx = i"
						@mouseleave="hoveredRouteIdx = null"
					>
						<span
							class="rp-route-badge"
							:style="{
								backgroundColor:
									ROUTE_BADGE_COLORS[
										i % ROUTE_BADGE_COLORS.length
									],
							}"
						>
							{{ i + 1 }}
						</span>
						<div class="rp-route-top">
							<div class="rp-route-times">
								<span class="rp-route-board">
									{{ r.boardTime }}
								</span>
								<span class="rp-route-arrow">→</span>
								<span class="rp-route-alight">
									{{ r.alightTime }}
								</span>
							</div>
							<span class="rp-route-duration">
								{{ r.durationMin }} 分鐘
							</span>
						</div>
						<div class="rp-route-reliability">
							<span
								class="rp-reliability-badge"
								:class="`rp-reliability-badge--${reliabilityOf(r).status}`"
							>
								{{ reliabilityOf(r).label }}
							</span>
							<span class="rp-reliability-reason">
								{{ reliabilityOf(r).reason }}
							</span>
						</div>
						<div class="rp-route-icons">
							<template
								v-for="(it, idx) in transitIcons(r)"
								:key="it.key"
							>
								<span
									class="rp-icon rp-route-icon"
									:style="{ color: it.color }"
								>
									{{ it.icon }}
								</span>
								<span
									v-if="idx !== transitIcons(r).length - 1"
									class="rp-route-sep"
								>
									›
								</span>
							</template>
						</div>
						<div class="rp-route-meta">
							{{ r.summary }} · 轉乘 {{ r.transferCount }} 次
						</div>
					</li>
				</ul>
				<p v-else class="rp-empty">
					{{ planningLoading ? "正在取得建議路線" : "尚無建議路線" }}
				</p>
			</section>

			<!-- Phase: detail (single backend route) -->
			<section v-else-if="phase === 'detail'" class="rp-section">
				<header class="rp-header rp-header--with-back">
					<button class="rp-back" @click="backToList">
						<span class="rp-icon">arrow_back</span>
					</button>
					<div>
						<h2>路線詳情</h2>
						<p class="rp-sub">
							{{ formData.start }} → {{ formData.end }} ·
							{{ selectedRoute.boardTime }}–{{
								selectedRoute.alightTime
							}}
							（{{ selectedRoute.durationMin }} 分鐘）
						</p>
						<div class="rp-route-reliability">
							<span
								class="rp-reliability-badge"
								:class="`rp-reliability-badge--${reliabilityOf(selectedRoute).status}`"
							>
								{{ reliabilityOf(selectedRoute).label }}
							</span>
							<span class="rp-reliability-reason">
								{{ reliabilityOf(selectedRoute).reason }}
							</span>
						</div>
					</div>
				</header>

				<ol class="rp-timeline">
					<li
						v-for="(step, idx) in selectedRoute.steps"
						:key="idx"
						class="rp-step"
						:class="`rp-step--${step.kind}`"
					>
						<div
							class="rp-step-rail"
							:style="{
								'--rail-color':
									step.kind === 'walk' ? '#888' : step.color,
							}"
						/>
						<div class="rp-step-body">
							<div
								v-if="step.kind === 'walk'"
								class="rp-step-walk"
							>
								<div class="rp-step-line">
									<span class="rp-icon">directions_walk</span>
									<strong>步行</strong>
									<span class="rp-step-mut">
										{{ step.durationMin }} 分鐘
									</span>
								</div>
								<div class="rp-step-route">
									{{ step.from.name }} → {{ step.to.name }}
								</div>
							</div>
							<div v-else>
								<div class="rp-step-line">
									<span
										class="rp-icon"
										:style="{ color: step.color }"
									>
										{{ step.modeIcon }}
									</span>
									<strong :style="{ color: step.color }">
										{{ step.lineName }}
									</strong>
									<span class="rp-step-mut">
										{{ step.boardTime }}–{{
											step.alightTime
										}}
										· {{ step.durationMin }} 分鐘
									</span>
								</div>
								<div class="rp-step-stations">
									<div class="rp-station-row">
										<span class="rp-station-name">
											{{ stationOf(step.from).name }}
										</span>
										<span
											class="rp-badge"
											:style="{
												color: punctualityOf(step.from)
													.color,
											}"
										>
											<span class="rp-icon">
												{{
													punctualityOf(step.from)
														.icon
												}}
											</span>
											{{ punctualityOf(step.from).label }}
										</span>
										<button
											class="rp-detail-btn"
											@click="
												showStationDetail(step.from)
											"
										>
											站點詳情
										</button>
									</div>
									<div class="rp-station-row">
										<span class="rp-station-name">
											{{ stationOf(step.to).name }}
										</span>
										<span
											class="rp-badge"
											:style="{
												color: punctualityOf(step.to)
													.color,
											}"
										>
											<span class="rp-icon">
												{{
													punctualityOf(step.to).icon
												}}
											</span>
											{{ punctualityOf(step.to).label }}
										</span>
										<button
											class="rp-detail-btn"
											@click="showStationDetail(step.to)"
										>
											站點詳情
										</button>
									</div>
								</div>
							</div>
							<div
								class="rp-step-reliability"
								:class="`rp-step-reliability--${reliabilityOf(step).status}`"
							>
								<span
									class="rp-reliability-badge"
									:class="`rp-reliability-badge--${reliabilityOf(step).status}`"
								>
									{{ reliabilityOf(step).label }}
								</span>
								<span class="rp-step-reliability-reason">
									{{ reliabilityOf(step).reason }}
								</span>
								<span
									v-if="
										reliabilityMetricText(
											reliabilityOf(step),
										)
									"
									class="rp-step-reliability-metrics"
								>
									{{
										reliabilityMetricText(
											reliabilityOf(step),
										)
									}}
								</span>
							</div>
						</div>
					</li>
				</ol>
			</section>

			<!-- Phase: custom (self-planned) -->
			<section v-else-if="phase === 'custom'" class="rp-section">
				<header class="rp-header rp-header--with-back">
					<button class="rp-back" @click="backToForm">
						<span class="rp-icon">arrow_back</span>
					</button>
					<div>
						<h2>自訂路線</h2>
						<p class="rp-sub">
							{{ formData.start }} → {{ formData.end }} ·
							{{ formData.date }} {{ formData.time }} 出發
						</p>
					</div>
				</header>

				<div v-if="customRoute" class="rp-custom-summary">
					<span>
						{{ customRoute.boardTime }} →
						{{ customRoute.alightTime }}
					</span>
					<span>{{ customRoute.durationMin }} 分鐘</span>
					<span>轉乘 {{ customRoute.transferCount }} 次</span>
				</div>

				<ol v-if="customRoute" class="rp-timeline">
					<li
						v-for="(step, idx) in customRoute.steps"
						:key="idx"
						class="rp-step"
					>
						<div
							class="rp-step-rail"
							:style="{ '--rail-color': step.color }"
						/>
						<div class="rp-step-body">
							<div class="rp-step-line">
								<span
									class="rp-icon"
									:style="{ color: step.color }"
								>
									{{ step.modeIcon }}
								</span>
								<strong :style="{ color: step.color }">
									{{ step.lineName }}
								</strong>
								<span class="rp-step-mut">
									{{ step.boardTime }}–{{ step.alightTime }} ·
									{{ step.durationMin }} 分鐘
								</span>
								<button
									class="rp-step-remove"
									title="移除節點"
									@click="removeStep(idx)"
								>
									<span class="rp-icon">close</span>
								</button>
							</div>
							<div class="rp-step-stations">
								<div class="rp-station-row">
									<span class="rp-station-name">
										{{ stationOf(step.from).name }}
									</span>
									<span
										class="rp-badge"
										:style="{
											color: punctualityOf(step.from)
												.color,
										}"
									>
										<span class="rp-icon">
											{{ punctualityOf(step.from).icon }}
										</span>
										{{ punctualityOf(step.from).label }}
									</span>
								</div>
								<div class="rp-station-row">
									<span class="rp-station-name">
										{{ stationOf(step.to).name }}
									</span>
									<span
										class="rp-badge"
										:style="{
											color: punctualityOf(step.to).color,
										}"
									>
										<span class="rp-icon">
											{{ punctualityOf(step.to).icon }}
										</span>
										{{ punctualityOf(step.to).label }}
									</span>
								</div>
							</div>
						</div>
					</li>
				</ol>
				<p v-else class="rp-empty">
					尚未新增任何節點，請按下方按鈕開始規劃。
				</p>

				<!-- Add-node form -->
				<div v-if="newStep.open" class="rp-add-form">
					<div class="rp-add-row">
						<label
							class="rp-mode-pill"
							:class="{
								'rp-mode-pill--on': newStep.mode === 'mrt',
							}"
						>
							<input
								v-model="newStep.mode"
								type="radio"
								value="mrt"
							/>
							<span class="rp-icon">subway</span>
							捷運
						</label>
						<label
							class="rp-mode-pill"
							:class="{
								'rp-mode-pill--on': newStep.mode === 'bus',
							}"
						>
							<input
								v-model="newStep.mode"
								type="radio"
								value="bus"
							/>
							<span class="rp-icon">directions_bus</span>
							公車
						</label>
					</div>

					<label class="rp-field">
						<span class="rp-field-label">路線</span>
						<select v-model="newStep.lineId">
							<option value="" disabled>請選擇路線</option>
							<option
								v-for="l in availableLines"
								:key="l.id"
								:value="l.id"
							>
								{{ l.name }}
							</option>
						</select>
					</label>

					<label class="rp-field">
						<span class="rp-field-label">上車站點</span>
						<select
							v-model="newStep.from"
							:disabled="!newStep.lineId"
						>
							<option value="" disabled>請選擇</option>
							<option
								v-for="s in stationsForNewLine"
								:key="s.id"
								:value="s.id"
							>
								{{ s.name }} —
								{{ PUNCTUALITY[s.punctuality].label }}
							</option>
						</select>
					</label>

					<label class="rp-field">
						<span class="rp-field-label">下車站點</span>
						<select
							v-model="newStep.to"
							:disabled="!newStep.lineId"
						>
							<option value="" disabled>請選擇</option>
							<option
								v-for="s in stationsForNewLine"
								:key="s.id"
								:value="s.id"
								:disabled="s.id === newStep.from"
							>
								{{ s.name }} —
								{{ PUNCTUALITY[s.punctuality].label }}
							</option>
						</select>
					</label>

					<div class="rp-add-actions">
						<button class="rp-btn" @click="cancelAddNode">
							取消
						</button>
						<button
							class="rp-btn rp-btn--primary"
							:disabled="
								!newStep.lineId ||
								!newStep.from ||
								!newStep.to ||
								newStep.from === newStep.to
							"
							@click="commitAddNode"
						>
							加入節點
						</button>
					</div>
				</div>

				<button
					v-else
					class="rp-btn rp-btn--ghost rp-add-btn"
					@click="openAddNode"
				>
					<span class="rp-icon">add</span>
					新增節點
				</button>
			</section>
		</aside>

		<!-- RIGHT MAP ─────────────────────────────────────────────── -->
		<div class="rp-map-wrap">
			<RoutePlannerMap
				:geometry="mapGeometry"
				:hovered-route-idx="hoveredRouteIdx"
			/>
		</div>
	</div>
</template>

<style scoped lang="scss">
.rp {
	display: flex;
	width: 100%;
	height: calc(100vh - 60px);
	height: calc(var(--vh) * 100 - 60px);
	background-color: var(--color-background);
}

// ── Left panel ───────────────────────────────────────────────
.rp-panel {
	width: 380px;
	min-width: 380px;
	height: 100%;
	border-right: 1px solid var(--color-border);
	background-color: var(--color-component-background);
	overflow-y: auto;
	padding: var(--font-m);
	box-sizing: border-box;

	@media (max-width: 1000px) {
		width: 320px;
		min-width: 320px;
	}
}

.rp-section {
	display: flex;
	flex-direction: column;
	gap: 14px;
}

.rp-header {
	margin-bottom: 4px;

	h2 {
		margin: 0;
	}

	p,
	.rp-sub {
		margin-top: 4px;
		font-size: var(--font-s);
		color: var(--color-complement-text);
	}

	&--with-back {
		display: flex;
		align-items: flex-start;
		gap: 10px;
	}
}

.rp-back {
	margin-top: 2px;
	width: 28px;
	height: 28px;
	display: flex;
	align-items: center;
	justify-content: center;
	border-radius: 50%;
	background-color: rgba(255, 255, 255, 0.06);
	transition: background-color 0.15s;

	&:hover {
		background-color: rgba(90, 156, 248, 0.2);
	}

	.rp-icon {
		font-size: 18px;
	}
}

// ── Generic form field ──────────────────────────────────────
.rp-field {
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.rp-field-label {
	display: flex;
	align-items: center;
	gap: 6px;
	font-size: var(--font-s);
	color: var(--color-complement-text);
}

.rp-pin {
	display: inline-block;
	width: 8px;
	height: 8px;
	border-radius: 50%;

	&--start {
		background-color: #3fb950;
	}

	&--end {
		background-color: #f85149;
	}
}

.rp-or {
	display: flex;
	align-items: center;
	gap: 10px;
	margin: 4px 0;
	font-size: var(--font-s);
	color: var(--color-complement-text);

	&::before,
	&::after {
		content: "";
		flex: 1;
		border-top: 1px solid var(--color-border);
	}
}

.rp-btn {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: 6px;
	padding: 8px 14px;
	border-radius: 5px;
	background-color: rgba(255, 255, 255, 0.06);
	color: var(--color-normal-text);
	font-size: var(--font-ms);
	transition:
		background-color 0.15s,
		opacity 0.15s;

	&:hover:not(:disabled) {
		background-color: rgba(255, 255, 255, 0.12);
	}

	&:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	&--primary {
		background-color: var(--color-highlight);
		color: #fff;

		&:hover:not(:disabled) {
			background-color: var(--color-highlight);
			filter: brightness(1.1);
		}
	}

	&--ghost {
		border: 1px dashed var(--color-border);
		background-color: transparent;
	}
}

.rp-icon {
	font-family: var(--font-icon);
	font-size: var(--font-m);
	line-height: 1;
}

// ── Route list ──────────────────────────────────────────────
.rp-routes {
	list-style: none;
	display: flex;
	flex-direction: column;
	gap: 10px;
}

.rp-route {
	position: relative;
	padding: 12px 12px 12px 44px;
	border-radius: 6px;
	border: 1px solid var(--color-border);
	background-color: rgba(255, 255, 255, 0.02);
	cursor: pointer;
	transition:
		border-color 0.15s,
		background-color 0.15s,
		opacity 0.15s;

	&:hover {
		border-color: var(--color-highlight);
		background-color: rgba(90, 156, 248, 0.06);
	}
}

.rp-route--dimmed {
	opacity: 0.45;
}

.rp-route--hovered {
	border-color: var(--color-highlight);
	background-color: rgba(90, 156, 248, 0.08);
}

.rp-route-badge {
	position: absolute;
	top: 12px;
	left: 12px;
	width: 24px;
	height: 24px;
	border-radius: 50%;
	display: flex;
	align-items: center;
	justify-content: center;
	color: #fff;
	font-size: 13px;
	font-weight: 700;
	line-height: 1;
}

.rp-route-top {
	display: flex;
	justify-content: space-between;
	align-items: baseline;
	margin-bottom: 8px;
}

.rp-route-times {
	display: flex;
	align-items: baseline;
	gap: 6px;
	font-size: var(--font-m);
	font-weight: 600;
}

.rp-route-arrow {
	color: var(--color-complement-text);
	font-size: var(--font-s);
}

.rp-route-duration {
	font-size: var(--font-s);
	color: var(--color-highlight);
}

.rp-route-icons {
	display: flex;
	align-items: center;
	gap: 4px;
	margin-bottom: 6px;
}

.rp-route-reliability {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-bottom: 8px;
	min-width: 0;
}

.rp-reliability-badge {
	flex-shrink: 0;
	display: inline-flex;
	align-items: center;
	padding: 2px 7px;
	border-radius: 4px;
	font-size: 11px;
	font-weight: 600;

	&--green {
		background-color: rgba(63, 185, 80, 0.16);
		color: #3fb950;
	}

	&--yellow {
		background-color: rgba(210, 153, 34, 0.16);
		color: #d29922;
	}

	&--red {
		background-color: rgba(248, 81, 73, 0.16);
		color: #f85149;
	}

	&--unknown {
		background-color: rgba(139, 148, 158, 0.16);
		color: #8b949e;
	}
}

.rp-reliability-reason {
	min-width: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--color-complement-text);
	font-size: var(--font-s);
}

.rp-route-icon {
	font-size: 18px;
}

.rp-route-sep {
	color: var(--color-complement-text);
	font-size: 14px;
}

.rp-route-meta {
	font-size: var(--font-s);
	color: var(--color-complement-text);
}

// ── Vertical timeline (detail + custom) ─────────────────────
.rp-timeline {
	list-style: none;
	display: flex;
	flex-direction: column;
}

.rp-step {
	display: flex;
	gap: 12px;
	min-height: 60px;
	padding-bottom: 4px;
}

.rp-step-rail {
	width: 4px;
	flex-shrink: 0;
	background-color: var(--rail-color, #5a9cf8);
	border-radius: 2px;
	margin-top: 6px;

	.rp-step--walk & {
		background-color: transparent;
		background-image: linear-gradient(
			to bottom,
			var(--rail-color) 50%,
			transparent 50%
		);
		background-size: 4px 8px;
	}
}

.rp-step-body {
	flex: 1;
	padding: 4px 0 14px;
	border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.rp-step:last-child .rp-step-body {
	border-bottom: none;
}

.rp-step-line {
	display: flex;
	align-items: center;
	gap: 6px;
	font-size: var(--font-ms);
}

.rp-step-mut {
	font-size: var(--font-s);
	color: var(--color-complement-text);
	margin-left: auto;
}

.rp-step-route {
	margin-top: 4px;
	font-size: var(--font-s);
	color: var(--color-complement-text);
}

.rp-step-stations {
	margin-top: 8px;
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.rp-step-reliability {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 6px;
	margin-top: 8px;
	padding: 7px 8px;
	border-radius: 5px;
	background-color: rgba(255, 255, 255, 0.035);
	font-size: var(--font-s);

	&--green {
		border-left: 3px solid #3fb950;
	}

	&--yellow {
		border-left: 3px solid #d29922;
	}

	&--red {
		border-left: 3px solid #f85149;
	}

	&--unknown {
		border-left: 3px solid #8b949e;
	}
}

.rp-step-reliability-reason {
	color: var(--color-normal-text);
}

.rp-step-reliability-metrics {
	width: 100%;
	color: var(--color-complement-text);
}

.rp-station-row {
	display: flex;
	align-items: center;
	gap: 8px;
	font-size: var(--font-s);
}

.rp-station-name {
	flex: 1;
	font-size: var(--font-ms);
}

.rp-badge {
	display: inline-flex;
	align-items: center;
	gap: 3px;
	padding: 2px 6px;
	border-radius: 999px;
	background-color: rgba(255, 255, 255, 0.06);
	font-size: 11px;

	.rp-icon {
		font-size: 13px;
	}
}

.rp-detail-btn {
	padding: 2px 8px;
	border-radius: 4px;
	background-color: rgba(255, 255, 255, 0.06);
	color: var(--color-complement-text);
	font-size: 11px;
	transition:
		background-color 0.15s,
		color 0.15s;

	&:hover {
		background-color: rgba(90, 156, 248, 0.15);
		color: var(--color-highlight);
	}
}

.rp-step-remove {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 22px;
	height: 22px;
	border-radius: 50%;
	color: var(--color-complement-text);

	&:hover {
		background-color: rgba(248, 81, 73, 0.15);
		color: #f85149;
	}

	.rp-icon {
		font-size: 14px;
	}
}

// ── Custom plan extras ──────────────────────────────────────
.rp-custom-summary {
	display: flex;
	flex-wrap: wrap;
	gap: 12px;
	padding: 8px 10px;
	border-radius: 5px;
	background-color: rgba(90, 156, 248, 0.08);
	font-size: var(--font-s);
	color: var(--color-highlight);
}

.rp-empty {
	padding: 14px;
	border: 1px dashed var(--color-border);
	border-radius: 6px;
	text-align: center;
	font-size: var(--font-s);
	color: var(--color-complement-text);
}

.rp-add-btn {
	width: 100%;
}

.rp-add-form {
	display: flex;
	flex-direction: column;
	gap: 10px;
	padding: 12px;
	border: 1px solid var(--color-border);
	border-radius: 6px;
	background-color: rgba(255, 255, 255, 0.02);
}

.rp-add-row {
	display: flex;
	gap: 8px;
}

.rp-mode-pill {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	padding: 5px 12px;
	border-radius: 999px;
	border: 1px solid var(--color-border);
	font-size: var(--font-s);
	cursor: pointer;
	user-select: none;
	transition:
		border-color 0.15s,
		background-color 0.15s,
		color 0.15s;

	input {
		display: none;
	}

	&--on {
		border-color: var(--color-highlight);
		background-color: rgba(90, 156, 248, 0.1);
		color: var(--color-highlight);
	}
}

.rp-add-actions {
	display: flex;
	justify-content: flex-end;
	gap: 8px;
}

// ── Map ─────────────────────────────────────────────────────
.rp-map-wrap {
	flex: 1;
	height: 100%;
	padding: var(--font-m);
	box-sizing: border-box;
}
</style>
