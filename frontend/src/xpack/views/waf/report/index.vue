<template>
    <div class="waf-report" v-loading="loading">
        <!-- Top Filter & Action Bar -->
        <el-card shadow="never" class="filter-card mb-3">
            <div class="filter-bar">
                <div class="filter-left">
                    <el-radio-group v-model="range" @change="onPresetChange">
                        <el-radio-button value="today">{{ $t('pro.waf.today') }}</el-radio-button>
                        <el-radio-button value="yesterday">{{ $t('pro.waf.yesterday') }}</el-radio-button>
                        <el-radio-button value="7d">{{ $t('pro.waf.last7') }}</el-radio-button>
                        <el-radio-button value="30d">{{ $t('pro.waf.last30') }}</el-radio-button>
                        <el-radio-button value="custom">{{ $t('pro.waf.custom') }}</el-radio-button>
                    </el-radio-group>
                    <el-date-picker
                        v-model="customRange"
                        type="datetimerange"
                        value-format="YYYY-MM-DD HH:mm:ss"
                        format="YYYY-MM-DD HH:mm"
                        range-separator="~"
                        :start-placeholder="$t('pro.waf.startTime')"
                        :end-placeholder="$t('pro.waf.endTime')"
                        :clearable="false"
                        class="report-date-picker"
                        @change="onCustomChange"
                    />
                </div>
                <div class="filter-right">
                    <el-button :icon="Refresh" circle @click="load" />
                </div>
            </div>
        </el-card>

        <!-- 4 Symmetrical KPI Summary Cards -->
        <div class="kpi-grid">
            <div class="kpi-card kpi-card--danger">
                <div class="kpi-label">{{ $t('pro.waf.totalIntercept') }}</div>
                <div class="kpi-value kpi-value--num" :class="{ 'is-danger': data.total > 0 }">
                    {{ (data.total || 0).toLocaleString() }}
                </div>
                <div class="kpi-sub ellipsis" :title="windowFullText">{{ windowShortText }}</div>
            </div>

            <div class="kpi-card kpi-card--warning">
                <div class="kpi-label">{{ $t('pro.waf.topAttackType') }}</div>
                <div class="kpi-value ellipsis" :title="topType ? ruleLabel(topType.name) : '-'">
                    {{ topType ? ruleLabel(topType.name) : '-' }}
                </div>
                <div class="kpi-sub">
                    <template v-if="topType">
                        {{ topType.count.toLocaleString() }} {{ $t('pro.waf.times') }} · {{ percentStr(topType.count) }}
                    </template>
                    <template v-else>{{ $t('pro.waf.noData') }}</template>
                </div>
            </div>

            <div class="kpi-card kpi-card--primary">
                <div class="kpi-label">{{ $t('pro.waf.topAttackerIp') }}</div>
                <div
                    class="kpi-value mono ellipsis"
                    :class="{ 'is-clickable': Boolean(topIp?.name) }"
                    :title="topIp?.name || '-'"
                    @click="topIp?.name && copyText(topIp.name)"
                >
                    {{ topIp?.name || '-' }}
                </div>
                <div class="kpi-sub">
                    <template v-if="topIp">
                        {{ topIp.count.toLocaleString() }} {{ $t('pro.waf.times') }} · {{ percentStr(topIp.count) }}
                    </template>
                    <template v-else>{{ $t('pro.waf.noData') }}</template>
                </div>
            </div>

            <div class="kpi-card kpi-card--success">
                <div class="kpi-label">{{ $t('pro.waf.topSourceRegion') }}</div>
                <div class="kpi-value ellipsis" :title="topRegion?.name || '-'">
                    {{ topRegion?.name || '-' }}
                </div>
                <div class="kpi-sub">
                    <template v-if="topRegion">
                        {{ topRegion.count.toLocaleString() }} {{ $t('pro.waf.times') }} · {{ percentStr(topRegion.count) }}
                    </template>
                    <template v-else>{{ $t('pro.waf.noData') }}</template>
                </div>
            </div>
        </div>

        <!-- Row 1: 2 Equal-Height Analytical Cards -->
        <div class="row1-grid">
            <!-- Left: Attack Type Donut + Ranked Breakdown -->
            <el-card shadow="never" class="report-card">
                <template #header>
                    <div class="card-head">
                        <span class="card-title">{{ $t('pro.waf.attackType') }}</span>
                    </div>
                </template>
                <div v-if="data.types.length > 0" class="type-split">
                    <div class="donut-wrap">
                        <div ref="donutRef" class="donut-canvas" />
                    </div>
                    <div class="type-list">
                        <div
                            v-for="(item, idx) in data.types"
                            :key="item.name"
                            class="type-row"
                            @mouseenter="highlightSlice(idx, true)"
                            @mouseleave="highlightSlice(idx, false)"
                        >
                            <span
                                class="type-dot"
                                :style="{ backgroundColor: CHART_COLORS[idx % CHART_COLORS.length] }"
                            />
                            <span class="type-name ellipsis" :title="ruleLabel(item.name)">
                                {{ ruleLabel(item.name) }}
                            </span>
                            <span class="type-count">{{ item.count.toLocaleString() }}</span>
                            <span class="type-pct">{{ percentStr(item.count) }}</span>
                        </div>
                    </div>
                </div>
                <div v-else class="empty-box">
                    <el-empty :image-size="68" :description="$t('pro.waf.noData')" />
                </div>
            </el-card>

            <!-- Right: Source Region Ranking (Country / CN Province) -->
            <el-card shadow="never" class="report-card">
                <template #header>
                    <div class="card-head">
                        <span class="card-title">{{ $t('pro.waf.topCountry') }}</span>
                        <el-radio-group v-model="geoTab" size="small">
                            <el-radio-button value="country">{{ $t('pro.waf.geoCountryTab') }}</el-radio-button>
                            <el-radio-button value="province">{{ $t('pro.waf.geoProvinceTab') }}</el-radio-button>
                        </el-radio-group>
                    </div>
                </template>
                <div v-if="geoRows.length > 0" class="rank-list rank-list--scroll">
                    <div v-for="(item, idx) in geoRows" :key="item.name" class="rank-item">
                        <div class="rank-top">
                            <div class="rank-main">
                                <span class="rank-badge" :class="rankClass(idx)">{{ idx + 1 }}</span>
                                <span class="rank-name ellipsis" :title="item.name">{{ item.name }}</span>
                            </div>
                            <div class="rank-metrics">
                                <span class="rank-count">{{ item.count.toLocaleString() }}</span>
                                <span class="rank-pct">{{ percentStr(item.count) }}</span>
                            </div>
                        </div>
                        <div class="rank-bar-track">
                            <div
                                class="rank-bar-fill"
                                :class="barClass(idx)"
                                :style="{ width: `${barWidth(item.count)}%` }"
                            />
                        </div>
                    </div>
                </div>
                <div v-else class="empty-box">
                    <el-empty :image-size="68" :description="$t('pro.waf.noData')" />
                </div>
            </el-card>
        </div>

        <!-- Row 2: 3 Equal-Height Top-10 Ranking Cards -->
        <div class="row2-grid">
            <!-- Card A: Top Attacker IPs -->
            <el-card shadow="never" class="report-card">
                <template #header>
                    <div class="card-head">
                        <span class="card-title">{{ $t('pro.waf.topIp') }}</span>
                    </div>
                </template>
                <div v-if="topIps.length > 0" class="rank-list">
                    <div v-for="(item, idx) in topIps" :key="item.name" class="rank-item">
                        <div class="rank-top">
                            <div class="rank-main">
                                <span class="rank-badge" :class="rankClass(idx)">{{ idx + 1 }}</span>
                                <span
                                    class="rank-name mono is-clickable ellipsis"
                                    :title="item.name"
                                    @click="copyText(item.name)"
                                >
                                    {{ item.name }}
                                </span>
                            </div>
                            <div class="rank-metrics">
                                <el-button
                                    v-if="!isMaster"
                                    link
                                    type="danger"
                                    size="small"
                                    class="row-act-btn"
                                    :loading="blockingIp === item.name"
                                    @click.stop="blockIp(item.name)"
                                >
                                    {{ $t('pro.waf.blockShort') }}
                                </el-button>
                                <el-button
                                    v-else
                                    link
                                    type="primary"
                                    size="small"
                                    class="row-act-btn"
                                    @click.stop="copyText(item.name)"
                                >
                                    {{ $t('pro.waf.copyIp') }}
                                </el-button>
                                <span class="rank-count">{{ item.count.toLocaleString() }}</span>
                                <span class="rank-pct">{{ percentStr(item.count) }}</span>
                            </div>
                        </div>
                        <div class="rank-bar-track">
                            <div
                                class="rank-bar-fill"
                                :class="barClass(idx)"
                                :style="{ width: `${barWidth(item.count)}%` }"
                            />
                        </div>
                    </div>
                </div>
                <div v-else class="empty-box">
                    <el-empty :image-size="64" :description="$t('pro.waf.noData')" />
                </div>
            </el-card>

            <!-- Card B: Top Targeted Websites -->
            <el-card shadow="never" class="report-card">
                <template #header>
                    <div class="card-head">
                        <span class="card-title">{{ $t('pro.waf.topSite') }}</span>
                    </div>
                </template>
                <div v-if="topSites.length > 0" class="rank-list">
                    <div v-for="(item, idx) in topSites" :key="item.name" class="rank-item">
                        <div class="rank-top">
                            <div class="rank-main">
                                <span class="rank-badge" :class="rankClass(idx)">{{ idx + 1 }}</span>
                                <span class="rank-name ellipsis" :title="item.name">{{ item.name }}</span>
                                <el-tag
                                    v-if="item.name === 'unknown'"
                                    size="small"
                                    type="info"
                                    effect="plain"
                                    class="unbound-tag"
                                >
                                    {{ $t('pro.waf.unboundDomain') }}
                                </el-tag>
                            </div>
                            <div class="rank-metrics">
                                <span class="rank-count">{{ item.count.toLocaleString() }}</span>
                                <span class="rank-pct">{{ percentStr(item.count) }}</span>
                            </div>
                        </div>
                        <div class="rank-bar-track">
                            <div
                                class="rank-bar-fill"
                                :class="barClass(idx)"
                                :style="{ width: `${barWidth(item.count)}%` }"
                            />
                        </div>
                    </div>
                </div>
                <div v-else class="empty-box">
                    <el-empty :image-size="64" :description="$t('pro.waf.noData')" />
                </div>
            </el-card>

            <!-- Card C: Top Targeted URLs -->
            <el-card shadow="never" class="report-card">
                <template #header>
                    <div class="card-head">
                        <span class="card-title">{{ $t('pro.waf.topUrl') }}</span>
                    </div>
                </template>
                <div v-if="topUrls.length > 0" class="rank-list">
                    <div v-for="(item, idx) in topUrls" :key="item.name" class="rank-item">
                        <div class="rank-top">
                            <div class="rank-main">
                                <span class="rank-badge" :class="rankClass(idx)">{{ idx + 1 }}</span>
                                <span
                                    class="rank-name mono is-clickable ellipsis"
                                    :title="item.name"
                                    @click="copyText(item.name)"
                                >
                                    {{ item.name }}
                                </span>
                            </div>
                            <div class="rank-metrics">
                                <span class="rank-count">{{ item.count.toLocaleString() }}</span>
                                <span class="rank-pct">{{ percentStr(item.count) }}</span>
                            </div>
                        </div>
                        <div class="rank-bar-track">
                            <div
                                class="rank-bar-fill"
                                :class="barClass(idx)"
                                :style="{ width: `${barWidth(item.count)}%` }"
                            />
                        </div>
                    </div>
                </div>
                <div v-else class="empty-box">
                    <el-empty :image-size="64" :description="$t('pro.waf.noData')" />
                </div>
            </el-card>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { Refresh } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import echarts from '@/utils/echarts';
