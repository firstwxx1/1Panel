<template>
    <div>
        <ProTabBar v-model="activeTab" :tabs="tabItems" />
        <component :is="current" :key="`${activeTab}-${globalStore.currentNode}`" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
import Overview from './overview/index.vue';
import Report from './report/index.vue';
import Logs from './logs/index.vue';
import Blocks from './blocks/index.vue';
import Lists from './lists/index.vue';
import SiteSetting from './settings/site.vue';
import GlobalSetting from './settings/global.vue';

const { t } = useI18n();
const route = useRoute();
const globalStore = GlobalStore();
const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'attackReport', comp: Report },
    { key: 'interceptLog', comp: Logs },
    { key: 'blockLog', comp: Blocks },
    { key: 'blackWhite', comp: Lists },
    { key: 'siteSetting', comp: SiteSetting },
    { key: 'globalSetting', comp: GlobalSetting },
];
const validTabs = new Set(tabs.map((x) => x.key));
const resolveTab = (raw: unknown): string | null => {
    const key = typeof raw === 'string' ? raw : '';
    return validTabs.has(key) ? key : null;
};
const tabItems = computed(() => tabs.map((x) => ({ key: x.key, label: t('pro.waf.' + x.key) })));
const activeTab = ref(resolveTab(route.query.tab) || 'overview');
watch(
    () => route.query.tab,
    (val) => {
        const next = resolveTab(val);
        if (next) activeTab.value = next;
    },
);
const current = computed(() => tabs.find((x) => x.key === activeTab.value)?.comp);
</script>
