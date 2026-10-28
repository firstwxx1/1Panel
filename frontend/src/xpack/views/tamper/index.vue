<template>
    <div>
        <el-tabs v-model="active" type="card" class="mb-3" @tab-change="onTab">
            <el-tab-pane v-for="k in keys" :key="k" :label="$t('pro.tamper.' + k)" :name="k" />
        </el-tabs>

        <div v-if="active === 'sites'">
            <el-card shadow="never" class="mb-3">
                <div class="desc">{{ $t('pro.tamper.desc') }}</div>
                <div class="bar">
                    <el-switch v-model="enabled" :active-text="$t('pro.tamper.globalSwitch')" />
                    <span>{{ $t('pro.tamper.interval') }}</span>
                    <el-input-number v-model="interval" :min="10" :max="3600" size="small" />
                    <el-button type="primary" size="small" @click="saveGlobal">{{ $t('pro.tamper.saveGlobal') }}</el-button>
                </div>
            </el-card>
            <el-table :data="sites" v-loading="loading">
                <el-table-column :label="$t('pro.tamper.site')" min-width="180">
                    <template #default="{ row }">{{ row.domain || row.alias }}</template>
                </el-table-column>
                <el-table-column prop="root" :label="$t('pro.tamper.root')" min-width="260" show-overflow-tooltip />
                <el-table-column :label="$t('pro.tamper.protect')" width="90">
                    <template #default="{ row }">
                        <el-switch v-model="row.enabled" :loading="busy === row.alias" @change="saveSite(row)" />
                    </template>
                </el-table-column>
                <el-table-column width="120">
                    <template #header>
                        <el-tooltip :content="$t('pro.tamper.restoreTip')">
                            <span>{{ $t('pro.tamper.restore') }}</span>
                        </el-tooltip>
                    </template>
                    <template #default="{ row }">
                        <el-switch v-model="row.restore" :disabled="!row.enabled" @change="saveSite(row)" />
                    </template>
                </el-table-column>
                <el-table-column prop="files" :label="$t('pro.tamper.files')" width="90" />
                <el-table-column :label="$t('pro.tamper.baselineAt')" width="170">
                    <template #default="{ row }">{{ row.baselineAt || $t('pro.tamper.noBaseline') }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.tamper.pending')" width="90">
                    <template #default="{ row }">
                        <el-tag v-if="row.pending" type="danger">{{ row.pending }}</el-tag>
                        <span v-else>0</span>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.tamper.operation')" width="300" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openExcludes(row)">{{ $t('pro.tamper.excludes') }}</el-button>
                        <el-button link type="primary" :disabled="!row.enabled" @click="rebuild(row)">
                            {{ $t('pro.tamper.rebuild') }}
                        </el-button>
                        <el-button link type="primary" :disabled="!row.enabled" @click="scan(row)">
                            {{ $t('pro.tamper.scan') }}
                        </el-button>
                    </template>
                </el-table-column>
            </el-table>
        </div>

        <div v-if="active === 'events'">
            <div class="bar mb-3">
                <el-select v-model="evSite" clearable style="width: 220px" :placeholder="$t('pro.tamper.all')" @change="loadEvents(1)">
                    <el-option v-for="s in sites" :key="s.alias" :label="s.domain || s.alias" :value="s.alias" />
                </el-select>
                <el-button @click="loadEvents(page)">{{ $t('pro.tamper.refresh') }}</el-button>
                <el-button @click="clearEvents">{{ $t('pro.tamper.clear') }}</el-button>
            </div>
            <el-table :data="events" v-loading="loading">
                <el-table-column :label="$t('pro.tamper.time')" width="180">
                    <template #default="{ row }">{{ new Date(row.time).toLocaleString() }}</template>
                </el-table-column>
                <el-table-column prop="site" :label="$t('pro.tamper.site')" width="160" />
                <el-table-column :label="$t('pro.tamper.kind')" width="110">
                    <template #default="{ row }">{{ $t('pro.tamper.k_' + row.kind) }}</template>
                </el-table-column>
                <el-table-column prop="path" :label="$t('pro.tamper.path')" min-width="300" show-overflow-tooltip />
                <el-table-column :label="$t('pro.tamper.action')" width="120">
                    <template #default="{ row }">
                        <el-tag :type="row.action === 'none' || row.action === 'restore_failed' ? 'danger' : 'success'">
                            {{ $t('pro.tamper.a_' + row.action) }}
                        </el-tag>
                    </template>
                </el-table-column>
            </el-table>
            <el-pagination
                class="pager"
                v-model:current-page="page"
                :page-size="20"
                :total="total"
                layout="total, prev, pager, next"
                @current-change="loadEvents"
            />
        </div>

        <el-dialog v-model="exVisible" :title="$t('pro.tamper.excludes')" width="480px">
            <div class="desc mb-2">{{ $t('pro.tamper.excludesTip') }}</div>
            <el-input v-model="exText" type="textarea" :rows="8" />
            <template #footer>
                <el-button @click="exVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="saveExcludes">{{ $t('pro.tamper.save') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import {
    TamperEvent,
    TamperSite,
    tamperClearEvents,
    tamperEvents,
    tamperGetSettings,
    tamperRebuild,
    tamperSaveGlobal,
    tamperSaveSite,
    tamperScan,
} from '@/xpack/api/tamper';
import i18n from '@/lang';

