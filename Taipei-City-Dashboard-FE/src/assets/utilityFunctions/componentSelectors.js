import youbikeStationLabels from "../configs/selectors/youbikeStationLabels.json";

const selectorLabelRegistry = {
	youbike_station_labels: youbikeStationLabels,
};

export function hasSelectorConfig(component) {
	return Array.isArray(component?.selector_config?.selectors);
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
	if (!hasSelectorConfig(component)) return {};

	const selectorValues = { ...(component.selector_values || {}) };

	component.selector_config.selectors.forEach((selector) => {
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
	if (!hasSelectorConfig(component)) return {};

	const selectorValues = initializeComponentSelectors(component);
	return component.selector_config.selectors.reduce((params, selector) => {
		if (selectorValues[selector.key]) {
			params[selector.key] = selectorValues[selector.key];
		}
		return params;
	}, {});
}

function resetDependentSelectors(component, changedKey) {
	const selectors = component.selector_config?.selectors || [];

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
