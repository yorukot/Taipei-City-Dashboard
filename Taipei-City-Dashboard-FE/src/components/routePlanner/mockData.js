// Mock data used by the route planner. Coordinates are approximate Taipei MRT
// station locations — close enough to draw believable polylines on the map.

export const PUNCTUALITY = {
	on_time: { label: "常準時", color: "#3fb950", icon: "check_circle" },
	sometimes_late: { label: "偶爾遲到", color: "#d29922", icon: "schedule" },
	often_late: { label: "常遲到", color: "#f85149", icon: "error" },
};

// Taipei MRT line colours
const RED = "#E3002C";
const BLUE = "#0070BD";
const BROWN = "#B57A2C";
const GREEN = "#008659";
const ORANGE = "#F8B62D";

// A handful of stations with coordinates and a fake punctuality verdict.
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

// Transit lines used for both backend & custom planning. Each line lists the
// station ids it serves in order so a custom plan can pick a board+alight pair.
export const transitLines = [
	{
		id: "mrt_red",
		type: "mrt",
		name: "淡水信義線",
		color: RED,
		stationIds: [
			"minquan_w",
			"shuanglian",
			"zhongshan",
			"taipei_main",
			"cks_hall",
			"dongmen",
			"daan_park",
			"daan",
			"xinyi_anhe",
			"taipei_101",
		],
	},
	{
		id: "mrt_blue",
		type: "mrt",
		name: "板南線",
		color: BLUE,
		stationIds: [
			"bannan_fuzhong",
			"jiangzicui",
			"taipei_main",
			"zhongxiao_fuxing",
			"zhongxiao_dunhua",
		],
	},
	{
		id: "mrt_brown",
		type: "mrt",
		name: "文湖線",
		color: BROWN,
		stationIds: ["daan", "zhongxiao_fuxing", "nanjing_fuxing", "songshan"],
	},
	{
		id: "mrt_green",
		type: "mrt",
		name: "松山新店線",
		color: GREEN,
		stationIds: [
			"songshan",
			"nanjing_sanmin",
			"zhongshan",
			"guting",
			"gongguan",
		],
	},
	{
		id: "bus_295",
		type: "bus",
		name: "295 公車",
		color: ORANGE,
		stationIds: [
			"taipei_main",
			"zhongshan",
			"nanjing_fuxing",
			"nanjing_sanmin",
			"songshan",
		],
	},
	{
		id: "bus_5",
		type: "bus",
		name: "5 公車",
		color: ORANGE,
		stationIds: ["taipei_main", "cks_hall", "guting", "gongguan"],
	},
];

export function getLine(id) {
	return transitLines.find((l) => l.id === id);
}

export function getStation(id) {
	return stations[id];
}

// Helper: build the polyline coords for a transit segment (board → alight).
export function lineCoords(lineId, fromStationId, toStationId) {
	const line = getLine(lineId);
	if (!line) return [];
	const i = line.stationIds.indexOf(fromStationId);
	const j = line.stationIds.indexOf(toStationId);
	if (i < 0 || j < 0) return [];
	const slice =
		i <= j
			? line.stationIds.slice(i, j + 1)
			: line.stationIds.slice(j, i + 1).reverse();
	return slice.map((id) => stations[id].coord);
}

// Mock origin / destination markers (used when the user types free-text).
const ORIGIN = [121.5235, 25.056];
const DEST = [121.563, 25.0335];

