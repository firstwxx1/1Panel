<template>
    <div>
        <div v-if="cfg && !cfg.enabled" class="banner mb-3">
            <div class="banner-text">
                <span>{{ $t('pro.monitor.notEnabled') }}</span>
            </div>
            <el-switch :model-value="false" :loading="switching" @change="enableAll" />
        </div>

        <el-card shadow="never" class="mb-3">
            <template #header>
                <div class="head">
                    <span class="card-title">{{ $t('pro.monitor.todayStatus') }}</span>
                    <div class="head-tools">
                        <SiteSelect v-model="site" :sites="sites" all @update:model-value="reload" />
                        <el-button :icon="Refresh" circle @click="reload" />
                    </div>
                </div>
            </template>
            <div class="status-grid">
                <div v-for="c in cards" :key="c.key" class="status-cell">
                    <div class="status-label">{{ c.label }}</div>
                    <div class="status-num" :class="c.tone">{{ c.value }}</div>
                </div>
            </div>
        </el-card>

        <div class="mid-grid mb-3">
            <el-card shadow="never" class="map-card">
                <template #header>
                    <div class="head">
                        <span class="card-title">{{ $t('pro.monitor.visitorMap') }}</span>
                        <el-radio-group v-model="mapKind" size="small">
                            <el-radio-button value="world">{{ $t('pro.monitor.world') }}</el-radio-button>
                            <el-radio-button value="china">{{ $t('pro.monitor.china') }}</el-radio-button>
                        </el-radio-group>
                    </div>
                </template>
                <div class="map-wrap">
                    <div class="map-pane">
                        <WafMap :kind="mapKind" :data="mapData" />
                    </div>
                    <div class="side-pane">
                        <div class="side-row side-head">
                            <span>{{ $t('pro.monitor.source') }}</span>
                            <span>{{ $t('pro.monitor.count') }}</span>
                        </div>
                        <template v-if="mapData.length">
                            <div v-for="(c, idx) in mapData.slice(0, 10)" :key="c.name" class="side-row">
                                <div class="side-left">
                                    <span class="rank-badge" :class="{ 'is-top': idx < 3 }">{{ idx + 1 }}</span>
                                    <span class="ellipsis" :title="c.name">{{ c.name }}</span>
                                </div>
                                <span class="side-val">{{ fmtNum(c.count) }}</span>
                            </div>
                        </template>
                        <el-empty v-else :image-size="48" :description="$t('pro.monitor.noData')" />
                    </div>
                </div>
            </el-card>

            <div class="live-col">
                <el-card shadow="never" class="live-card">
                    <template #header>
                        <span class="card-title">{{ $t('pro.monitor.liveReq') }}</span>
                    </template>
                    <div class="live-body">
                        <div class="big">{{ fmtNum(live.req) }}</div>
                        <LineChart id="monLiveReq" height="110px" :option="liveOption('req')" />
                    </div>
                </el-card>
                <el-card shadow="never" class="live-card">
                    <template #header>
                        <span class="card-title">{{ $t('pro.monitor.liveBytes') }}</span>
                    </template>
                    <div class="live-body">
                        <div class="big">{{ fmtBytes(live.bytes) }}</div>
                        <LineChart id="monLiveBytes" height="110px" :option="liveOption('bytes')" />
                    </div>
                </el-card>
            </div>
        </div>

        <el-card shadow="never">
            <template #header>
                <div class="head">
                    <span class="card-title">{{ $t('pro.monitor.visitorTrend') }}</span>
                    <div class="head-tools">
                        <el-radio-group v-model="trendMetric" size="small">
                            <el-radio-button value="uv">{{ $t('pro.monitor.uv') }}</el-radio-button>
                            <el-radio-button value="pv">{{ $t('pro.monitor.pv') }}</el-radio-button>
                            <el-radio-button value="req">{{ $t('pro.monitor.req') }}</el-radio-button>
                            <el-radio-button value="bytes">{{ $t('pro.monitor.traffic') }}</el-radio-button>
                        </el-radio-group>
                        <el-radio-group v-model="range" size="small" @change="loadTrend">
                            <el-radio-button value="today">{{ $t('pro.monitor.today') }}</el-radio-button>
                            <el-radio-button value="7d">{{ $t('pro.monitor.d7') }}</el-radio-button>
                            <el-radio-button value="30d">{{ $t('pro.monitor.d30') }}</el-radio-button>
                        </el-radio-group>
                    </div>
                </div>
            </template>
            <LineChart id="monVisitorTrend" height="300px" :option="trendOpt" v-if="trend.length" />
            <el-empty v-else :image-size="60" :description="$t('pro.monitor.noData')" />
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import LineChart from '@/components/v-charts/components/Line.vue';
import WafMap from '@/xpack/views/waf/map/index.vue';
import SiteSelect from './SiteSelect.vue';
import {
    MonDistItem,
    monDistributionAgg,
    monGetSettings,
    monLiveAgg,
    monOverviewAgg,
    monSaveSettings,
    MonSettings,
    MonSiteItem,
    MonTotals,
    MonTrend,
    splitSite,
} from '@/xpack/api/monitor';
import { isMasterView } from '@/xpack/api/aggregate';
import { fmtBytes, fmtNum, trendOption } from './util';

