<template>
    <div v-loading="loading">
        <NodeTargetBar v-model="targetNode" @change="load">
            <template #extra="{ nodes }">
                <el-button
                    v-if="nodes.length > 1"
                    size="small"
                    type="primary"
                    plain
                    :loading="syncingAll"
                    @click="syncToAllNodes(nodes)"
                >
                    {{ $t('pro.waf.syncAllNodes') }}
                </el-button>
            </template>
        </NodeTargetBar>

        <el-card class="mb-3">
            <div class="master">
                <div>
                    <span class="title">{{ $t('pro.waf.masterTitle') }}</span>
                    <el-tag :type="enabled ? 'success' : 'info'" size="small" class="ml-2">
                        {{ enabled ? $t('pro.waf.enabled') : $t('pro.waf.disabled') }}
                    </el-tag>
                    <div class="note">{{ $t('pro.waf.masterNote') }}</div>
                </div>
                <div class="master-right">
                    <span class="state">{{ enabled ? $t('pro.waf.on') : $t('pro.waf.off') }}</span>
                    <el-switch
                        v-model="wafState"
                        active-value="on"
                        inactive-value="off"
                        size="large"
                        @change="saveMaster"
                    />
                </div>
            </div>
        </el-card>
        <el-alert :title="$t('pro.waf.globalNotice')" type="info" :closable="false" class="mb-3" />
        <el-tabs v-model="tab">
            <el-tab-pane :label="$t('pro.waf.tabRate')" name="rate">
                <RateLimit :config="config" :node="targetNode" @reload="load" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabDefault')" name="rules">
                <DefaultRules :config="config" :node="targetNode" @reload="load" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabGeo')" name="geo">
                <GeoRestrict :config="config" :node="targetNode" @reload="load" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabCustom')" name="custom">
                <CustomRules :node="targetNode" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabConfig')" name="config">
                <ConfigTab :config="config" :node="targetNode" @reload="load" />
            </el-tab-pane>
        </el-tabs>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { useI18n } from 'vue-i18n';
import http from '@/api';
import { GlobalStore } from '@/store';
import NodeTargetBar from '@/xpack/components/NodeTargetBar.vue';
import { NodeRow } from '@/xpack/api/node';
import { getWafGlobal, updateWafGlobal } from '@/xpack/api/waf';
import RateLimit from './ratelimit.vue';
import DefaultRules from './defaultrules.vue';
import GeoRestrict from './georestrict.vue';
import CustomRules from './customrules.vue';
import ConfigTab from './config.vue';

const { t } = useI18n();
const globalStore = GlobalStore();
const targetNode = ref(globalStore.currentNode || 'local');
const config = ref<Record<string, any>>({});
const tab = ref('rate');
const wafState = ref('on');
const loading = ref(false);
const syncingAll = ref(false);
const enabled = computed(() => wafState.value === 'on');

const SITE_APPLY_KEYS = [
    'cc',
    'urlcc',
    'sql',
    'xss',
    'args',
    'cookie',
    'header',
    'fileExt',
    'methodWhite',
    'geoRestrict',
    'defaultUaBlack',
    'defaultUrlBlack',
    'defaultIpBlack',
    'vuln',
    'allowSpider',
    'cdn',
    'strict',
];

const load = async () => {
    loading.value = true;
    try {
        config.value = (await getWafGlobal(targetNode.value)).data || {};
        wafState.value = config.value.waf?.state || 'off';
    } catch {
        config.value = {};
        wafState.value = 'off';
    } finally {
        loading.value = false;
    }
};

const saveMaster = async () => {
    config.value.waf = { ...(config.value.waf || {}), state: wafState.value };
    await updateWafGlobal({ waf: config.value.waf }, targetNode.value);
    ElMessage.success(t('pro.waf.saveSuccess'));
};

const syncToAllNodes = async (nodes: NodeRow[]) => {
    await ElMessageBox.confirm(t('pro.waf.syncAllConfirm'), t('pro.waf.syncAllNodes'), { type: 'warning' });
    syncingAll.value = true;
    try {
        // Keep each node's own OpenResty WAF auth secret untouched
        const patch: Record<string, any> = { ...config.value };
        if (patch.waf && typeof patch.waf === 'object') {
            const { secret: _secret, ...wafRest } = patch.waf;
            patch.waf = wafRest;
        }
        const keysToApply = SITE_APPLY_KEYS.filter((k) => patch[k] !== undefined);
        let ok = 0;
        const failed: string[] = [];
        for (const n of nodes) {
            const nodeName = n.isLocal ? 'local' : n.name;
            try {
                await http.postWithConfig(
                    '/xpack/waf/global/update',
                    { patch },
                    { headers: { CurrentNode: nodeName }, skipErrorMessage: true },
                );
                if (keysToApply.length) {
                    await http.postWithConfig(
                        '/xpack/waf/global/apply',
                        { keys: keysToApply },
                        { headers: { CurrentNode: nodeName }, skipErrorMessage: true },
                    );
                }
                ok++;
            } catch {
                failed.push(nodeName);
            }
        }
        if (!failed.length) {
            ElMessage.success(t('pro.waf.syncAllSuccess', { ok }));
        } else {
            ElMessage.warning(t('pro.waf.syncAllPartial', { ok, fail: failed.length, names: failed.join(', ') }));
        }
    } finally {
        syncingAll.value = false;
    }
};

onMounted(load);
</script>

<style scoped>
.master {
    display: flex;
    align-items: center;
    justify-content: space-between;
}
.master-right {
    display: flex;
    align-items: center;
    gap: 10px;
}
.state {
    font-size: 13px;
    color: var(--el-text-color-regular);
}
.title {
    font-size: 16px;
    font-weight: 600;
}
.note {
    margin-top: 6px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
