<template>
    <div>
        <div class="bar">
            <el-select v-model="file" style="width: 240px" @change="load">
                <el-option v-for="f in files" :key="f" :value="f" :label="`${fileLabel(f)} (${f})`" />
            </el-select>
            <div>
                <span class="hint">{{ $t('pro.waf.customHint') }}</span>
                <el-button type="primary" class="ml-3" @click="openAdd">{{ $t('pro.waf.addRule') }}</el-button>
            </div>
        </div>
        <el-table :data="items" v-loading="loading" size="small" @selection-change="(v: WafRule[]) => (selected = v)">
            <el-table-column type="selection" width="45" />
            <el-table-column prop="rule" :label="$t('pro.waf.matchContent')" show-overflow-tooltip min-width="280" />
            <el-table-column prop="name" :label="$t('pro.waf.ruleName')" min-width="140" />
            <el-table-column :label="$t('pro.waf.status')" width="100">
                <template #default="{ row }">
                    <el-switch :model-value="row.state === 'on'" @change="(v: any) => toggle(row, Boolean(v))" />
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="140">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEdit(row)">{{ $t('commons.button.edit') }}</el-button>
                    <el-button link type="danger" @click="remove([row])">{{ $t('commons.button.delete') }}</el-button>
                </template>
            </el-table-column>
        </el-table>
        <div class="mt-3">
            <el-button :disabled="!selected.length" @click="remove(selected)">{{ $t('commons.button.delete') }}</el-button>
        </div>
        <el-dialog v-model="dialog" :title="editing ? $t('commons.button.edit') : $t('pro.waf.addRule')" width="520px">
            <el-form label-position="top">
                <el-form-item :label="$t('pro.waf.matchContent')">
                    <el-input
                        v-model="form.rule"
                        type="textarea"
                        :rows="3"
                        :placeholder="file === 'fileExt' ? $t('pro.waf.fileExtHint') : ''"
                    />
                </el-form-item>
                <el-form-item :label="$t('pro.waf.ruleName')">
                    <el-input v-model="form.name" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="dialog = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { deleteWafRules, saveWafRule, searchWafRules, WafRule } from '@/xpack/api/waf';

const props = defineProps<{ node?: string }>();
const { t, te } = useI18n();
const files = ['args', 'cookie', 'header', 'fileExt'];
const file = ref('args');
const loading = ref(false);
const items = ref<WafRule[]>([]);
const selected = ref<WafRule[]>([]);
const dialog = ref(false);
const editing = ref(false);
const editingRow = ref<WafRule | null>(null);
const oldId = ref('');
const form = reactive<{ rule: string; name: string }>({ rule: '', name: '' });

const fileLabel = (f: string) => (te('pro.waf.mod_' + f) ? t('pro.waf.mod_' + f) : f);

// Unique identifier: prefer id or rule regex, NEVER remark/name
const ruleKeyOf = (r: WafRule) => r.id || r.rule || '';

const load = async () => {
    loading.value = true;
    try {
        const res = await searchWafRules(file.value, '', '', 1, 0, props.node);
        items.value = res.data.items || [];
    } catch {
        items.value = [];
    } finally {
        loading.value = false;
    }
};

watch(
    () => props.node,
    () => load(),
);

const openAdd = () => {
    editing.value = false;
    editingRow.value = null;
    oldId.value = '';
    Object.assign(form, { rule: '', name: '' });
    dialog.value = true;
};

const openEdit = (row: WafRule) => {
    editing.value = true;
    editingRow.value = { ...row };
    oldId.value = ruleKeyOf(row);
    Object.assign(form, { rule: row.rule || '', name: row.name || '' });
    dialog.value = true;
};

const submit = async () => {
    const ruleVal = (form.rule || '').trim();
    if (!ruleVal) return;
    const prev = editing.value ? editingRow.value : null;
    const entry: WafRule = {
        ...(prev || {}),
        state: prev?.state || 'on',
        type: file.value,
        rule: ruleVal,
        name: (form.name || '').trim() || ruleVal,
    };
    await saveWafRule(file.value, '', entry, oldId.value, props.node);
    dialog.value = false;
    ElMessage.success(t('pro.waf.saveSuccess'));
    load();
};

const toggle = async (row: WafRule, on: boolean) => {
    await saveWafRule(file.value, '', { ...row, state: on ? 'on' : 'off' }, ruleKeyOf(row), props.node);
    load();
};

const remove = async (rows: WafRule[]) => {
    const ids = rows.map(ruleKeyOf).filter(Boolean);
    if (!ids.length) return;
    await deleteWafRules(file.value, '', ids, props.node);
    ElMessage.success(t('commons.msg.deleteSuccess'));
    load();
};

onMounted(load);
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