const props = defineProps<{ sites: MonSiteItem[] }>();
const emit = defineEmits(['changed']);
const { t } = useI18n();
const site = ref('');
const range = ref('today');
const trendMetric = ref<'uv' | 'pv' | 'req' | 'bytes'>('uv');
const cfg = ref<MonSettings | null>(null);
const switching = ref(false);
const totals = ref<MonTotals | null>(null);
const trend = ref<MonTrend[]>([]);
const mapKind = ref<'world' | 'china'>('world');
const countries = ref<MonDistItem[]>([]);
const provinces = ref<MonDistItem[]>([]);
const live = ref<{ req: number; bytes: number; series: { time: number; req: number; bytes: number }[] }>({
    req: 0,
    bytes: 0,
    series: [],
});
let timer: number | undefined;

const mapData = computed(() => (mapKind.value === 'china' ? provinces.value : countries.value));
const cards = computed(() => {
    const c = totals.value;
    const v = (n?: number) => n ?? 0;
    return [
        { key: 'pv', label: t('pro.monitor.pv'), value: fmtNum(v(c?.pv)), tone: 'is-primary' },
        { key: 'uv', label: t('pro.monitor.uv'), value: fmtNum(v(c?.uv)), tone: 'is-primary' },
        { key: 'ip', label: t('pro.monitor.ip'), value: fmtNum(v(c?.ip)), tone: '' },
        { key: 'req', label: t('pro.monitor.req'), value: fmtNum(v(c?.req)), tone: '' },
        { key: 'traffic', label: t('pro.monitor.traffic'), value: fmtBytes(v(c?.bytes)), tone: '' },
        { key: 'spiders', label: t('pro.monitor.spiders'), value: fmtNum(v(c?.spiders)), tone: '' },
        {
            key: 'err4',
            label: t('pro.monitor.err4'),
            value: fmtNum(v(c?.err4)),
            tone: v(c?.err4) > 0 ? 'is-warning' : 'is-muted',
        },
        {
            key: 'err5',
            label: t('pro.monitor.err5'),
            value: fmtNum(v(c?.err5)),
            tone: v(c?.err5) > 0 ? 'is-danger' : 'is-muted',
        },
    ];
});

const metricMeta: Record<'uv' | 'pv' | 'req' | 'bytes', { labelKey: string; color: string }> = {
    uv: { labelKey: 'uv', color: '#005EEB' },
    pv: { labelKey: 'pv', color: '#10B981' },
    req: { labelKey: 'req', color: '#F59E0B' },
    bytes: { labelKey: 'traffic', color: '#8B5CF6' },
};

const trendOpt = computed(() => {
    const m = metricMeta[trendMetric.value];
    return trendOption(trend.value, t('pro.monitor.' + m.labelKey), trendMetric.value, m.color);
});

const liveOption = (key: 'req' | 'bytes') => ({
    tooltip: {
        trigger: 'axis',
        formatter: (params: any) => {
            const p = Array.isArray(params) ? params[0] : params;
            if (!p) return '';
            const val = key === 'bytes' ? fmtBytes(Number(p.data || 0)) : fmtNum(Number(p.data || 0));
            return `${p.marker} <b>${val}</b>`;
        },
    },
    grid: { left: 8, right: 8, top: 8, bottom: 8 },
    legend: { show: false },
    xAxis: { type: 'category', show: false, boundaryGap: false },
    yAxis: [{ type: 'value', show: false }],
    xData: live.value.series.map((_, i) => i),
    yData: [
        {
            name: key,
            data: live.value.series.map((p) => p[key]),
            showSymbol: false,
            areaStyle: { opacity: 0.15 },
        },
    ],
});

