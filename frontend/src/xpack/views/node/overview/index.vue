<template>
    <div class="ov">
        <div class="top">
            <div class="left">
                <el-select v-model="groupID" style="width: 140px" @change="filter">
                    <el-option :value="0" :label="$t('pro.node.allGroups')" />
                    <el-option v-for="g in groups" :key="g.id" :value="g.id" :label="g.isDefault && g.name.toLowerCase() === 'default' ? '默认' : g.name" />
                </el-select>
                <el-input v-model="info" clearable :placeholder="$t('pro.node.search')" style="width: 200px" @keyup.enter="filter" @clear="filter">
                    <template #append>
                        <el-button :icon="Search" @click="filter" />
                    </template>
                </el-input>
            </div>
            <div class="right">
                <el-button :icon="Refresh" circle @click="refreshAll" />
            </div>
        </div>

        <el-row :gutter="12">
            <el-col :xs="24" :md="8" :lg="7">
                <div v-loading="loading">
                    <el-card
                        v-for="n in nodes"
                        :key="n.name"
                        shadow="never"
                        class="node"
                        :class="{ active: n.name === current }"
                        @click="current = n.name"
                    >
                        <div class="line">
                            <span class="nname">{{ n.isLocal ? $t('pro.node.master') : n.name }}</span>
                            <el-tag size="small" :type="statusType(n.status)" effect="light">{{ $t('pro.node.status' + n.status) }}</el-tag>
                            <span class="addr">{{ n.addr }}</span>
                        </div>
                        <div class="res">
                            <template v-if="n.cpuTotal">
                                CPU {{ n.cpuUsedPercent.toFixed(2) }}% / {{ n.cpuTotal }} {{ $t('pro.node.cores') }} |
                                {{ $t('pro.node.memory') }} {{ n.memoryUsedPercent.toFixed(2) }}% / {{ gb(n.memoryTotal) }}
                            </template>
                            <template v-else>-</template>
                        </div>
                        <div class="links" @click.stop>
                            <el-link type="primary" :underline="false" :disabled="n.status !== 'Healthy'" @click="go(n, '/hosts/terminal')">
                                {{ $t('pro.node.terminal') }}
                            </el-link>
                            <el-link type="primary" :underline="false" :disabled="n.status !== 'Healthy'" @click="go(n, '/logs')">
                                {{ $t('pro.node.log') }}
                            </el-link>
                            <el-link v-if="!n.isLocal" type="primary" :underline="false" @click="restart(n)">
                                {{ $t('pro.node.restart') }}
                            </el-link>
                        </div>
                    </el-card>
                    <el-empty v-if="!nodes.length && !loading" :description="$t('pro.node.noData')" />
                </div>
                <div class="foot">
                    <span>{{ $t('pro.node.total', { n: total }) }}</span>
                    <el-pagination
                        v-model:current-page="page"
                        v-model:page-size="pageSize"
                        :total="total"
                        :page-sizes="[10, 20, 50]"
                        layout="sizes, prev, pager, next"
                        small
                        @change="search"
                    />
                </div>
            </el-col>

            <el-col :xs="24" :md="16" :lg="17">
                <el-empty v-if="!current" :description="$t('pro.node.noNodeSelected')" />
                <template v-else-if="currentOnline">
                    <Section :title="$t('pro.node.agents')" :node="current" :token="token" :loader="loadAgents">
                        <el-table-column prop="name" :label="$t('pro.node.name')" />
                        <el-table-column prop="status" :label="$t('commons.table.status')" />
                    </Section>

                    <Section :title="$t('pro.node.apps')" :node="current" :token="token + (upgradable ? 1000 : 0)" :loader="loadApps">
                        <template #tools>
                            <el-checkbox v-model="upgradable">{{ $t('pro.node.upgradable') }}</el-checkbox>
                        </template>
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="120" show-overflow-tooltip />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="100" />
                        <el-table-column prop="version" :label="$t('pro.node.version')" width="100" />
                        <el-table-column :label="$t('pro.node.installedAt')" width="160">
                            <template #default="{ row }">{{ fmt(row.createdAt) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('pro.node.operate')" width="120">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/logs')">{{ $t('pro.node.log') }}</el-link>
                                <el-link type="primary" :underline="false" class="ml" @click="go(currentRow, '/apps/installed')">
                                    {{ $t('pro.node.moreAction') }}
                                </el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.websites')" :node="current" :token="token" :loader="loadSites">
                        <el-table-column prop="primaryDomain" :label="$t('pro.node.name')" min-width="140" show-overflow-tooltip />
                        <el-table-column prop="type" :label="$t('commons.table.type')" width="100" />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="100" />
                        <el-table-column :label="$t('pro.node.expire')" width="130">
                            <template #default="{ row }">{{ (row.expireDate || '').slice(0, 10) }}</template>
                        </el-table-column>
                        <el-table-column :label="$t('pro.node.operate')" width="80">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/websites')">{{ $t('pro.node.backup') }}</el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.certs')" :node="current" :token="token" :loader="loadCerts">
                        <el-table-column prop="primaryDomain" :label="$t('pro.node.name')" min-width="140" show-overflow-tooltip />
                        <el-table-column prop="provider" :label="$t('commons.table.type')" width="120" />
                        <el-table-column :label="$t('pro.node.expire')" width="130">
                            <template #default="{ row }">{{ (row.expireDate || '').slice(0, 10) }}</template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.databases')" :node="current" :token="token + dbToken" :loader="loadDbs">
                        <template #tools>
                            <el-select v-model="dbType" size="small" style="width: 110px" @change="onDbType">
                                <el-option value="mysql" label="MySQL" />
                                <el-option value="postgresql" label="PostgreSQL" />
                            </el-select>
                            <el-select v-model="dbServer" size="small" style="width: 120px" @change="dbToken++">
                                <el-option v-for="s in dbServers" :key="s" :value="s" :label="s" />
                            </el-select>
                        </template>
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="120" show-overflow-tooltip />
                        <el-table-column prop="permission" :label="$t('pro.node.authUsers')" min-width="120" show-overflow-tooltip />
                        <el-table-column prop="description" :label="$t('pro.node.description')" min-width="120" show-overflow-tooltip />
                        <el-table-column :label="$t('pro.node.operate')" width="80">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/databases')">{{ $t('pro.node.backup') }}</el-link>
                            </template>
                        </el-table-column>
                    </Section>

                    <Section :title="$t('pro.node.crons')" :node="current" :token="token" :loader="loadCrons">
                        <el-table-column prop="name" :label="$t('pro.node.name')" min-width="140" show-overflow-tooltip />
                        <el-table-column prop="status" :label="$t('commons.table.status')" width="100" />
                        <el-table-column prop="lastRecordStatus" :label="$t('pro.node.lastRun')" min-width="120" show-overflow-tooltip />
                        <el-table-column :label="$t('pro.node.operate')" width="80">
                            <template #default>
                                <el-link type="primary" :underline="false" @click="go(currentRow, '/cronjobs')">{{ $t('pro.node.execute') }}</el-link>
                            </template>
                        </el-table-column>
                    </Section>
                </template>
                <el-empty v-else :description="$t('pro.node.offlineHint')" />
            </el-col>
        </el-row>
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Refresh, Search } from '@element-plus/icons-vue';
import { useI18n } from 'vue-i18n';
import router from '@/routers';
import { GlobalStore } from '@/store';
import { getGroupList } from '@/api/modules/group';
import { NodeRow, nodeQuery, restartNode, searchNodes } from '@/xpack/api/node';
import Section from './section.vue';

