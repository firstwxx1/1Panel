<template>
    <div>
        <el-card>
            <div class="toolbar">
                <el-input v-model="keyword" :placeholder="$t('pro.waf.searchSite')" clearable style="width: 320px">
                    <template #prefix><el-icon><Search /></el-icon></template>
                </el-input>
                <el-button :icon="Refresh" @click="load" />
            </div>
            <el-table :data="paged" v-loading="loading" size="small">
                <el-table-column :label="$t('pro.waf.website')" min-width="200">
                    <template #default="{ row }">{{ (row.domains || []).join(', ') || row.key }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.remark')" min-width="120">
                    <template #default="{ row }">{{ row.remark || '-' }}</template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.switch')" width="80">
                    <template #default="{ row }">
                        <el-switch v-model="row.config.waf.state" active-value="on" inactive-value="off" @change="save(row)" />
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.policy')" width="210">
                    <template #default="{ row }">
                        <el-radio-group v-model="row.config.waf.mode" size="small" @change="save(row)">
                            <el-radio-button value="protection">{{ $t('pro.waf.protection') }}</el-radio-button>
                            <el-radio-button value="observation">{{ $t('pro.waf.observation') }}</el-radio-button>
                        </el-radio-group>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.strength')" width="190">
                    <template #default="{ row }">
                        <el-radio-group v-model="row.config.strict.state" size="small" @change="save(row)">
                            <el-radio-button value="off">{{ $t('pro.waf.standard') }}</el-radio-button>
                            <el-radio-button value="on">{{ $t('pro.waf.strictMode') }}</el-radio-button>
                        </el-radio-group>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.waf.rateLimit')" width="90">
                    <template #default="{ row }">
                        <el-switch v-model="row.config.cc.state" active-value="on" inactive-value="off" @change="save(row)" />
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.operate')" width="120">
                    <template #default="{ row }">
                        <el-button link type="primary" @click="openDetail(row)">{{ $t('pro.waf.detail') }}</el-button>
                    </template>
                </el-table-column>
            </el-table>
            <div class="pager">
                <el-pagination
                    v-model:current-page="page"
                    v-model:page-size="size"
                    :total="filtered.length"
                    :page-sizes="[20, 50, 100]"
                    layout="total, sizes, prev, pager, next, jumper"
                    small
                />
            </div>
        </el-card>

        <el-drawer v-model="drawer" :title="current?.domains?.join(', ') || current?.key" size="560px">
            <el-form label-width="150px" v-if="current">
                <el-form-item v-for="m in modules" :key="m.key" :label="label(m.key)">
                    <el-switch v-model="m.item.state" active-value="on" inactive-value="off" @change="saveDetail" />
                    <span class="ml-3" v-if="m.item.step !== undefined">
                        <el-input-number v-model="m.item.threshold" :min="1" size="small" controls-position="right" style="width: 100px" @change="saveDetail" />
                        <span class="ml-2 text-xs">{{ $t('pro.waf.times') }} / {{ m.item.step }}
                            {{ $t('pro.waf.second') }}</span>
                    </span>
                </el-form-item>
                <el-form-item :label="$t('pro.waf.blockTime')" v-if="current.cc">
                    <el-input-number v-model="current.cc.ipBlockTime" :min="1" size="small" controls-position="right" style="width: 120px" />
                    <span class="ml-2 text-xs">{{ $t('pro.waf.second') }}</span>
                    <el-button class="ml-3" size="small" type="primary" @click="saveDetail">{{ $t('pro.waf.configSave') }}</el-button>
                </el-form-item>
            </el-form>
        </el-drawer>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { listWafSites, updateWafSite, WafSite } from '@/xpack/api/waf';

const { t, te } = useI18n();
const sites = ref<WafSite[]>([]);
const keyword = ref('');
const loading = ref(false);
const page = ref(1);
const size = ref(20);
const drawer = ref(false);
const current = ref<WafSite | null>(null);

const filtered = computed(() => {
    const k = keyword.value.trim().toLowerCase();
    if (!k) return sites.value;
    return sites.value.filter((s) =>
        [s.key, s.remark, ...(s.domains || [])].some((v) => (v || '').toLowerCase().includes(k)),
    );
});

const paged = computed(() => filtered.value.slice((page.value - 1) * size.value, page.value * size.value));

const label = (key: string) => (te('pro.waf.mod_' + key) ? t('pro.waf.mod_' + key) : key);

const load = async () => {
    loading.value = true;
    try {
        sites.value = (await listWafSites()).data || [];
    } finally {
        loading.value = false;
    }
};

const save = async (row: WafSite) => {
    await updateWafSite(row.key, { waf: row.config.waf, strict: row.config.strict, cc: row.config.cc });
    ElMessage.success(t('pro.waf.saveSuccess'));
};

const modules = computed(() => {
    if (!current.value) return [];
    const cfg = current.value.config;
    return Object.keys(cfg)
        .filter((k) => cfg[k] && typeof cfg[k] === 'object' && 'state' in cfg[k] && !['waf', 'strict', 'cc'].includes(k))
        .sort()
        .map((k) => ({ key: k, item: cfg[k] }));
});

const saveDetail = async () => {
    if (!current.value) return;
    await updateWafSite(current.value.key, current.value.config);
    ElMessage.success(t('pro.waf.saveSuccess'));
};

const openDetail = (row: WafSite) => {
    current.value = row;
    drawer.value = true;
};

onMounted(load);
</script>

<style scoped>
.toolbar {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    margin-bottom: 12px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
