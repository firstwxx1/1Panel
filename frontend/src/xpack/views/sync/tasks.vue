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
                        :status="row.status === 'failed' ? 'exception' : row.status === 'success' ? 'success' : ''"
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.status')" width="90">
                <template #default="{ row }">{{ $t('pro.sync.' + row.status) }}</template>
            </el-table-column>
            <el-table-column prop="createdAt" :label="$t('pro.sync.createdAt')" width="190">
                <template #default="{ row }">{{ new Date(row.createdAt).toLocaleString() }}</template>
            </el-table-column>
        </el-table>
    </el-card>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { deleteSyncTasks, listSyncTasks, SyncTask } from '@/xpack/api/sync';

const rows = ref<SyncTask[]>([]);
const loading = ref(false);
let timer: number | undefined;

const load = async () => {
    const res = await listSyncTasks();
    rows.value = res.data || [];
};
const clean = async () => {
    await deleteSyncTasks(rows.value.filter((r) => r.status !== 'running').map((r) => r.id));
    load();
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
