<script setup>
import { computed } from "vue";

const props = defineProps([
	"chart_config",
	"activeChart",
	"series",
	"map_config",
	"map_filter",
	"map_filter_on",
]);

const svgWidth = 920;
const svgHeight = 320;
const margin = {
	top: 22,
	right: 28,
	bottom: 48,
	left: 68,
};
const plotWidth = svgWidth - margin.left - margin.right;
const plotHeight = svgHeight - margin.top - margin.bottom;

const onTimeColor = computed(() => props.chart_config.color?.[0] || "#4CC9F0");
const earlyColor = computed(() => props.chart_config.color?.[1] || "#F72585");
const whiskerColor = computed(() => props.chart_config.color?.[2] || "#9CA3AF");
const unit = computed(() => props.chart_config.unit || "分鐘");

const boxRows = computed(() => {
	const source = props.series?.[0]?.data || [];
	return source
		.map((point) => {
			const values = Array.isArray(point.y)
				? point.y
				: [point.min, point.q1, point.median, point.q3, point.max];
			const [min, q1, median, q3, max] = values.map(Number);
			if (
				[min, q1, median, q3, max].some((value) => Number.isNaN(value))
			) {
				return null;
			}
			return {
				x: String(point.x ?? ""),
				min,
				q1,
				median,
				q3,
				max,
			};
		})
		.filter(Boolean);
});

const yDomain = computed(() => {
	if (boxRows.value.length === 0) return { min: -6, max: 12 };

	const minValue = Math.min(0, ...boxRows.value.map((item) => item.min));
	const maxValue = Math.max(0, ...boxRows.value.map((item) => item.max));
	const span = Math.max(maxValue - minValue, 1);
	const padding = Math.max(span * 0.12, 1);

	return {
		min: niceFloor(minValue - padding),
		max: niceCeil(maxValue + padding),
	};
});

const yTicks = computed(() => {
	const { min, max } = yDomain.value;
	const tickCount = 7;
	const step = niceStep((max - min) / (tickCount - 1));
	const first = Math.ceil(min / step) * step;
	const ticks = [];

	for (let value = first; value <= max + step * 0.5; value += step) {
		ticks.push(roundTick(value));
	}

	if (!ticks.includes(0)) {
		ticks.push(0);
		ticks.sort((a, b) => a - b);
	}

	return ticks;
});

const boxWidth = computed(() => {
	const count = Math.max(boxRows.value.length, 1);
	return Math.min(22, Math.max(10, plotWidth / count / 3.5));
});

function xScale(index) {
	if (boxRows.value.length <= 1) return margin.left + plotWidth / 2;
	return margin.left + (plotWidth / (boxRows.value.length - 1)) * index;
}

function yScale(value) {
	const { min, max } = yDomain.value;
	return margin.top + ((max - value) / (max - min)) * plotHeight;
}

function boxColor(row) {
	return row.median >= 0 ? onTimeColor.value : earlyColor.value;
}

function labelForTick(value) {
	if (value === 0) return "0";
	const prefix = value > 0 ? "+" : "";
	return `${prefix}${formatNumber(value)} ${unit.value}`;
}

function tooltipText(row) {
	return [
		`${row.x}`,
		`最大值 ${formatNumber(row.max)} ${unit.value}`,
		`Q3 ${formatNumber(row.q3)} ${unit.value}`,
		`中位數 ${formatNumber(row.median)} ${unit.value}`,
		`Q1 ${formatNumber(row.q1)} ${unit.value}`,
		`最小值 ${formatNumber(row.min)} ${unit.value}`,
	].join("\n");
}

function niceFloor(value) {
	return Math.floor(value / 2) * 2;
}

function niceCeil(value) {
	return Math.ceil(value / 2) * 2;
}

function niceStep(value) {
	const raw = Math.max(value, 1);
	const power = 10 ** Math.floor(Math.log10(raw));
	const normalized = raw / power;
	if (normalized <= 1) return power;
	if (normalized <= 2) return 2 * power;
	if (normalized <= 5) return 5 * power;
	return 10 * power;
}

function roundTick(value) {
	return Math.round(value * 10) / 10;
}

function formatNumber(value) {
	return Number.isInteger(value) ? String(value) : value.toFixed(1);
}
</script>

