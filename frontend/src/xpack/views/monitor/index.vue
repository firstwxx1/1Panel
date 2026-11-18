<template>
    <div>
        <ProTabBar v-model="activeTab" :tabs="tabItems">
            <template v-if="sites.length" #right>
                <el-tag size="small" type="info" effect="plain">
                    {{ $t('pro.monitor.siteCount', { n: sites.length }) }}
                </el-tag>
            </template>
        </ProTabBar>
        <component :is="current" :key="`${activeTab}-${globalStore.currentNode}`" :sites="sites" @changed="loadSites" />
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
import { mergedSiteList, MonSiteItem } from '@/xpack/api/monitor';
import { GlobalStore } from '@/store';
import Overview from './overview.vue';
import Visits from './visits.vue';
import Trend from './trend.vue';
import Logs from './logs.vue';
import SiteList from './sites.vue';
import Settings from './settings.vue';

const { t } = useI18n();
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
const sites = ref<MonSiteItem[]>([]);
const tabItems = computed(() =>
    tabs.map((tab) => ({
        key: tab.key,
        label: t('pro.monitor.' + tab.key),
    })),
);
const current = computed(() => tabs.find((x) => x.key === activeTab.value)?.comp);

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
