<template>
    <div>
        <el-card shadow="never">
            <el-alert v-if="kind === 'panels'" type="info" show-icon :closable="false" class="mb-3" :title="$t('pro.node.panelTip')" />
            <div class="bar">
                <div class="left">
                    <el-button type="primary" @click="openAdd">
                        {{ kind === 'nodes' ? $t('pro.node.addNode') : $t('pro.node.addPanel') }}
                    </el-button>
                    <el-button v-if="kind === 'nodes'" :loading="upgrading" @click="onBatchUpgrade">
                        {{ selectedBoundNodes.length ? $t('pro.node.upgradeSelected', { n: selectedBoundNodes.length }) : $t('pro.node.batchUpgrade') }}
                    </el-button>
                    <el-button :disabled="!selected.length" @click="openGroup">{{ $t('pro.node.group') }}</el-button>
                    <el-button type="danger" plain :disabled="!selected.length" @click="removeSelected">{{ $t('pro.node.delete') }}</el-button>
                </div>
                <div class="right">
                    <el-select v-model="query.groupID" style="width: 140px" @change="filter">
                        <el-option :value="0" :label="$t('pro.node.allGroups')" />
                        <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
                    </el-select>
                    <el-input v-model="query.info" clearable :placeholder="$t('pro.node.search')" style="width: 190px" @keyup.enter="filter" @clear="filter">
                        <template #append>
                            <el-button :icon="Search" @click="filter" />
                        </template>
                    </el-input>
                    <el-select v-model="refresh" style="width: 115px">
                        <el-option :value="0" :label="$t('pro.node.noRefresh')" />
                        <el-option :value="10" label="10s" />
                        <el-option :value="30" label="30s" />
                        <el-option :value="60" label="60s" />
                    </el-select>
                    <el-button :icon="Refresh" circle :loading="loading" @click="search()" />
                </div>
            </div>

            <el-table :data="items" v-loading="loading || upgrading" @selection-change="(v: NodeRow[]) => (selected = v)">
                <el-table-column type="selection" width="45" :selectable="(row: NodeRow) => !row.isLocal" />
                <el-table-column :label="$t('pro.node.name')" min-width="175">
                    <template #default="{ row }">
                        <div class="name">
                            <span class="name-text">{{ row.isLocal ? globalStore.getMasterAlias() : row.name }}</span>
                            <el-tag v-if="row.isLocal" size="small" type="primary" effect="plain">{{ $t('pro.node.master') }}</el-tag>
                            <el-icon v-if="!row.isLocal" class="edit" @click="openEdit(row)"><Edit /></el-icon>
                        </div>
                        <div class="sub mono">
                            {{ kind === 'panels' && row.port ? `${formatHost(row.addr)}:${row.port}` : row.addr || '127.0.0.1' }}
                        </div>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('commons.table.status')" width="110">
                    <template #default="{ row }">
                        <el-tooltip :disabled="!row.message" :content="row.message" placement="top">
                            <el-tag :type="statusType(row.status)" effect="light">{{ statusText(row.status) }}</el-tag>
                        </el-tooltip>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.group')" width="110" prop="groupBelong" />
                <el-table-column :label="$t('pro.node.version')" min-width="130">
                    <template #default="{ row }">
                        <template v-if="kind === 'nodes'">
                            <div>{{ $t('pro.node.pro') }}</div>
                            <div class="sub">{{ row.version }}</div>
                        </template>
                        <template v-else>{{ row.version || '-' }}</template>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.resource')" min-width="195">
                    <template #default="{ row }">
                        <div v-if="row.cpuTotal" class="res-cell">
                            <div class="res-line">
                                <span>CPU {{ row.cpuUsedPercent.toFixed(1) }}%</span>
                                <span class="sub">{{ row.cpuTotal }} {{ $t('pro.node.cores') }}</span>
                            </div>
                            <div class="res-bar">
                                <div class="res-fill" :class="usageTone(row.cpuUsedPercent)" :style="{ width: `${clampPct(row.cpuUsedPercent)}%` }" />
                            </div>
                            <div class="res-line mt-1">
                                <span>{{ $t('pro.node.memory') }} {{ row.memoryUsedPercent.toFixed(1) }}%</span>
                                <span class="sub">{{ gb(row.memoryTotal) }}</span>
                            </div>
                            <div class="res-bar">
                                <div class="res-fill" :class="usageTone(row.memoryUsedPercent)" :style="{ width: `${clampPct(row.memoryUsedPercent)}%` }" />
                            </div>
                        </div>
                        <template v-else>-</template>
                    </template>
                </el-table-column>
                <el-table-column v-if="kind === 'nodes'" :label="$t('pro.node.dataSync')" width="110">
                    <template #default>
                        <el-tag type="success" effect="light">{{ $t('pro.node.syncSuccess') }}</el-tag>
                    </template>
                </el-table-column>
                <el-table-column :label="$t('pro.node.description')" min-width="140" show-overflow-tooltip prop="description" />
                <el-table-column :label="$t('pro.node.operate')" width="240" fixed="right">
                    <template #default="{ row }">
                        <template v-if="!row.isLocal">
                            <el-button
                                v-if="kind === 'nodes' && row.status === 'Pending'"
                                link
                                type="primary"
                                @click="showPendingJoinCmd(row)"
                            >
                                {{ $t('pro.node.copyJoinCmd') }}
                            </el-button>
                            <el-button
                                v-if="kind === 'nodes' && row.status !== 'Pending'"
                                link
                                type="primary"
                                :disabled="upgrading"
                                @click="upgradeOne(row)"
                            >
                                {{ $t('pro.node.upgrade') }}
                            </el-button>
                            <el-button link type="primary" @click="openEdit(row)">{{ $t('pro.node.edit') }}</el-button>
                            <el-button link type="danger" @click="removeOne(row)">{{ $t('pro.node.delete') }}</el-button>
                            <el-dropdown trigger="click" @command="(cmd: string) => more(cmd, row)">
                                <el-button link type="primary">{{ $t('pro.node.moreAction') }}</el-button>
                                <template #dropdown>
                                    <el-dropdown-menu>
                                        <el-dropdown-item command="test">{{ $t('pro.node.test') }}</el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'nodes' && row.status === 'Pending'" command="joinCmd">
                                            {{ $t('pro.node.copyJoinCmd') }}
                                        </el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'nodes' && row.status !== 'Pending'" command="upgrade">
                                            {{ $t('pro.node.upgrade') }}
                                        </el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'nodes'" command="restart">
                                            {{ $t('pro.node.restart') }}
                                        </el-dropdown-item>
                                        <el-dropdown-item v-if="kind === 'panels'" command="open">
                                            {{ $t('pro.node.open') }}
                                        </el-dropdown-item>
                                    </el-dropdown-menu>
                                </template>
                            </el-dropdown>
                        </template>
                    </template>
                </el-table-column>
                <template #empty>
                    <el-empty :image-size="60" :description="$t('pro.node.noData')" />
                </template>
            </el-table>
            <div class="pager">
                <el-pagination
                    v-model:current-page="query.page"
                    v-model:page-size="query.pageSize"
                    :total="total"
                    :page-sizes="[20, 50, 100]"
                    layout="total, sizes, prev, pager, next, jumper"
                    @change="search"
                />
            </div>
        </el-card>

        <!-- add / edit -->
        <el-dialog v-model="formVisible" :title="formTitle" width="560px" :close-on-click-modal="false" destroy-on-close>
            <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
                <el-form-item v-if="kind === 'nodes' && !editing" :label="$t('pro.node.joinMode')">
                    <el-radio-group v-model="mode">
                        <el-radio-button value="command">{{ $t('pro.node.modeCommand') }}</el-radio-button>
                        <el-radio-button value="ssh">{{ $t('pro.node.modeSSH') }}</el-radio-button>
                    </el-radio-group>
                </el-form-item>
                <el-form-item :label="$t('pro.node.nodeName')" prop="name">
                    <el-input v-model="form.name" />
                    <span class="hint">{{ $t('pro.node.nodeNameRule') }}</span>
                </el-form-item>
                <el-form-item :label="$t('pro.node.group')" prop="groupID">
                    <el-select v-model="form.groupID" style="width: 100%">
                        <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
                    </el-select>
                </el-form-item>

                <template v-if="kind === 'nodes' && !editing">
                    <el-form-item :label="$t('pro.node.agentPort')">
                        <el-input-number v-model="form.agentPort" :min="1" :max="65535" controls-position="right" />
                    </el-form-item>
                    <template v-if="mode === 'ssh'">
                        <el-alert type="info" :closable="false" class="mb-3" :title="$t('pro.node.sshHelper')" />
                        <el-row :gutter="12">
                            <el-col :span="16">
                                <el-form-item :label="$t('pro.node.sshAddr')" prop="sshAddr">
                                    <el-input v-model="ssh.addr" />
                                </el-form-item>
                            </el-col>
                            <el-col :span="8">
                                <el-form-item :label="$t('pro.node.sshPort')">
                                    <el-input-number v-model="ssh.port" :min="1" :max="65535" controls-position="right" />
                                </el-form-item>
                            </el-col>
                        </el-row>
                        <el-form-item :label="$t('pro.node.sshUser')" prop="sshUser">
                            <el-input v-model="ssh.user" />
                        </el-form-item>
                        <el-form-item>
                            <el-radio-group v-model="ssh.authMode">
                                <el-radio value="password">{{ $t('pro.node.authPassword') }}</el-radio>
                                <el-radio value="key">{{ $t('pro.node.authKey') }}</el-radio>
                            </el-radio-group>
                        </el-form-item>
                        <el-form-item v-if="ssh.authMode === 'password'" :label="$t('pro.node.password')" prop="sshPassword">
                            <el-input v-model="ssh.password" type="password" show-password />
                        </el-form-item>
                        <template v-else>
                            <el-form-item :label="$t('pro.node.privateKey')" prop="sshPrivateKey">
                                <el-input v-model="ssh.privateKey" type="textarea" :rows="4" />
                            </el-form-item>
                            <el-form-item :label="$t('pro.node.passPhrase')">
                                <el-input v-model="ssh.passPhrase" type="password" show-password />
                            </el-form-item>
                        </template>
                    </template>
                </template>

                <template v-if="kind === 'panels'">
                    <el-row :gutter="12">
                        <el-col :span="8">
                            <el-form-item :label="$t('pro.node.protocol')" prop="protocol">
                                <el-select v-model="form.protocol">
                                    <el-option value="http" label="http" />
                                    <el-option value="https" label="https" />
                                </el-select>
                            </el-form-item>
                        </el-col>
                        <el-col :span="10">
                            <el-form-item :label="$t('pro.node.panelAddr')" prop="addr">
                                <el-input v-model="form.addr" />
                            </el-form-item>
                        </el-col>
                        <el-col :span="6">
                            <el-form-item :label="$t('pro.node.panelPort')" prop="port">
                                <el-input-number v-model="form.port" :min="1" :max="65535" :controls="false" style="width: 100%" />
                            </el-form-item>
                        </el-col>
                    </el-row>
                    <el-form-item :label="$t('pro.node.entrance')">
                        <el-input v-model="form.entrance" />
                    </el-form-item>
                    <el-form-item :label="$t('pro.node.apiKey')" :prop="editing ? '' : 'apiKey'">
                        <el-input v-model="form.apiKey" type="password" show-password :placeholder="editing ? $t('pro.node.apiKeyKeep') : ''" />
                        <span class="hint">{{ $t('pro.node.apiKeyHelper') }}</span>
                    </el-form-item>
                </template>

                <el-form-item :label="$t('pro.node.description')">
                    <el-input v-model="form.description" type="textarea" :rows="2" maxlength="200" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="formVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="submit">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>

        <!-- join command -->
        <el-dialog v-model="cmdVisible" :title="$t('pro.node.commandTitle')" width="640px">
            <el-alert type="info" :closable="false" class="mb-3" :title="$t('pro.node.commandHelper')" />
            <el-input :model-value="command" type="textarea" :rows="3" readonly />
            <template #footer>
                <el-button @click="cmdVisible = false">{{ $t('commons.button.close') }}</el-button>
                <el-button type="primary" @click="copyCommand">{{ $t('pro.node.copy') }}</el-button>
            </template>
        </el-dialog>

        <!-- upgrade results -->
        <el-dialog v-model="upgradeResultVisible" :title="$t('pro.node.upgradeResultTitle')" width="580px">
            <el-table :data="upgradeResults" size="small">
                <el-table-column prop="name" :label="$t('pro.node.name')" width="150" />
                <el-table-column :label="$t('commons.table.status')" width="110">
                    <template #default="{ row }">
                        <el-tag :type="row.ok ? 'success' : 'danger'" size="small">
                            {{ row.ok ? $t('pro.node.upgradeSuccess') : $t('pro.node.upgradeFailed') }}
                        </el-tag>
                    </template>
                </el-table-column>
                <el-table-column prop="message" :label="$t('pro.node.description')" show-overflow-tooltip />
            </el-table>
            <template #footer>
                <el-button type="primary" @click="upgradeResultVisible = false">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>

        <!-- group -->
        <el-dialog v-model="groupVisible" :title="$t('pro.node.changeGroup')" width="420px">
            <el-select v-model="targetGroup" style="width: 100%">
                <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="groupLabel(g)" />
            </el-select>
            <template #footer>
                <el-button @click="groupVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :disabled="!targetGroup" @click="applyGroup">{{ $t('commons.button.confirm') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ElMessage, ElMessageBox, FormInstance } from 'element-plus';
