<template>
    <div v-if="props.type === 'ssl'">
        <el-form-item prop="pushNode">
            <el-checkbox
                :model-value="props.pushNode"
                :label="$t('pro.node.pushSsl')"
                @change="onPushNodeChange"
            />
        </el-form-item>
        <el-form-item v-if="props.pushNode" :label="$t('pro.node.pushSslNodes')" prop="pushNodes">
            <el-select
                :model-value="selectedNodes"
                multiple
                filterable
                clearable
                style="width: 100%"
                :loading="loading"
                :placeholder="$t('pro.node.selectNodes')"
                @update:model-value="onNodesChange"
            >
                <el-option
                    v-for="item in childNodes"
                    :key="item.name"
                    :label="item.addr ? `${item.name} (${item.addr})` : item.name"
                    :value="item.name"
                    :disabled="item.status !== 'Healthy' || item.isBound === false"
                >
                    <div class="ssl-node-opt">
                        <span>{{ item.name }}</span>
                        <span class="ssl-node-sub">
                            {{ item.addr }}
                            <el-tag
                                v-if="item.status !== 'Healthy'"
                                size="small"
                                type="danger"
                                effect="plain"
                                class="ml-1"
                            >
                                {{ $t('pro.node.status' + item.status) }}
                            </el-tag>
                        </span>
                    </div>
                </el-option>
            </el-select>
        </el-form-item>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { listAllNodes, NodeRow } from '@/xpack/api/node';

const props = withDefaults(
    defineProps<{
        pushNode?: boolean;
        nodes?: string[] | string;
        type?: string;
    }>(),
    {
        pushNode: false,
        nodes: () => [],
        type: 'ssl',
    },
);

const emit = defineEmits<{
    (e: 'update:pushNode', val: boolean): void;
    (e: 'update:nodes', val: any): void;
}>();

const loading = ref(false);
const childNodes = ref<NodeRow[]>([]);

const selectedNodes = computed<string[]>(() => {
    if (Array.isArray(props.nodes)) {
        return props.nodes;
    }
    if (typeof props.nodes === 'string' && props.nodes.trim()) {
        return props.nodes
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean);
    }
    return [];
});

const emitNodes = (vals: string[]) => {
    if (typeof props.nodes === 'string') {
        emit('update:nodes', vals.join(','));
    } else {
        emit('update:nodes', vals);
    }
};

const onPushNodeChange = (val: any) => {
    const enabled = Boolean(val);
    emit('update:pushNode', enabled);
    if (!enabled) {
        emitNodes([]);
    }
};

const onNodesChange = (vals: string[]) => {
    emitNodes(vals);
};

const loadNodes = async () => {
    if (props.type !== 'ssl') return;
    loading.value = true;
    try {
        const res = await listAllNodes();
        childNodes.value = (res.data || []).filter((n) => !n.isLocal && n.kind === 'node');
    } catch {
        childNodes.value = [];
    } finally {
        loading.value = false;
    }
};

onMounted(loadNodes);
</script>

<style scoped>
.ssl-node-opt {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.ssl-node-sub {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
