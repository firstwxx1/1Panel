<template>
    <div>
        <div v-if="cfg && !cfg.enabled" class="banner">
            <span>{{ $t('pro.monitor.notEnabled') }}</span>
            <el-switch :model-value="false" :loading="switching" @change="enableAll" />
        </div>
        <el-card shadow="never" class="mb-3">
            <SiteSelect v-model="site" :sites="sites" all @update:model-value="reload" />
        </el-card>
        <el-card shadow="never" class="mb-3">
            <template #header>
                <span class="card-title">{{ $t('pro.monitor.todayStatus') }}</span>
            </template>
            <el-row>
                <el-col v-for="c in cards" :key="c.label" :span="3">
                    <div class="label">{{ c.label }}</div>
                    <div class="num">{{ c.value }}</div>
                </el-col>
            </el-row>
        </el-card>
        <el-row :gutter="12" class="mb-3">
            <el-col :span="16">
                <el-card shadow="never" class="tall">
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
                        <div class="map"><WafMap :kind="mapKind" :data="mapData" /></div>
                        <div class="side">
                            <div class="side-row side-head">
                                <span>{{ $t('pro.monitor.source') }}</span>
                                <span>{{ $t('pro.monitor.count') }}</span>
                            </div>
                            <div v-for="c in mapData.slice(0, 12)" :key="c.name" class="side-row">
                                <span class="ellipsis" :title="c.name">{{ c.name }}</span>
                                <span>{{ c.count }}</span>
                            </div>
                            <el-empty v-if="!mapData.length" :image-size="48" :description="$t('pro.monitor.noData')" />
                        </div>
                    </div>
                </el-card>
            </el-col>
            <el-col :span="8">
                <el-card shadow="never" class="half mb-3">
                    <template #header>
                        <span class="card-title">{{ $t('pro.monitor.liveReq') }}</span>
                    </template>
                    <div class="big">{{ live.req }}</div>
                    <LineChart id="monLiveReq" height="90px" :option="liveOption('req')" />
                </el-card>
                <el-card shadow="never" class="half">
                    <template #header>
                        <span class="card-title">{{ $t('pro.monitor.liveBytes') }}</span>
                    </template>
                    <div class="big">{{ fmtBytes(live.bytes) }}</div>
                    <LineChart id="monLiveBytes" height="90px" :option="liveOption('bytes')" />
                </el-card>
            </el-col>
        </el-row>
        <el-card shadow="never">
            <template #header>
                <div class="head">
                    <span class="card-title">{{ $t('pro.monitor.visitorTrend') }}</span>
                    <el-radio-group v-model="range" size="small" @change="loadTrend">
                        <el-radio-button value="today">{{ $t('pro.monitor.today') }}</el-radio-button>
                        <el-radio-button value="7d">{{ $t('pro.monitor.d7') }}</el-radio-button>
                        <el-radio-button value="30d">{{ $t('pro.monitor.d30') }}</el-radio-button>
                    </el-radio-group>
                </div>
            </template>
            <LineChart id="monVisitorTrend" height="300px" :option="trendOpt" v-if="trend.length" />
            <el-empty v-else :description="$t('pro.monitor.noData')" />
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import LineChart from '@/components/v-charts/components/Line.vue';
import WafMap from '@/xpack/views/waf/map/index.vue';
import SiteSelect from './SiteSelect.vue';
import {
    monDistribution,
    MonDistItem,
    monGetSettings,
    monLive,
    monOverview,
    monSaveSettings,
    MonSettings,
    MonSiteItem,
    MonTotals,
    MonTrend,
} from '@/xpack/api/monitor';
import { fmtBytes, trendOption } from './util';
import i18n from '@/lang';

