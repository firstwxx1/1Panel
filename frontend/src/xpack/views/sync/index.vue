<template>
    <div>
        <el-tabs v-model="active" type="card" class="mb-3">
            <el-tab-pane v-for="k in keys" :key="k" :label="$t('pro.sync.' + k)" :name="k" />
        </el-tabs>
        <Files v-if="active === 'files'" @created="active = 'tasks'" />
        <Resource v-else-if="active === 'images'" kind="image" :key="'i'" @created="active = 'tasks'" />
        <Resource v-else-if="active === 'certs'" kind="cert" :key="'c'" @created="active = 'tasks'" />
        <el-card v-else-if="active === 'apps'" shadow="never">
            <div class="title">{{ $t('pro.sync.appsTitle') }}</div>
            <el-empty :description="$t('pro.sync.appsDesc')" />
        </el-card>
        <Tasks v-else-if="active === 'tasks'" />
    </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import Files from './files.vue';
import Resource from './resource.vue';
import Tasks from './tasks.vue';

const keys = ['files', 'images', 'apps', 'certs', 'tasks'];
const active = ref('files');
</script>

<style scoped>
.title {
    font-weight: 600;
}
</style>