<template>
	<div v-if="activeChart === 'BoxPlotChart'" class="boxplotchart">
		<svg
			class="boxplotchart-svg"
			:viewBox="`0 0 ${svgWidth} ${svgHeight}`"
			role="img"
			aria-label="公車到站誤差盒鬚圖"
			preserveAspectRatio="xMidYMid meet"
		>
			<g class="boxplotchart-grid">
				<line
					v-for="tick in yTicks"
					:key="`grid-${tick}`"
					:x1="margin.left"
					:x2="svgWidth - margin.right"
					:y1="yScale(tick)"
					:y2="yScale(tick)"
					:class="{ 'boxplotchart-zero-line': tick === 0 }"
				/>
				<line
					v-for="(row, index) in boxRows"
					:key="`x-grid-${row.x}`"
					:x1="xScale(index)"
					:x2="xScale(index)"
					:y1="margin.top"
					:y2="svgHeight - margin.bottom"
				/>
			</g>

			<g class="boxplotchart-yaxis">
				<text
					v-for="tick in yTicks"
					:key="`tick-${tick}`"
					:x="margin.left - 12"
					:y="yScale(tick) + 4"
					text-anchor="end"
				>
					{{ labelForTick(tick) }}
				</text>
				<text
					class="boxplotchart-axis-title"
					:x="18"
					:y="margin.top + plotHeight / 2"
					text-anchor="middle"
					:transform="`rotate(-90 18 ${margin.top + plotHeight / 2})`"
				>
					到站誤差（正值為誤點，負值為提前）
				</text>
			</g>

			<g class="boxplotchart-boxes">
				<g
					v-for="(row, index) in boxRows"
					:key="row.x"
					class="boxplotchart-box"
				>
					<title>{{ tooltipText(row) }}</title>
					<line
						:x1="xScale(index)"
						:x2="xScale(index)"
						:y1="yScale(row.min)"
						:y2="yScale(row.max)"
					/>
					<line
						:x1="xScale(index) - boxWidth * 0.36"
						:x2="xScale(index) + boxWidth * 0.36"
						:y1="yScale(row.min)"
						:y2="yScale(row.min)"
					/>
					<line
						:x1="xScale(index) - boxWidth * 0.36"
						:x2="xScale(index) + boxWidth * 0.36"
						:y1="yScale(row.max)"
						:y2="yScale(row.max)"
					/>
					<rect
						:x="xScale(index) - boxWidth / 2"
						:y="Math.min(yScale(row.q1), yScale(row.q3))"
						:width="boxWidth"
						:height="
							Math.max(
								Math.abs(yScale(row.q1) - yScale(row.q3)),
								3,
							)
						"
						:fill="boxColor(row)"
					/>
					<line
						class="boxplotchart-median"
						:x1="xScale(index) - boxWidth / 2"
						:x2="xScale(index) + boxWidth / 2"
						:y1="yScale(row.median)"
						:y2="yScale(row.median)"
					/>
				</g>
			</g>

			<g class="boxplotchart-xaxis">
				<text
					v-for="(row, index) in boxRows"
					:key="`label-${row.x}`"
					:x="xScale(index)"
					:y="svgHeight - margin.bottom + 24"
					text-anchor="middle"
				>
					{{ row.x }}
				</text>
				<text
					class="boxplotchart-axis-title"
					:x="margin.left + plotWidth / 2"
					:y="svgHeight - 10"
					text-anchor="middle"
				>
					時間
				</text>
			</g>

			<text
				class="boxplotchart-late-label"
				:x="margin.left + 10"
				:y="margin.top + 18"
			>
				遲到 ↑
			</text>
			<text
				class="boxplotchart-early-label"
				:x="margin.left + 10"
				:y="svgHeight - margin.bottom - 14"
			>
				提前 ↓
			</text>
			<text
				class="boxplotchart-zero-label"
				:x="svgWidth - margin.right - 8"
				:y="yScale(0) - 8"
				text-anchor="end"
			>
				0 = 準時
			</text>
			<text
				class="boxplotchart-note"
				:x="svgWidth - margin.right - 8"
				:y="margin.top + 16"
				text-anchor="end"
			>
				每根 K 線代表一小時
			</text>
		</svg>
		<div v-if="boxRows.length === 0" class="boxplotchart-empty">
			查無資料
		</div>
	</div>
</template>

<style scoped lang="scss">
.boxplotchart {
	position: relative;
	width: 100%;
	height: 100%;
	min-height: 260px;
}

.boxplotchart-svg {
	display: block;
	width: 100%;
	height: 100%;
	min-height: 260px;
	overflow: visible;
	font-family: var(--font-family);
}

.boxplotchart-grid {
	line {
		stroke: rgba(255, 255, 255, 0.07);
		stroke-width: 1;
	}

	.boxplotchart-zero-line {
		stroke: rgba(255, 255, 255, 0.55);
		stroke-width: 1.2;
	}
}

.boxplotchart-yaxis,
.boxplotchart-xaxis {
	text {
		fill: var(--color-complement-text);
		font-size: 12px;
	}
}

.boxplotchart-axis-title {
	fill: var(--color-complement-text);
	font-size: 12px;
}

.boxplotchart-box {
	line {
		stroke: v-bind(whiskerColor);
		stroke-width: 1.4;
		stroke-linecap: round;
	}

	rect {
		opacity: 0.92;
		rx: 1;
	}

	.boxplotchart-median {
		stroke: rgba(255, 255, 255, 0.62);
		stroke-width: 1.3;
	}
}

.boxplotchart-late-label {
	fill: v-bind(onTimeColor);
	font-size: 12px;
	font-weight: 700;
}

.boxplotchart-early-label {
	fill: v-bind(earlyColor);
	font-size: 12px;
	font-weight: 700;
}

.boxplotchart-zero-label,
.boxplotchart-note {
	fill: var(--color-complement-text);
	font-size: 11px;
}

.boxplotchart-empty {
	position: absolute;
	inset: 0;
	display: grid;
	place-items: center;
	color: var(--color-complement-text);
	font-size: var(--font-s);
}
</style>