const { t } = useI18n();
const globalStore = GlobalStore();

const groups = ref<{ id: number; name: string; isDefault: boolean }[]>([]);
const groupID = ref(0);
const info = ref('');
const nodes = ref<NodeRow[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const current = ref('');
const token = ref(0);
let timer: number | undefined;

const nodeName = (n: NodeRow) => (n.isLocal ? 'local' : n.name);
const currentRow = computed(() => nodes.value.find((n) => nodeName(n) === current.value) as NodeRow);
const currentOnline = computed(() => currentRow.value?.status === 'Healthy');
const statusType = (s: string) => (s === 'Healthy' ? 'success' : s === 'Installing' || s === 'Pending' ? 'warning' : 'danger');
const gb = (n: number) => (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
const fmt = (s: string) => (s ? s.replace('T', ' ').slice(0, 19) : '-');

const search = async (silent: any = false) => {
    if (silent !== true) loading.value = true;
    try {
        const res = await searchNodes('nodes', {
            page: page.value,
            pageSize: pageSize.value,
            info: info.value,
            groupID: groupID.value || undefined,
        });
        // the list is keyed by name on the cards below; the master carries the name "local"
        nodes.value = (res.data.items || []).map((n) => ({ ...n, name: nodeName(n) }));
        total.value = res.data.total;
        if (!nodes.value.some((n) => n.name === current.value)) current.value = nodes.value[0]?.name || '';
    } finally {
        loading.value = false;
    }
};
const filter = () => {
    page.value = 1;
    search();
};
const refreshAll = () => {
    search();
    token.value++;
};

// jumping to a node's own page works by making it the operating node first
const go = (n: NodeRow, path: string) => {
    globalStore.currentNode = nodeName(n);
    globalStore.currentNodeAddr = n.addr;
    localStorage.removeItem('dashboardCache');
    router.push(path);
};

const restart = async (n: NodeRow) => {
    await ElMessageBox.confirm(t('pro.node.restartConfirm'), t('pro.node.restart'), { type: 'warning' });
    await restartNode(n.id);
    ElMessage.success(t('pro.node.restartOk'));
};

// --- per-node resource loaders (all go through the master proxy) ---
const upgradable = ref(false);
const loadAgents = async (q: any) => {
    const res = await nodeQuery(current.value, '/ai/agents/search', { page: q.page, pageSize: q.pageSize, name: q.info });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadApps = async (q: any) => {
    const res = await nodeQuery(current.value, '/apps/installed/search', {
        page: q.page,
        pageSize: q.pageSize,
        name: q.info,
        update: upgradable.value,
        type: '',
        sync: false,
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadSites = async (q: any) => {
    const res = await nodeQuery(current.value, '/websites/search', {
        page: q.page,
        pageSize: q.pageSize,
        name: q.info,
        orderBy: 'created_at',
        order: 'null',
        websiteGroupId: 0,
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadCerts = async (q: any) => {
    const res = await nodeQuery(current.value, '/websites/ssl/search', { page: q.page, pageSize: q.pageSize, search: q.info });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};
const loadCrons = async (q: any) => {
    const res = await nodeQuery(current.value, '/cronjobs/search', {
        page: q.page,
        pageSize: q.pageSize,
        info: q.info,
        orderBy: 'created_at',
        order: 'null',
    });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};

const dbType = ref('mysql');
const dbServer = ref('');
const dbServers = ref<string[]>([]);
const dbToken = ref(0);
const loadDbServers = async () => {
    dbServers.value = [];
    dbServer.value = '';
    try {
        const res = await nodeQuery(current.value, '/databases/db/search', { type: dbType.value, page: 1, pageSize: 50, info: '' });
        dbServers.value = (res.data?.items || []).map((d: any) => d.name);
        dbServer.value = dbServers.value[0] || '';
    } catch {
        // no such database engine on this node
    }
    dbToken.value++;
};
const onDbType = () => loadDbServers();
const loadDbs = async (q: any) => {
    if (!dbServer.value) return { items: [], total: 0 };
    const url = dbType.value === 'postgresql' ? '/databases/pg/search' : '/databases/search';
    const res = await nodeQuery(current.value, url, { page: q.page, pageSize: q.pageSize, info: q.info, database: dbServer.value });
    return { items: res.data?.items || [], total: res.data?.total || 0 };
};

watch(current, () => current.value && loadDbServers());

onMounted(async () => {
    groups.value = (await getGroupList('node')).data || [];
    await search();
    // node health changes on its own; keep the cards fresh without reloading the lists
    timer = window.setInterval(() => search(true), 15000);
});
onBeforeUnmount(() => {
    if (timer) window.clearInterval(timer);
});
</script>

<style scoped>
.top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
}
.left {
    display: flex;
    gap: 8px;
}
.node {
    margin-bottom: 10px;
    cursor: pointer;
    border: 1px solid var(--el-border-color-lighter);
}
.node.active {
    border-color: var(--el-color-primary);
    background: var(--el-color-primary-light-9);
}
.line {
    display: flex;
    align-items: center;
    gap: 8px;
}
.nname {
    font-weight: 600;
}
.addr {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.res {
    margin: 8px 0;
    font-size: 12px;
    color: var(--el-text-color-regular);
}
.links {
    display: flex;
    gap: 14px;
    font-size: 12px;
}
.ml {
    margin-left: 10px;
}
.foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12px;
    color: var(--el-text-color-secondary);
}
</style>
