<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="filter-grid">
                <SiteSelect v-model="f.site" :sites="sites" all @update:model-value="search" />
                <el-date-picker
                    v-model="dates"
                    type="datetimerange"
                    :clearable="false"
                    class="dt-picker"
                    :start-placeholder="$t('pro.monitor.startTime')"
                    :end-placeholder="$t('pro.monitor.endTime')"
                    @change="search"
                />
                <el-select v-model="f.method" clearable style="width: 165px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.method') }}</template>
                    <el-option v-for="m in methods" :key="m" :value="m" :label="m" />
                </el-select>
                <el-select v-model="f.status" clearable style="width: 165px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.status') }}</template>
                    <el-option v-for="m in statuses" :key="m" :value="m" :label="m" />
                </el-select>
                <el-select v-model="f.spider" clearable style="width: 155px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.spider') }}</template>
                    <el-option value="yes" :label="$t('commons.true')" />
                    <el-option value="no" :label="$t('commons.false')" />
                </el-select>
                <el-input
                    v-model="f.url"
                    clearable
                    style="width: 220px"
                    :placeholder="$t('pro.monitor.inputUrl')"
                    @keyup.enter="search"
                    @clear="search"
                />
                <el-input
                    v-model="f.ip"
                    clearable
                    style="width: 180px"
                    :placeholder="$t('pro.monitor.inputIp')"
                    @keyup.enter="search"
                    @clear="search"
                />
                <el-input
                    v-model="f.region"
                    clearable
                    style="width: 200px"
                    :placeholder="$t('pro.monitor.inputRegion')"
                    @keyup.enter="search"
                    @clear="search"
                />
                <div class="filter-actions">
                    <el-button type="primary" :icon="Search" @click="search">
                        {{ $t('commons.button.search') }}
                    </el-button>
                    <el-button :icon="RefreshLeft" @click="resetFilters">
                        {{ $t('commons.button.reset') }}
                    </el-button>
                </div>
            </div>
        </el-card>

        <div class="kpi-strip mb-3">
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.req') }}</span>
                <span class="kpi-val is-primary">{{ fmtNum(stats.req) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.ok200') }}</span>
                <span class="kpi-val is-success">{{ fmtNum(stats.ok) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.errors') }}</span>
                <span class="kpi-val" :class="stats.err > 0 ? 'is-danger' : ''">{{ fmtNum(stats.err) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.ipCount') }}</span>
                <span class="kpi-val">{{ fmtNum(stats.ips) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.spiderReq') }}</span>
                <span class="kpi-val">{{ fmtNum(stats.spiders) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.avgRt') }}</span>
                <span class="kpi-val">{{ stats.avgRt > 0 ? Math.round(stats.avgRt * 1000) + ' ms' : '-' }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.totalBytes') }}</span>
                <span class="kpi-val">{{ fmtBytes(stats.bytes) }}</span>
            </div>
            <div class="kpi-item">
                <span class="kpi-label">{{ $t('pro.monitor.logSize') }}</span>
                <span class="kpi-val">{{ fmtBytes(logSize) }}</span>
            </div>
        </div>

        <el-card shadow="never">
            <div class="table-toolbar">
                <div class="toolbar-left">
                    <el-button type="danger" plain :icon="Delete" @click="clear">
                        {{ $t('pro.monitor.clearLog') }}
                    </el-button>
                    <el-button type="primary" plain :icon="Download" @click="exportCsv">
                        {{ $t('pro.monitor.exportLog') }}
                    </el-button>
                </div>
                <div class="toolbar-right">
                    <el-select v-model="refresh" style="width: 115px" @change="setTimer">
                        <el-option :value="0" :label="$t('pro.monitor.noRefresh')" />
                        <el-option :value="5" label="5s" />
                        <el-option :value="30" label="30s" />
                    </el-select>
                    <el-button :icon="Refresh" circle :loading="loading" @click="load" />
                </div>
            </div>

            <el-table :data="items" v-loading="loading" @sort-change="onSort">
                <el-table-column v-if="isMaster" prop="node" :label="$t('pro.monitor.node')" min-width="110" show-overflow-tooltip>
                    <template #default="{ row }">{{ row.node || '-' }}</template>
                </el-table-column>
                <el-table-column prop="ip" label="IP" min-width="135">
                    <template #default="{ row }">
                        <span class="mono">{{ row.ip }}</span>
                    </template>
                </el-table-column>
                <el-table-column prop="region" :label="$t('pro.monitor.ipRegion')" min-width="130" show-overflow-tooltip>
                    <template #default="{ row }">{{ row.region || '-' }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.monitor.domain')" min-width="150" show-overflow-tooltip>
                    <template #default="{ row }">{{ domainOf(row.site, row.node) }}</template>
                </el-table-column>
                <el-table-column prop="url" label="URL" min-width="240" show-overflow-tooltip>
                    <template #default="{ row }">
                        <span v-if="row.method" class="method-pill">{{ row.method }}</span>
                        <span class="mono">{{ row.url }}</span>
                    </template>
                </el-table-column>
                <el-table-column prop="status" :label="$t('pro.monitor.status')" width="95">
                    <template #default="{ row }">
                        <el-tag size="small" :type="statusTagType(row.status)" effect="light">
                            {{ row.status }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column prop="bytes" :label="$t('pro.monitor.respBytes')" sortable="custom" min-width="110">
                    <template #default="{ row }">{{ fmtBytes(row.bytes) }}</template>
                </el-table-column>
                <el-table-column prop="rt" :label="$t('pro.monitor.respTime')" sortable="custom" min-width="110">
                    <template #default="{ row }">{{ row.rt > 0 ? Math.round(row.rt * 1000) + ' ms' : '-' }}</template>
                </el-table-column>
                <el-table-column prop="time" :label="$t('pro.monitor.time')" min-width="165">
                    <template #default="{ row }">{{ new Date(row.time * 1000).toLocaleString() }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="90" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="detail = row">{{ $t('commons.button.view') }}</el-button>
                    </template>
                </el-table-column>
                <template #empty>
                    <el-empty :image-size="60" :description="$t('pro.monitor.noData')" />
                </template>
            </el-table>

            <el-pagination
                class="pager"
                v-model:current-page="page"
                v-model:page-size="size"
                :total="total"
                :page-sizes="[20, 50, 100]"
                layout="total, sizes, prev, pager, next, jumper"
                @change="load"
            />
        </el-card>

        <el-dialog :model-value="!!detail" :title="$t('pro.monitor.logDetail')" width="640px" @close="detail = null">
            <el-descriptions v-if="detail" :column="1" border>
                <el-descriptions-item label="IP">
                    <span class="mono">{{ detail.ip }}</span>
                    <span v-if="detail.region" class="ml-2 text-secondary">({{ detail.region }})</span>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.domain')">
                    {{ domainOf(detail.site, detail.node) }}
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.method')">
                    <span class="method-pill">{{ detail.method }}</span>
                </el-descriptions-item>
                <el-descriptions-item label="URL">
                    <span class="mono break-all">{{ detail.url }}</span>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.status')">
                    <el-tag size="small" :type="statusTagType(detail.status)" effect="light">{{ detail.status }}</el-tag>
                </el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.referer')">
                    <span class="mono break-all">{{ detail.referer || '-' }}</span>
                </el-descriptions-item>
                <el-descriptions-item label="User-Agent">
                    <span class="mono break-all">{{ detail.ua || '-' }}</span>
                </el-descriptions-item>
            </el-descriptions>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Delete, Download, Refresh, RefreshLeft, Search } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import SiteSelect from './SiteSelect.vue';
import {
    monClear,
    monLogsAgg,
    MonHitNode,
    MonLogReq,
    MonLogStats,
    MonSiteItem,
    splitSite,
} from '@/xpack/api/monitor';
import { isMasterView, nodeExec } from '@/xpack/api/aggregate';
import { fmtBytes, fmtNum, timeParams } from './util';

const props = defineProps<{ sites: MonSiteItem[] }>();
const { t } = useI18n();
const methods = ['GET', 'POST', 'PUT', 'DELETE', 'HEAD', 'OPTIONS'];
const statuses = ['2xx', '3xx', '4xx', '5xx', '200', '301', '302', '304', '403', '404', '500', '502'];
const f = reactive({ site: '', method: '', status: '', spider: '', url: '', ip: '', region: '' });

const defaultDayTimeRange = (): [Date, Date] => {
    const n = new Date();
    return [
        new Date(n.getFullYear(), n.getMonth(), n.getDate()),
        new Date(n.getFullYear(), n.getMonth(), n.getDate(), 23, 59, 59),
    ];
};

const dates = ref<Date[]>(defaultDayTimeRange());
const items = ref<MonHitNode[]>([]);
const stats = ref<MonLogStats>({ req: 0, ok: 0, err: 0, ips: 0, spiders: 0, avgRt: 0, bytes: 0 });
const logSize = ref(0);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const refresh = ref(0);
const detail = ref<MonHitNode | null>(null);
const sortBy = ref<'rt' | 'bytes' | undefined>(undefined);
const sortOrder = ref<'asc' | 'desc' | undefined>(undefined);
const isMaster = computed(() => isMasterView());
let timer: number | undefined;

const statusTagType = (s: number): 'success' | 'info' | 'warning' | 'danger' => {
    if (s >= 500) return 'danger';
    if (s >= 400) return 'warning';
    if (s >= 300) return 'info';
    if (s >= 200) return 'success';
    return 'info';
};

const domainOf = (alias: string, node?: string) => {
    if (node) {
        const exact = props.sites.find((x) => x.alias === `${node}::${alias}`);
        if (exact) return exact.domain || alias;
    }
    const s = props.sites.find((x) => splitSite(x.alias).alias === alias);
    return s?.domain || alias;
};

const req = (p: number, s: number): MonLogReq => ({
    ...f,
    ...timeParams(dates.value),
    ...(sortBy.value && sortOrder.value ? { orderBy: sortBy.value, order: sortOrder.value } : {}),
    page: p,
    size: s,
});

const load = async () => {
    loading.value = true;
    try {
        const d = await monLogsAgg(req(page.value, size.value));
        items.value = d.items || [];
        stats.value = d.stats;
        total.value = d.total;
        logSize.value = d.logSize;
    } finally {
        loading.value = false;
    }
};
const search = () => {
    page.value = 1;
    load();
};
const resetFilters = () => {
    f.site = '';
    f.method = '';
    f.status = '';
    f.spider = '';
    f.url = '';
    f.ip = '';
    f.region = '';
    dates.value = defaultDayTimeRange();
    search();
};
const onSort = (s: { prop: string; order: string | null }) => {
    if ((s.prop === 'rt' || s.prop === 'bytes') && s.order) {
        sortBy.value = s.prop;
        sortOrder.value = s.order === 'ascending' ? 'asc' : 'desc';
    } else {
        sortBy.value = undefined;
        sortOrder.value = undefined;
    }
    page.value = 1;
    load();
};
const setTimer = () => {
    timer && window.clearInterval(timer);
    timer = refresh.value ? window.setInterval(load, refresh.value * 1000) : undefined;
};
const clear = async () => {
    const { node, alias } = splitSite(f.site);
    if (isMaster.value && !node) {
        ElMessage.warning(t('pro.monitor.clearNeedSite'));
        return;
    }
    await ElMessageBox.confirm(t('pro.monitor.clearConfirm'), t('pro.monitor.clearLog'), { type: 'warning' });
    if (isMaster.value) {
        await nodeExec(node, '/xpack/monitor/clear', { site: alias });
    } else {
        await monClear(f.site);
    }
    ElMessage.success(t('pro.monitor.saved'));
    search();
};
const csv = (v: any) => '"' + String(v ?? '').replace(/"/g, '""') + '"';
const exportCsv = async () => {
    const res = await monLogsAgg(req(1, 5000));
    const head = ['time', 'node', 'ip', 'region', 'domain', 'method', 'url', 'status', 'bytes', 'rt', 'referer', 'ua'];
    const lines = (res.items || []).map((h) =>
        [
            new Date(h.time * 1000).toISOString(),
            h.node,
            h.ip,
            h.region,
            domainOf(h.site, h.node),
            h.method,
            h.url,
            h.status,
            h.bytes,
            h.rt,
            h.referer,
            h.ua,
        ]
            .map(csv)
            .join(','),
    );
    const blob = new Blob(['\ufeff' + head.join(',') + '\n' + lines.join('\n')], { type: 'text/csv;charset=utf-8' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'access-log.csv';
    a.click();
    URL.revokeObjectURL(a.href);
};
onMounted(load);
onBeforeUnmount(() => timer && window.clearInterval(timer));
</script>

<style scoped>
.filter-grid {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
}
.dt-picker {
    width: 360px;
    max-width: 100%;
}
.filter-actions {
    display: flex;
    align-items: center;
    gap: 8px;
}

.kpi-strip {
    display: grid;
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: 8px;
}
.kpi-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 10px 12px;
    border-radius: 6px;
    background: var(--el-bg-color-overlay);
    border: 1px solid var(--el-border-color-lighter);
}
.kpi-label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}
.kpi-val {
    font-size: 16px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}
.kpi-val.is-primary {
    color: var(--el-color-primary);
}
.kpi-val.is-success {
    color: var(--el-color-success);
}
.kpi-val.is-danger {
    color: var(--el-color-danger);
}

.table-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 12px;
}
.toolbar-left,
.toolbar-right {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
}

.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 12.5px;
}
.method-pill {
    display: inline-block;
    padding: 1px 6px;
    margin-right: 6px;
    border-radius: 3px;
    font-size: 11px;
    font-weight: 600;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    color: var(--el-text-color-regular);
    background: var(--el-fill-color-light);
}
.text-secondary {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.break-all {
    word-break: break-all;
}
.pager {
    margin-top: 14px;
    justify-content: flex-end;
}

@media (max-width: 1359px) {
    .kpi-strip {
        grid-template-columns: repeat(4, minmax(0, 1fr));
    }
}
@media (max-width: 640px) {
    .kpi-strip {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
</style>
