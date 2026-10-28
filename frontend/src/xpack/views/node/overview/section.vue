<template>
    <el-card class="section" shadow="never">
        <div class="head">
            <div class="title">
                <span class="bar" />
                <span>{{ title }}</span>
                <span class="count">{{ total }}</span>
            </div>
            <div class="tools">
                <slot name="tools" />
                <el-input v-model="info" clearable size="small" :placeholder="$t('pro.node.search')" style="width: 160px" @keyup.enter="reload" @clear="reload">
                    <template #append>
                        <el-button :icon="Search" @click="reload" />
                    </template>
                </el-input>
                <el-button size="small" :icon="Refresh" circle @click="load" />
            </div>
        </div>
        <el-table :data="items" v-loading="loading" size="small" :max-height="320">
            <slot />
            <template #empty>{{ $t('pro.node.noData') }}</template>
        </el-table>
        <div v-if="total > pageSize" class="pager">
            <el-pagination
                v-model:current-page="page"
                v-model:page-size="pageSize"
                :total="total"
                :page-sizes="[5, 10, 20]"
                layout="total, sizes, prev, pager, next"
                small
                @change="load"
            />
        </div>
    </el-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { Refresh, Search } from '@element-plus/icons-vue';

type Result = { items: any[]; total: number };
const props = defineProps<{
    title: string;
    node: string;
    loader: (q: { page: number; pageSize: number; info: string }) => Promise<Result>;
    // bump to force a reload from the parent (extra filters, global refresh)
    token?: number;
}>();

const info = ref('');
const page = ref(1);
const pageSize = ref(5);
const total = ref(0);
const items = ref<any[]>([]);
const loading = ref(false);

const load = async () => {
    if (!props.node) return;
    loading.value = true;
    try {
        const res = await props.loader({ page: page.value, pageSize: pageSize.value, info: info.value });
        items.value = res.items || [];
        total.value = res.total || 0;
    } catch {
        // an unreachable node or a feature not installed there simply shows empty
        items.value = [];
        total.value = 0;
    } finally {
        loading.value = false;
    }
};
const reload = () => {
    page.value = 1;
    load();
};

watch(
    () => [props.node, props.token],
    () => {
        page.value = 1;
        load();
    },
    { immediate: true },
);
</script>

<style scoped>
.section {
    margin-bottom: 12px;
}
.head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
}
.title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
}
.bar {
    width: 3px;
    height: 14px;
    background: var(--el-color-primary);
    border-radius: 2px;
}
.count {
    min-width: 22px;
    padding: 0 6px;
    text-align: center;
    border-radius: 10px;
    font-size: 12px;
    line-height: 18px;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
}
.tools {
    display: flex;
    align-items: center;
    gap: 8px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 8px;
}
</style>
