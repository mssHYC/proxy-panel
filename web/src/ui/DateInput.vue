<template>
  <VueDatePicker
    v-model="proxyValue"
    :format="format"
    :model-type="modelType"
    :enable-time-picker="enableTime"
    :time-picker-inline="true"
    :auto-apply="!enableTime"
    :clearable="clearable"
    :placeholder="placeholder"
    :disabled="disabled"
    :range="range"
    :preview-format="previewFormat"
    :menu-class-name="'dp-menu'"
    :input-class-name="['dp-input', { 'dp-input--invalid': invalid }] as any"
    :dark="false"
    teleport="body"
    :z-index="300"
    :format-locale="zhCN"
    :locale="zhCN"
    cancel-text="取消"
    select-text="选择"
    now-button-label="今天"
    week-num-name="周"
    month-name-format="short"
    :day-names="['一', '二', '三', '四', '五', '六', '日']"
  />
</template>

<script setup lang="ts">
import { computed } from "vue";
import { VueDatePicker } from "@vuepic/vue-datepicker";
import { zhCN } from "date-fns/locale/zh-CN";
import "@vuepic/vue-datepicker/dist/main.css";

const props = withDefaults(
  defineProps<{
    modelValue?: string | string[] | Date | Date[] | null;
    format?: string;
    modelType?: string;
    previewFormat?: string;
    placeholder?: string;
    disabled?: boolean;
    invalid?: boolean;
    enableTime?: boolean;
    range?: boolean;
    clearable?: boolean;
  }>(),
  {
    format: "yyyy-MM-dd",
    modelType: "yyyy-MM-dd",
    enableTime: false,
    range: false,
    clearable: true,
  },
);

const emit = defineEmits<{ "update:modelValue": [v: any] }>();

const proxyValue = computed({
  get: () => props.modelValue,
  set: (v: any) => emit("update:modelValue", v ?? null),
});
</script>

<style>
/* Restyle vuepic to match our tokens */
.dp__theme_light {
  --dp-background-color: var(--color-surface-raised);
  --dp-text-color: var(--color-ink-strong);
  --dp-hover-color: var(--color-surface-sunken);
  --dp-hover-text-color: var(--color-ink-strong);
  --dp-hover-icon-color: var(--color-accent);
  --dp-primary-color: var(--color-accent);
  --dp-primary-text-color: white;
  --dp-secondary-color: var(--color-ink-muted);
  --dp-border-color: var(--color-ink-faint);
  --dp-menu-border-color: var(--color-ink-faint);
  --dp-border-color-hover: var(--color-ink-soft);
  --dp-disabled-color: var(--color-ink-soft);
  --dp-scroll-bar-background: var(--color-surface-sunken);
  --dp-scroll-bar-color: var(--color-ink-soft);
  --dp-success-color: var(--color-status-ok);
  --dp-success-color-disabled: var(--color-status-ok-soft);
  --dp-icon-color: var(--color-ink-muted);
  --dp-danger-color: var(--color-status-crit);
  --dp-marker-color: var(--color-accent);
  --dp-tooltip-color: var(--color-surface-sunken);
  --dp-disabled-color-text: var(--color-ink-soft);
  --dp-highlight-color: oklch(0.48 0.13 28 / 0.18);
  --dp-range-between-dates-background-color: var(--color-accent-soft);
  --dp-range-between-dates-text-color: var(--color-accent-ink);
  --dp-range-between-border-color: transparent;

  --dp-font-family: var(--font-sans);
  --dp-border-radius: 6px;
  --dp-cell-border-radius: 4px;
  --dp-button-height: 32px;
  --dp-month-year-row-height: 36px;
  --dp-cell-size: 32px;
  --dp-input-padding: 0 12px;
  --dp-input-icon-padding: 36px;
  --dp-font-size: 14px;
}
.dp__input {
  height: 36px;
  border: 1px solid var(--color-ink-faint);
  border-radius: 6px;
  font-family: var(--font-sans);
  color: var(--color-ink-strong);
}
.dp__input:focus {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px oklch(0.48 0.13 28 / 0.18);
}
.dp__input::placeholder {
  color: var(--color-ink-soft);
}
.dp__input_icon {
  color: var(--color-ink-muted);
}
.dp-input--invalid .dp__input {
  border-color: var(--color-status-crit);
}

.dp__outer_menu_wrap,
.dp-menu {
  font-family: var(--font-sans);
  box-shadow: var(--shadow-raised) !important;
  border-radius: 10px !important;
  z-index: 300 !important;
}
.dp__menu {
  border: 1px solid var(--color-ink-faint);
}
.dp__calendar_header_item,
.dp__calendar_item {
  font-family: var(--font-mono);
  font-size: 12px;
}
.dp__today {
  border-color: var(--color-accent);
}
.dp__active_date,
.dp__range_start,
.dp__range_end {
  background: var(--color-accent);
  color: white;
}
.dp__action_button {
  font-family: var(--font-sans);
  font-size: 13px;
}
.dp__action_select {
  background: var(--color-accent);
  border-color: var(--color-accent);
}
.dp__action_select:hover {
  background: var(--color-accent-ink);
}
</style>
