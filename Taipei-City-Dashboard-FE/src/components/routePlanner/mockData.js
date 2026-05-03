// Shared helpers for the route planner. The `stations` registry is a small
// hardcoded set of Taipei MRT coordinates used to backfill leg coordinates by
// name when the routing API doesn't return them. `defaultEndpoints` is the
// placeholder origin/destination shown before the user submits the form.

export const PUNCTUALITY = {
	on_time: { label: "常準時", color: "#3fb950", icon: "check_circle" },
	sometimes_late: { label: "偶爾遲到", color: "#d29922", icon: "schedule" },
	often_late: { label: "常遲到", color: "#f85149", icon: "error" },
};

export const stations = {
	taipei_main: {
		name: "台北車站",
		coord: [121.517, 25.0478],
		punctuality: "on_time",
	},
	zhongshan: {
		name: "中山",
		coord: [121.5202, 25.0526],
		punctuality: "sometimes_late",
	},
	shuanglian: {
		name: "雙連",
		coord: [121.5202, 25.0578],
		punctuality: "on_time",
	},
	minquan_w: {
		name: "民權西路",
		coord: [121.5193, 25.0625],
		punctuality: "on_time",
	},
	daan: {
		name: "大安",
		coord: [121.5435, 25.0331],
		punctuality: "often_late",
	},
	daan_park: {
		name: "大安森林公園",
		coord: [121.535, 25.0334],
		punctuality: "on_time",
	},
	dongmen: {
		name: "東門",
		coord: [121.529, 25.0335],
		punctuality: "sometimes_late",
	},
	cks_hall: {
		name: "中正紀念堂",
		coord: [121.5183, 25.0322],
		punctuality: "on_time",
	},
	guting: {
		name: "古亭",
		coord: [121.5226, 25.0265],
		punctuality: "on_time",
	},
	gongguan: {
		name: "公館",
		coord: [121.5347, 25.0149],
		punctuality: "sometimes_late",
	},
	taipei_101: {
		name: "台北101/世貿",
		coord: [121.5644, 25.033],
		punctuality: "sometimes_late",
	},
	xinyi_anhe: {
		name: "信義安和",
		coord: [121.5527, 25.0331],
		punctuality: "on_time",
	},
	zhongxiao_fuxing: {
		name: "忠孝復興",
		coord: [121.5436, 25.0414],
		punctuality: "often_late",
	},
	zhongxiao_dunhua: {
		name: "忠孝敦化",
		coord: [121.5511, 25.0415],
		punctuality: "sometimes_late",
	},
	songshan: {
		name: "松山",
		coord: [121.5778, 25.0497],
		punctuality: "on_time",
	},
	nanjing_fuxing: {
		name: "南京復興",
		coord: [121.5436, 25.0521],
		punctuality: "sometimes_late",
	},
	nanjing_sanmin: {
		name: "南京三民",
		coord: [121.564, 25.0519],
		punctuality: "on_time",
	},
	jiangzicui: {
		name: "江子翠",
		coord: [121.4711, 25.0303],
		punctuality: "on_time",
	},
	bannan_fuzhong: {
		name: "府中",
		coord: [121.4593, 25.0083],
		punctuality: "on_time",
	},
};

export function getStation(id) {
	return stations[id];
}

const ORIGIN = [121.5235, 25.056];
const DEST = [121.563, 25.0335];

export const defaultEndpoints = { ORIGIN, DEST };

// Build map geometry (polylines + station markers) for a route object.
export function routeGeometry(route) {
	if (!route) return { polylines: [], markers: [] };
	const polylines = [];
	const markers = [];
	const seen = new Set();
	const addMarker = (id, name, coord, color, kind) => {
		const key = `${name}|${coord.join(",")}`;
		if (seen.has(key)) return;
		seen.add(key);
		markers.push({ id, name, coord, color, kind });
	};
	route.steps.forEach((step, idx) => {
		const fromPoint = routePoint(step.from);
		const toPoint = routePoint(step.to);
		if (step.kind === "walk") {
			if (fromPoint?.coord && toPoint?.coord) {
				polylines.push({
					id: `walk-${idx}`,
					coords: [fromPoint.coord, toPoint.coord],
					color: "#888888",
					dashed: true,
				});
			}
			if (fromPoint?.coord) {
				addMarker(
					`m-${idx}-a`,
					fromPoint.name,
					fromPoint.coord,
					"#888",
					"walk",
				);
			}
			if (toPoint?.coord) {
				addMarker(
					`m-${idx}-b`,
					toPoint.name,
					toPoint.coord,
					"#888",
					"walk",
				);
			}
		} else {
			const coords = [fromPoint?.coord, toPoint?.coord].filter(Boolean);
			if (coords.length >= 2) {
				polylines.push({
					id: `transit-${idx}`,
					coords,
					color: step.color,
					dashed: false,
				});
			}
			if (fromPoint?.coord) {
				addMarker(
					`m-${idx}-a`,
					fromPoint.name,
					fromPoint.coord,
					step.color,
					"transit",
				);
			}
			if (toPoint?.coord) {
				addMarker(
					`m-${idx}-b`,
					toPoint.name,
					toPoint.coord,
					step.color,
					"transit",
				);
			}
		}
	});
	return { polylines, markers };
}

function routePoint(point) {
	if (!point) return null;
	if (typeof point === "string") return stations[point] || null;
	return point;
}

// Build geometry for the "two pins only" form preview (start + end).
export function endpointsGeometry(startCoord, endCoord) {
	const markers = [];
	if (startCoord) {
		markers.push({
			id: "start",
			name: "起點",
			coord: startCoord,
			color: "#3fb950",
			kind: "endpoint",
		});
	}
	if (endCoord) {
		markers.push({
			id: "end",
			name: "終點",
			coord: endCoord,
			color: "#f85149",
			kind: "endpoint",
		});
	}
	const polylines = [];
	if (startCoord && endCoord) {
		polylines.push({
			id: "endpoints",
			coords: [startCoord, endCoord],
			color: "#888888",
			dashed: true,
		});
	}
	return { polylines, markers };
}