import { GlobalStore } from '@/store';
import { copyText } from '@/utils/clipboard';
import { isMasterView } from '@/xpack/api/aggregate';
import { getWafReportAgg, saveWafRule, WafReport } from '@/xpack/api/waf';

const CHART_COLORS = [
    '#3b82f6',
    '#ef4444',
    '#f59e0b',
    '#10b981',
    '#8b5cf6',
    '#06b6d4',
    '#ec4899',
    '#6366f1',
    '#14b8a6',
    '#f97316',
];

const { t, te, locale } = useI18n();
const globalStore = GlobalStore();
const isMaster = computed(() => isMasterView());

const loading = ref(false);
const blockingIp = ref('');
const range = ref<'today' | 'yesterday' | '7d' | '30d' | 'custom'>('today');
const geoTab = ref<'country' | 'province'>('country');

const fmt = (d: Date) => {
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};

const computePresetWindow = (preset: string): [string, string] => {
    const now = new Date();
    const day0 = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    let start = day0;
    let end = now;
    if (preset === 'yesterday') {
        start = new Date(day0.getTime() - 86400000);
        end = new Date(day0.getTime() - 1000);
    } else if (preset === '7d') {
        start = new Date(day0.getTime() - 6 * 86400000);
    } else if (preset === '30d') {
        start = new Date(day0.getTime() - 29 * 86400000);
    }
    return [fmt(start), fmt(end)];
};

