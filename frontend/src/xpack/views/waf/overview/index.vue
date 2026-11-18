<template>
    <el-row :gutter="12" class="overview-row" v-loading="loading">
        <el-col :xs="24" :md="16" class="overview-col">
            <el-card shadow="never" class="mb-3">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.waf.todayStatus') }}</span>
                        <el-button :icon="Refresh" circle size="small" @click="load" />
                    </div>
                </template>
                <div class="stats">
                    <div v-for="c in cards" :key="c.key" class="stat">
                        <div class="label">{{ c.label }}</div>
                        <div class="num" :class="c.accent">{{ c.value }}</div>
                    </div>
                </div>
            </el-card>

            <el-card shadow="never" class="grow-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.waf.mapTitle') }}</span>
                        <el-radio-group v-model="mapKind" size="small">
                            <el-radio-button value="world">{{ $t('pro.waf.world') }}</el-radio-button>
                            <el-radio-button value="china">{{ $t('pro.waf.china') }}</el-radio-button>
                        </el-radio-group>
                    </div>
                </template>
                <div class="map-wrap">
                    <div class="map">
                        <WafMap :kind="mapKind" :data="mapData" />
                    </div>
                    <div class="side">
                        <div class="side-head">
                            <span>{{ $t('pro.waf.source') }}</span>
                            <span>{{ $t('pro.waf.count') }}</span>
                        </div>
                        <div class="side-body">
                            <div v-for="(c, idx) in topCountries" :key="c.name" class="side-row">
                                <div class="side-name">
                                    <span class="side-rank" :class="rankClass(idx)">{{ idx + 1 }}</span>
                                    <span class="ellipsis" :title="c.name">{{ c.name }}</span>
                                </div>
                                <span class="side-count">{{ (c.count || 0).toLocaleString() }}</span>
                            </div>
                            <div v-if="!topCountries.length" class="side-empty">
                                <el-empty :image-size="48" :description="$t('pro.waf.noData')" />
                            </div>
                        </div>
                    </div>
                </div>
                <div class="legend">
                    <span>{{ $t('pro.waf.low') }}</span>
                    <span class="gradient" :class="{ 'is-dark': isDark }" />
                    <span>{{ $t('pro.waf.high') }}</span>
                    <span class="swatch" :class="{ 'is-dark': isDark }" />
                    <span>{{ $t('pro.waf.interceptCount') }}</span>
                </div>
            </el-card>
        </el-col>

        <el-col :xs="24" :md="8" class="overview-col">
            <el-card shadow="never" class="mb-3 grow-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.waf.requestTrend') }}</span>
                    </div>
                </template>
                <div class="trend-chart-box">
                    <LineChart v-if="loaded" id="reqTrend" height="100%" :option="reqOption" />
                </div>
            </el-card>

            <el-card shadow="never" class="grow-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.waf.interceptTrend') }}</span>
                    </div>
                </template>
                <div class="trend-chart-box">
                    <LineChart v-if="loaded" id="blkTrend" height="100%" :option="blkOption" />
                </div>
            </el-card>
        </el-col>
    </el-row>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import LineChart from '@/components/v-charts/components/Line.vue';
import WafMap from '@/xpack/views/waf/map/index.vue';
import { getWafOverviewAgg, getWafReportAgg, WafBucket, WafDayStat } from '@/xpack/api/waf';

const { t } = useI18n();
const globalStore = GlobalStore();
const htmlDark = ref(document.documentElement.classList.contains('dark'));
const isDark = computed(() => globalStore.isDarkTheme || htmlDark.value);
let themeObserver: MutationObserver | undefined;

const loading = ref(false);
const loaded = ref(false);
const trend = ref<WafDayStat[]>([]);
const countries = ref<WafBucket[]>([]);
const provinces = ref<WafBucket[]>([]);
const today = ref<WafDayStat>({ day: '', reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 });
const mapKind = ref<'world' | 'china'>('world');

const cards = computed(() => [
    {
        key: 'req',
        label: t('pro.waf.request'),
        value: (today.value.reqCount || 0).toLocaleString(),
        accent: 'is-primary',
    },
    {
        key: 'intercept',
        label: t('pro.waf.intercept'),
        value: (today.value.attackCount || 0).toLocaleString(),
        accent: 'is-danger',
    },
    {
        key: '4xx',
        label: t('pro.waf.count4xx'),
        value: (today.value.count4xx || 0).toLocaleString(),
        accent: 'is-warning',
    },
    {
        key: '5xx',
        label: t('pro.waf.count5xx'),
        value: (today.value.count5xx || 0).toLocaleString(),
        accent: (today.value.count5xx || 0) > 0 ? 'is-danger' : 'is-neutral',
    },
]);

const rankClass = (idx: number) => {
    if (idx === 0) return 'is-rank-1';
    if (idx === 1) return 'is-rank-2';
    if (idx === 2) return 'is-rank-3';
    return '';
};

const mapData = computed(() => (mapKind.value === 'china' ? provinces.value : countries.value) || []);
const topCountries = computed(() => mapData.value.slice(0, 8));

// the engine only writes a row for days with traffic and uses its own (UTC) day
// boundary, so anchor the window on the newest recorded day instead of the local
// clock — otherwise the last point always drops to zero
const days = computed(() => {
    const byDay = new Map(trend.value.map((d) => [d.day, d]));
    const out: WafDayStat[] = [];
    const latest = trend.value.length ? trend.value[trend.value.length - 1].day : '';
    const anchor = latest ? new Date(latest.replace(/-/g, '/')) : new Date();
    for (let i = 6; i >= 0; i--) {
        const dt = new Date(anchor.getFullYear(), anchor.getMonth(), anchor.getDate() - i);
        const key = `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`;
        out.push(byDay.get(key) || { day: key, reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 });
    }
    return out;
});

