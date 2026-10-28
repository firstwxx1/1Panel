<template>
    <div>
        <el-card shadow="never" class="mb-3">
            <div class="bar">
                <SiteSelect v-model="site" :sites="sites" all @update:model-value="load" />
                <el-date-picker
                    v-model="dates"
                    type="daterange"
                    style="width: 320px; flex: none"
                    :clearable="false"
                    :start-placeholder="$t('pro.monitor.startDate')"
                    :end-placeholder="$t('pro.monitor.endDate')"
                    @change="load"
                />
            </div>
        </el-card>
        <el-row :gutter="12" class="mb-3">
            <el-col v-for="d in first" :key="d.dim" :span="8">
                <el-card shadow="never" class="list">
                    <template #header>
                        <div class="head">
                            <span class="card-title">{{ d.title }}</span>
                            <el-link type="primary" :underline="false" @click="more(d)">{{ $t('pro.monitor.more') }}</el-link>
                        </div>
                    </template>
                    <List :items="d.items" />
                </el-card>
            </el-col>
        </el-row>
        <el-row :gutter="12">
            <el-col v-for="d in second" :key="d.dim" :span="6">
                <el-card shadow="never" class="list">
                    <template #header>
                        <div class="head">
                            <span class="card-title">{{ d.title }}</span>
                            <el-link v-if="d.more" type="primary" :underline="false" @click="more(d)">{{ $t('pro.monitor.more') }}</el-link>
                        </div>
                    </template>
                    <List :items="d.items" />
                </el-card>
            </el-col>
        </el-row>
        <el-dialog v-model="dialog" :title="dialogTitle" width="560px">
            <List :items="dialogItems" />
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue';
import SiteSelect from './SiteSelect.vue';
import { monDistribution, MonDistItem, MonSiteItem } from '@/xpack/api/monitor';
import { dayParams, todayRange } from './util';
import i18n from '@/lang';

defineProps<{ sites: MonSiteItem[] }>();
const t = i18n.global.t;
const site = ref('');
const dates = ref<Date[]>(todayRange());

interface Block {
    dim: string;
    title: string;
    more: boolean;
    items: MonDistItem[];
}
const spec = {
    first: [
        { dim: 'url', key: 'url', more: true },
        { dim: 'referer', key: 'source', more: true },
        { dim: 'ip', key: 'ipTop', more: true },
    ],
    second: [
        { dim: 'browser', key: 'browser', more: true },
        { dim: 'os', key: 'os', more: false },
        { dim: 'device', key: 'device', more: false },
        { dim: 'status', key: 'status', more: false },
    ],
};
const mk = (list: typeof spec.first): Block[] => list.map((s) => ({ dim: s.dim, title: t('pro.monitor.' + s.key), more: s.more, items: [] }));
const first = ref<Block[]>(mk(spec.first));
const second = ref<Block[]>(mk(spec.second));

const List = (p: { items: MonDistItem[] }) =>
    p.items.length
        ? h(
              'div',
              p.items.map((it) =>
                  h('div', { class: 'row' }, [h('span', { class: 'name', title: it.name }, it.name), h('span', String(it.count))]),
              ),
          )
        : h('div', { class: 'empty' }, t('pro.monitor.noData'));

const query = (dim: string, limit: number) => monDistribution({ site: site.value, dim, limit, ...dayParams(dates.value) });
const load = async () => {
    const all = [...first.value, ...second.value];
    const res = await Promise.all(all.map((b) => query(b.dim, 10)));
    all.forEach((b, i) => (b.items = res[i].data || []));
};

const dialog = ref(false);
const dialogTitle = ref('');
const dialogItems = ref<MonDistItem[]>([]);
const more = async (b: Block) => {
    dialogTitle.value = b.title;
    dialogItems.value = (await query(b.dim, 100)).data || [];
    dialog.value = true;
};
onMounted(load);
</script>

<style scoped>
.bar {
    display: flex;
    gap: 12px;
}
.head {
    display: flex;
    justify-content: space-between;
    align-items: center;
}
.list {
    min-height: 300px;
}
:deep(.row) {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 0;
    font-size: 13px;
    border-bottom: 1px solid var(--el-border-color-extra-light);
}
:deep(.name) {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
}
:deep(.empty) {
    padding: 90px 0;
    text-align: center;
    color: var(--el-text-color-placeholder);
}
</style>
