<template>
    <div>
        <el-tabs v-model="activeTab" type="card" class="mb-3">
            <el-tab-pane v-for="tab in tabs" :key="tab.key" :label="$t('pro.monitor.' + tab.key)" :name="tab.key" />
        </el-tabs>
        <component :is="current" :key="activeTab" :sites="sites" @changed="loadSites" />
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { mergedSiteList, MonSiteItem } from '@/xpack/api/monitor';
import { GlobalStore } from '@/store';
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
const globalStore = GlobalStore();
const current = computed(() => tabs.find((t) => t.key === activeTab.value)?.comp);
const sites = ref<MonSiteItem[]>([]);
const loadSites = async () => {
    // on the master this unions every node's sites (aliases prefixed with the node);
    // on a single node it is just that node's siteList
    sites.value = await mergedSiteList();
};
watch(
    () => globalStore.currentNode,
    () => loadSites(),
);
onMounted(loadSites);
</script>
