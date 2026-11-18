<template>
    <div>
        <el-card shadow="never">
            <div class="toolbar">
                <div class="toolbar-left">
                    <el-radio-group v-model="preset" @change="onPreset">
                        <el-radio-button value="today">{{ $t('pro.monitor.today') }}</el-radio-button>
                        <el-radio-button value="yesterday">{{ $t('pro.monitor.yesterday') }}</el-radio-button>
                        <el-radio-button value="7d">{{ $t('pro.monitor.d7') }}</el-radio-button>
                        <el-radio-button value="30d">{{ $t('pro.monitor.d30') }}</el-radio-button>
                    </el-radio-group>
                    <el-date-picker
                        v-model="dates"
                        type="daterange"
                        class="date-picker"
                        :clearable="false"
                        :start-placeholder="$t('pro.monitor.startDate')"
                        :end-placeholder="$t('pro.monitor.endDate')"
                        @change="onDateChange"
                    />
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

            <el-table :data="pageRows" v-loading="loading" @sort-change="onSort">
                <el-table-column v-if="isMaster" prop="node" :label="$t('pro.monitor.node')" sortable="custom" min-width="120">
                    <template #default="{ row }">{{ row.node || '-' }}</template>
                </el-table-column>
                <el-table-column prop="name" :label="$t('pro.monitor.website')" sortable="custom" min-width="170" show-overflow-tooltip>
                    <template #default="{ row }">
                        <span class="site-name">{{ row.name }}</span>
                    </template>
                </el-table-column>
                <el-table-column
                    v-for="c in cols"
                    :key="c.prop"
                    :prop="c.prop"
                    :label="$t('pro.monitor.' + c.labelKey)"
                    sortable="custom"
                    min-width="105"
                >
                    <template #default="{ row }">
                        <span :class="cellClass(c.prop, row[c.prop])">
                            {{ c.fmt ? c.fmt(row[c.prop]) : fmtNum(row[c.prop]) }}
                        </span>
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
                :total="rows.length"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next, jumper"
            />
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { monSitesAgg, MonSiteItem, MonSiteRowNode } from '@/xpack/api/monitor';
import { isMasterView } from '@/xpack/api/aggregate';
import { DatePreset, dayParams, detectPreset, fmtBytes, fmtNum, presetRange, todayRange } from './util';

defineProps<{ sites: MonSiteItem[] }>();
const preset = ref<string>('today');
const dates = ref<Date[]>(todayRange());
const loading = ref(false);
const rows = ref<(MonSiteRowNode & { name: string })[]>([]);
const isMaster = computed(() => isMasterView());
const page = ref(1);
const size = ref(20);
const refresh = ref(0);
const sort = ref<{ prop: string; order: string | null }>({ prop: '', order: null });
let timer: number | undefined;

const cols: { prop: string; labelKey: string; fmt?: (n: number) => string }[] = [
    { prop: 'pv', labelKey: 'pv' },
    { prop: 'uv', labelKey: 'uv' },
    { prop: 'ip', labelKey: 'ip' },
    { prop: 'bytes', labelKey: 'traffic', fmt: fmtBytes },
    { prop: 'spiders', labelKey: 'spiders' },
    { prop: 'req', labelKey: 'req' },
    { prop: 'err4', labelKey: 'err4' },
    { prop: 'err5', labelKey: 'err5' },
];

const cellClass = (prop: string, val: number) => {
    if (prop === 'err5' && val > 0) return 'num-cell is-danger';
    if (prop === 'err4' && val > 0) return 'num-cell is-warning';
    return 'num-cell';
};

const pageRows = computed(() => {
    const list = [...rows.value];
    const { prop, order } = sort.value;
    if (prop && order) {
        const dir = order === 'ascending' ? 1 : -1;
        list.sort((a: any, b: any) => (a[prop] > b[prop] ? dir : a[prop] < b[prop] ? -dir : 0));
    }
    return list.slice((page.value - 1) * size.value, page.value * size.value);
});
const onSort = (s: { prop: string; order: string | null }) => (sort.value = s);

const onPreset = (val: string | number | boolean | undefined) => {
    if (!val) return;
    dates.value = presetRange(val as DatePreset);
    load();
};
const onDateChange = () => {
    preset.value = detectPreset(dates.value);
    load();
};

const load = async () => {
    loading.value = true;
    try {
        const list = await monSitesAgg({ site: '', ...dayParams(dates.value) });
        rows.value = (list || []).map((r) => ({ ...r, name: r.domain || r.alias }));
    } finally {
        loading.value = false;
    }
};
const setTimer = () => {
    timer && window.clearInterval(timer);
    timer = refresh.value ? window.setInterval(load, refresh.value * 1000) : undefined;
};
onMounted(load);
onBeforeUnmount(() => timer && window.clearInterval(timer));
</script>

<style scoped>
.toolbar {
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
.date-picker {
    width: 280px;
    flex: none;
}
.site-name {
    font-weight: 500;
    color: var(--el-text-color-primary);
}
.num-cell {
    font-variant-numeric: tabular-nums;
}
.num-cell.is-warning {
    color: var(--el-color-warning);
    font-weight: 600;
}
.num-cell.is-danger {
    color: var(--el-color-danger);
    font-weight: 600;
}
.pager {
    margin-top: 14px;
    justify-content: flex-end;
}
@media (max-width: 720px) {
    .date-picker {
        width: 100%;
    }
}
</style>
