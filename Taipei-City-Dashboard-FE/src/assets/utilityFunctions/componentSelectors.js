import busRouteStopDirectionLabels from "../configs/selectors/busRouteStopDirectionLabels.json";
import newTaipeiStations from "../configs/selectors/newtaipei-stations.json";
import taipeiStations from "../configs/selectors/taipei-stations.json";

const selectorLabelRegistry = {
	bus_route_stop_direction_labels: busRouteStopDirectionLabels,
	youbike_station_labels: {
		taipei: toYoubikeStationOptions(taipeiStations),
		new_tpe: toYoubikeStationOptions(newTaipeiStations),
	},
};

const youbikeSelectorConfig = {
	selectors: [
		{
			key: "selector_1",
			label: "縣市",
			type: "select",
			default: "taipei",
			options: [
				{ label: "臺北市", value: "taipei" },
				{ label: "新北市", value: "new_tpe" },
			],
		},
		{
			key: "selector_2",
			label: "站點",
			type: "search-select",
			depends_on: "selector_1",
			label_key: "youbike_station_labels",
			default: {
				taipei: "TPE500101001",
				new_tpe: "NWT500201001",
			},
		},
	],
};

const selectorConfigFallbacks = {
	bus_top_delay_routes: {
		selectors: [
			{
				key: "selector_1",
				label: "站點",
				type: "search-select",
				label_key: "bus_route_stop_direction_labels",
			},
		],
	},
	youbike_availability_trend: youbikeSelectorConfig,
	youbike_probability_trend: youbikeSelectorConfig,
	train_station_reliability_trend: {
		selectors: [
			{
				key: "selector_1",
				label: "車站",
				type: "select",
				default: "1000",
				options: [
					{ label: "臺北", value: "1000" },
					{ label: "板橋", value: "1020" },
					{ label: "松山", value: "0990" },
					{ label: "花蓮", value: "7000" },
					{ label: "臺南", value: "4220" },
				],
			},
			{
				key: "selector_2",
				label: "車種",
				type: "select",
				default: "all",
				options: [
					{ label: "全部", value: "all" },
					{ label: "區間", value: "區間" },
					{ label: "對號", value: "對號" },
				],
			},
		],
	},
};

export function hasSelectorConfig(component) {
	return Array.isArray(getComponentSelectorConfig(component)?.selectors);
}

export function getComponentSelectorConfig(component) {
	const selectorConfig = normalizeSelectorConfig(component?.selector_config);

	if (Array.isArray(selectorConfig?.selectors)) {
		return selectorConfig;
	}

	return selectorConfigFallbacks[component?.index] || null;
}

export function getSelectorOptions(selector, selectorValues = {}) {
	if (!selector) return [];
	if (Array.isArray(selector.options)) return selector.options;

	const labelSource = selectorLabelRegistry[selector.label_key];
	if (!labelSource) return [];
	if (Array.isArray(labelSource)) return labelSource;

	if (selector.depends_on) {
		return labelSource[selectorValues[selector.depends_on]] || [];
	}

	return Object.values(labelSource).flat();
}

export function initializeComponentSelectors(component) {
	const selectorConfig = getComponentSelectorConfig(component);
	if (!Array.isArray(selectorConfig?.selectors)) return {};

	const selectorValues = { ...(component.selector_values || {}) };

	selectorConfig.selectors.forEach((selector) => {
		const options = getSelectorOptions(selector, selectorValues);
		const selectedValue = selectorValues[selector.key];
		const selectedExists =
			selectedValue &&
			(options.length === 0 ||
				options.some((option) => option.value === selectedValue));

		if (selectedExists) return;

		const defaultValue = getSelectorDefault(selector, selectorValues);
		const defaultExists = options.some(
			(option) => option.value === defaultValue,
		);

		selectorValues[selector.key] =
			defaultExists || options.length === 0
				? defaultValue
				: options[0]?.value || "";
	});

	component.selector_values = selectorValues;
	return selectorValues;
}

export function updateComponentSelectorValue(component, key, value) {
	const selectorValues = {
		...initializeComponentSelectors(component),
		[key]: value,
	};

	component.selector_values = selectorValues;
	resetDependentSelectors(component, key);

	return component.selector_values;
}

export function getComponentSelectorParams(component) {
	const selectorConfig = getComponentSelectorConfig(component);
	if (!Array.isArray(selectorConfig?.selectors)) return {};

	const selectorValues = initializeComponentSelectors(component);
	return selectorConfig.selectors.reduce((params, selector) => {
		if (selectorValues[selector.key]) {
			params[selector.key] = selectorValues[selector.key];
		}
		return params;
	}, {});
}

function resetDependentSelectors(component, changedKey) {
	const selectors = getComponentSelectorConfig(component)?.selectors || [];

	selectors
		.filter((selector) => selector.depends_on === changedKey)
		.forEach((selector) => {
			const selectorValues = component.selector_values || {};
			const options = getSelectorOptions(selector, selectorValues);
			const defaultValue = getSelectorDefault(selector, selectorValues);
			const defaultExists = options.some(
				(option) => option.value === defaultValue,
			);

			selectorValues[selector.key] =
				defaultExists || options.length === 0
					? defaultValue
					: options[0]?.value || "";

			component.selector_values = selectorValues;
			resetDependentSelectors(component, selector.key);
		});
}

function getSelectorDefault(selector, selectorValues) {
	if (!selector || selector.default === undefined) return "";
	if (
		selector.default &&
		typeof selector.default === "object" &&
		!Array.isArray(selector.default)
	) {
		const dependencyValue = selector.depends_on
			? selectorValues[selector.depends_on]
			: undefined;

		if (dependencyValue && selector.default[dependencyValue]) {
			return selector.default[dependencyValue];
		}

		return Object.values(selector.default)[0] || "";
	}

	return selector.default || "";
}

function normalizeSelectorConfig(selectorConfig) {
	if (typeof selectorConfig !== "string") return selectorConfig;
	if (!selectorConfig.trim()) return null;

	try {
		return JSON.parse(selectorConfig);
	} catch (err) {
		console.warn("Invalid selector_config JSON", err);
		return null;
	}
}

function toYoubikeStationOptions(stations) {
	return stations.map((station) => ({
		label:
			station.StationName?.Zh_tw ||
			station.StationName?.En ||
			station.StationID,
		value: station.StationUID,
	}));
}
