<template>
    <div>
        <ProTabBar v-model="active" :tabs="tabItems" />
        <Files v-if="active === 'files'" @created="active = 'tasks'" />
        <Resource v-else-if="active === 'images'" kind="image" :key="'i'" @created="active = 'tasks'" />
        <Resource v-else-if="active === 'certs'" kind="cert" :key="'c'" @created="active = 'tasks'" />
        <Resource v-else-if="active === 'apps'" kind="app" :key="'a'" @created="active = 'tasks'" />
        <Tasks v-else-if="active === 'tasks'" />
    </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ProTabBar from '@/xpack/components/ProTabBar.vue';
import Files from './files.vue';
import Resource from './resource.vue';
import Tasks from './tasks.vue';

const { t } = useI18n();
const keys = ['files', 'images', 'apps', 'certs', 'tasks'];
const active = ref('files');
const tabItems = computed(() =>
    keys.map((k) => ({
        key: k,
        label: t('pro.sync.' + k),
    })),
);
</script>
