<template>
    <el-card shadow="never" class="panel">
        <template #header>
            <div class="pane-head">
                <div class="pane-title">
                    <span class="pane-dot" :class="side" />
                    <span>{{ side === 'right' ? $t('pro.sync.rightPane') : $t('pro.sync.leftPane') }}</span>
                    <el-tag v-if="selected.length" size="small" type="primary" effect="light">
                        {{ $t('pro.sync.selectedCount', { n: selected.length }) }}
                    </el-tag>
                </div>
                <span v-if="currentNodeAddr" class="pane-addr mono">{{ currentNodeAddr }}</span>
            </div>
        </template>

        <div class="bar">
            <el-select v-model="node" style="width: 170px; flex-shrink: 0" @change="goHome">
                <el-option v-for="n in nodes" :key="n.name" :label="n.name" :value="n.name" />
            </el-select>
            <el-input
                v-model="keyword"
                clearable
                :placeholder="$t('pro.sync.search')"
                @keyup.enter="load"
                @clear="load"
            />
            <el-button :icon="Refresh" circle :loading="loading" @click="load" />
        </div>

        <div class="crumb mono">
            <el-icon class="crumb-icon"><FolderOpened /></el-icon>
            <el-link type="primary" :underline="false" @click="goHome">/</el-link>
            <template v-for="(seg, i) in segments" :key="i">
                <span class="crumb-sep">/</span>
                <el-link type="primary" :underline="false" @click="goSeg(i)">{{ seg }}</el-link>
            </template>
        </div>

        <el-table :data="rows" v-loading="loading" height="380" @selection-change="(v: Row[]) => (selected = v)">
            <el-table-column type="selection" width="42" />
            <el-table-column :label="$t('pro.sync.name')" min-width="180" show-overflow-tooltip>
                <template #default="{ row }">
                    <el-link v-if="row.isDir" type="primary" :underline="false" class="entry-link" @click="open(row)">
                        <el-icon class="dir-icon"><Folder /></el-icon>
                        <span>{{ row.name }}</span>
                    </el-link>
                    <span v-else class="entry-file">
                        <el-icon class="file-icon"><Document /></el-icon>
                        <span>{{ row.name }}</span>
                    </span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.size')" width="100">
                <template #default="{ row }">
                    <span class="size-cell">{{ row.isDir ? '-' : fmt(row.size) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.sync.operation')" width="105" fixed="right">
                <template #default="{ row }">
                    <el-button link type="primary" @click="$emit('send', [row])">
                        {{ $t('pro.sync.transferTo') }}
                    </el-button>
                </template>
            </el-table-column>
            <template #empty>
                <el-empty :image-size="48" :description="$t('commons.msg.noneData')" />
            </template>
        </el-table>

        <el-pagination
            class="pager"
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            small
            @change="load"
        />
    </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { Document, Folder, FolderOpened, Refresh } from '@element-plus/icons-vue';
import { nodeQuery, NodeRow } from '@/xpack/api/node';

export interface Row {
    name: string;
    path: string;
    isDir: boolean;
    size: number;
}

const props = withDefaults(
    defineProps<{
        nodes: NodeRow[];
        initNode: string;
        side?: 'left' | 'right';
    }>(),
    {
        side: 'left',
    },
);
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

const currentNodeAddr = computed(() => {
    const found = props.nodes.find((n) => n.name === node.value);
    return found?.addr || (found?.isLocal ? '127.0.0.1' : '');
});

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
defineExpose({ node, path, selected, load, goHome });
</script>

<style scoped>
.panel {
    flex: 1;
    min-width: 0;
}
.pane-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
}
.pane-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    font-size: 14px;
    color: var(--el-text-color-primary);
}
.pane-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--el-color-primary);
}
.pane-dot.right {
    background: var(--el-color-success);
}
.pane-addr {
    font-size: 12px;
    padding: 2px 7px;
    border-radius: 4px;
    color: var(--el-text-color-secondary);
    background: var(--el-fill-color-light);
}
.bar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
}
.crumb {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 6px 10px;
    margin-bottom: 10px;
    border-radius: 4px;
    font-size: 12.5px;
    background: var(--el-fill-color-lighter);
    border: 1px solid var(--el-border-color-extra-light);
    flex-wrap: wrap;
}
.crumb-icon {
    color: var(--el-color-warning);
    margin-right: 2px;
}
.crumb-sep {
    color: var(--el-text-color-placeholder);
}
.entry-link,
.entry-file {
    display: inline-flex;
    align-items: center;
    gap: 6px;
}
.dir-icon {
    color: var(--el-color-warning);
    font-size: 15px;
    flex-shrink: 0;
}
.file-icon {
    color: var(--el-text-color-secondary);
    font-size: 14px;
    flex-shrink: 0;
}
.size-cell {
    font-variant-numeric: tabular-nums;
    font-size: 12.5px;
    color: var(--el-text-color-regular);
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.pager {
    margin-top: 10px;
    justify-content: flex-end;
}
</style>