import { Edit, Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import { GlobalStore } from '@/store';
import { getGroupList } from '@/api/modules/group';
import { invalidateNodeCache } from '@/xpack/api/aggregate';
import {
    changeNodeGroup,
    createNode,
    deleteNodes,
    getPendingNodeJoinCmd,
    NodeCreate,
    NodeRow,
    nodeJoinCommand,
    NodeUpgradeResult,
    restartNode,
    searchNodes,
    testNode,
    updateNode,
    upgradeNodes,
} from '@/xpack/api/node';

const props = defineProps<{ kind: 'nodes' | 'panels' }>();
const { t } = useI18n();
const globalStore = GlobalStore();

const query = reactive({ page: 1, pageSize: 20, info: '', groupID: 0 });
const items = ref<NodeRow[]>([]);
const total = ref(0);
const loading = ref(false);
const upgrading = ref(false);
const selected = ref<NodeRow[]>([]);
const selectedBoundNodes = computed(() => selected.value.filter((r) => !r.isLocal && r.status !== 'Pending'));
const groups = ref<{ id: number; name: string; isDefault: boolean }[]>([]);
const refresh = ref(0);
let timer: number | undefined;

const groupLabel = (g: { name: string; isDefault: boolean }) =>
    g.isDefault && g.name.toLowerCase() === 'default' ? '默认' : g.name;

const formatHost = (addr: string) => {
    const a = (addr || '').trim();
    if (a.includes(':') && !a.startsWith('[')) {
        return `[${a}]`;
    }
    return a;
};

const statusType = (s: string) => {
    if (s === 'Healthy') return 'success';
    if (s === 'Installing' || s === 'Pending') return 'warning';
    return 'danger';
};
const statusText = (s: string) => t('pro.node.status' + s);
const gb = (n: number) => (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
const clampPct = (n?: number) => Math.min(100, Math.max(0, Number(n || 0)));
const usageTone = (n?: number) => {
    const p = clampPct(n);
    if (p >= 85) return 'is-danger';
    if (p >= 65) return 'is-warning';
    return 'is-success';
};

const search = async (silent = false) => {
    if (silent !== true) loading.value = true;
    try {
        const res = await searchNodes(props.kind, { ...query, groupID: query.groupID || undefined });
        items.value = res.data.items || [];
        total.value = res.data.total;
    } finally {
        loading.value = false;
    }
};
const filter = () => {
    query.page = 1;
    search();
};

// --- form ---
const formVisible = ref(false);
const editing = ref(false);
const saving = ref(false);
const mode = ref<'command' | 'ssh'>('command');
const formRef = ref<FormInstance>();
const blankForm = () => ({
    id: 0,
    name: '',
    groupID: groups.value.find((g) => g.isDefault)?.id || 0,
    description: '',
    agentPort: 9999,
    addr: '',
    port: 9999 as number | undefined,
    protocol: 'http',
    entrance: '',
    apiKey: '',
});
const form = reactive(blankForm());
const ssh = reactive({
    addr: '',
    port: 22,
    user: 'root',
    authMode: 'password',
    password: '',
    privateKey: '',
    passPhrase: '',
});

const nameRule = /^[一-龥A-Za-z0-9_-]{1,30}$/;
const rules = computed(() => ({
    name: [
        {
            required: true,
            trigger: 'blur',
            validator: (_: any, v: string, cb: any) =>
                nameRule.test(v) && v !== 'local' ? cb() : cb(new Error(t('pro.node.nodeNameRule'))),
        },
    ],
    sshAddr: [
        {
            required: mode.value === 'ssh',
            validator: (_: any, __: string, cb: any) =>
                mode.value !== 'ssh' || (ssh.addr || '').trim() ? cb() : cb(new Error(' ')),
            trigger: 'blur',
        },
    ],
    sshUser: [
        {
            required: mode.value === 'ssh',
            validator: (_: any, __: string, cb: any) =>
                mode.value !== 'ssh' || (ssh.user || '').trim() ? cb() : cb(new Error(' ')),
            trigger: 'blur',
        },
    ],
    sshPassword: [
        {
            required: mode.value === 'ssh' && ssh.authMode === 'password',
            validator: (_: any, __: string, cb: any) =>
                mode.value !== 'ssh' || ssh.authMode !== 'password' || ssh.password
                    ? cb()
                    : cb(new Error(t('pro.node.sshPassRequired'))),
            trigger: 'blur',
        },
    ],
    sshPrivateKey: [
        {
            required: mode.value === 'ssh' && ssh.authMode === 'key',
            validator: (_: any, __: string, cb: any) =>
                mode.value !== 'ssh' || ssh.authMode !== 'key' || (ssh.privateKey || '').trim()
                    ? cb()
                    : cb(new Error(t('pro.node.sshKeyRequired'))),
            trigger: 'blur',
        },
    ],
    protocol: [
        {
            required: props.kind === 'panels',
            validator: (_: any, v: string, cb: any) =>
                props.kind !== 'panels' || v === 'http' || v === 'https'
                    ? cb()
                    : cb(new Error(t('pro.node.invalidProtocol'))),
            trigger: 'change',
        },
    ],
    addr: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
    port: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
    apiKey: [{ required: props.kind === 'panels', message: ' ', trigger: 'blur' }],
}));

const formTitle = computed(() => {
    if (editing.value) return t('pro.node.edit');
    return props.kind === 'nodes' ? t('pro.node.addNode') : t('pro.node.addPanel');
});

const openAdd = () => {
    editing.value = false;
    mode.value = 'command';
    Object.assign(form, blankForm());
    if (props.kind === 'panels') form.port = 9999;
    Object.assign(ssh, {
        addr: '',
        port: 22,
        user: 'root',
        authMode: 'password',
        password: '',
        privateKey: '',
        passPhrase: '',
    });
    formVisible.value = true;
};

const openEdit = (row: NodeRow) => {
    editing.value = true;
    Object.assign(form, blankForm(), {
        id: row.id,
        name: row.name,
        groupID: row.groupID,
        description: row.description,
        addr: row.addr,
        port: row.port,
        protocol: row.protocol === 'https' ? 'https' : 'http',
        entrance: row.securityEntrance,
        apiKey: '',
    });
    formVisible.value = true;
};

const cmdVisible = ref(false);
const command = ref('');
const copyCommand = async () => {
    try {
        await navigator.clipboard.writeText(command.value);
    } catch {
        const ta = document.createElement('textarea');
        ta.value = command.value;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
    }
    ElMessage.success(t('pro.node.copied'));
};

const showPendingJoinCmd = async (row: NodeRow) => {
    const res = await getPendingNodeJoinCmd(row.id);
    command.value = res.data.command || '';
    cmdVisible.value = true;
    await copyCommand();
};

const upgradeResultVisible = ref(false);
const upgradeResults = ref<NodeUpgradeResult[]>([]);

const runUpgrade = async (ids: number[]) => {
    upgrading.value = true;
    try {
        const res = await upgradeNodes(ids);
        upgradeResults.value = res.data?.results || [];
        invalidateNodeCache();
        if (upgradeResults.value.length === 1) {
            const r = upgradeResults.value[0];
            if (r.ok) {
                ElMessage.success(`${r.name}: ${r.message || t('pro.node.upgradeSuccess')}`);
            } else {
                ElMessage.error(`${r.name}: ${r.message || t('pro.node.upgradeFailed')}`);
            }
        } else if (upgradeResults.value.length > 1) {
            upgradeResultVisible.value = true;
        } else {
            ElMessage.info(t('pro.node.noData'));
        }
        await search(true);
    } finally {
        upgrading.value = false;
    }
};

const upgradeOne = async (row: NodeRow) => {
    await ElMessageBox.confirm(t('pro.node.upgradeConfirmOne', { name: row.name }), t('pro.node.upgrade'), {
        type: 'warning',
    });
    await runUpgrade([row.id]);
};

const onBatchUpgrade = async () => {
    const ids = selectedBoundNodes.value.map((r) => r.id);
    const msg = ids.length
        ? t('pro.node.upgradeConfirmSelected', { n: ids.length })
        : t('pro.node.upgradeConfirmAll');
    await ElMessageBox.confirm(msg, t('pro.node.batchUpgrade'), { type: 'warning' });
    await runUpgrade(ids);
};

const submit = async () => {
    if (!formRef.value) return;
    const ok = await formRef.value.validate().catch(() => false);
    if (!ok) return;
    saving.value = true;
    try {
        const safeProto = form.protocol === 'https' ? 'https' : 'http';
        if (editing.value) {
            await updateNode(props.kind, {
                id: form.id,
                name: form.name,
                groupID: form.groupID,
                description: form.description,
                ...(props.kind === 'panels'
                    ? {
                          addr: form.addr.trim(),
                          port: form.port,
                          protocol: safeProto,
                          entrance: form.entrance.trim(),
                          apiKey: form.apiKey,
                      }
                    : {}),
            });
            ElMessage.success(t('commons.msg.updateSuccess'));
        } else {
            const req: NodeCreate = { name: form.name, groupID: form.groupID, description: form.description };
            if (props.kind === 'nodes') {
                req.agentPort = form.agentPort;
                if (mode.value === 'ssh') req.ssh = { ...ssh, addr: ssh.addr.trim(), user: ssh.user.trim() };
            } else {
                Object.assign(req, {
                    addr: form.addr.trim(),
                    port: form.port,
                    protocol: safeProto,
                    entrance: form.entrance.trim(),
                    apiKey: form.apiKey,
                });
            }
            const res = await createNode(props.kind, req);
            if (props.kind === 'nodes' && mode.value === 'command') {
                command.value = (await nodeJoinCommand(res.data.token)).data;
                cmdVisible.value = true;
                ElMessage.success(t('pro.node.created'));
            } else if (props.kind === 'nodes') {
                ElMessage.success(t('pro.node.sshStarted'));
            } else {
                ElMessage.success(t('commons.msg.createSuccess'));
            }
        }
        invalidateNodeCache();
        formVisible.value = false;
        search();
    } finally {
        saving.value = false;
    }
};

// --- delete / group / more ---
const doDelete = async (ids: number[]) => {
    const key = props.kind === 'nodes' ? 'pro.node.delConfirm' : 'pro.node.delPanelConfirm';
    await ElMessageBox.confirm(t(key, { n: ids.length }), t('pro.node.delete'), { type: 'warning' });
    await deleteNodes(props.kind, ids);
    invalidateNodeCache();
    ElMessage.success(t('commons.msg.deleteSuccess'));
    search();
};
const removeOne = (row: NodeRow) => doDelete([row.id]);
const removeSelected = () => doDelete(selected.value.map((r) => r.id));

const groupVisible = ref(false);
const targetGroup = ref(0);
const openGroup = () => {
    if (!selected.value.length) {
        ElMessage.warning(t('pro.node.selectFirst'));
        return;
    }
    targetGroup.value = groups.value.find((g) => g.isDefault)?.id || 0;
    groupVisible.value = true;
};
const applyGroup = async () => {
    await changeNodeGroup(
        props.kind,
        selected.value.map((r) => r.id),
        targetGroup.value,
    );
    groupVisible.value = false;
    ElMessage.success(t('commons.msg.operationSuccess'));
    search();
};

const openPanelBookmark = (row: NodeRow) => {
    const proto = row.protocol === 'https' ? 'https' : 'http';
    const host = formatHost(row.addr);
    if (!host) return;
    const portPart = row.port ? `:${row.port}` : '';
    const cleanEntrance = (row.securityEntrance || '').replace(/^\/+/, '');
    const entrancePart = cleanEntrance ? `/${encodeURIComponent(cleanEntrance)}` : '';
    window.open(`${proto}://${host}${portPart}${entrancePart}`, '_blank', 'noopener,noreferrer');
};

const more = async (cmd: string, row: NodeRow) => {
    if (cmd === 'test') {
        await testNode(props.kind, row.id);
        invalidateNodeCache();
        ElMessage.success(t('pro.node.testOk'));
        search(true);
    } else if (cmd === 'joinCmd') {
        await showPendingJoinCmd(row);
    } else if (cmd === 'upgrade') {
        await upgradeOne(row);
    } else if (cmd === 'restart') {
        await ElMessageBox.confirm(t('pro.node.restartConfirm'), t('pro.node.restart'), { type: 'warning' });
        await restartNode(row.id);
        ElMessage.success(t('pro.node.restartOk'));
    } else if (cmd === 'open') {
        openPanelBookmark(row);
    }
};

watch(refresh, (v) => {
    if (timer) window.clearInterval(timer);
    timer = v > 0 ? window.setInterval(() => search(true), v * 1000) : undefined;
});

onMounted(async () => {
    groups.value = (await getGroupList('node')).data || [];
    search();
});
onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px;
    margin-bottom: 12px;
}
.left {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
}
.right {
    display: flex;
    align-items: center;
    gap: 8px;
}
.name {
    display: flex;
    align-items: center;
    gap: 6px;
}
.name-text {
    font-weight: 500;
    color: var(--el-text-color-primary);
}
.edit {
    cursor: pointer;
    color: var(--el-color-primary);
}
.sub {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.res-cell {
    font-size: 12px;
    font-variant-numeric: tabular-nums;
}
.res-line {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    line-height: 1.4;
}
.res-bar {
    height: 4px;
    width: 100%;
    border-radius: 2px;
    background: var(--el-fill-color);
    overflow: hidden;
    margin-top: 2px;
}
.res-fill {
    height: 100%;
    border-radius: 2px;
}
.res-fill.is-success {
    background: var(--el-color-success);
}
.res-fill.is-warning {
    background: var(--el-color-warning);
}
.res-fill.is-danger {
    background: var(--el-color-danger);
}
.mt-1 {
    margin-top: 4px;
}
.hint {
    display: block;
    width: 100%;
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 18px;
}
.pager {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
}
</style>
