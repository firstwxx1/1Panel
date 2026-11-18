<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="toolbar">
                <div class="toolbar-left">
                    <SiteSelect v-model="site" :sites="sites" all @update:model-value="load" />
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
                    <el-button :icon="Refresh" circle :loading="loading" @click="load" />
                </div>
            </div>
        </el-card>

        <div class="trend-grid" v-loading="loading">
            <el-card v-for="c in charts" :key="c.key" shadow="never" class="trend-card">
                <template #header>
                    <span class="card-title">{{ $t('pro.monitor.' + c.labelKey) }}</span>
                </template>
                <LineChart :id="'monTrend' + c.key" height="260px" :option="option(c)" v-if="trend.length" />
                <el-empty v-else :image-size="60" :description="$t('pro.monitor.noData')" />
            </el-card>
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import LineChart from '@/components/v-charts/components/Line.vue';
import SiteSelect from './SiteSelect.vue';
import { monOverviewAgg, MonSiteItem, MonTrend } from '@/xpack/api/monitor';
import { DatePreset, dayParams, detectPreset, presetRange, todayRange, trendOption } from './util';

defineProps<{ sites: MonSiteItem[] }>();
const { t } = useI18n();
const site = ref('');
const preset = ref<string>('today');
const dates = ref<Date[]>(todayRange());
const loading = ref(false);
const trend = ref<MonTrend[]>([]);

const charts = [
    { key: 'pv', labelKey: 'pv', color: '#005EEB' },
    { key: 'uv', labelKey: 'uv', color: '#10B981' },
    { key: 'bytes', labelKey: 'traffic', color: '#8B5CF6' },
    { key: 'req', labelKey: 'req', color: '#F59E0B' },
] as const;

const option = (c: (typeof charts)[number]) => trendOption(trend.value, t('pro.monitor.' + c.labelKey), c.key, c.color);

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
        trend.value = (await monOverviewAgg({ site: site.value, ...dayParams(dates.value) })).trend || [];
    } finally {
        loading.value = false;
    }
};
onMounted(load);
</script>

<style scoped>
.toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.toolbar-left {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
}
.toolbar-right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.date-picker {
    width: 280px;
    flex: none;
}
.trend-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
}
.trend-card {
    min-height: 330px;
    display: flex;
    flex-direction: column;
}
.trend-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: center;
}
.card-title {
    font-weight: 600;
    font-size: 14px;
    color: var(--el-text-color-primary);
}

@media (max-width: 960px) {
    .trend-grid {
        grid-template-columns: minmax(0, 1fr);
    }
    .date-picker {
        width: 100%;
    }
}
</style>
