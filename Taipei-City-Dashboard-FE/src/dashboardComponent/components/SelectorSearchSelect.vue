<script setup>
import { computed, ref, watch } from "vue";

const props = defineProps({
	modelValue: { type: String, default: "" },
	options: { type: Array, default: () => [] },
	placeholder: { type: String, default: "搜尋" },
	disabled: { type: Boolean, default: false },
});

const emit = defineEmits(["update:modelValue"]);

const isOpen = ref(false);
const searchText = ref("");

const selectedOption = computed(() =>
	props.options.find((option) => option.value === props.modelValue),
);

const filteredOptions = computed(() => {
	const keyword = searchText.value.trim().toLowerCase();

	if (!keyword) return props.options;

	return props.options.filter(
		(option) =>
			option.label?.toLowerCase().includes(keyword) ||
			option.value?.toLowerCase().includes(keyword),
	);
});

watch(
	() => [props.modelValue, props.options],
	() => {
		searchText.value = selectedOption.value?.label || "";
	},
	{ immediate: true, deep: true },
);

function updateSearch(event) {
	searchText.value = event.target.value;
	isOpen.value = true;
}

function chooseOption(option) {
	emit("update:modelValue", option.value);
	searchText.value = option.label;
	isOpen.value = false;
}

function closeOptions() {
	window.setTimeout(() => {
		searchText.value = selectedOption.value?.label || "";
		isOpen.value = false;
	}, 120);
}
</script>

<template>
  <div class="selector-search-select">
    <input
      :value="searchText"
      :placeholder="placeholder"
      :disabled="disabled"
      @focus="isOpen = true"
      @input="updateSearch"
      @blur="closeOptions"
    >
    <div
      v-if="isOpen && !disabled"
      class="selector-search-select-options"
    >
      <button
        v-for="option in filteredOptions"
        :key="option.value"
        type="button"
        :class="{
          'selector-search-select-option': true,
          'selector-search-select-option-active':
            option.value === modelValue,
        }"
        @mousedown.prevent="chooseOption(option)"
      >
        {{ option.label }}
      </button>
      <p v-if="filteredOptions.length === 0">
        查無站點
      </p>
    </div>
  </div>
</template>

<style scoped lang="scss">
.selector-search-select {
	position: relative;
	width: min(240px, 34vw);
	overflow: visible;

	input {
		width: 100%;
		box-sizing: border-box;
		padding: 4px 8px;
		border: 1px solid var(--color-border);
		border-radius: 5px;
		background-color: var(--color-component-background);
		color: var(--color-normal-text);
		font-size: var(--font-s);
	}

	&-options {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		z-index: 20;
		width: 100%;
		max-height: min(320px, 45vh);
		overflow-y: auto;
		border: 1px solid var(--color-border);
		border-radius: 5px;
		background-color: var(--color-component-background);
		box-shadow: 0 8px 18px rgba(0, 0, 0, 0.25);

		p {
			padding: 8px;
			color: var(--color-complement-text);
			font-size: var(--font-s);
		}
	}

	&-option {
		display: block;
		width: 100%;
		padding: 6px 8px;
		color: var(--color-normal-text);
		font-size: var(--font-s);
		text-align: left;
		white-space: nowrap;
		background-color: transparent;

		&:hover,
		&-active {
			background-color: rgba(255, 255, 255, 0.08);
			color: var(--color-complement-text);
		}
	}
}

@media (max-width: 760px) {
	.selector-search-select {
		width: min(220px, 52vw);
	}
}
</style>
