<template>
    <el-card shadow="never">
        <div class="toolbar">
            <div class="toolbar-left">
                <el-button :icon="Delete" :disabled="!hasFinished" @click="clean">
                    {{ $t('pro.sync.clean') }}
                </el-button>
            </div>
            <div class="toolbar-right">
                <el-button :icon="Refresh" circle :loading="loading" @click="load" />
            </div>
        </div>

        <el-table :data="rows" v-loading="loading" row-key="id">
            <el-table-column type="expand">
                <template #default="{ row }">
                    <div class="log-panel">
                        <div class="log-head">{{ $t('pro.sync.log') }}</div>
                        <pre class="log-pre">{{ (row.logs || []).join('\n') || row.message || $t('pro.sync.noLogs') }}</pre>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.kind')" width="115">
                <template #default="{ row }">
                    <el-tag size="small" type="info" effect="plain">
                        {{ $t('pro.sync.' + row.kind + 's') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="name" :label="$t('pro.sync.name')" min-width="200" show-overflow-tooltip>
                <template #default="{ row }">
                    <span class="mono">{{ row.name }}</span>
                </template>
            </el-table-column>
            <el-table-column prop="src" :label="$t('pro.sync.source')" width="135">
                <template #default="{ row }">
                    <span class="node-pill mono">{{ row.src }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.target')" min-width="150">
                <template #default="{ row }">
                    <div class="target-pills">
                        <span v-for="d in row.dsts || []" :key="d" class="node-pill mono">{{ d }}</span>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.progress')" width="170">
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
            <el-table-column :label="$t('pro.sync.status')" width="120">
                <template #default="{ row }">
                    <el-tag size="small" :type="statusTagType(row.status)" effect="light" class="status-tag">
                        <el-icon v-if="row.status === 'running'" class="is-loading"><Loading /></el-icon>
                        <el-icon v-else-if="row.status === 'success'"><CircleCheck /></el-icon>
                        <el-icon v-else-if="row.status === 'failed'"><CircleClose /></el-icon>
                        <el-icon v-else><Warning /></el-icon>
                        <span>{{ $t('pro.sync.' + row.status) }}</span>
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="createdAt" :label="$t('pro.sync.createdAt')" width="175">
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
            <template #empty>
                <el-empty :image-size="60" :description="$t('commons.msg.noneData')" />
            </template>
        </el-table>
    </el-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { CircleCheck, CircleClose, Delete, Loading, Refresh, Warning } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { cancelSyncTask, deleteSyncTasks, listSyncTasks, SyncTask } from '@/xpack/api/sync';

const { t } = useI18n();
const rows = ref<SyncTask[]>([]);
const loading = ref(false);
const cancelling = ref('');
let timer: number | undefined;

const hasFinished = computed(() => rows.value.some((r) => r.status !== 'running'));

const statusTagType = (s: SyncTask['status']): 'primary' | 'success' | 'danger' | 'warning' => {
    if (s === 'running') return 'primary';
    if (s === 'success') return 'success';
    if (s === 'failed') return 'danger';
    return 'warning';
};

const load = async () => {
    const res = await listSyncTasks();
    rows.value = res.data || [];
};
const clean = async () => {
    const ids = rows.value.filter((r) => r.status !== 'running').map((r) => r.id);
    if (!ids.length) return;
    await deleteSyncTasks(ids);
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
.toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 12px;
}
.status-tag :deep(.el-tag__content) {
    display: inline-flex;
    align-items: center;
    gap: 4px;
}
.target-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
}
.node-pill {
    display: inline-block;
    padding: 1px 7px;
    border-radius: 4px;
    font-size: 12px;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.log-panel {
    margin: 4px 16px 8px 48px;
    border-radius: 6px;
    border: 1px solid var(--el-border-color-lighter);
    background: var(--el-fill-color-darker);
    overflow: hidden;
}
.log-head {
    padding: 6px 12px;
    font-size: 12px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    border-bottom: 1px solid var(--el-border-color-lighter);
    background: var(--el-fill-color-light);
}
.log-pre {
    margin: 0;
    padding: 12px 14px;
    max-height: 260px;
    overflow-y: auto;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 12px;
    line-height: 1.55;
    color: var(--el-text-color-primary);
}
</style>
