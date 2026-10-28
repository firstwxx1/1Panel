<template>
    <div>
        <div class="actions">
            <el-button type="primary" :disabled="!left?.selected.length" @click="send(left, right)">
                {{ $t('pro.sync.transfer') }} →
            </el-button>
            <el-button type="primary" :disabled="!right?.selected.length" @click="send(right, left)">
                ← {{ $t('pro.sync.transfer') }}
            </el-button>
        </div>
        <div class="panels" v-if="nodes.length">
            <FilePanel ref="left" :nodes="nodes" :initNode="nodes[0].name" @send="(r) => sendRows(left, right, r)" />
            <FilePanel ref="right" :nodes="nodes" :initNode="(nodes[1] || nodes[0]).name" @send="(r) => sendRows(right, left, r)" />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { listAllNodes, NodeRow } from '@/xpack/api/node';
import { syncFiles } from '@/xpack/api/sync';
import i18n from '@/lang';
import FilePanel from './FilePanel.vue';

const emit = defineEmits<{ (e: 'created'): void }>();
const t = i18n.global.t;
const nodes = ref<NodeRow[]>([]);
const left = ref<InstanceType<typeof FilePanel>>();
const right = ref<InstanceType<typeof FilePanel>>();

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
.panels {
    display: flex;
    gap: 12px;
}
.actions {
    display: flex;
    justify-content: center;
    gap: 12px;
    margin-bottom: 10px;
}
</style>
