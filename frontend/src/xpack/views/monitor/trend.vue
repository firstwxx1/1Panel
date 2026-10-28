<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="bar">
                <SiteSelect v-model="site" :sites="sites" all @update:model-value="load" />
                <el-date-picker
                    v-model="dates"
                    type="daterange"
                    style="width: 320px; flex: none"
                    :clearable="false"
                    :start-placeholder="$t('pro.monitor.startDate')"
                    :end-placeholder="$t('pro.monitor.endDate')"
                    @change="load"
                />
            </div>
        </el-card>
        <el-row :gutter="12">
            <el-col v-for="c in charts" :key="c.key" :span="12" class="mb-3">
                <el-card shadow="never">
                    <template #header>
                        <span class="card-title">{{ c.title }}</span>
                    </template>
                    <LineChart :id="'monTrend' + c.key" height="260px" :option="option(c)" v-if="trend.length" />
                </el-card>
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import LineChart from '@/components/v-charts/components/Line.vue';
import SiteSelect from './SiteSelect.vue';
import { monOverview, MonSiteItem, MonTrend } from '@/xpack/api/monitor';
import { dayParams, todayRange, trendOption } from './util';
import i18n from '@/lang';

defineProps<{ sites: MonSiteItem[] }>();
const t = i18n.global.t;
const site = ref('');
const dates = ref<Date[]>(todayRange());
const trend = ref<MonTrend[]>([]);
const charts = [
    { key: 'pv', title: t('pro.monitor.pv') },
    { key: 'uv', title: t('pro.monitor.uv') },
    { key: 'bytes', title: t('pro.monitor.traffic') },
    { key: 'req', title: t('pro.monitor.req') },
] as const;
const option = (c: (typeof charts)[number]) => trendOption(trend.value, c.title, c.key);
const load = async () => {
    trend.value = (await monOverview({ site: site.value, ...dayParams(dates.value) })).data.trend || [];
};
onMounted(load);
</script>

<style scoped>
.bar {
    display: flex;
    gap: 12px;
}
</style>
