<template>
    <div>
        <el-tabs v-model="activeTab" type="card" class="mb-3">
            <el-tab-pane v-for="tab in tabs" :key="tab.key" :label="$t('pro.monitor.' + tab.key)" :name="tab.key" />
        </el-tabs>
        <component :is="current" :key="activeTab" :sites="sites" @changed="loadSites" />
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { monGetSettings, MonSiteItem } from '@/xpack/api/monitor';
import Overview from './overview.vue';
import Visits from './visits.vue';
import Trend from './trend.vue';
import Logs from './logs.vue';
import SiteList from './sites.vue';
import Settings from './settings.vue';

const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'visits', comp: Visits },
    { key: 'trend', comp: Trend },
    { key: 'logs', comp: Logs },
    { key: 'sites', comp: SiteList },
    { key: 'settings', comp: Settings },
];
const activeTab = ref('overview');
const current = computed(() => tabs.find((t) => t.key === activeTab.value)?.comp);
const sites = ref<MonSiteItem[]>([]);
const loadSites = async () => {
    const res = await monGetSettings();
    sites.value = res.data.siteList || [];
};
onMounted(loadSites);
</script>
