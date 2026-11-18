<template>
    <LayoutContent :title="$t('commons.button.set')">
        <template #main>
            <el-form
                :model="config"
                label-position="left"
                label-width="260px"
                class="ml-2.5 app-setting-form"
                v-loading="loading"
                :rules="rules"
                ref="configForm"
            >
                <el-row>
                    <el-col :xs="24" :sm="20" :md="15" :lg="12" :xl="12">
                        <el-form-item :label="$t('app.uninstallDeleteBackup')" prop="uninstallDeleteBackup">
                            <el-switch
                                v-permission
                                v-model="config.uninstallDeleteBackup"
                                active-value="Enable"
                                inactive-value="Disable"
                                :loading="loading"
                                @change="updateConfig('UninstallDeleteBackup', config.uninstallDeleteBackup)"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('app.uninstallDeleteImage')" prop="uninstallDeleteImage">
                            <el-switch
                                v-permission
                                v-model="config.uninstallDeleteImage"
                                active-value="Enable"
                                inactive-value="Disable"
                                :loading="loading"
                                @change="updateConfig('UninstallDeleteImage', config.uninstallDeleteImage)"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('app.upgradeBackup')" prop="upgradeBackup">
                            <el-switch
                                v-permission
                                v-model="config.upgradeBackup"
                                active-value="Enable"
                                inactive-value="Disable"
                                :loading="loading"
                                @change="updateConfig('UpgradeBackup', config.upgradeBackup)"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('app.upgradeDeleteImage')" prop="upgradeDeleteImage">
                            <el-switch
                                v-permission
                                v-model="config.upgradeDeleteImage"
                                active-value="Enable"
                                inactive-value="Disable"
                                :loading="loading"
                                @change="updateConfig('UpgradeDeleteImage', config.upgradeDeleteImage)"
                            />
                        </el-form-item>
                        <el-form-item :label="$t('app.installAllowPort')" prop="installAllowPort">
                            <el-switch
                                v-permission
                                v-model="config.installAllowPort"
                                active-value="Enable"
                                inactive-value="Disable"
                                :loading="loading"
                                @change="updateConfig('InstallAllowPort', config.installAllowPort)"
                            />
                        </el-form-item>
                        <CustomSetting v-if="customAppAvailable && isXpackOrEE" />
                    </el-col>
                </el-row>
            </el-form>
        </template>
    </LayoutContent>
    <LicenseImport ref="licenseRef" />
</template>

<script setup lang="ts">
import { getCurrentNodeCustomAppConfig } from '@/api/modules/app';
import { getAppStoreConfig, updateAppStoreConfig } from '@/api/modules/setting';
import { FormRules } from 'element-plus';
import { MsgSuccess } from '@/utils/message';
import i18n from '@/lang';
import { defineAsyncComponent } from 'vue';
import { loadOptionalComponent } from '@/extensions/optional';
import { useGlobalStore } from '@/composables/useGlobalStore';
const { isXpackOrEE } = useGlobalStore();

const customAppModules = import.meta.glob('/src/xpack/views/appstore/index.vue');
const customAppAvailable = Object.keys(customAppModules).length > 0;
const CustomSetting = customAppAvailable
    ? defineAsyncComponent(() => loadOptionalComponent('/src/xpack/views/appstore/index.vue'))
    : null;

const rules = ref<FormRules>({});
const config = ref({
    uninstallDeleteImage: '',
    uninstallDeleteBackup: '',
    upgradeBackup: '',
    upgradeDeleteImage: '',
    installAllowPort: '',
});
const loading = ref(false);
const configForm = ref();
const useCustomApp = ref(false);
const isInitializing = ref(true);
const licenseRef = ref();

const search = async () => {
    loading.value = true;
    try {
        const res = await getAppStoreConfig();
        if (res && res.data) {
            isInitializing.value = true;
            config.value = res.data;
            setTimeout(() => {
                isInitializing.value = false;
            }, 0);
        }
    } catch (error) {
    } finally {
        loading.value = false;
    }
};

const toUpload = () => {
    licenseRef.value.acceptParams();
};

const getNodeConfig = async () => {
    if (!customAppAvailable || isXpackOrEE.value) {
        return;
    }
    try {
        const res = await getCurrentNodeCustomAppConfig();
        if (res && res.data) {
            useCustomApp.value = res.data.status === 'enable';
        }
    } catch {}
};

const updateConfig = async (scope: string, value: string) => {
    if (isInitializing.value) {
        return;
    }
    loading.value = true;
    try {
        const req = {
            scope: scope,
            status: value,
        };
        await updateAppStoreConfig(req);
        MsgSuccess(i18n.global.t('commons.msg.updateSuccess'));
        search();
    } catch (error) {
    } finally {
        loading.value = false;
    }
};

onMounted(() => {
    search();
    getNodeConfig();
});
</script>

<style lang="css" scoped>
.app-setting-form :deep(.el-form-item) {
    align-items: flex-start;
}

.app-setting-form :deep(.el-form-item__label) {
    white-space: normal;
    line-height: 20px;
    word-break: break-word;
    padding-top: 6px;
}

.logText {
    line-height: 22px;
    font-size: 12px;
    .link {
        font-size: 12px !important;
        margin-top: -3px;
    }
}
</style>
