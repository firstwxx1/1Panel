<template>
    <el-card shadow="never" class="panel">
        <div class="bar">
            <el-select v-model="node" style="width: 160px" @change="goHome">
                <el-option v-for="n in nodes" :key="n.name" :label="n.name" :value="n.name" />
            </el-select>
            <el-input v-model="keyword" clearable :placeholder="$t('pro.sync.search')" @keyup.enter="load" @clear="load" />
            <el-button :icon="Refresh" @click="load" />
        </div>
        <div class="crumb">
            <el-link type="primary" @click="goHome">/</el-link>
            <template v-for="(seg, i) in segments" :key="i">
                <span>/</span>
                <el-link type="primary" @click="goSeg(i)">{{ seg }}</el-link>
            </template>
        </div>
        <el-table :data="rows" v-loading="loading" height="360" @selection-change="(v: Row[]) => (selected = v)">
            <el-table-column type="selection" width="40" />
            <el-table-column :label="$t('pro.sync.name')" min-width="180">
                <template #default="{ row }">
                    <el-link v-if="row.isDir" type="primary" @click="open(row)">📁 {{ row.name }}</el-link>
                    <span v-else>{{ row.name }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.size')" width="100">
                <template #default="{ row }">{{ row.isDir ? '-' : fmt(row.size) }}</template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.operation')" width="110">
                <template #default="{ row }">
                    <el-button link type="primary" @click="$emit('send', [row])">{{ $t('pro.sync.transferTo') }}</el-button>
                </template>
            </el-table-column>
        </el-table>
        <el-pagination
            class="pager"
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @change="load"
        />
    </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { nodeQuery, NodeRow } from '@/xpack/api/node';

interface Row {
    name: string;
    path: string;
    isDir: boolean;
    size: number;
}

const props = defineProps<{ nodes: NodeRow[]; initNode: string }>();
defineEmits<{ (e: 'send', rows: Row[]): void }>();

const node = ref(props.initNode);
const path = ref('/');
const keyword = ref('');
const rows = ref<Row[]>([]);
const selected = ref<Row[]>([]);
const loading = ref(false);
const page = ref(1);
const size = ref(50);
const total = ref(0);

const segments = computed(() => path.value.split('/').filter(Boolean));
const fmt = (n: number) => {
    if (!n) return '0 B';
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + u[i];
};

const load = async () => {
    loading.value = true;
    try {
        const res = await nodeQuery(node.value, '/files/search', {
            path: path.value,
            expand: true,
            showHidden: true,
            search: keyword.value,
            page: page.value,
            pageSize: size.value,
            sortBy: 'name',
            sortOrder: 'ascending',
        });
        rows.value = res.data?.items || [];
        total.value = res.data?.itemTotal || 0;
        // the agent reports the resolved path; keep ours in sync
        path.value = res.data?.path || path.value;
    } catch {
        rows.value = [];
        total.value = 0;
    } finally {
        loading.value = false;
    }
};
const goHome = () => {
    path.value = '/';
    page.value = 1;
    load();
};
const goSeg = (i: number) => {
    path.value = '/' + segments.value.slice(0, i + 1).join('/');
    page.value = 1;
    load();
};
const open = (r: Row) => {
    path.value = r.path;
    page.value = 1;
    keyword.value = '';
    load();
};

watch(
    () => props.initNode,
    (v) => {
        if (!node.value) node.value = v;
    },
);
onMounted(load);
defineExpose({ node, path, selected, load });
</script>

<style scoped>
.panel {
    flex: 1;
    min-width: 0;
}
.bar {
    display: flex;
    gap: 8px;
    margin-bottom: 10px;
}
.crumb {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 8px;
    flex-wrap: wrap;
}
.pager {
    margin-top: 10px;
    justify-content: flex-end;
}
</style>
