<template>
    <div>
        <div class="bar">
            <el-radio-group v-model="cls" @change="reload">
                <el-radio-button value="">{{ $t('pro.monitor.all') }}</el-radio-button>
                <el-radio-button value="4xx">4xx</el-radio-button>
                <el-radio-button value="5xx">5xx</el-radio-button>
            </el-radio-group>
        </div>
        <el-table :data="items" v-loading="loading">
            <el-table-column prop="status" :label="$t('pro.monitor.status')" width="110">
                <template #default="{ row }">
                    <el-tag :type="row.status >= 500 ? 'danger' : 'warning'" effect="light">{{ row.status }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column prop="url" label="URL" min-width="300" show-overflow-tooltip />
            <el-table-column prop="count" :label="$t('pro.monitor.count')" width="120" />
            <el-table-column :label="$t('pro.monitor.last')" width="180">
                <template #default="{ row }">{{ fmt(row.last) }}</template>
            </el-table-column>
            <template #empty>{{ $t('pro.monitor.noData') }}</template>
        </el-table>
        <div class="pager">
            <el-pagination
                v-model:current-page="page"
                v-model:page-size="size"
                :total="total"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next"
                @change="load"
            />
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { monErrors } from '@/xpack/api/monitor';

const props = defineProps<{ site: string; range: string }>();
const cls = ref('');
const items = ref<any[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);

const pad = (n: number) => String(n).padStart(2, '0');
const fmt = (ts: number) => {
    if (!ts) return '-';
    const d = new Date(ts * 1000);
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
};
const load = async () => {
    loading.value = true;
    try {
        const res = await monErrors({ site: props.site, range: props.range, class: cls.value, page: page.value, size: size.value });
        items.value = res.data.items || [];
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};
const reload = () => {
    page.value = 1;
    load();
};
onMounted(load);
</script>

<style scoped>
.bar {
    margin-bottom: 12px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
