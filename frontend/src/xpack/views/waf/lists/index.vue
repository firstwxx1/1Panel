<template>
    <el-card>
        <el-tabs v-model="name" @tab-change="reset">
            <el-tab-pane v-for="n in names" :key="n" :label="$t('pro.waf.list_' + n)" :name="n" />
        </el-tabs>
        <div class="mb-3">
            <el-button type="primary" @click="openAdd">{{ $t('commons.button.add') }}</el-button>
            <el-button :disabled="selected.length === 0" @click="removeSelected">{{ $t('commons.button.delete') }}</el-button>
            <el-input v-model="keyword" class="ml-3" style="width: 240px" clearable @change="load" />
        </div>
        <el-table :data="items" size="small" @selection-change="(v: WafRule[]) => (selected = v)">
            <el-table-column type="selection" width="45" />
            <el-table-column prop="rule" :label="$t('pro.waf.rule')" show-overflow-tooltip />
            <el-table-column prop="name" :label="$t('pro.waf.remark')" show-overflow-tooltip />
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
        <el-pagination class="mt-3" v-model:current-page="page" v-model:page-size="size" :total="total" layout="total, prev, pager, next, sizes" @change="load" />
        <el-dialog v-model="dialog" :title="editing ? $t('commons.button.edit') : $t('commons.button.add')" width="480px">
            <el-form label-position="top">
                <el-form-item :label="$t('pro.waf.rule')">
                    <el-input v-model="form.rule" type="textarea" :rows="3" />
                </el-form-item>
                <el-form-item :label="$t('pro.waf.remark')">
                    <el-input v-model="form.name" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="dialog = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { deleteWafRules, saveWafRule, searchWafRules, WafRule } from '@/xpack/api/waf';

const names = ['ipBlack', 'ipWhite', 'urlBlack', 'urlWhite', 'uaBlack', 'uaWhite'];
const name = ref('ipBlack');
const keyword = ref('');
const page = ref(1);
const size = ref(20);
const total = ref(0);
const items = ref<WafRule[]>([]);
const selected = ref<WafRule[]>([]);
const dialog = ref(false);
const editing = ref(false);
const oldId = ref('');
const form = reactive<WafRule>({ rule: '', name: '' });

const idOf = (r: WafRule) => r.name || r.rule || '';

const load = async () => {
    const res = await searchWafRules(name.value, '', keyword.value, page.value, size.value);
    items.value = res.data.items;
    total.value = res.data.total;
};

const reset = () => {
    page.value = 1;
    load();
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
    const type = name.value.startsWith('ip') ? 'ip' : name.value.startsWith('url') ? 'url' : 'ua';
    const entry: WafRule = { state: 'on', type, rule: form.rule, name: form.name || form.rule };
    if (editing.value) {
        const old = items.value.find((r) => idOf(r) === oldId.value);
        entry.state = old?.state || 'on';
    }
    await saveWafRule(name.value, '', entry, oldId.value);
    dialog.value = false;
    ElMessage.success('OK');
    load();
};

const toggle = async (row: WafRule, on: boolean) => {
    await saveWafRule(name.value, '', { ...row, state: on ? 'on' : 'off' }, idOf(row));
    load();
};

const remove = async (rows: WafRule[]) => {
    await deleteWafRules(name.value, '', rows.map(idOf));
    ElMessage.success('OK');
    load();
};
const removeSelected = () => remove(selected.value);

onMounted(load);
</script>
