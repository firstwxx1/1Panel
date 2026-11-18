<template>
    <div>
        <ProTabBar v-model="activeTab" :tabs="tabItems" />
        <component :is="current" :key="activeTab" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
import Overview from './overview/index.vue';
import Manage from './manage/index.vue';
import Panels from './panels/index.vue';

const { t } = useI18n();
const route = useRoute();
const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'nodeManage', comp: Manage },
    { key: 'panelManage', comp: Panels },
];
const tabAliases: Record<string, string> = {
    panel: 'panelManage',
    nodes: 'nodeManage',
};
const validTabs = new Set(tabs.map((tab) => tab.key));
const resolveTab = (raw: unknown): string | null => {
    const key = typeof raw === 'string' ? tabAliases[raw] || raw : '';
    return validTabs.has(key) ? key : null;
};
const activeTab = ref(resolveTab(route.query.tab) || 'overview');
watch(
    () => route.query.tab,
    (val) => {
        const next = resolveTab(val);
        if (next) activeTab.value = next;
    },
);
const tabItems = computed(() =>
    tabs.map((tab) => ({
        key: tab.key,
        label: t('pro.node.' + tab.key),
    })),
);
const current = computed(() => tabs.find((x) => x.key === activeTab.value)?.comp);
</script>