const loadTotals = async () => {
    totals.value = (await monOverviewAgg({ site: site.value, range: 'today' })).current;
};
const loadTrend = async () => {
    trend.value = (await monOverviewAgg({ site: site.value, range: range.value })).trend || [];
};
const loadMap = async () => {
    const [c, p] = await Promise.all([
        monDistributionAgg({ site: site.value, range: '30d', dim: 'country', limit: 100 }),
        monDistributionAgg({ site: site.value, range: '30d', dim: 'province', limit: 100 }),
    ]);
    countries.value = c || [];
    provinces.value = p || [];
};
const loadLive = async () => {
    live.value = await monLiveAgg(site.value);
};
const reload = () => {
    loadTotals();
    loadTrend();
    loadMap();
    loadLive();
};
// on the master view there is no single switch/config — each node keeps its own, so the
// enable banner only makes sense (and only appears) on a single-node view
const loadCfg = async () => {
    if (isMasterView()) {
        cfg.value = null;
        return;
    }
    cfg.value = (await monGetSettings()).data;
};
const enableAll = async () => {
    if (!cfg.value) return;
    switching.value = true;
    try {
        const { siteList, hasRt, ...rest } = cfg.value;
        const sites: Record<string, boolean> = { ...rest.sites };
        props.sites.forEach((s) => (sites[splitSite(s.alias).alias] = true));
        await monSaveSettings({ ...rest, enabled: true, sites });
        ElMessage.success(t('pro.monitor.saved'));
        await loadCfg();
        emit('changed');
    } finally {
        switching.value = false;
    }
};
onMounted(() => {
    loadCfg();
    reload();
    timer = window.setInterval(() => {
        loadLive();
        loadTotals();
    }, 5000);
});
onBeforeUnmount(() => timer && window.clearInterval(timer));
</script>

<style scoped>
.banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 18px;
    border-radius: 6px;
    border: 1px solid var(--el-color-warning-light-5);
    color: var(--el-color-warning);
    background: var(--el-color-warning-light-9);
    font-size: 13px;
    font-weight: 500;
}
.head {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
}
.head-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
}
.card-title {
    font-weight: 600;
    font-size: 15px;
    color: var(--el-text-color-primary);
}
.status-grid {
    display: grid;
    grid-template-columns: repeat(8, minmax(0, 1fr));
    gap: 10px;
}
.status-cell {
    padding: 12px 10px;
    border-radius: 6px;
    background: var(--el-fill-color-lighter);
    border: 1px solid var(--el-border-color-extra-light);
    text-align: center;
    transition: border-color 0.15s ease;
}
.status-cell:hover {
    border-color: var(--el-border-color-light);
}
.status-label {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.4;
}
.status-num {
    margin-top: 8px;
    font-size: 22px;
    font-weight: 600;
    line-height: 1.2;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}
.status-num.is-primary {
    color: var(--el-color-primary);
}
.status-num.is-warning {
    color: var(--el-color-warning);
}
.status-num.is-danger {
    color: var(--el-color-danger);
}
.status-num.is-muted {
    color: var(--el-text-color-secondary);
}

.mid-grid {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
    gap: 12px;
    align-items: stretch;
}
.map-card {
    height: 100%;
    display: flex;
    flex-direction: column;
}
.map-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
}
.map-wrap {
    flex: 1;
    display: flex;
    align-items: stretch;
    gap: 16px;
}
.map-pane {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
}
.map-pane :deep(> div) {
    width: 100%;
}
.side-pane {
    width: 240px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
}
.side-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 7px 10px;
    font-size: 13px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
}
.side-head {
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color-light);
    border-radius: 4px;
    border-bottom: none;
    font-size: 12px;
    font-weight: 600;
    margin-bottom: 4px;
}
.side-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
}
.rank-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    flex-shrink: 0;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
}
.rank-badge.is-top {
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
}
.side-val {
    font-weight: 500;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}
.ellipsis {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
}

.live-col {
    display: flex;
    flex-direction: column;
    gap: 12px;
    height: 100%;
}
.live-card {
    flex: 1;
    display: flex;
    flex-direction: column;
}
.live-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
}
.live-body {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
}
.big {
    font-size: 34px;
    font-weight: 600;
    color: var(--el-color-primary);
    text-align: center;
    margin: 6px 0 10px;
    font-variant-numeric: tabular-nums;
}

@media (max-width: 1279px) {
    .status-grid {
        grid-template-columns: repeat(4, minmax(0, 1fr));
    }
}
@media (max-width: 1024px) {
    .mid-grid {
        grid-template-columns: minmax(0, 1fr);
    }
}
@media (max-width: 768px) {
    .map-wrap {
        flex-direction: column;
    }
    .side-pane {
        width: 100%;
    }
}
@media (max-width: 640px) {
    .status-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
}
</style>
