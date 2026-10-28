<template>
    <div>
        <el-tabs v-model="activeTab" type="card" class="mb-3">
            <el-tab-pane v-for="tab in tabs" :key="tab.key" :label="$t('pro.node.' + tab.key)" :name="tab.key" />
        </el-tabs>
        <component :is="current" :key="activeTab" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import Overview from './overview/index.vue';
import Manage from './manage/index.vue';
import Panels from './panels/index.vue';

const tabs = [
    { key: 'overview', comp: Overview },
    { key: 'nodeManage', comp: Manage },
    { key: 'panelManage', comp: Panels },
];
const activeTab = ref('overview');
const current = computed(() => tabs.find((t) => t.key === activeTab.value)?.comp);
</script>
