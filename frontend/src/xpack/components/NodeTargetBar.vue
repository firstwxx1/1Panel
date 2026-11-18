<template>
    <div v-if="showBar" class="node-target-bar mb-3">
        <div class="node-target-left">
            <span class="node-target-label">{{ $t('pro.node.targetNode') }}:</span>
            <el-select
                :model-value="modelValue || 'local'"
                size="small"
                filterable
                style="width: 240px"
                @change="onSelect"
            >
                <el-option
                    value="local"
                    :label="`${globalStore.getMasterAlias()} (local)`"
                />
                <el-option
                    v-for="n in childNodes"
                    :key="n.name"
                    :value="n.name"
                    :label="n.addr ? `${n.name} (${n.addr})` : n.name"
                    :disabled="n.status !== 'Healthy' || n.isBound === false"
                >
                    <div class="node-opt">
                        <span>{{ n.name }}</span>
                        <span class="node-opt-sub">
                            {{ n.addr }}
                            <el-tag
                                v-if="n.status !== 'Healthy'"
                                size="small"
                                type="danger"
                                effect="plain"
                                class="ml-1"
                            >
                                {{ $t('pro.node.status' + n.status) }}
                            </el-tag>
                        </span>
                    </div>
                </el-option>
            </el-select>
            <span class="node-target-tip">{{ $t('pro.node.targetNodeTip') }}</span>
        </div>
        <div class="node-target-extra">
            <slot name="extra" :nodes="healthyNodes" :child-nodes="healthyChildNodes" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { GlobalStore } from '@/store';
import { listAllNodes, NodeRow } from '@/xpack/api/node';

const props = withDefaults(
    defineProps<{
        modelValue?: string;
    }>(),
    {
        modelValue: 'local',
    },
);

const emit = defineEmits<{
    (e: 'update:modelValue', val: string): void;
    (e: 'change', val: string): void;
}>();

const globalStore = GlobalStore();
const allNodes = ref<NodeRow[]>([]);

const childNodes = computed(() => allNodes.value.filter((n) => !n.isLocal && n.kind === 'node'));
const healthyChildNodes = computed(() =>
    childNodes.value.filter((n) => n.isBound !== false && n.status === 'Healthy'),
);
const healthyNodes = computed(() =>
    allNodes.value.filter((n) => n.isLocal || (n.kind === 'node' && n.isBound !== false && n.status === 'Healthy')),
);

const showBar = computed(() => globalStore.currentNode === 'local' && childNodes.value.length > 0);

const onSelect = (val: string) => {
    emit('update:modelValue', val);
    emit('change', val);
};

const reloadNodes = async () => {
    if (globalStore.currentNode !== 'local') {
        allNodes.value = [];
        return;
    }
    try {
        const res = await listAllNodes();
        allNodes.value = res.data || [];
    } catch {
        allNodes.value = [];
    }
};

watch(
    () => globalStore.currentNode,
    (cur) => {
        if (cur !== 'local') {
            if (props.modelValue !== cur) {
                emit('update:modelValue', cur);
                emit('change', cur);
            }
        } else {
            reloadNodes();
            if (!props.modelValue) {
                emit('update:modelValue', 'local');
            }
        }
    },
    { immediate: true },
);

onMounted(() => {
    if (globalStore.currentNode === 'local') {
        reloadNodes();
    }
});

defineExpose({
    healthyNodes,
    healthyChildNodes,
    childNodes,
    reloadNodes,
});
</script>

<style scoped>
.node-target-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
    padding: 8px 14px;
    border-radius: 6px;
    border: 1px solid var(--el-border-color-lighter);
    background: var(--el-fill-color-blank);
}
.node-target-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
}
.node-target-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--el-text-color-primary);
}
.node-target-tip {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.node-target-extra {
    display: flex;
    align-items: center;
    gap: 8px;
}
.node-opt {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.node-opt-sub {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
