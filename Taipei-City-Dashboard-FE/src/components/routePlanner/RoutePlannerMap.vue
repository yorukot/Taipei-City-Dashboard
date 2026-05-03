<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import mapboxGl from "mapbox-gl";
import "mapbox-gl/dist/mapbox-gl.css";

const props = defineProps({
	geometry: {
		type: Object,
		default: () => ({ polylines: [], markers: [] }),
	},
	hoveredRouteIdx: {
		type: Number,
		default: null,
	},
});

const containerRef = ref(null);
let map = null;
let markers = [];
// Per-layer / per-marker routeIdx so hover updates can dim non-matching ones
// without rebuilding the whole map.
let layerRouteIdx = new Map();
let markerRouteIdx = new Map();
const SOURCE_PREFIX = "rp-line-";

function clearLayers() {
	if (!map) return;
	const style = map.getStyle();
	if (!style?.layers) return;
	style.layers
		.filter((l) => l.id.startsWith(SOURCE_PREFIX))
		.forEach((l) => map.getLayer(l.id) && map.removeLayer(l.id));
	Object.keys(style.sources || {})
		.filter((s) => s.startsWith(SOURCE_PREFIX))
		.forEach((s) => map.getSource(s) && map.removeSource(s));
	markers.forEach((m) => m.remove());
	markers = [];
	layerRouteIdx.clear();
	markerRouteIdx.clear();
}

function buildMarkerEl(marker) {
	const el = document.createElement("div");
	el.className = "rp-map-marker";
	el.style.setProperty("--mc", marker.color || "#5a9cf8");
	if (marker.kind === "endpoint") {
		el.classList.add("rp-map-marker--endpoint");
	}
	if (marker.kind === "route-label") {
		el.classList.add("rp-map-marker--route-label");
		const badge = document.createElement("div");
		badge.className = "rp-map-marker-badge";
		badge.textContent = marker.name;
		el.appendChild(badge);
		return el;
	}
	const dot = document.createElement("div");
	dot.className = "rp-map-marker-dot";
	const label = document.createElement("div");
	label.className = "rp-map-marker-label";
	label.textContent = marker.name;
	el.appendChild(dot);
	el.appendChild(label);
	return el;
}

function render() {
	if (!map || !map.isStyleLoaded()) return;
	clearLayers();
	const { polylines = [], markers: pts = [] } = props.geometry || {};

	polylines.forEach((line, idx) => {
		const id = `${SOURCE_PREFIX}${line.id || idx}`;
		map.addSource(id, {
			type: "geojson",
			data: {
				type: "Feature",
				geometry: { type: "LineString", coordinates: line.coords },
				properties: {},
			},
		});
		map.addLayer({
			id,
			type: "line",
			source: id,
			layout: { "line-cap": "round", "line-join": "round" },
			paint: {
				"line-color": line.color,
				"line-width": 5,
				"line-opacity": 0.85,
				...(line.dashed ? { "line-dasharray": [1.5, 1.5] } : {}),
			},
		});
		if (Number.isInteger(line.routeIdx)) {
			layerRouteIdx.set(id, line.routeIdx);
		}
	});

	pts.forEach((m) => {
		const marker = new mapboxGl.Marker({ element: buildMarkerEl(m) })
			.setLngLat(m.coord)
			.addTo(map);
		markers.push(marker);
		if (Number.isInteger(m.routeIdx)) {
			markerRouteIdx.set(marker, m.routeIdx);
		}
	});

	applyHover();

	// Fit bounds to all coords.
	const all = [];
	polylines.forEach((l) => l.coords.forEach((c) => all.push(c)));
	pts.forEach((m) => all.push(m.coord));
	if (all.length >= 2) {
		const bounds = all.reduce(
			(b, c) => b.extend(c),
			new mapboxGl.LngLatBounds(all[0], all[0]),
		);
		map.fitBounds(bounds, { padding: 60, duration: 600, maxZoom: 15 });
	} else if (all.length === 1) {
		map.easeTo({ center: all[0], zoom: 14, duration: 500 });
	}
}

onMounted(() => {
	mapboxGl.accessToken = import.meta.env.VITE_MAPBOXTOKEN;
	map = new mapboxGl.Map({
		container: containerRef.value,
		style: "mapbox://styles/mapbox/dark-v11",
		center: [121.5436, 25.0414],
		zoom: 12,
		minZoom: 9,
		maxZoom: 18,
	});
	map.addControl(new mapboxGl.NavigationControl(), "top-right");
	map.on("load", render);
});

onBeforeUnmount(() => {
	clearLayers();
	if (map) {
		map.remove();
		map = null;
	}
});

function applyHover() {
	if (!map) return;
	const hovered = props.hoveredRouteIdx;
	layerRouteIdx.forEach((routeIdx, id) => {
		if (!map.getLayer(id)) return;
		const dim = hovered !== null && hovered !== routeIdx;
		map.setPaintProperty(id, "line-opacity", dim ? 0.15 : 0.85);
	});
	markerRouteIdx.forEach((routeIdx, marker) => {
		const el = marker.getElement();
		if (!el) return;
		const dim = hovered !== null && hovered !== routeIdx;
		el.classList.toggle("rp-map-marker--dimmed", dim);
	});
}

watch(() => props.geometry, render, { deep: true });
watch(() => props.hoveredRouteIdx, applyHover);
</script>

<template>
	<div ref="containerRef" class="rp-map" />
</template>

<style lang="scss">
.rp-map {
	width: 100%;
	height: 100%;
	border-radius: 8px;
	overflow: hidden;
}

.rp-map-marker {
	display: flex;
	flex-direction: column;
	align-items: center;
	transform: translateY(-6px);
	pointer-events: none;
	transition: opacity 0.15s;
}

.rp-map-marker--dimmed {
	opacity: 0.2;
}

.rp-map-marker--route-label {
	transform: translate(0, calc(-50% - 18px));
}

.rp-map-marker-badge {
	width: 28px;
	height: 28px;
	border-radius: 50%;
	background-color: var(--mc, #5a9cf8);
	color: #fff;
	font-size: 14px;
	font-weight: 700;
	display: flex;
	align-items: center;
	justify-content: center;
	border: 2px solid #fff;
	box-shadow: 0 0 6px rgba(0, 0, 0, 0.7);
}

.rp-map-marker-dot {
	width: 14px;
	height: 14px;
	border-radius: 50%;
	background-color: var(--mc, #5a9cf8);
	border: 2px solid #fff;
	box-shadow: 0 0 4px rgba(0, 0, 0, 0.6);
}

.rp-map-marker--endpoint .rp-map-marker-dot {
	width: 18px;
	height: 18px;
	border-width: 3px;
}

.rp-map-marker-label {
	margin-top: 3px;
	padding: 1px 5px;
	border-radius: 3px;
	background-color: rgba(0, 0, 0, 0.75);
	color: #fff;
	font-size: 10px;
	white-space: nowrap;
}
</style>
