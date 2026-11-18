<template>
    <el-card shadow="never">
        <div class="bar">
            <el-button :icon="Refresh" @click="load" />
            <el-button @click="clean">{{ $t('pro.sync.clean') }}</el-button>
        </div>
        <el-table :data="rows" v-loading="loading" row-key="id">
            <el-table-column type="expand">
                <template #default="{ row }">
                    <pre class="log">{{ (row.logs || []).join('\n') || row.message }}</pre>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.kind')" width="110">
                <template #default="{ row }">{{ $t('pro.sync.' + row.kind + 's') }}</template>
            </el-table-column>
            <el-table-column prop="name" :label="$t('pro.sync.name')" min-width="200" show-overflow-tooltip />
            <el-table-column prop="src" :label="$t('pro.sync.source')" width="130" />
            <el-table-column :label="$t('pro.sync.target')" min-width="140">
                <template #default="{ row }">{{ (row.dsts || []).join(', ') }}</template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.progress')" width="160">
                <template #default="{ row }">
                    <el-progress
                        :percentage="row.progress"
                        :status="
                            row.status === 'failed'
                                ? 'exception'
                                : row.status === 'cancelled'
                                  ? 'warning'
                                  : row.status === 'success'
                                    ? 'success'
                                    : ''
                        "
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.status')" width="100">
                <template #default="{ row }">{{ $t('pro.sync.' + row.status) }}</template>
            </el-table-column>
            <el-table-column prop="createdAt" :label="$t('pro.sync.createdAt')" width="180">
                <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.operation')" width="100" fixed="right">
                <template #default="{ row }">
                    <el-button
                        v-if="row.status === 'running'"
                        link
                        type="danger"
                        :loading="cancelling === row.id"
                        @click="cancel(row)"
                    >
                        {{ $t('pro.sync.cancel') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>
    </el-card>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { cancelSyncTask, deleteSyncTasks, listSyncTasks, SyncTask } from '@/xpack/api/sync';
import i18n from '@/lang';

const t = i18n.global.t;
const rows = ref<SyncTask[]>([]);
const loading = ref(false);
const cancelling = ref('');
let timer: number | undefined;

const load = async () => {
    const res = await listSyncTasks();
    rows.value = res.data || [];
};
const clean = async () => {
    await deleteSyncTasks(rows.value.filter((r) => r.status !== 'running').map((r) => r.id));
    load();
};
const cancel = async (row: SyncTask) => {
    await ElMessageBox.confirm(t('pro.sync.cancelConfirm'), t('pro.sync.cancel'), { type: 'warning' });
    cancelling.value = row.id;
    try {
        await cancelSyncTask(row.id);
        ElMessage.success(t('pro.sync.cancelOk'));
        await load();
    } finally {
        cancelling.value = '';
    }
};
onMounted(async () => {
    loading.value = true;
    try {
        await load();
    } finally {
        loading.value = false;
    }
    timer = window.setInterval(load, 3000);
});
onBeforeUnmount(() => timer && window.clearInterval(timer));
</script>

<style scoped>
.bar {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
}
.log {
    margin: 0 0 0 48px;
    white-space: pre-wrap;
    font-size: 12px;
}
</style>
