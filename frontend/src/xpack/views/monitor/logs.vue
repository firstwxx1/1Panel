<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="bar">
                <SiteSelect v-model="f.site" :sites="sites" all @update:model-value="search" />
                <el-date-picker
                    v-model="dates"
                    type="datetimerange"
                    :clearable="false"
                    style="width: 380px"
                    :start-placeholder="$t('pro.monitor.startTime')"
                    :end-placeholder="$t('pro.monitor.endTime')"
                    @change="search"
                />
                <el-select v-model="f.method" clearable style="width: 190px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.method') }}</template>
                    <el-option v-for="m in methods" :key="m" :value="m" :label="m" />
                </el-select>
                <el-select v-model="f.status" clearable style="width: 190px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.status') }}</template>
                    <el-option v-for="m in statuses" :key="m" :value="m" :label="m" />
                </el-select>
            </div>
            <div class="bar">
                <el-select v-model="f.spider" clearable style="width: 190px" @change="search">
                    <template #prefix>{{ $t('pro.monitor.spider') }}</template>
                    <el-option value="yes" :label="$t('commons.true')" />
                    <el-option value="no" :label="$t('commons.false')" />
                </el-select>
                <el-input v-model="f.url" clearable style="width: 240px" :placeholder="$t('pro.monitor.inputUrl')" @change="search" />
                <el-input v-model="f.ip" clearable style="width: 240px" :placeholder="$t('pro.monitor.inputIp')" @change="search" />
                <el-input v-model="f.region" clearable style="width: 240px" :placeholder="$t('pro.monitor.inputRegion')" @change="search" />
            </div>
        </el-card>
        <el-card shadow="never">
            <div class="bar">
                <el-button type="primary" plain @click="clear">{{ $t('pro.monitor.clearLog') }}</el-button>
                <el-button type="primary" plain @click="exportCsv">{{ $t('pro.monitor.exportLog') }}</el-button>
                <el-tag type="primary">{{ $t('pro.monitor.req') }}: {{ stats.req }}</el-tag>
                <el-tag type="success">200: {{ stats.ok }}</el-tag>
                <el-tag type="danger">{{ $t('pro.monitor.errors') }}: {{ stats.err }}</el-tag>
                <el-tag type="info">{{ $t('pro.monitor.ipCount') }}: {{ stats.ips }}</el-tag>
                <el-tag type="info">{{ $t('pro.monitor.spiderReq') }}: {{ stats.spiders }}</el-tag>
                <el-tag type="info">{{ $t('pro.monitor.avgRt') }}: {{ Math.round(stats.avgRt * 1000) }}ms</el-tag>
                <el-tag type="info">{{ $t('pro.monitor.totalBytes') }}: {{ fmtBytes(stats.bytes) }}</el-tag>
                <el-tag type="info">{{ $t('pro.monitor.logSize') }}: {{ fmtBytes(logSize) }}</el-tag>
            </div>
            <div class="bar">
                <el-button :icon="Refresh" @click="load" />
                <el-select v-model="refresh" style="width: 110px" @change="setTimer">
                    <el-option :value="0" :label="$t('pro.monitor.noRefresh')" />
                    <el-option :value="5" label="5s" />
                    <el-option :value="30" label="30s" />
                </el-select>
            </div>
            <el-table :data="items" v-loading="loading" @sort-change="onSort">
                <el-table-column prop="ip" label="IP" min-width="130" />
                <el-table-column prop="region" :label="$t('pro.monitor.ipRegion')" min-width="130" />
                <el-table-column :label="$t('pro.monitor.domain')" min-width="150">
                    <template #default="{ row }">{{ domainOf(row.site) }}</template>
                </el-table-column>
                <el-table-column prop="url" label="URL" min-width="220" show-overflow-tooltip />
                <el-table-column :label="$t('pro.monitor.respBytes')" min-width="110">
                    <template #default="{ row }">{{ fmtBytes(row.bytes) }}</template>
                </el-table-column>
                <el-table-column prop="rt" :label="$t('pro.monitor.respTime')" sortable="custom" min-width="110">
                    <template #default="{ row }">{{ row.rt > 0 ? Math.round(row.rt * 1000) + 'ms' : '-' }}</template>
                </el-table-column>
                <el-table-column prop="status" :label="$t('pro.monitor.status')" min-width="80" />
                <el-table-column prop="time" :label="$t('pro.monitor.time')" min-width="160">
                    <template #default="{ row }">{{ new Date(row.time * 1000).toLocaleString() }}</template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="90" fixed="right">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="detail = row">{{ $t('commons.button.view') }}</el-button>
                    </template>
                </el-table-column>
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
                <el-descriptions-item label="IP">{{ detail.ip }} {{ detail.region }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.domain')">{{ domainOf(detail.site) }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.method')">{{ detail.method }}</el-descriptions-item>
                <el-descriptions-item label="URL">{{ detail.url }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.status')">{{ detail.status }}</el-descriptions-item>
                <el-descriptions-item :label="$t('pro.monitor.referer')">{{ detail.referer }}</el-descriptions-item>
                <el-descriptions-item label="User-Agent">{{ detail.ua }}</el-descriptions-item>
            </el-descriptions>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import SiteSelect from './SiteSelect.vue';
import { monClear, monLogs, MonHit, MonLogStats, MonSiteItem } from '@/xpack/api/monitor';
import { fmtBytes, timeParams } from './util';
import i18n from '@/lang';

const props = defineProps<{ sites: MonSiteItem[] }>();
const t = i18n.global.t;
const methods = ['GET', 'POST', 'PUT', 'DELETE', 'HEAD', 'OPTIONS'];
const statuses = ['2xx', '3xx', '4xx', '5xx', '200', '301', '302', '304', '403', '404', '500', '502'];
const f = reactive({ site: '', method: '', status: '', spider: '', url: '', ip: '', region: '' });
const now = new Date();
const dates = ref<Date[]>([new Date(now.getFullYear(), now.getMonth(), now.getDate()), new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59)]);
const items = ref<MonHit[]>([]);
const stats = ref<MonLogStats>({ req: 0, ok: 0, err: 0, ips: 0, spiders: 0, avgRt: 0, bytes: 0 });
const logSize = ref(0);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const refresh = ref(0);
const detail = ref<MonHit | null>(null);
const sortRt = ref<string | null>(null);
let timer: number | undefined;

