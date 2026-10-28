<template>
    <div>
        <div class="bar">
            <el-select v-model="file" style="width: 220px" @change="load">
                <el-option v-for="f in files" :key="f" :value="f" :label="f" />
            </el-select>
            <div>
                <span class="hint">{{ $t('pro.waf.customHint') }}</span>
                <el-button type="primary" class="ml-3" @click="openAdd">{{ $t('pro.waf.addRule') }}</el-button>
            </div>
        </div>
        <el-table :data="items" size="small" @selection-change="(v: WafRule[]) => (selected = v)">
            <el-table-column type="selection" width="45" />
            <el-table-column prop="rule" :label="$t('pro.waf.matchContent')" show-overflow-tooltip min-width="280" />
            <el-table-column prop="name" :label="$t('pro.waf.ruleName')" min-width="140" />
            <el-table-column :label="$t('pro.waf.status')" width="100">
                <template #default="{ row }">
                    <el-switch :model-value="row.state === 'on'" @change="(v: boolean) => toggle(row, v)" />
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
                    <el-input v-model="form.rule" type="textarea" :rows="3" />
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
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { deleteWafRules, saveWafRule, searchWafRules, WafRule } from '@/xpack/api/waf';

const { t } = useI18n();
const files = ['args', 'cookie', 'header', 'urlcc', 'acl'];
const file = ref('args');
const items = ref<WafRule[]>([]);
const selected = ref<WafRule[]>([]);
const dialog = ref(false);
const editing = ref(false);
const oldId = ref('');
const form = reactive<WafRule>({ rule: '', name: '' });

const idOf = (r: WafRule) => r.name || r.rule || '';

const load = async () => {
    const res = await searchWafRules(file.value, '', '', 1, 0);
    items.value = res.data.items;
};

const openAdd = () => {
    editing.value = false;
    oldId.value = '';
    Object.assign(form, { rule: '', name: '' });
    dialog.value = true;
};

const openEdit = (row: WafRule) => {
    editing.value = true;
    oldId.value = idOf(row);
    Object.assign(form, { rule: row.rule, name: row.name });
    dialog.value = true;
};

const submit = async () => {
    if (!form.rule) return;
    const entry: WafRule = { state: 'on', type: file.value, rule: form.rule, name: form.name || form.rule };
    if (editing.value) {
        const old = items.value.find((r) => idOf(r) === oldId.value);
        entry.state = old?.state || 'on';
    }
    await saveWafRule(file.value, '', entry, oldId.value);
    dialog.value = false;
    ElMessage.success(t('pro.waf.saveSuccess'));
    load();
};

const toggle = async (row: WafRule, on: boolean) => {
    await saveWafRule(file.value, '', { ...row, state: on ? 'on' : 'off' }, idOf(row));
    load();
};

const remove = async (rows: WafRule[]) => {
    await deleteWafRules(file.value, '', rows.map(idOf));
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
