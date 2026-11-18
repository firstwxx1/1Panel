<template>
    <div v-if="showBar" class="node-target-bar mb-3">
        <div class="node-target-left">
            <div class="node-target-title">
                <el-icon class="node-target-icon"><Monitor /></el-icon>
                <span class="node-target-label">{{ $t('pro.node.targetNode') }}</span>
            </div>
            <el-select
                :model-value="currentVal"
                size="small"
                filterable
                class="node-target-select"
                @change="onSelect"
            >
                <el-option value="local" :label="`${globalStore.getMasterAlias()} (local)`">
                    <div class="node-opt">
                        <span class="node-opt-name">{{ globalStore.getMasterAlias() }}</span>
                        <el-tag size="small" type="primary" effect="plain">{{ $t('pro.node.master') }}</el-tag>
                    </div>
                </el-option>
                <el-option
                    v-for="n in childNodes"
                    :key="n.name"
                    :value="n.name"
                    :label="n.addr ? `${n.name} (${n.addr})` : n.name"
                    :disabled="n.status !== 'Healthy' || n.isBound === false"
                >
                    <div class="node-opt">
                        <span class="node-opt-name">{{ n.name }}</span>
                        <span class="node-opt-sub">
                            <span class="mono">{{ n.addr }}</span>
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
            <el-tag
                size="small"
                :type="isSelectedLocal ? 'primary' : 'success'"
                effect="light"
                class="node-role-tag"
            >
                {{ isSelectedLocal ? $t('pro.node.master') : $t('pro.node.childNode') }}
            </el-tag>
            <span v-if="selectedNodeAddr" class="node-target-addr mono">{{ selectedNodeAddr }}</span>
            <span class="node-target-tip">{{ $t('pro.node.targetNodeTip') }}</span>
        </div>
        <div v-if="$slots.extra" class="node-target-extra">
            <slot name="extra" :nodes="healthyNodes" :child-nodes="healthyChildNodes" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { Monitor } from '@element-plus/icons-vue';
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

const currentVal = computed(() => props.modelValue || 'local');
const isSelectedLocal = computed(() => currentVal.value === 'local');

const childNodes = computed(() => allNodes.value.filter((n) => !n.isLocal && n.kind === 'node'));
const healthyChildNodes = computed(() =>
    childNodes.value.filter((n) => n.isBound !== false && n.status === 'Healthy'),
);
const healthyNodes = computed(() =>
    allNodes.value.filter((n) => n.isLocal || (n.kind === 'node' && n.isBound !== false && n.status === 'Healthy')),
);
const selectedNodeAddr = computed(() => {
    if (isSelectedLocal.value) return '';
    return childNodes.value.find((n) => n.name === currentVal.value)?.addr || '';
});

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
    border-left: 3px solid var(--el-color-primary);
    background: var(--el-bg-color-overlay);
}
.node-target-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
}
.node-target-title {
    display: inline-flex;
    align-items: center;
    gap: 6px;
}
.node-target-icon {
    font-size: 15px;
    color: var(--el-color-primary);
}
.node-target-label {
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary);
}
.node-target-select {
    width: 230px;
}
.node-role-tag {
    font-weight: 500;
}
.node-target-addr {
    font-size: 12px;
    padding: 2px 6px;
    border-radius: 4px;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
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
.node-opt-name {
    font-weight: 500;
}
.node-opt-sub {
    display: inline-flex;
    align-items: center;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