const domainOf = (alias: string) => props.sites.find((s) => s.alias === alias)?.domain || alias;
const req = (p: number, s: number) => ({ ...f, ...timeParams(dates.value), page: p, size: s });

const load = async () => {
    loading.value = true;
    try {
        const res = await monLogs(req(page.value, size.value));
        const d = res.data;
        items.value = d.items || [];
        if (sortRt.value) {
            const dir = sortRt.value === 'ascending' ? 1 : -1;
            items.value = [...items.value].sort((a, b) => (a.rt - b.rt) * dir);
        }
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
const onSort = (s: { order: string | null }) => {
    sortRt.value = s.order;
    load();
};
const setTimer = () => {
    timer && window.clearInterval(timer);
    timer = refresh.value ? window.setInterval(load, refresh.value * 1000) : undefined;
};
const clear = async () => {
    await ElMessageBox.confirm(t('pro.monitor.clearConfirm'), t('pro.monitor.clearLog'), { type: 'warning' });
    await monClear(f.site);
    ElMessage.success(t('pro.monitor.saved'));
    search();
};
const csv = (v: any) => '"' + String(v ?? '').replace(/"/g, '""') + '"';
const exportCsv = async () => {
    const res = await monLogs(req(1, 5000));
    const head = ['time', 'ip', 'region', 'domain', 'method', 'url', 'status', 'bytes', 'rt', 'referer', 'ua'];
    const lines = (res.data.items || []).map((h) =>
        [new Date(h.time * 1000).toISOString(), h.ip, h.region, domainOf(h.site), h.method, h.url, h.status, h.bytes, h.rt, h.referer, h.ua]
            .map(csv)
            .join(','),
    );
    const blob = new Blob(['﻿' + head.join(',') + '\n' + lines.join('\n')], { type: 'text/csv;charset=utf-8' });
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
.bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
}
.pager {
    margin-top: 12px;
    justify-content: flex-end;
}
</style>
