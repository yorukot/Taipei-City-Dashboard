<!-- Developed by Taipei Urban Intelligence Center 2023-2024-->

<!-- Grafana-style time range picker for the dashboard settings bar. -->
<!-- Opens a popup anchored below the trigger button. -->
<!-- Supports quick relative presets and a custom absolute range via datetime inputs. -->

<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useContentStore } from "../../../store/contentStore";

const contentStore = useContentStore();

const isOpen = ref(false);
const triggerRef = ref(null);
const popupStyle = ref({});
const customFrom = ref("");
const customTo = ref("");

const presets = [
	{ label: "Last 5 minutes", minutes: 5 },
	{ label: "Last 15 minutes", minutes: 15 },
	{ label: "Last 30 minutes", minutes: 30 },
	{ label: "Last 1 hour", minutes: 60 },
	{ label: "Last 3 hours", minutes: 180 },
	{ label: "Last 6 hours", minutes: 360 },
	{ label: "Last 24 hours", minutes: 1440 },
	{ label: "Last 2 days", minutes: 2880 },
	{ label: "Last 7 days", minutes: 10080 },
	{ label: "Last 1 month", minutes: 43200 },
	{ label: "Last 1 year", minutes: 525600 },
];

// Converts a Date to the 'YYYY-MM-DDTHH:mm' string used by datetime-local inputs.
// Uses the same tzoffset trick as dataTimeframe.js to produce UTC+8 wall-clock time.
function dateToLocalInput(date) {
	const tzoffset = new Date().getTimezoneOffset() * 60000;
	return new Date(date - tzoffset).toISOString().slice(0, 16);
}

// Converts a datetime-local string to an ISO+08:00 string for the API.
function localInputToISO(value) {
	// value is 'YYYY-MM-DDTHH:mm' representing UTC+8 wall-clock time
	return value + ":00+08:00";
}

function openPopup() {
	if (!triggerRef.value) return;

	// Pre-fill custom inputs with the current effective range
	const range = contentStore.dashboardTimeRange;
	if (range.mode === "absolute" && range.from && range.to) {
		customFrom.value = range.from.slice(0, 16);
		customTo.value = range.to.slice(0, 16);
	} else {
		const now = new Date();
		const from = new Date(now.getTime() - range.minutes * 60000);
		customFrom.value = dateToLocalInput(from);
		customTo.value = dateToLocalInput(now);
	}

	// Position popup fixed below the trigger button, right-aligned
	const rect = triggerRef.value.getBoundingClientRect();
	popupStyle.value = {
		top: rect.bottom + 6 + "px",
		right: window.innerWidth - rect.right + "px",
	};

	isOpen.value = true;
}

function togglePopup() {
	if (isOpen.value) {
		isOpen.value = false;
	} else {
		openPopup();
	}
}

function selectPreset(preset) {
	contentStore.setDashboardTimeRange({
		mode: "relative",
		label: preset.label,
		minutes: preset.minutes,
		from: null,
		to: null,
	});
	isOpen.value = false;
	contentStore.setCurrentDashboardAllChartData();
}

function applyCustomRange() {
	if (!customFrom.value || !customTo.value) return;
	if (customFrom.value >= customTo.value) return;

	const fromISO = localInputToISO(customFrom.value);
	const toISO = localInputToISO(customTo.value);

	// Build a short display label
	const label =
		customFrom.value.replace("T", " ") +
		" → " +
		customTo.value.replace("T", " ");

	contentStore.setDashboardTimeRange({
		mode: "absolute",
		label,
		minutes: null,
		from: fromISO,
		to: toISO,
	});
	isOpen.value = false;
	contentStore.setCurrentDashboardAllChartData();
}

function handleClickOutside(event) {
	if (triggerRef.value && !triggerRef.value.contains(event.target)) {
		// Also check the teleported popup
		const popup = document.querySelector(".trp-popup");
		if (popup && popup.contains(event.target)) return;
		isOpen.value = false;
	}
}

onMounted(() => {
	document.addEventListener("click", handleClickOutside, true);
});

onUnmounted(() => {
	document.removeEventListener("click", handleClickOutside, true);
});
</script>