const customRange = ref<[string, string]>(computePresetWindow('today'));

const data = reactive<WafReport>({
    total: 0,
    types: [],
    ips: [],
    websites: [],
    uris: [],
    countries: [],
    provinces: [],
});

const htmlDark = ref(document.documentElement.classList.contains('dark'));
const isDark = computed(() => globalStore.isDarkTheme || htmlDark.value);

const ruleLabel = (key?: string) => {
    if (!key) return '-';
    const i18nKey = 'pro.waf.rule_' + key;
    return te(i18nKey) ? t(i18nKey) : key;
};

const topType = computed(() => data.types[0] || null);
const topIp = computed(() => data.ips[0] || null);
const topRegion = computed(() => data.countries[0] || data.provinces[0] || null);

const geoRows = computed(() => (geoTab.value === 'province' ? data.provinces : data.countries).slice(0, 10));
const topIps = computed(() => data.ips.slice(0, 10));
const topSites = computed(() => data.websites.slice(0, 10));
const topUrls = computed(() => data.uris.slice(0, 10));

const windowFullText = computed(() => {
    if (!customRange.value || customRange.value.length !== 2) return '';
    return `${customRange.value[0]} ~ ${customRange.value[1]}`;
});

const windowShortText = computed(() => {
    if (!customRange.value || customRange.value.length !== 2) return '';
    return `${customRange.value[0].slice(5, 16)} ~ ${customRange.value[1].slice(5, 16)}`;
});

