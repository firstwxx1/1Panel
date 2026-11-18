<template>
    <div>
        <NodeTargetBar v-model="targetNode" @change="onNodeChange" />
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
                <el-table-column :label="$t('pro.waf.mod_geoRestrict')" width="90">
                    <template #default="{ row }">
                        <el-switch
                            v-model="row.config.geoRestrict.state"
                            active-value="on"
                            inactive-value="off"
                            @change="save(row)"
                        />
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

        <el-drawer v-model="drawer" :title="current?.domains?.join(', ') || current?.key" size="640px">
            <div v-if="current" class="drawer-body">
                <el-divider content-position="left">{{ $t('pro.waf.cdnTitle') }}</el-divider>
                <el-form label-width="140px">
                    <el-form-item :label="$t('pro.waf.cdnSwitch')">
                        <el-switch v-model="current.config.cdn.state" active-value="on" inactive-value="off" />
                    </el-form-item>
                </el-form>

                <el-divider content-position="left">{{ $t('pro.waf.accessRate') }}</el-divider>
                <el-form label-width="140px" v-if="current.config.cc">
                    <el-form-item :label="$t('pro.waf.mod_cc')">
                        <el-switch v-model="current.config.cc.state" active-value="on" inactive-value="off" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.mode')">
                        <el-select v-model="current.config.cc.mode" style="width: 220px">
                            <el-option value="uri" :label="$t('pro.waf.urlMode')" />
                            <el-option value="global" :label="$t('pro.waf.globalMode')" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.period')">
                        <el-input-number
                            v-model="current.config.cc.duration"
                            :min="1"
                            size="small"
                            controls-position="right"
                            style="width: 130px"
                        />
                        <span class="ml-2 text-xs">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.frequency')">
                        <el-input-number
                            v-model="current.config.cc.threshold"
                            :min="1"
                            size="small"
                            controls-position="right"
                            style="width: 130px"
                        />
                        <span class="ml-2 text-xs">{{ $t('pro.waf.times') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.action')">
                        <el-select v-model="current.config.cc.action" style="width: 220px">
                            <el-option v-for="a in ccActions" :key="a" :value="a" :label="$t('pro.waf.act_' + a)" />
                        </el-select>
                    </el-form-item>
                    <el-form-item
                        v-if="current.config.cc.action === 'five_seconds' || current.config.cc.action === 'captcha'"
                        :label="$t('pro.waf.tokenTimeOut')"
                    >
                        <el-input-number
                            v-model="current.config.cc.tokenTimeOut"
                            :min="10"
                            :max="86400"
                            size="small"
                            controls-position="right"
                            style="width: 130px"
                        />
                        <span class="ml-2 text-xs">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.autoBlockIp')">
                        <el-switch v-model="current.config.cc.ipBlock" active-value="on" inactive-value="off" />
                    </el-form-item>
                    <el-form-item v-if="current.config.cc.ipBlock !== 'off'" :label="$t('pro.waf.blockTime')">
                        <el-input-number
                            v-model="current.config.cc.ipBlockTime"
                            :min="1"
                            size="small"
                            controls-position="right"
                            style="width: 130px"
                        />
                        <span class="ml-2 text-xs">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                </el-form>

                <el-divider content-position="left">{{ $t('pro.waf.geoTitle') }}</el-divider>
                <GeoRestrict v-model="current.config.geoRestrict" site-mode />

                <el-divider content-position="left">{{ $t('pro.waf.basic') }}</el-divider>
                <el-form label-width="150px">
                    <el-form-item v-for="m in modules" :key="m.key" :label="label(m.key)">
                        <el-switch v-model="m.item.state" active-value="on" inactive-value="off" />
                        <span class="ml-3" v-if="m.item.step !== undefined">
                            <el-input-number
                                v-model="m.item.threshold"
                                :min="1"
                                size="small"
                                controls-position="right"
                                style="width: 100px"
                            />
                            <span class="ml-2 text-xs">
                                {{ $t('pro.waf.times') }} / {{ m.item.step }} {{ $t('pro.waf.second') }}
                            </span>
                        </span>
                    </el-form-item>
                </el-form>
            </div>
            <template #footer>
                <el-button @click="drawer = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="savingDetail" @click="saveDetail">
                    {{ $t('pro.waf.configSave') }}
                </el-button>
            </template>
        </el-drawer>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import NodeTargetBar from '@/xpack/components/NodeTargetBar.vue';
import { listWafSites, updateWafSite, WafSite } from '@/xpack/api/waf';
import GeoRestrict from './georestrict.vue';
import { normalizeGeoRestrict } from './geo-options';

const { t, te } = useI18n();
const globalStore = GlobalStore();
const targetNode = ref(globalStore.currentNode || 'local');
const sites = ref<WafSite[]>([]);
const keyword = ref('');
const loading = ref(false);
const savingDetail = ref(false);
const page = ref(1);
const size = ref(20);
const drawer = ref(false);
const current = ref<WafSite | null>(null);
const ccActions = ['deny', 'five_seconds', 'captcha', 'observe'];

const normalizeSite = (s: WafSite): WafSite => {
    const cfg = s.config || {};
    cfg.waf = { state: 'on', mode: 'protection', ...(cfg.waf || {}) };
    cfg.strict = { state: 'off', ...(cfg.strict || {}) };
    cfg.cc = {
        state: 'off',
        type: 'cc',
        mode: 'global',
        duration: 60,
        threshold: 120,
        action: 'deny',
        code: 403,
        ipBlock: 'on',
        ipBlockTime: 600,
        tokenTimeOut: 1200,
        ...(cfg.cc || {}),
    };
    cfg.cdn = { state: 'off', ...(cfg.cdn || {}) };
    cfg.geoRestrict = normalizeGeoRestrict(cfg.geoRestrict);
    return { ...s, config: cfg };
};

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
        const list = (await listWafSites(targetNode.value)).data || [];
        sites.value = list.map(normalizeSite);
    } catch {
        sites.value = [];
    } finally {
        loading.value = false;
    }
};

const onNodeChange = () => {
    page.value = 1;
    drawer.value = false;
    current.value = null;
    load();
};

const save = async (row: WafSite) => {
    await updateWafSite(
        row.key,
        {
            waf: row.config.waf,
            strict: row.config.strict,
            cc: row.config.cc,
            geoRestrict: normalizeGeoRestrict(row.config.geoRestrict),
        },
        targetNode.value,
    );
    ElMessage.success(t('pro.waf.saveSuccess'));
};

const DEDICATED_KEYS = new Set(['waf', 'strict', 'cc', 'cdn', 'geoRestrict']);

const modules = computed(() => {
    if (!current.value) return [];
    const cfg = current.value.config;
    return Object.keys(cfg)
        .filter((k) => cfg[k] && typeof cfg[k] === 'object' && 'state' in cfg[k] && !DEDICATED_KEYS.has(k))
        .sort()
        .map((k) => ({ key: k, item: cfg[k] }));
});

const saveDetail = async () => {
    if (!current.value) return;
    savingDetail.value = true;
    try {
        current.value.config.geoRestrict = normalizeGeoRestrict(current.value.config.geoRestrict);
        await updateWafSite(current.value.key, current.value.config, targetNode.value);
        ElMessage.success(t('pro.waf.saveSuccess'));
        drawer.value = false;
    } finally {
        savingDetail.value = false;
    }
};

const openDetail = (row: WafSite) => {
    current.value = normalizeSite(row);
    row.config = current.value.config;
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
.drawer-body {
    padding-right: 8px;
}
</style>