// LineChart builds series from xData/yData; a raw `series` array is ignored
const xData = computed(() => days.value.map((d) => d.day.slice(5)));
const yData = (key: keyof WafDayStat, name: string) => [
    { name, data: days.value.map((d) => (d[key] as number) || 0), showSymbol: true, symbolSize: 5 },
];
const axis = {
    tooltip: { trigger: 'axis' },
    grid: { left: 42, right: 16, top: 24, bottom: 28 },
    xAxis: { type: 'category', boundaryGap: false, axisLabel: { hideOverlap: true } },
    // one series per chart, so a legend would only crowd the last axis label
    legend: { show: false },
    // explicit yAxis keeps LineChart from drawing a "( undefined )" axis name
    yAxis: [{ type: 'value', minInterval: 1 }],
};

const reqOption = computed(() => ({
    ...axis,
    xData: xData.value,
    yData: yData('reqCount', t('pro.waf.request')),
}));

const blkOption = computed(() => ({
    ...axis,
    xData: xData.value,
    yData: yData('attackCount', t('pro.waf.intercept')),
}));

const fmtDateTime = (d: Date) => {
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

const load = async () => {
    loading.value = true;
    try {
        const now = new Date();
        const day0 = new Date(now.getFullYear(), now.getMonth(), now.getDate());
        const start30d = new Date(day0.getTime() - 29 * 86400000);
        const [ov, rp] = await Promise.all([
            getWafOverviewAgg(),
            getWafReportAgg({ start: fmtDateTime(start30d), end: fmtDateTime(now) }),
        ]);
        today.value = ov.today || { day: '', reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 };
        trend.value = ov.trend || [];
        countries.value = rp.countries || [];
        provinces.value = rp.provinces || [];
        loaded.value = true;
    } finally {
        loading.value = false;
    }
};

onMounted(() => {
    themeObserver = new MutationObserver(() => {
        htmlDark.value = document.documentElement.classList.contains('dark');
    });
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    load();
});

onBeforeUnmount(() => {
    themeObserver?.disconnect();
});
</script>

<style scoped>
.overview-row {
    display: flex;
    flex-wrap: wrap;
    align-items: stretch;
}

.overview-col {
    display: flex;
    flex-direction: column;
}

.grow-card {
    flex: 1;
    display: flex;
    flex-direction: column;
}

.grow-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
}

.card-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
}

.head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-height: 24px;
}

.stats {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
}

.stat {
    padding: 4px 8px;
    text-align: center;
    border-right: 1px solid var(--el-border-color-lighter);
}

.stat:last-child {
    border-right: none;
}

.label {
    font-size: 13px;
    color: var(--el-text-color-secondary);
}

.num {
    margin-top: 8px;
    font-size: 28px;
    font-weight: 700;
    line-height: 1.2;
    font-variant-numeric: tabular-nums;
}

.num.is-primary {
    color: var(--el-color-primary);
}

.num.is-danger {
    color: var(--el-color-danger);
}

.num.is-warning {
    color: var(--el-color-warning);
}

.num.is-neutral {
    color: var(--el-text-color-primary);
}

.map-wrap {
    flex: 1;
    display: flex;
    gap: 14px;
    min-height: 0;
}

.map {
    flex: 1;
    min-width: 0;
}

.side {
    width: 220px;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
}

.side-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 10px;
    font-size: 12px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color-light);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 6px 6px 0 0;
}

.side-body {
    flex: 1;
    max-height: 382px;
    overflow-y: auto;
    border: 1px solid var(--el-border-color-lighter);
    border-top: none;
    border-radius: 0 0 6px 6px;
}

.side-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 7px 10px;
    font-size: 12px;
    border-bottom: 1px solid var(--el-border-color-lighter);
    transition: background-color 0.15s ease;
}

.side-row:last-child {
    border-bottom: none;
}

.side-row:hover {
    background: var(--el-fill-color-lighter);
}

.side-name {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    color: var(--el-text-color-primary);
}

.side-rank {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 700;
    flex-shrink: 0;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
    font-variant-numeric: tabular-nums;
}

.side-rank.is-rank-1 {
    color: #fff;
    background: #ef4444;
}

.side-rank.is-rank-2 {
    color: #fff;
    background: #f97316;
}

.side-rank.is-rank-3 {
    color: #fff;
    background: #eab308;
}

.side-count {
    font-weight: 600;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
}

.side-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 180px;
}

.ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.legend {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 10px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}

.gradient {
    width: 120px;
    height: 10px;
    border-radius: 3px;
    background: linear-gradient(90deg, #dbeafe, #006edd);
}

.gradient.is-dark {
    background: linear-gradient(90deg, #1e3a5f, #3b82f6);
}

.swatch {
    width: 12px;
    height: 12px;
    margin-left: 12px;
    border-radius: 3px;
    background: #006edd;
}

.swatch.is-dark {
    background: #3b82f6;
}

.trend-chart-box {
    position: relative;
    flex: 1;
    width: 100%;
    min-height: 255px;
}

.trend-chart-box :deep(> div) {
    position: absolute;
    inset: 0;
}

@media (max-width: 991px) {
    .overview-col:first-child {
        margin-bottom: 12px;
    }
}

@media (max-width: 767px) {
    .map-wrap {
        flex-direction: column;
    }
    .side {
        width: 100%;
    }
    .stats {
        grid-template-columns: repeat(2, minmax(0, 1fr));
        row-gap: 12px;
    }
    .stat:nth-child(2) {
        border-right: none;
    }
}
</style>