const t = i18n.global.t;
const keys = ['sites', 'events'];
const active = ref('sites');
const loading = ref(false);
const busy = ref('');
const enabled = ref(false);
const interval = ref(60);
const sites = ref<TamperSite[]>([]);
const events = ref<TamperEvent[]>([]);
const evSite = ref('');
const page = ref(1);
const total = ref(0);
const exVisible = ref(false);
const exText = ref('');
let exRow: TamperSite | null = null;

const load = async () => {
    loading.value = true;
    try {
        const res = (await tamperGetSettings()).data;
        enabled.value = res.enabled;
        interval.value = res.interval;
        sites.value = res.sites || [];
    } finally {
        loading.value = false;
    }
};
const loadEvents = async (p: number) => {
    page.value = p || 1;
    loading.value = true;
    try {
        const res = (await tamperEvents({ site: evSite.value || '', page: page.value, pageSize: 20 })).data;
        events.value = res.items || [];
        total.value = res.total;
    } finally {
        loading.value = false;
    }
};
const onTab = (k: any) => (k === 'events' ? loadEvents(1) : load());
const saveGlobal = async () => {
    await tamperSaveGlobal({ enabled: enabled.value, interval: interval.value });
    ElMessage.success(t('pro.tamper.saved'));
};
const saveSite = async (row: TamperSite) => {
    busy.value = row.alias;
    try {
        await tamperSaveSite({ alias: row.alias, enabled: row.enabled, restore: row.restore, excludes: row.excludes });
        await load();
    } catch {
        await load();
    } finally {
        busy.value = '';
    }
};
const openExcludes = (row: TamperSite) => {
    exRow = row;
    exText.value = (row.excludes || []).join('\n');
    exVisible.value = true;
};
const saveExcludes = async () => {
    if (!exRow) return;
    exRow.excludes = exText.value.split('\n');
    await saveSite(exRow);
    exVisible.value = false;
};
const rebuild = async (row: TamperSite) => {
    await ElMessageBox.confirm(t('pro.tamper.rebuildConfirm'), t('pro.tamper.rebuild'), { type: 'warning' });
    const res = await tamperRebuild(row.alias);
    ElMessage.success(t('pro.tamper.rebuilt', { n: res.data.files }));
    load();
};
const scan = async (row: TamperSite) => {
    const res = await tamperScan(row.alias);
    ElMessage.success(t('pro.tamper.scanResult', { n: res.data.found }));
    load();
};
const clearEvents = async () => {
    await ElMessageBox.confirm(t('pro.tamper.clear') + '?', { type: 'warning' });
    await tamperClearEvents();
    loadEvents(1);
};

onMounted(load);
</script>

<style scoped>
.desc {
    color: var(--el-text-color-secondary);
    font-size: 13px;
    line-height: 1.6;
}
.bar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 12px;
}
.pager {
    margin-top: 10px;
    justify-content: flex-end;
}
</style>
