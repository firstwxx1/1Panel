<template>
    <div>
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
                    <el-switch v-model="wafState" active-value="on" inactive-value="off" size="large" @change="saveMaster" />
                </div>
            </div>
        </el-card>
        <el-alert :title="$t('pro.waf.globalNotice')" type="info" :closable="false" class="mb-3" />
        <el-tabs v-model="tab">
            <el-tab-pane :label="$t('pro.waf.tabRate')" name="rate">
                <RateLimit :config="config" @reload="load" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabDefault')" name="rules">
                <DefaultRules :config="config" @reload="load" />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabCustom')" name="custom">
                <CustomRules />
            </el-tab-pane>
            <el-tab-pane :label="$t('pro.waf.tabConfig')" name="config">
                <ConfigTab :config="config" @reload="load" />
            </el-tab-pane>
        </el-tabs>
    </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { getWafGlobal, updateWafGlobal } from '@/xpack/api/waf';
import RateLimit from './ratelimit.vue';
import DefaultRules from './defaultrules.vue';
import CustomRules from './customrules.vue';
import ConfigTab from './config.vue';

const { t } = useI18n();
const config = ref<Record<string, any>>({});
const tab = ref('rate');
const wafState = ref('on');
const enabled = computed(() => wafState.value === 'on');

const load = async () => {
    config.value = (await getWafGlobal()).data || {};
    wafState.value = config.value.waf?.state || 'off';
};

const saveMaster = async () => {
    config.value.waf = { ...(config.value.waf || {}), state: wafState.value };
    await updateWafGlobal({ waf: config.value.waf });
    ElMessage.success(t('pro.waf.saveSuccess'));
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
