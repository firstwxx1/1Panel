<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="head">
                <div class="title">{{ $t('pro.sync.' + (kind === 'image' ? 'imageTitle' : 'certTitle')) }}</div>
                <div class="desc">{{ $t('pro.sync.' + (kind === 'image' ? 'imageDesc' : 'certDesc')) }}</div>
            </div>
            <div class="bar">
                <el-select v-model="src" style="width: 180px" @change="onSrc">
                    <el-option v-for="n in nodes" :key="n.name" :label="n.name" :value="n.name" />
                </el-select>
                <el-input v-model="keyword" clearable style="width: 240px" :placeholder="$t('pro.sync.search')" />
                <el-button :icon="Refresh" @click="load" />
            </div>
            <el-table :data="pageRows" v-loading="loading" @selection-change="(v: any[]) => (selected = v)">
                <el-table-column type="selection" width="40" />
                <template v-if="kind === 'image'">
                    <el-table-column prop="id" label="ID" min-width="140" show-overflow-tooltip />
                    <el-table-column :label="$t('pro.sync.tags')" min-width="200">
                        <template #default="{ row }">
                            <el-tag v-for="tg in row.tags" :key="tg" class="mr-1">{{ tg }}</el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('pro.sync.size')" width="120">
                        <template #default="{ row }">{{ fmtSize(row.size) }}</template>
                    </el-table-column>
                </template>
                <template v-else>
                    <el-table-column prop="primaryDomain" :label="$t('pro.sync.domain')" min-width="200" />
                    <el-table-column prop="provider" :label="$t('pro.sync.provider')" width="140" />
                    <el-table-column prop="expireDate" :label="$t('pro.sync.expire')" width="180" />
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
        </el-card>
        <el-card shadow="never">
            <div class="head">
                <div class="title">{{ $t('pro.sync.targetNodes') }}</div>
                <div class="desc">{{ $t('pro.sync.targetDesc') }}</div>
            </div>
            <el-empty v-if="!targets.length" :description="$t('pro.sync.noTarget')" :image-size="60" />
            <el-checkbox-group v-else v-model="dsts">
                <el-checkbox v-for="n in targets" :key="n.name" :value="n.name" border>{{ n.name }}</el-checkbox>
            </el-checkbox-group>
            <div class="foot">
                <span v-if="!selected.length" class="hint">{{ $t('pro.sync.selectFirst') }}</span>
                <el-button type="primary" :disabled="!selected.length || !dsts.length" :loading="busy" @click="run">
                    {{ $t('pro.sync.execute') }}
                </el-button>
            </div>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import { listAllNodes, nodeQuery, NodeRow } from '@/xpack/api/node';
import { syncCerts, syncImages } from '@/xpack/api/sync';
import i18n from '@/lang';

const props = defineProps<{ kind: 'image' | 'cert' }>();
const emit = defineEmits<{ (e: 'created'): void }>();
const t = i18n.global.t;

const nodes = ref<NodeRow[]>([]);
const src = ref('');
const keyword = ref('');
const rows = ref<any[]>([]);
const selected = ref<any[]>([]);
const dsts = ref<string[]>([]);
const loading = ref(false);
const busy = ref(false);
const page = ref(1);
const size = ref(10);

const fmtSize = (v: any) => {
    const n = Number(v);
    if (!n) return String(v ?? '-');
    const u = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    return (n / Math.pow(1024, i)).toFixed(i ? 1 : 0) + ' ' + u[i];
};
const targets = computed(() => nodes.value.filter((n) => n.name !== src.value));
const filtered = computed(() => {
    const k = keyword.value.trim().toLowerCase();
    if (!k) return rows.value;
    return rows.value.filter((r) =>
        props.kind === 'image'
            ? (r.tags || []).join(' ').toLowerCase().includes(k) || String(r.id).includes(k)
            : String(r.primaryDomain).toLowerCase().includes(k),
    );
});
const pageRows = computed(() => filtered.value.slice((page.value - 1) * size.value, page.value * size.value));

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
            await syncImages({
                src: src.value,
                dsts: dsts.value,
                images: selected.value.map((i) => i.tags[0]),
            });
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
.head {
    margin-bottom: 12px;
}
.title {
    font-weight: 600;
}
.desc,
.hint {
    color: var(--el-text-color-secondary);
    font-size: 13px;
}
.bar {
    display: flex;
    gap: 8px;
    margin-bottom: 10px;
}
.pager {
    margin-top: 10px;
    justify-content: flex-end;
}
.foot {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 12px;
    margin-top: 12px;
}
</style>
