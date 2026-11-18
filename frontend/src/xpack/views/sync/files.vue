<template>
    <div>
        <div v-if="nodes.length" class="commander-grid">
            <FilePanel
                ref="left"
                side="left"
                :nodes="nodes"
                :initNode="nodes[0].name"
                @send="(r) => sendRows(left, right, r)"
            />

            <div class="transfer-col">
                <el-button
                    type="primary"
                    class="xfer-btn"
                    :disabled="!leftCount"
                    @click="send(left, right)"
                >
                    <span>{{ $t('pro.sync.transfer') }}</span>
                    <span v-if="leftCount" class="xfer-count">({{ leftCount }})</span>
                    <span class="xfer-arrow">→</span>
                </el-button>
                <el-button
                    circle
                    size="small"
                    :icon="Switch"
                    class="swap-btn"
                    @click="swapPanes"
                />
                <el-button
                    type="primary"
                    plain
                    class="xfer-btn"
                    :disabled="!rightCount"
                    @click="send(right, left)"
                >
                    <span class="xfer-arrow">←</span>
                    <span>{{ $t('pro.sync.transfer') }}</span>
                    <span v-if="rightCount" class="xfer-count">({{ rightCount }})</span>
                </el-button>
            </div>

            <FilePanel
                ref="right"
                side="right"
                :nodes="nodes"
                :initNode="(nodes[1] || nodes[0]).name"
                @send="(r) => sendRows(right, left, r)"
            />
        </div>
        <el-card v-else shadow="never">
            <el-empty :image-size="60" :description="$t('pro.sync.noTarget')" />
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Switch } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { listAllNodes, NodeRow } from '@/xpack/api/node';
import { syncFiles } from '@/xpack/api/sync';
import FilePanel from './FilePanel.vue';

const emit = defineEmits<{ (e: 'created'): void }>();
const { t } = useI18n();
const nodes = ref<NodeRow[]>([]);
const left = ref<InstanceType<typeof FilePanel>>();
const right = ref<InstanceType<typeof FilePanel>>();

const leftCount = computed(() => left.value?.selected.length || 0);
const rightCount = computed(() => right.value?.selected.length || 0);

const swapPanes = () => {
    if (!left.value || !right.value) return;
    const tmp = left.value.node;
    left.value.node = right.value.node;
    right.value.node = tmp;
    left.value.goHome();
    right.value.goHome();
};

const sendRows = async (from?: InstanceType<typeof FilePanel>, to?: InstanceType<typeof FilePanel>, rows?: any[]) => {
    if (!from || !to || !rows?.length) return;
    if (from.node === to.node) {
        ElMessage.warning(t('pro.sync.sameNode'));
        return;
    }
    await ElMessageBox.confirm(t('pro.sync.confirm', { node: to.node, dir: to.path }), t('pro.sync.transfer'), {
        type: 'warning',
    });
    await syncFiles({
        src: from.node,
        dst: to.node,
        dstDir: to.path,
        items: rows.map((r) => ({ path: r.path, isDir: r.isDir })),
    });
    ElMessage.success(t('pro.sync.sent'));
    emit('created');
};
const send = (from?: InstanceType<typeof FilePanel>, to?: InstanceType<typeof FilePanel>) =>
    sendRows(from, to, from?.selected);

onMounted(async () => {
    const res = await listAllNodes();
    nodes.value = (res.data || []).filter((n) => n.kind !== 'panel' && n.status !== 'Pending');
});
</script>

<style scoped>
.commander-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    gap: 12px;
    align-items: stretch;
}
.transfer-col {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 8px 2px;
}
.xfer-btn {
    min-width: 108px;
    margin-left: 0 !important;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
}
.xfer-count {
    font-size: 12px;
    font-weight: 600;
}
.xfer-arrow {
    font-weight: 700;
    margin: 0 2px;
}
.swap-btn {
    margin-left: 0 !important;
}

@media (max-width: 1023px) {
    .commander-grid {
        grid-template-columns: minmax(0, 1fr);
    }
    .transfer-col {
        flex-direction: row;
        padding: 4px 0;
    }
}
</style>