const effectiveTotal = computed(() => {
    if (data.total > 0) return data.total;
    const sumTypes = data.types.reduce((s, x) => s + (x.count || 0), 0);
    return sumTypes > 0 ? sumTypes : 0;
});

const percentVal = (count: number) => {
    if (!count || count <= 0 || effectiveTotal.value <= 0) return 0;
    return Math.min(100, (count / effectiveTotal.value) * 100);
};

const percentStr = (count: number) => {
    if (!count || count <= 0 || effectiveTotal.value <= 0) return '0.0%';
    const p = percentVal(count);
    if (p > 0 && p < 0.1) return '<0.1%';
    return `${p.toFixed(1)}%`;
};

const barWidth = (count: number) => {
    if (!count || count <= 0 || effectiveTotal.value <= 0) return 0;
    return Math.max(2, Math.min(100, Number(percentVal(count).toFixed(1))));
};

const rankClass = (idx: number) => {
    if (idx === 0) return 'is-rank-1';
    if (idx === 1) return 'is-rank-2';
    if (idx === 2) return 'is-rank-3';
    return '';
};

const barClass = (idx: number) => {
    if (idx === 0) return 'is-bar-1';
    if (idx === 1) return 'is-bar-2';
    if (idx === 2) return 'is-bar-3';
    return '';
};

const inferIpType = (ip: string) => {
    if (ip.includes('/') || ip.includes('-')) return 'ipArr';
    if (ip.includes(':')) return 'ipv6';
    return 'ipv4';
};

const blockIp = async (ip: string) => {
    if (!ip || isMaster.value) return;
    blockingIp.value = ip;
    try {
        await saveWafRule('ipBlack', '', { state: 'on', name: ip, rule: ip, type: inferIpType(ip) }, '');
        ElMessage.success(t('pro.waf.blockSuccess'));
    } finally {
        blockingIp.value = '';
    }
};

// Donut Chart Management
const donutRef = ref<HTMLElement | null>(null);
let chart: echarts.ECharts | null = null;
let ro: ResizeObserver | undefined;
let themeObserver: MutationObserver | undefined;

