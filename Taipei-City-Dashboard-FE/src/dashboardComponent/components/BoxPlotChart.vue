<script setup>
import { computed, ref } from "vue";
import VueApexCharts from "vue3-apexcharts";

const props = defineProps([
	"chart_config",
	"activeChart",
	"series",
	"map_config",
	"map_filter",
	"map_filter_on",
]);

const boxColor = computed(
	() => props.chart_config.color?.[0] || "var(--color-highlight)",
);
const upperColor = computed(
	() => props.chart_config.color?.[1] || boxColor.value,
);

const boxSeries = computed(() => {
	const source = props.series?.[0]?.data || [];
	const data = source
		.map((point) => {
			const y = Array.isArray(point.y)
				? point.y
				: [point.min, point.q1, point.median, point.q3, point.max];
			if (!y || y.length < 5) return null;
			return { x: String(point.x ?? ""), y: y.map(Number) };
		})
		.filter(
			(point) => point && point.y.every((value) => !Number.isNaN(value)),
		);
	return [{ type: "boxPlot", data }];
});

const chartOptions = ref({
	chart: {
		type: "boxPlot",
		toolbar: { show: false },
		zoom: { allowMouseWheelZoom: false },
	},
	colors: [boxColor.value],
	dataLabels: { enabled: false },
	grid: { show: false },
	legend: { show: false },
	plotOptions: {
		boxPlot: {
			colors: {
				upper: upperColor.value,
				lower: boxColor.value,
			},
		},
	},
	stroke: {
		colors: ["#bfbfbf"],
		width: 1,
	},
	tooltip: {
		custom: function ({ seriesIndex, dataPointIndex, w }) {
			const point = w.config.series[seriesIndex].data[dataPointIndex];
			const [min, q1, median, q3, max] = point.y;
			const unit = props.chart_config.unit || "";
			return (
				'<div class="chart-tooltip">' +
				`<h6>${point.x}</h6>` +
				`<span>最大值 ${max} ${unit}</span><br/>` +
				`<span>Q3 ${q3} ${unit}</span><br/>` +
				`<span>中位數 ${median} ${unit}</span><br/>` +
				`<span>Q1 ${q1} ${unit}</span><br/>` +
				`<span>最小值 ${min} ${unit}</span>` +
				"</div>"
			);
		},
	},
	xaxis: {
		axisBorder: { show: false },
		axisTicks: { show: false },
		labels: {
			rotate: -30,
			rotateAlways: false,
			trim: true,
			style: { fontSize: "11px" },
		},
		type: "category",
	},
	yaxis: {
		labels: {
			formatter: (value) =>
				typeof value === "number" ? value.toFixed(1) : value,
		},
	},
});
</script>

<template>
	<div v-if="activeChart === 'BoxPlotChart'" class="boxplotchart">
		<VueApexCharts
			width="100%"
			height="280px"
			type="boxPlot"
			:options="chartOptions"
			:series="boxSeries"
		/>
	</div>
</template>

<style scoped lang="scss">
.boxplotchart {
	width: 100%;
	height: 100%;
}
</style>