const props = defineProps<{ sites: MonSiteItem[] }>();
const emit = defineEmits(['changed']);
const t = i18n.global.t;
const site = ref('');
const range = ref('today');
const cfg = ref<MonSettings | null>(null);
const switching = ref(false);
const totals = ref<MonTotals | null>(null);
const trend = ref<MonTrend[]>([]);
const mapKind = ref<'world' | 'china'>('world');
const countries = ref<MonDistItem[]>([]);
const provinces = ref<MonDistItem[]>([]);
const live = ref<{ req: number; bytes: number; series: { req: number; bytes: number }[] }>({ req: 0, bytes: 0, series: [] });
let timer: number | undefined;

const mapData = computed(() => (mapKind.value === 'china' ? provinces.value : countries.value));
const cards = computed(() => {
    const c = totals.value;
    const v = (n?: number) => n ?? 0;
    return [
        { label: t('pro.monitor.pv'), value: v(c?.pv) },
        { label: t('pro.monitor.uv'), value: v(c?.uv) },
        { label: t('pro.monitor.ip'), value: v(c?.ip) },
        { label: t('pro.monitor.traffic'), value: fmtBytes(v(c?.bytes)) },
        { label: t('pro.monitor.spiders'), value: v(c?.spiders) },
        { label: t('pro.monitor.req'), value: v(c?.req) },
        { label: t('pro.monitor.err4'), value: v(c?.err4) },
        { label: t('pro.monitor.err5'), value: v(c?.err5) },
    ];
});
const trendOpt = computed(() => trendOption(trend.value, t('pro.monitor.uv'), 'uv'));
const liveOption = (key: 'req' | 'bytes') => ({
    tooltip: { trigger: 'axis' },
    grid: { left: 8, right: 8, top: 8, bottom: 8 },
    legend: { show: false },
    xAxis: { type: 'category', show: false, boundaryGap: false },
    yAxis: [{ type: 'value', show: false }],
    xData: live.value.series.map((_, i) => i),
    yData: [{ name: key, data: live.value.series.map((p) => p[key]), showSymbol: false, areaStyle: { opacity: 0.15 } }],
});

const loadTotals = async () => {
    totals.value = (await monOverview({ site: site.value, range: 'today' })).data.current;
};
const loadTrend = async () => {
    trend.value = (await monOverview({ site: site.value, range: range.value })).data.trend || [];
};
const loadMap = async () => {
    const [c, p] = await Promise.all([
        monDistribution({ site: site.value, range: '30d', dim: 'country', limit: 100 }),
        monDistribution({ site: site.value, range: '30d', dim: 'province', limit: 100 }),
    ]);
    countries.value = c.data || [];
    provinces.value = p.data || [];
};
const loadLive = async () => {
    live.value = (await monLive(site.value)).data;
};
const reload = () => {
    loadTotals();
    loadTrend();
    loadMap();
    loadLive();
};
const loadCfg = async () => {
    cfg.value = (await monGetSettings()).data;
};
const enableAll = async () => {
    if (!cfg.value) return;
    switching.value = true;
    try {
        const { siteList, hasRt, ...rest } = cfg.value;
        const sites: Record<string, boolean> = { ...rest.sites };
        props.sites.forEach((s) => (sites[s.alias] = true));
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
    gap: 12px;
    padding: 14px 20px;
    margin-bottom: 12px;
    border-radius: 4px;
    color: var(--el-color-warning);
    background: var(--el-color-warning-light-9);
}
.label {
    color: var(--el-text-color-secondary);
    font-size: 14px;
    text-align: center;
}
.num {
    font-size: 28px;
    color: var(--el-color-primary);
    margin-top: 12px;
    text-align: center;
}
.big {
    font-size: 40px;
    color: var(--el-color-primary);
    text-align: center;
    margin: 4px 0;
}
.head {
    display: flex;
    justify-content: space-between;
    align-items: center;
}
.tall {
    min-height: 560px;
}
.half {
    min-height: 270px;
}
.map-wrap {
    display: flex;
    gap: 16px;
}
.map {
    flex: 1;
    min-width: 0;
}
.side {
    width: 230px;
}
.side-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 8px;
    font-size: 13px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
}
.side-head {
    color: #fff;
    background: var(--el-color-primary);
    font-weight: 600;
}
.ellipsis {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
}
</style>
