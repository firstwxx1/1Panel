<template>
    <el-card class="router_card mb-3">
        <div class="pro-tab-wrap">
            <el-radio-group :model-value="modelValue" @update:model-value="onSelect" class="pro-tab-group">
                <el-radio-button
                    v-for="tab in tabs"
                    :key="tab.key"
                    :value="tab.key"
                    :label="tab.key"
                    size="large"
                    class="router_card_button"
                >
                    <span class="pro-tab-label">
                        <span>{{ tab.label }}</span>
                        <span v-if="tab.count" class="pro-tab-badge">{{ tab.count }}</span>
                    </span>
                </el-radio-button>
            </el-radio-group>
            <div v-if="$slots.right" class="pro-tab-right">
                <slot name="right" />
            </div>
        </div>
    </el-card>
</template>

<script setup lang="ts">
export interface ProTabItem {
    key: string;
    label: string;
    count?: number;
}

defineProps<{
    modelValue: string;
    tabs: ProTabItem[];
}>();

const emit = defineEmits<{
    'update:modelValue': [val: string];
    change: [val: string];
}>();

const onSelect = (val: string | number | boolean | undefined) => {
    const key = String(val ?? '');
    emit('update:modelValue', key);
    emit('change', key);
};
</script>

<style lang="scss" scoped>
.router_card {
    --el-card-padding: 0;
    border-radius: 6px;

    :deep(.el-card__body) {
        padding: 4px 6px;
    }
}

.pro-tab-wrap {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
}

.pro-tab-group {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
}

.router_card_button {
    :deep(.el-radio-button__inner) {
        min-width: 96px;
        height: 100%;
        padding: 10px 18px;
        font-weight: 500;
        background-color: var(--panel-button-active) !important;
        box-shadow: none !important;
        outline: none !important;
        border: 2px solid transparent !important;
        border-radius: 4px !important;
        color: var(--el-text-color-regular) !important;
        transition: all 0.15s ease;
    }

    :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
        color: var(--panel-button-text-color) !important;
        background-color: var(--panel-button-bg-color) !important;
        border-color: var(--panel-color-primary) !important;
        border-radius: 4px !important;
        font-weight: 600;
    }
}

.pro-tab-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
}

.pro-tab-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    font-size: 11px;
    line-height: 1;
    border-radius: 9px;
    color: #fff;
    background: var(--el-color-danger);
}

.pro-tab-right {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-right: 6px;
}
</style>
