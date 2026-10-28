<template>
    <div>
        <el-tabs v-model="activeTab" type="card" class="mb-3">
            <el-tab-pane v-for="tab in tabs" :key="tab.key" :label="$t('pro.waf.' + tab.key)" :name="tab.key" />
        </el-tabs>
        <component :is="current" :key="activeTab" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import Overview from './overview/index.vue';
import Report from './report/index.vue';
import Logs from './logs/index.vue';
import Blocks from './blocks/index.vue';
import Lists from './lists/index.vue';
import SiteSetting from './settings/site.vue';
import GlobalSetting from './settings/global.vue';

const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'attackReport', comp: Report },
    { key: 'interceptLog', comp: Logs },
    { key: 'blockLog', comp: Blocks },
    { key: 'blackWhite', comp: Lists },
    { key: 'siteSetting', comp: SiteSetting },
    { key: 'globalSetting', comp: GlobalSetting },
];
const activeTab = ref('overview');
const current = computed(() => tabs.find((t) => t.key === activeTab.value)?.comp);
</script>
