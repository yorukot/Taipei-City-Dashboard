<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import mapboxGl from "mapbox-gl";
import "mapbox-gl/dist/mapbox-gl.css";

const props = defineProps({
	geometry: {
		type: Object,
		default: () => ({ polylines: [], markers: [] }),
	},
});

const containerRef = ref(null);
let map = null;
let markers = [];
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
}

function buildMarkerEl(marker) {
	const el = document.createElement("div");
	el.className = "rp-map-marker";
	el.style.setProperty("--mc", marker.color || "#5a9cf8");
	if (marker.kind === "endpoint") {
		el.classList.add("rp-map-marker--endpoint");
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
	});

	pts.forEach((m) => {
		const marker = new mapboxGl.Marker({ element: buildMarkerEl(m) })
			.setLngLat(m.coord)
			.addTo(map);
		markers.push(marker);
	});

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

watch(() => props.geometry, render, { deep: true });
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