<template>
	<div ref="triggerRef" class="trp">
		<button class="trp-trigger" @click="togglePopup">
			<span class="trp-trigger-icon">schedule</span>
			<span class="trp-trigger-label">
				{{ contentStore.dashboardTimeRange.label }}
			</span>
			<span class="trp-trigger-arrow">
				{{ isOpen ? "arrow_drop_up" : "arrow_drop_down" }}
			</span>
		</button>
	</div>

	<Teleport to="body">
		<Transition name="trp-fade">
			<div v-if="isOpen" class="trp-popup" :style="popupStyle">
				<!-- Left: absolute custom range -->
				<div class="trp-popup-custom">
					<p class="trp-popup-section-title">自訂時間範圍</p>
					<label class="trp-popup-label" for="trp-from">開始</label>
					<input
						id="trp-from"
						v-model="customFrom"
						class="trp-popup-input"
						type="datetime-local"
					/>
					<label class="trp-popup-label" for="trp-to">結束</label>
					<input
						id="trp-to"
						v-model="customTo"
						class="trp-popup-input"
						type="datetime-local"
					/>
					<button
						class="trp-popup-apply"
						:disabled="
							!customFrom || !customTo || customFrom >= customTo
						"
						@click="applyCustomRange"
					>
						套用
					</button>
				</div>

				<!-- Divider -->
				<div class="trp-popup-divider" />

				<!-- Right: quick presets -->
				<div class="trp-popup-presets">
					<p class="trp-popup-section-title">快速選擇</p>
					<button
						v-for="preset in presets"
						:key="preset.label"
						class="trp-popup-preset"
						:class="{
							'trp-popup-preset--active':
								contentStore.dashboardTimeRange.mode ===
									'relative' &&
								contentStore.dashboardTimeRange.label ===
									preset.label,
						}"
						@click="selectPreset(preset)"
					>
						{{ preset.label }}
					</button>
				</div>
			</div>
		</Transition>
	</Teleport>
</template>

<style scoped lang="scss">
/* ── Trigger button ── */
.trp {
	display: flex;
	align-items: center;
}

.trp-trigger {
	display: flex;
	align-items: center;
	gap: 4px;
	padding: 3px 8px;
	border-radius: 5px;
	border: solid 1px var(--color-border);
	background-color: var(--color-component-background);
	cursor: pointer;
	transition:
		border-color 0.2s,
		background-color 0.2s;

	&:hover {
		border-color: var(--color-highlight);
		background-color: rgba(90, 156, 248, 0.08);
	}

	&-icon,
	&-arrow {
		font-family: var(--font-icon);
		font-size: calc(var(--font-m) * var(--font-to-icon));
		color: var(--color-complement-text);
		transition: color 0.2s;
	}

	&-label {
		font-size: var(--font-s);
		white-space: nowrap;
		max-width: 180px;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	&:hover &-icon,
	&:hover &-arrow,
	&:hover &-label {
		color: var(--color-highlight);
	}
}

/* ── Popup (teleported to body, position: fixed) ── */
.trp-popup {
	position: fixed;
	z-index: 200;
	display: flex;
	flex-direction: row;
	background-color: #1e1e1e;
	border: solid 1px var(--color-border);
	border-radius: 6px;
	box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
	overflow: visible;
}

/* ── Section: custom range (left) ── */
.trp-popup-custom {
	display: flex;
	flex-direction: column;
	gap: 6px;
	padding: 14px 16px;
	min-width: 200px;
}

.trp-popup-section-title {
	font-size: var(--font-s);
	color: var(--color-complement-text);
	text-transform: uppercase;
	letter-spacing: 0.05em;
	margin-bottom: 4px;
}

.trp-popup-label {
	font-size: var(--font-s);
	color: var(--color-complement-text);
}

.trp-popup-input {
	padding: 4px 6px;
	border-radius: 4px;
	border: solid 1px var(--color-border);
	background-color: transparent;
	font-size: var(--font-s);
	color: var(--color-normal-text);
	cursor: pointer;
	/* Ensure no extra overflow conflicts */
	overflow: visible;

	&:focus {
		outline: none;
		border-color: var(--color-highlight);
	}

	/* Style the calendar icon picker on WebKit */
	&::-webkit-calendar-picker-indicator {
		filter: invert(0.6);
		cursor: pointer;
	}
}

.trp-popup-apply {
	margin-top: 6px;
	padding: 5px 0;
	border-radius: 4px;
	background-color: var(--color-highlight);
	color: #fff;
	font-size: var(--font-s);
	font-weight: 600;
	cursor: pointer;
	transition: opacity 0.2s;

	&:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}

	&:not(:disabled):hover {
		opacity: 0.85;
	}
}

/* ── Divider ── */
.trp-popup-divider {
	width: 1px;
	background-color: var(--color-border);
	margin: 10px 0;
}

/* ── Section: quick presets (right) ── */
.trp-popup-presets {
	display: flex;
	flex-direction: column;
	padding: 14px 16px;
	min-width: 180px;
	gap: 1px;
}

.trp-popup-preset {
	display: flex;
	justify-content: space-between;
	align-items: center;
	width: 100%;
	text-align: left;
	padding: 5px 8px;
	border-radius: 4px;
	font-size: var(--font-s);
	color: var(--color-complement-text);
	cursor: pointer;
	transition:
		background-color 0.15s,
		color 0.15s;

	&:hover {
		background-color: rgba(90, 156, 248, 0.1);
		color: var(--color-normal-text);
	}

	&--active {
		color: var(--color-highlight);
		background-color: rgba(90, 156, 248, 0.12);

		&::after {
			content: " ◀";
			font-size: 0.6rem;
		}
	}
}

/* ── Transition ── */
.trp-fade-enter-from,
.trp-fade-leave-to {
	opacity: 0;
	transform: translateY(-6px);
}

.trp-fade-enter-active,
.trp-fade-leave-active {
	transition:
		opacity 0.18s ease,
		transform 0.18s ease;
}
</style>
