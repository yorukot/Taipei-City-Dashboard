// Synchronous name-indexed lookup over the bundled metro stations JSON
// (125 stations across 7 lines). Used by the OTP planner path to resolve
// station names returned by OpenTripPlanner to coordinates.
//
// Match is strict equality on station name to avoid substring collisions
// like "中山國中" matching "中山".

import metroStationsRaw from "../../assets/configs/selectors/metro-stations.json";

function buildIndex() {
	const byName = new Map();
	for (const [lineName, stops] of Object.entries(metroStationsRaw)) {
		for (const s of stops) {
			if (!s?.name || !Number.isFinite(s.lat) || !Number.isFinite(s.lon)) {
				continue;
			}
			const entry = {
				name: s.name,
				stationId: s.stationId,
				line: lineName,
				coord: [s.lon, s.lat],
			};
			const list = byName.get(s.name);
			if (list) list.push(entry);
			else byName.set(s.name, [entry]);
		}
	}
	return { byName };
}

const index = buildIndex();

export function metroStationByName(name) {
	if (!name) return null;
	const entries = index.byName.get(name);
	if (!entries || entries.length === 0) return null;
	return entries[0];
}
