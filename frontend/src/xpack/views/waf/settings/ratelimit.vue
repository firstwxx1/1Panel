<template>
    <el-row :gutter="12">
        <el-col :span="6">
            <el-menu :default-active="active" class="menu">
                <el-menu-item v-for="m in menus" :key="m.key" :index="m.key" @click="active = m.key">
                    {{ $t('pro.waf.' + m.label) }}
                </el-menu-item>
            </el-menu>
        </el-col>
        <el-col :span="18">
            <div v-if="item" class="panel">
                <el-switch v-model="item.state" active-value="on" inactive-value="off" size="large" />
                <div class="summary">
                    {{ summary }}
                    <el-tag size="small" type="primary" effect="light" class="ml-2">{{ $t('pro.waf.siteTag') }}</el-tag>
                </div>
                <el-form label-width="120px" class="mt-4">
                    <el-form-item v-if="active === 'cc'" :label="$t('pro.waf.mode')" required>
                        <el-select v-model="item.mode" style="width: 240px">
                            <el-option value="uri" :label="$t('pro.waf.urlMode')" />
                            <el-option value="global" :label="$t('pro.waf.globalMode')" />
                        </el-select>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.period')" required>
                        <el-input-number v-model="item.duration" :min="1" controls-position="right" style="width: 140px" />
                        <span class="unit">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.frequency')" required>
                        <el-input-number v-model="item.threshold" :min="1" controls-position="right" style="width: 140px" />
                        <span class="unit">{{ $t('pro.waf.times') }}</span>
                    </el-form-item>
                    <el-form-item :label="$t('pro.waf.blockTime')" required>
                        <el-input-number v-model="item.ipBlockTime" :min="1" controls-position="right" style="width: 140px" />
                        <span class="unit">{{ $t('pro.waf.second') }}</span>
                    </el-form-item>
                    <el-form-item>
                        <el-button type="primary" @click="save(false)">{{ $t('pro.waf.saveDefault') }}</el-button>
                        <el-button type="primary" plain @click="save(true)">{{ $t('pro.waf.applyToSites') }}</el-button>
                    </el-form-item>
                </el-form>
                <div class="note" v-if="active === 'cc'">{{ $t('pro.waf.globalModeNote') }}<br />{{ $t('pro.waf.urlModeNote') }}</div>
                <div class="warn">{{ $t('pro.waf.blockWarning') }}</div>
            </div>
        </el-col>
    </el-row>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { applyWafGlobal, updateWafGlobal } from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any> }>();
const emit = defineEmits<{ reload: [] }>();
const { t } = useI18n();

const menus = [
    { key: 'cc', label: 'accessRate' },
    { key: 'attackCount', label: 'attackRate' },
    { key: 'notFoundCount', label: 'notFoundRate' },
    { key: 'urlcc', label: 'urlRate' },
];
const active = ref('cc');
const item = computed(() => props.config[active.value]);

const target = computed(() => (active.value === 'cc' && item.value?.mode === 'uri' ? t('pro.waf.targetUrl') : t('pro.waf.targetAny')));

const humanTime = (sec: number) => {
    if (!sec) return `0${t('pro.waf.second')}`;
    if (sec % 3600 === 0) return `${sec / 3600}${t('pro.waf.hour')}`;
    if (sec % 60 === 0) return `${sec / 60}${t('pro.waf.minute')}`;
    return `${sec}${t('pro.waf.second')}`;
};

const summary = computed(() =>
    t('pro.waf.summary', {
        period: item.value?.duration ?? 0,
        target: target.value,
        threshold: item.value?.threshold ?? 0,
        time: humanTime(item.value?.ipBlockTime ?? 0),
    }),
);

const save = async (apply: boolean) => {
    await updateWafGlobal({ [active.value]: item.value });
    if (apply) await applyWafGlobal([active.value]);
    ElMessage.success(apply ? t('pro.waf.applySuccess') : t('pro.waf.saveSuccess'));
    emit('reload');
};
</script>

<style scoped>
.menu {
    border-right: 1px solid var(--el-border-color-light);
}
.panel {
    padding: 0 8px;
}
.summary {
    margin-top: 12px;
    color: var(--el-text-color-regular);
}
.unit {
    margin-left: 8px;
    color: var(--el-text-color-secondary);
}
.note {
    margin-top: 8px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 20px;
}
.warn {
    margin-top: 12px;
    font-size: 12px;
    color: var(--el-color-warning);
}
</style>
