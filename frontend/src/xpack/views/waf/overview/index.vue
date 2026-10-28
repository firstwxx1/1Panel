<template>
    <el-row :gutter="12">
        <el-col :xs="24" :md="16">
            <el-card class="mb-3">
                <template #header>
                    <span class="card-title">{{ $t('pro.waf.todayStatus') }}</span>
                </template>
                <div class="stats">
                    <div class="stat" v-for="c in cards" :key="c.label">
                        <div class="label">{{ c.label }}</div>
                        <div class="num">{{ c.value }}</div>
                    </div>
                </div>
            </el-card>
            <el-card>
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
                            <div v-for="c in topCountries" :key="c.name" class="side-row">
                                <span class="ellipsis" :title="c.name">{{ c.name }}</span>
                                <span>{{ c.count }}</span>
                            </div>
                            <el-empty v-if="!topCountries.length" :image-size="48" :description="$t('pro.waf.noData')" />
                        </div>
                    </div>
                </div>
                <div class="legend">
                    <span class="low">{{ $t('pro.waf.low') }}</span>
                    <span class="gradient" />
                    <span class="high">{{ $t('pro.waf.high') }}</span>
                    <span class="swatch" />
                    <span>{{ $t('pro.waf.interceptCount') }}</span>
                </div>
            </el-card>
        </el-col>
        <el-col :xs="24" :md="8">
            <el-card class="mb-3">
                <template #header><span class="card-title">{{ $t('pro.waf.requestTrend') }}</span></template>
                <LineChart id="reqTrend" height="260px" :option="reqOption" v-if="loaded" />
            </el-card>
            <el-card>
                <template #header><span class="card-title">{{ $t('pro.waf.interceptTrend') }}</span></template>
                <LineChart id="blkTrend" height="260px" :option="blkOption" v-if="loaded" />
            </el-card>
        </el-col>
    </el-row>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import LineChart from '@/components/v-charts/components/Line.vue';
import WafMap from '@/xpack/views/waf/map/index.vue';
import { getWafOverview, getWafReport, WafBucket, WafDayStat } from '@/xpack/api/waf';
import i18n from '@/lang';

const t = i18n.global.t;
const trend = ref<WafDayStat[]>([]);
const countries = ref<WafBucket[]>([]);
const provinces = ref<WafBucket[]>([]);
const today = ref<WafDayStat>({ day: '', reqCount: 0, attackCount: 0, count4xx: 0, count5xx: 0 });
const loaded = ref(false);
const mapKind = ref<'world' | 'china'>('world');

const cards = computed(() => [
    { label: t('pro.waf.request'), value: today.value.reqCount },
    { label: t('pro.waf.intercept'), value: today.value.attackCount },
    { label: t('pro.waf.count4xx'), value: today.value.count4xx },
    { label: t('pro.waf.count5xx'), value: today.value.count5xx },
]);

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
    grid: { left: 40, right: 16, top: 24, bottom: 28 },
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

onMounted(async () => {
    const [ov, rp] = await Promise.all([getWafOverview(), getWafReport({})]);
    today.value = ov.data.today;
    trend.value = ov.data.trend || [];
    countries.value = rp.data.countries || [];
    provinces.value = rp.data.provinces || [];
    loaded.value = true;
});
</script>

<style scoped>
.card-title {
    font-weight: 600;
}
.head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}
.stats {
    display: flex;
    align-items: center;
}
.stat {
    flex: 1;
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
    font-size: 30px;
    line-height: 1.2;
    color: var(--el-color-primary);
}
.map-wrap {
    display: flex;
    gap: 12px;
}
.map {
    flex: 1;
    min-width: 0;
}
.side {
    width: 200px;
    display: flex;
    flex-direction: column;
}
.side-head {
    display: flex;
    justify-content: space-between;
    padding: 6px 8px;
    font-size: 12px;
    color: #fff;
    background: var(--el-color-primary);
    border-radius: 3px 3px 0 0;
}
.side-body {
    flex: 1;
    max-height: 380px;
    overflow: auto;
    border: 1px solid var(--el-border-color-lighter);
    border-top: none;
}
.side-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 8px;
    font-size: 12px;
    border-bottom: 1px solid var(--el-border-color-lighter);
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
    margin-top: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
.gradient {
    width: 120px;
    height: 10px;
    border-radius: 2px;
    background: linear-gradient(90deg, #e0f3ff, #006edd);
}
.swatch {
    width: 12px;
    height: 12px;
    margin-left: 12px;
    border-radius: 2px;
    background: #006edd;
}
</style>
