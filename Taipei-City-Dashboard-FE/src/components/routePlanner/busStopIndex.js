// Lazy-loaded index over /mapData/bus_stop_tpe.geojson (28k stops).
// Same name can appear at multiple physical stops (上行 / 下行); StopUID is
// unique. `byName` therefore maps to an array; callers pass a hint coord
// to disambiguate.

import axios from "axios";

const GEOJSON_URL = "/mapData/bus_stop_tpe.geojson";

let cache = null;
let inflight = null;

function buildIndex(features) {
	const byUid = new Map();
	const byName = new Map();
	for (const f of features) {
		const coord = f?.geometry?.coordinates;
		const model = f?.properties?.model;
		if (!coord || !model?.StopUID) continue;
		const entry = { coord, name: model.StopName, uid: model.StopUID };
		byUid.set(model.StopUID, entry);
		const list = byName.get(model.StopName);
		if (list) list.push(entry);
		else byName.set(model.StopName, [entry]);
	}
	return { byUid, byName };
}

export async function getBusStopIndex() {
	if (cache) return cache;
	if (!inflight) {
		inflight = axios
			.get(GEOJSON_URL)
			.then((rs) => {
				cache = buildIndex(rs.data?.features || []);
				return cache;
			})
			.catch((e) => {
				inflight = null;
				console.error("getBusStopIndex failed", e);
				throw e;
			});
	}
	return inflight;
}

export function getBusStopIndexSync() {
	return cache;
}

// Pick the entry closest to `hintCoord` ([lon, lat]). When entries is a
// single-element list this is a no-op. Without a hint, returns the first.
export function pickClosest(entries, hintCoord) {
	if (!entries || entries.length === 0) return null;
	if (entries.length === 1 || !hintCoord) return entries[0];
	let best = entries[0];
	let bestD = squaredDistance(best.coord, hintCoord);
	for (let i = 1; i < entries.length; i++) {
		const d = squaredDistance(entries[i].coord, hintCoord);
		if (d < bestD) {
			best = entries[i];
			bestD = d;
		}
	}
	return best;
}

function squaredDistance(a, b) {
	const dx = a[0] - b[0];
	const dy = a[1] - b[1];
	return dx * dx + dy * dy;
}