const renderDonut = async () => {
    await nextTick();
    const el = donutRef.value;
    if (!el || !data.types.length) {
        if (chart) {
            chart.dispose();
            chart = null;
        }
        ro?.disconnect();
        ro = undefined;
        return;
    }
    if (chart && chart.getDom() !== el) {
        chart.dispose();
        chart = null;
    }
    if (!chart) {
        chart = echarts.init(el);
        ro?.disconnect();
        ro = new ResizeObserver(() => chart?.resize());
        ro.observe(el);
    }

    const dark = isDark.value;
    const seriesData = data.types.map((item, idx) => ({
        name: ruleLabel(item.name),
        value: item.count,
        itemStyle: { color: CHART_COLORS[idx % CHART_COLORS.length] },
    }));

    chart.setOption(
        {
            tooltip: {
                trigger: 'item',
                backgroundColor: dark ? '#262729' : '#ffffff',
                borderColor: dark ? '#414243' : '#e4e7ed',
                textStyle: { color: dark ? '#e5eaf3' : '#303133', fontSize: 12 },
                formatter: (p: any) =>
                    `${p.marker || ''}${p.name}: <b>${Number(p.value || 0).toLocaleString()}</b> (${p.percent ?? 0}%)`,
            },
            legend: { show: false },
            title: {
                text: (data.total || 0).toLocaleString(),
                subtext: t('pro.waf.intercept'),
                left: 'center',
                top: '41%',
                itemGap: 4,
                textStyle: {
                    fontSize: 20,
                    fontWeight: 700,
                    color: dark ? '#e5eaf3' : '#303133',
                },
                subtextStyle: {
                    fontSize: 12,
                    color: dark ? '#a3a6ad' : '#909399',
                },
            },
            series: [
                {
                    type: 'pie',
                    radius: ['48%', '72%'],
                    center: ['50%', '50%'],
                    avoidLabelOverlap: true,
                    label: { show: false },
                    labelLine: { show: false },
                    itemStyle: {
                        borderColor: dark ? '#1d1e1f' : '#ffffff',
                        borderWidth: 2,
                        borderRadius: 4,
                    },
                    emphasis: {
                        scale: true,
                        scaleSize: 5,
                    },
                    data: seriesData,
                },
            ],
        },
        true,
    );
    chart.resize();
};

const highlightSlice = (idx: number, active: boolean) => {
    if (!chart) return;
    chart.dispatchAction({
        type: active ? 'highlight' : 'downplay',
        seriesIndex: 0,
        dataIndex: idx,
    });
};

const onPresetChange = (val: string | number | boolean | undefined) => {
    const preset = String(val || 'today');
    if (preset !== 'custom') {
        customRange.value = computePresetWindow(preset);
        load();
    } else if (!customRange.value || customRange.value.length !== 2) {
        customRange.value = computePresetWindow('today');
    }
};

const onCustomChange = (val: [string, string] | null) => {
    if (val && val.length === 2) {
        range.value = 'custom';
        load();
    }
};

const load = async () => {
    if (range.value !== 'custom') {
        customRange.value = computePresetWindow(range.value);
    }
    if (!customRange.value || customRange.value.length !== 2) {
        customRange.value = computePresetWindow('today');
    }
    loading.value = true;
    try {
        const res = await getWafReportAgg({
            start: customRange.value[0],
            end: customRange.value[1],
        });
        data.total = res?.total || 0;
        data.types = res?.types || [];
        data.ips = res?.ips || [];
        data.websites = res?.websites || [];
        data.uris = res?.uris || [];
        data.countries = res?.countries || [];
        data.provinces = res?.provinces || [];
    } finally {
        loading.value = false;
        renderDonut();
    }
};

watch([isDark, locale], () => {
    if (data.types.length > 0) {
        renderDonut();
    }
});

onMounted(() => {
    themeObserver = new MutationObserver(() => {
        htmlDark.value = document.documentElement.classList.contains('dark');
    });
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    load();
});

onBeforeUnmount(() => {
    themeObserver?.disconnect();
    ro?.disconnect();
    chart?.dispose();
    chart = null;
});
</script>

<style scoped>
.filter-card :deep(.el-card__body) {
    padding: 12px 16px;
}

.filter-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 10px;
}

.filter-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
}

.filter-right {
    display: flex;
    align-items: center;
    gap: 8px;
}

.report-date-picker {
    max-width: 340px;
}

/* KPI Summary Grid */
.kpi-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 12px;
    margin-bottom: 12px;
}

.kpi-card {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: 16px 18px;
    background: var(--el-bg-color-overlay);
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 8px;
    min-width: 0;
    overflow: hidden;
}

.kpi-card::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 3px;
    opacity: 0.85;
}

.kpi-card--danger::before {
    background: var(--el-color-danger);
}

.kpi-card--warning::before {
    background: var(--el-color-warning);
}

.kpi-card--primary::before {
    background: var(--el-color-primary);
}

.kpi-card--success::before {
    background: var(--el-color-success);
}

.kpi-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--el-text-color-secondary);
}

.kpi-value {
    margin-top: 10px;
    font-size: 20px;
    font-weight: 700;
    line-height: 1.3;
    color: var(--el-text-color-primary);
}

.kpi-value--num {
    font-size: 26px;
    font-variant-numeric: tabular-nums;
    color: var(--el-color-primary);
}

