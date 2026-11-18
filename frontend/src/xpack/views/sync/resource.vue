<template>
    <div>
        <el-card shadow="never">
            <template #header>
                <div class="card-head">
                    <div>
                        <div class="title">{{ $t('pro.sync.' + kind + 'Title') }}</div>
                        <div class="desc">{{ $t('pro.sync.' + kind + 'Desc') }}</div>
                    </div>
                    <el-tag v-if="selected.length" type="primary" effect="light">
                        {{ $t('pro.sync.selectedCount', { n: selected.length }) }}
                    </el-tag>
                </div>
            </template>

            <div class="route-strip mb-3">
                <div class="route-block">
                    <span class="route-label">{{ $t('pro.sync.source') }}</span>
                    <el-select v-model="src" style="width: 190px" @change="onSrc">
                        <el-option v-for="n in nodes" :key="n.name" :label="n.name" :value="n.name" />
                    </el-select>
                </div>

                <div class="route-arrow">
                    <el-icon><Right /></el-icon>
                </div>

                <div class="route-block route-targets">
                    <span class="route-label">{{ $t('pro.sync.targetNodes') }}</span>
                    <template v-if="targets.length">
                        <el-checkbox-group v-model="dsts" class="target-checks">
                            <el-checkbox v-for="n in targets" :key="n.name" :value="n.name" border size="small">
                                {{ n.name }}
                            </el-checkbox>
                        </el-checkbox-group>
                        <el-link
                            v-if="targets.length > 1"
                            type="primary"
                            :underline="false"
                            class="select-all-link"
                            @click="toggleAllTargets"
                        >
                            {{ dsts.length === targets.length ? $t('pro.sync.clearSelect') : $t('pro.sync.selectAll') }}
                        </el-link>
                    </template>
                    <el-tag v-else type="info" effect="plain" size="small">{{ $t('pro.sync.noTarget') }}</el-tag>
                </div>
            </div>

            <div class="table-toolbar">
                <el-input
                    v-model="keyword"
                    clearable
                    style="width: 260px; max-width: 100%"
                    :placeholder="$t('pro.sync.search')"
                >
                    <template #prefix>
                        <el-icon><Search /></el-icon>
                    </template>
                </el-input>
                <el-button :icon="Refresh" circle :loading="loading" @click="load" />
            </div>

            <el-table
                ref="tableRef"
                :data="pageRows"
                :class="{ 'hide-select-all': kind === 'app' }"
                v-loading="loading"
                @select="onSelect"
                @select-all="onSelectAll"
                @selection-change="(v: any[]) => (selected = v)"
            >
                <el-table-column type="selection" width="42" />
                <template v-if="kind === 'image'">
                    <el-table-column prop="id" label="ID" min-width="150" show-overflow-tooltip>
                        <template #default="{ row }">
                            <span class="mono">{{ row.id }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('pro.sync.tags')" min-width="220">
                        <template #default="{ row }">
                            <el-tag v-for="tg in row.tags" :key="tg" size="small" effect="light" class="mr-1 mb-1">
                                {{ tg }}
                            </el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('pro.sync.size')" width="120">
                        <template #default="{ row }">{{ fmtSize(row.size) }}</template>
                    </el-table-column>
                </template>
                <template v-else-if="kind === 'app'">
                    <el-table-column prop="name" :label="$t('pro.sync.name')" min-width="160" />
                    <el-table-column prop="appKey" :label="$t('pro.sync.appName')" min-width="140" />
                    <el-table-column prop="version" :label="$t('pro.sync.version')" width="140">
                        <template #default="{ row }">
                            <span class="mono">{{ row.version }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column prop="status" :label="$t('pro.sync.status')" width="120">
                        <template #default="{ row }">
                            <el-tag size="small" :type="row.status === 'Running' ? 'success' : 'info'" effect="light">
                                {{ row.status }}
                            </el-tag>
                        </template>
                    </el-table-column>
                </template>
                <template v-else>
                    <el-table-column prop="primaryDomain" :label="$t('pro.sync.domain')" min-width="200">
                        <template #default="{ row }">
                            <span class="mono">{{ row.primaryDomain }}</span>
                        </template>
                    </el-table-column>
                    <el-table-column prop="provider" :label="$t('pro.sync.provider')" width="140" />
                    <el-table-column prop="expireDate" :label="$t('pro.sync.expire')" width="180" />
                </template>
                <template #empty>
                    <el-empty :image-size="56" :description="$t('commons.msg.noneData')" />
                </template>
            </el-table>

            <el-pagination
                class="pager"
                v-model:current-page="page"
                v-model:page-size="size"
                :total="filtered.length"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next"
            />

            <div class="action-footer">
                <div class="footer-left">
                    <el-checkbox v-if="kind === 'app'" v-model="stopSrc">{{ $t('pro.sync.stopSrc') }}</el-checkbox>
                    <span v-else class="hint">{{ $t('pro.sync.targetDesc') }}</span>
                </div>
                <div class="footer-right">
                    <span v-if="!selected.length" class="hint">{{ $t('pro.sync.selectFirst') }}</span>
                    <el-button type="primary" :disabled="!selected.length || !dsts.length" :loading="busy" @click="run">
                        {{ $t('pro.sync.execute') }}
                    </el-button>
                </div>
            </div>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { Refresh, Right, Search } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { listAllNodes, nodeQuery, NodeRow } from '@/xpack/api/node';
import { syncApps, syncCerts, syncImages } from '@/xpack/api/sync';

const props = defineProps<{ kind: 'image' | 'cert' | 'app' }>();
const emit = defineEmits<{ (e: 'created'): void }>();
const { t } = useI18n();

const nodes = ref<NodeRow[]>([]);
const src = ref('');
const keyword = ref('');
const rows = ref<any[]>([]);
const selected = ref<any[]>([]);
const dsts = ref<string[]>([]);
const stopSrc = ref(true);
const tableRef = ref();
const loading = ref(false);
const busy = ref(false);
const page = ref(1);
const size = ref(10);

watch(keyword, () => {
    page.value = 1;
});

const fmtSize = (v: any) => {
    const n = Number(v);
    if (!n) return String(v ?? '-');
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + u[i];
};
const targets = computed(() => nodes.value.filter((n) => n.name !== src.value));
const toggleAllTargets = () => {
    if (dsts.value.length === targets.value.length) {
        dsts.value = [];
    } else {
        dsts.value = targets.value.map((n) => n.name);
    }
};

const filtered = computed(() => {
    const k = keyword.value.trim().toLowerCase();
    if (!k) return rows.value;
    return rows.value.filter((r) =>
        props.kind === 'image'
            ? (r.tags || []).join(' ').toLowerCase().includes(k) || String(r.id).includes(k)
            : props.kind === 'app'
              ? String(r.name).toLowerCase().includes(k)
              : String(r.primaryDomain).toLowerCase().includes(k),
    );
});
const pageRows = computed(() => filtered.value.slice((page.value - 1) * size.value, page.value * size.value));

// an app is transferred one at a time
const onSelect = (sel: any[], row: any) => {
    if (props.kind !== 'app') return;
    tableRef.value?.clearSelection();
    if (sel.includes(row)) tableRef.value?.toggleRowSelection(row, true);
};
const onSelectAll = () => {
    if (props.kind === 'app') {
        tableRef.value?.clearSelection();
    }
};
const load = async () => {
    if (!src.value) return;
    loading.value = true;
    try {
        if (props.kind === 'image') {
            const res = await nodeQuery(src.value, '/containers/image/search', {
                page: 1,
                pageSize: 1000,
                info: '',
                orderBy: 'createdAt',
                order: 'null',
            });
            rows.value = (res.data?.items || []).filter((i: any) => i.tags?.length);
        } else if (props.kind === 'app') {
            const res = await nodeQuery(src.value, '/apps/installed/search', {
                page: 1,
                pageSize: 1000,
                name: '',
                type: '',
                update: false,
                sync: false,
            });
            rows.value = (res.data?.items || []).map((i: any) => ({ ...i, appKey: i.appKey || i.app?.key || i.appName }));
        } else {
            const res = await nodeQuery(src.value, '/websites/ssl/search', { page: 1, pageSize: 1000 });
            rows.value = res.data?.items || [];
        }
    } catch {
        rows.value = [];
    } finally {
        loading.value = false;
    }
};
const onSrc = () => {
    dsts.value = [];
    selected.value = [];
    page.value = 1;
    load();
};
const run = async () => {
    busy.value = true;
    try {
        if (props.kind === 'image') {
            const allTags = [...new Set(selected.value.flatMap((i) => i.tags || []).filter(Boolean))];
            await syncImages({
                src: src.value,
                dsts: dsts.value,
                images: allTags,
            });
        } else if (props.kind === 'app') {
            await syncApps({ src: src.value, dsts: dsts.value, name: selected.value[0].name, stop: stopSrc.value });
        } else {
            await syncCerts({ src: src.value, dsts: dsts.value, ids: selected.value.map((c) => c.id) });
        }
        ElMessage.success(t('pro.sync.sent'));
        emit('created');
    } finally {
        busy.value = false;
    }
};

onMounted(async () => {
    const res = await listAllNodes();
    nodes.value = (res.data || []).filter((n) => n.kind !== 'panel' && n.status !== 'Pending');
    src.value = nodes.value[0]?.name || 'local';
    load();
});
</script>

<style scoped>
.card-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
}
.title {
    font-weight: 600;
    font-size: 15px;
    color: var(--el-text-color-primary);
}
.desc,
.hint {
    color: var(--el-text-color-secondary);
    font-size: 12.5px;
    margin-top: 3px;
}

.route-strip {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 14px;
    padding: 12px 16px;
    border-radius: 6px;
    background: var(--el-fill-color-lighter);
    border: 1px solid var(--el-border-color-lighter);
}
.route-block {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
}
.route-targets {
    flex: 1;
    min-width: 240px;
}
.route-label {
    font-size: 13px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    white-space: nowrap;
}
.route-arrow {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
    flex-shrink: 0;
}
.target-checks {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
}
.target-checks :deep(.el-checkbox) {
    margin-right: 0;
    background: var(--el-bg-color-overlay);
}
.select-all-link {
    font-size: 12.5px;
}

.table-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 10px;
}
.hide-select-all :deep(th.el-table-column--selection .el-checkbox) {
    display: none;
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.mr-1 {
    margin-right: 4px;
}
.mb-1 {
    margin-bottom: 4px;
}
.pager {
    margin-top: 12px;
    justify-content: flex-end;
}
.action-footer {
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--el-border-color-lighter);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.footer-left,
.footer-right {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
}
</style>
