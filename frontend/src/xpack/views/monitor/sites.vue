<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <el-date-picker
                v-model="dates"
                type="daterange"
                    style="width: 320px; flex: none"
                :clearable="false"
                :start-placeholder="$t('pro.monitor.startDate')"
                :end-placeholder="$t('pro.monitor.endDate')"
                @change="load"
            />
        </el-card>
        <el-card shadow="never">
            <div class="tools">
                <el-button :icon="Refresh" @click="load" />
                <el-select v-model="refresh" style="width: 110px" @change="setTimer">
                    <el-option :value="0" :label="$t('pro.monitor.noRefresh')" />
                    <el-option :value="5" label="5s" />
                    <el-option :value="30" label="30s" />
                </el-select>
            </div>
            <el-table :data="pageRows" v-loading="loading" @sort-change="onSort">
                <el-table-column prop="name" :label="$t('pro.monitor.website')" sortable="custom" min-width="160" />
                <el-table-column v-for="c in cols" :key="c.prop" :prop="c.prop" :label="c.label" sortable="custom" min-width="100">
                    <template #default="{ row }">{{ c.fmt ? c.fmt(row[c.prop]) : row[c.prop] }}</template>
                </el-table-column>
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
import { monSites, MonSiteItem, MonSiteRow } from '@/xpack/api/monitor';
import { dayParams, fmtBytes, todayRange } from './util';
import i18n from '@/lang';

defineProps<{ sites: MonSiteItem[] }>();
const t = i18n.global.t;
const dates = ref<Date[]>(todayRange());
const loading = ref(false);
const rows = ref<(MonSiteRow & { name: string })[]>([]);
const page = ref(1);
const size = ref(20);
const refresh = ref(0);
const sort = ref<{ prop: string; order: string | null }>({ prop: '', order: null });
let timer: number | undefined;

const cols = [
    { prop: 'pv', label: t('pro.monitor.pv') },
    { prop: 'uv', label: t('pro.monitor.uv') },
    { prop: 'ip', label: t('pro.monitor.ip') },
    { prop: 'bytes', label: t('pro.monitor.traffic'), fmt: fmtBytes },
    { prop: 'spiders', label: t('pro.monitor.spiders') },
    { prop: 'req', label: t('pro.monitor.req') },
    { prop: 'err4', label: t('pro.monitor.err4') },
    { prop: 'err5', label: t('pro.monitor.err5') },
] as { prop: string; label: string; fmt?: (n: number) => string }[];

const pageRows = computed(() => {
    let list = [...rows.value];
    const { prop, order } = sort.value;
    if (prop && order) {
        const dir = order === 'ascending' ? 1 : -1;
        list.sort((a: any, b: any) => (a[prop] > b[prop] ? dir : a[prop] < b[prop] ? -dir : 0));
    }
    return list.slice((page.value - 1) * size.value, page.value * size.value);
});
const onSort = (s: { prop: string; order: string | null }) => (sort.value = s);
const load = async () => {
    loading.value = true;
    try {
        const res = await monSites({ site: '', ...dayParams(dates.value) });
        rows.value = (res.data || []).map((r) => ({ ...r, name: r.domain || r.alias }));
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
.tools {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-bottom: 12px;
}
.pager {
    margin-top: 12px;
    justify-content: flex-end;
}
</style>