// Three pre-computed "backend" routes from 中山 area → 台北101 area.
export const mockRoutes = [
	{
		id: "route_1",
		summary: "推薦路線",
		boardTime: "08:30",
		alightTime: "08:55",
		durationMin: 25,
		transferCount: 0,
		steps: [
			{
				kind: "walk",
				durationMin: 4,
				from: { name: "起點", coord: ORIGIN },
				to: { name: "中山", coord: stations.zhongshan.coord },
			},
			{
				kind: "transit",
				lineId: "mrt_red",
				lineName: "淡水信義線",
				color: RED,
				modeIcon: "subway",
				boardTime: "08:34",
				alightTime: "08:51",
				durationMin: 17,
				from: "zhongshan",
				to: "taipei_101",
			},
			{
				kind: "walk",
				durationMin: 4,
				from: {
					name: "台北101/世貿",
					coord: stations.taipei_101.coord,
				},
				to: { name: "終點", coord: DEST },
			},
		],
	},
	{
		id: "route_2",
		summary: "捷運+捷運",
		boardTime: "08:30",
		alightTime: "09:02",
		durationMin: 32,
		transferCount: 1,
		steps: [
			{
				kind: "walk",
				durationMin: 5,
				from: { name: "起點", coord: ORIGIN },
				to: { name: "中山", coord: stations.zhongshan.coord },
			},
			{
				kind: "transit",
				lineId: "mrt_red",
				lineName: "淡水信義線",
				color: RED,
				modeIcon: "subway",
				boardTime: "08:36",
				alightTime: "08:42",
				durationMin: 6,
				from: "zhongshan",
				to: "taipei_main",
			},
			{
				kind: "walk",
				durationMin: 3,
				from: {
					name: "台北車站轉乘",
					coord: stations.taipei_main.coord,
				},
				to: {
					name: "台北車站 (板南線)",
					coord: stations.taipei_main.coord,
				},
			},
			{
				kind: "transit",
				lineId: "mrt_blue",
				lineName: "板南線",
				color: BLUE,
				modeIcon: "subway",
				boardTime: "08:46",
				alightTime: "08:54",
				durationMin: 8,
				from: "taipei_main",
				to: "zhongxiao_dunhua",
			},
			{
				kind: "walk",
				durationMin: 8,
				from: {
					name: "忠孝敦化",
					coord: stations.zhongxiao_dunhua.coord,
				},
				to: { name: "終點", coord: DEST },
			},
		],
	},
	{
		id: "route_3",
		summary: "公車路線",
		boardTime: "08:30",
		alightTime: "09:10",
		durationMin: 40,
		transferCount: 1,
		steps: [
			{
				kind: "walk",
				durationMin: 3,
				from: { name: "起點", coord: ORIGIN },
				to: { name: "中山", coord: stations.zhongshan.coord },
			},
			{
				kind: "transit",
				lineId: "bus_295",
				lineName: "295 公車",
				color: ORANGE,
				modeIcon: "directions_bus",
				boardTime: "08:34",
				alightTime: "08:55",
				durationMin: 21,
				from: "zhongshan",
				to: "nanjing_sanmin",
			},
			{
				kind: "transit",
				lineId: "mrt_green",
				lineName: "松山新店線",
				color: GREEN,
				modeIcon: "subway",
				boardTime: "09:00",
				alightTime: "09:06",
				durationMin: 6,
				from: "nanjing_sanmin",
				to: "songshan",
			},
			{
				kind: "walk",
				durationMin: 4,
				from: { name: "松山", coord: stations.songshan.coord },
				to: { name: "終點", coord: DEST },
			},
		],
	},
];

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
		if (step.kind === "walk") {
			polylines.push({
				id: `walk-${idx}`,
				coords: [step.from.coord, step.to.coord],
				color: "#888888",
				dashed: true,
			});
			addMarker(
				`m-${idx}-a`,
				step.from.name,
				step.from.coord,
				"#888",
				"walk",
			);
			addMarker(
				`m-${idx}-b`,
				step.to.name,
				step.to.coord,
				"#888",
				"walk",
			);
		} else {
			const coords = lineCoords(step.lineId, step.from, step.to);
			polylines.push({
				id: `transit-${idx}`,
				coords,
				color: step.color,
				dashed: false,
			});
			const a = stations[step.from];
			const b = stations[step.to];
			addMarker(`m-${idx}-a`, a.name, a.coord, step.color, "transit");
			addMarker(`m-${idx}-b`, b.name, b.coord, step.color, "transit");
		}
	});
	return { polylines, markers };
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

export const defaultEndpoints = { ORIGIN, DEST };
