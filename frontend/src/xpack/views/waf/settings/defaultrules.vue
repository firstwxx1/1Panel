<template>
    <div>
        <div class="bar">
            <span class="hint">{{ $t('pro.waf.defaultRulesHint') }}</span>
            <el-button type="primary" plain size="small" @click="apply">{{ $t('pro.waf.applyToSites') }}</el-button>
        </div>
        <el-table :data="rows" size="small">
            <el-table-column :label="$t('pro.waf.ruleName')" min-width="220">
                <template #default="{ row }">{{ label(row.key) }}</template>
            </el-table-column>
            <el-table-column :label="$t('pro.waf.status')" width="120">
                <template #default="{ row }">
                    <el-switch v-model="row.item.state" active-value="on" inactive-value="off" @change="save(row.key)" />
                </template>
            </el-table-column>
            <el-table-column :label="$t('pro.waf.action')" width="180">
                <template #default="{ row }">
                    <el-select v-if="row.item.action" v-model="row.item.action" size="small" @change="save(row.key)">
                        <el-option value="deny" label="deny" />
                        <el-option value="allow" label="allow" />
                    </el-select>
                </template>
            </el-table-column>
        </el-table>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { applyWafGlobal, updateWafGlobal } from '@/xpack/api/waf';

const props = defineProps<{ config: Record<string, any> }>();
const emit = defineEmits<{ reload: [] }>();
const { t, te } = useI18n();

const keys = [
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
    'unknownWebsite',
    'cdn',
    'strict',
];

const rows = computed(() => keys.filter((k) => props.config[k]).map((k) => ({ key: k, item: props.config[k] })));
const label = (key: string) => (te('pro.waf.mod_' + key) ? t('pro.waf.mod_' + key) : key);

const save = async (key: string) => {
    await updateWafGlobal({ [key]: props.config[key] });
    emit('reload');
};

const apply = async () => {
    await applyWafGlobal(rows.value.map((r) => r.key));
    ElMessage.success(t('pro.waf.applySuccess'));
};
</script>

<style scoped>
.bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
}
.hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
