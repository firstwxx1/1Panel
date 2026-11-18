<template>
    <div>
        <ProTabBar v-model="activeTab" :tabs="tabItems" />
        <component :is="current" :key="activeTab" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
import Overview from './overview/index.vue';
import Manage from './manage/index.vue';
import Panels from './panels/index.vue';

const { t } = useI18n();
const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'nodeManage', comp: Manage },
    { key: 'panelManage', comp: Panels },
];
const activeTab = ref('overview');
const tabItems = computed(() =>
    tabs.map((tab) => ({
        key: tab.key,
        label: t('pro.node.' + tab.key),
    })),
);
const current = computed(() => tabs.find((x) => x.key === activeTab.value)?.comp);
</script>