.kpi-value--num.is-danger {
    color: var(--el-color-danger);
}

.kpi-sub {
    margin-top: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    font-variant-numeric: tabular-nums;
}

/* Analytical Grids */
.row1-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    margin-bottom: 12px;
}

.row2-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
}

.report-card {
    height: 100%;
    display: flex;
    flex-direction: column;
    border-radius: 8px;
}

.report-card :deep(.el-card__header) {
    padding: 12px 16px;
    border-bottom: 1px solid var(--el-border-color-lighter);
}

.report-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
    padding: 14px 16px;
    min-height: 0;
}

.card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-height: 24px;
}

.card-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--el-text-color-primary);
}

/* Row 1 Left: Donut + Breakdown */
.type-split {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(190px, 44%) minmax(0, 1fr);
    align-items: center;
    gap: 14px;
    min-height: 268px;
}

.donut-wrap {
    width: 100%;
    height: 256px;
}

.donut-canvas {
    width: 100%;
    height: 100%;
}

.type-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
    max-height: 268px;
    overflow-y: auto;
    padding-right: 4px;
}

.type-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border-radius: 6px;
    font-size: 13px;
    transition: background-color 0.15s ease;
}

.type-row:hover {
    background: var(--el-fill-color-light);
}

.type-dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    flex-shrink: 0;
}

.type-name {
    flex: 1;
    min-width: 0;
    color: var(--el-text-color-primary);
}

.type-count {
    font-weight: 600;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}

.type-pct {
    width: 52px;
    text-align: right;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    font-variant-numeric: tabular-nums;
}

/* Ranked Lists (Row 1 Right & Row 2 Cards) */
.rank-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
}

.rank-list--scroll {
    max-height: 268px;
    overflow-y: auto;
    padding-right: 4px;
}

.rank-item {
    padding: 6px 8px;
    border-radius: 6px;
    transition: background-color 0.15s ease;
}

.rank-item:hover {
    background: var(--el-fill-color-lighter);
}

.rank-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-width: 0;
}

.rank-main {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;
}

.rank-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 5px;
    font-size: 11px;
    font-weight: 700;
    flex-shrink: 0;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color);
    font-variant-numeric: tabular-nums;
}

.rank-badge.is-rank-1 {
    color: #fff;
    background: #ef4444;
}

.rank-badge.is-rank-2 {
    color: #fff;
    background: #f97316;
}

.rank-badge.is-rank-3 {
    color: #fff;
    background: #eab308;
}

.rank-name {
    font-size: 13px;
    color: var(--el-text-color-primary);
    min-width: 0;
}

.unbound-tag {
    flex-shrink: 0;
    height: 18px;
    padding: 0 5px;
    font-size: 11px;
}

.rank-metrics {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
}

.row-act-btn {
    padding: 0 2px;
    height: auto;
    font-size: 12px;
    opacity: 0.78;
    transition: opacity 0.15s ease;
}

.rank-item:hover .row-act-btn {
    opacity: 1;
}

.rank-count {
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    font-variant-numeric: tabular-nums;
}

.rank-pct {
    min-width: 46px;
    text-align: right;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    font-variant-numeric: tabular-nums;
}

.rank-bar-track {
    margin-top: 5px;
    margin-left: 28px;
    height: 5px;
    border-radius: 3px;
    background: var(--el-fill-color-light);
    overflow: hidden;
}

.rank-bar-fill {
    height: 100%;
    border-radius: 3px;
    background: var(--el-color-primary);
    transition: width 0.3s ease;
}

.rank-bar-fill.is-bar-1 {
    background: var(--el-color-danger);
}

.rank-bar-fill.is-bar-2 {
    background: #f97316;
}

.rank-bar-fill.is-bar-3 {
    background: var(--el-color-warning);
}

.empty-box {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 220px;
}

.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.is-clickable {
    cursor: pointer;
}

.is-clickable:hover {
    color: var(--el-color-primary);
}

.ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

/* Responsive Breakpoints */
@media (max-width: 1199px) {
    .kpi-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .row2-grid {
        grid-template-columns: 1fr;
    }
}

@media (max-width: 991px) {
    .row1-grid {
        grid-template-columns: 1fr;
    }
}

@media (max-width: 640px) {
    .kpi-grid {
        grid-template-columns: 1fr;
    }
    .type-split {
        grid-template-columns: 1fr;
    }
    .donut-wrap {
        height: 220px;
    }
    .report-date-picker {
        max-width: 100%;
        width: 100%;
    }
}
</style>
