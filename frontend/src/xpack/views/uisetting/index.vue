<template>
    <div v-loading="loading">
        <el-alert type="info" show-icon :closable="false" class="mb-3" :title="$t('pro.uisetting.desc')" />

        <div class="settings-grid">
            <el-card shadow="never" class="setting-card">
                <template #header>
                    <span class="title">{{ $t('pro.uisetting.brand') }}</span>
                </template>
                <div class="card-body-inner">
                    <el-form label-width="165px" class="setting-form">
                        <el-form-item :label="$t('pro.uisetting.panelName')">
                            <el-input v-model="form.title" maxlength="60" />
                            <div class="hint">{{ $t('pro.uisetting.panelNameHint') }}</div>
                        </el-form-item>
                        <el-form-item :label="$t('pro.uisetting.masterAlias')">
                            <el-input v-model="form.masterAlias" maxlength="30" />
                        </el-form-item>
                        <el-form-item v-for="k in brandKinds" :key="k" :label="$t('pro.uisetting.' + k)">
                            <div class="upload-row">
                                <div v-if="form[k]" class="thumb-wrap">
                                    <img :src="imgUrl(k)" class="thumb" />
                                </div>
                                <el-tag size="small" :type="form[k] ? 'success' : 'info'" effect="light">
                                    {{ form[k] ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                                </el-tag>
                                <el-upload
                                    :show-file-list="false"
                                    :auto-upload="false"
                                    accept="image/*"
                                    :on-change="(f: any) => pick(k, f)"
                                >
                                    <el-button size="small" type="primary" plain>{{ $t('pro.uisetting.upload') }}</el-button>
                                </el-upload>
                                <el-button size="small" :disabled="!form[k]" @click="reset(k)">
                                    {{ $t('pro.uisetting.reset') }}
                                </el-button>
                                <span class="size-hint">{{ $t('pro.uisetting.sizeHint') }}</span>
                            </div>
                        </el-form-item>
                    </el-form>
                    <div class="card-actions">
                        <el-button type="primary" @click="saveText">{{ $t('pro.uisetting.save') }}</el-button>
                    </div>
                </div>
            </el-card>

            <el-card shadow="never" class="setting-card">
                <template #header>
                    <span class="title">{{ $t('pro.uisetting.loginPage') }}</span>
                </template>
                <div class="card-body-inner">
                    <el-form label-width="165px" class="setting-form">
                        <el-form-item :label="$t('pro.uisetting.loginImage')">
                            <div class="upload-row">
                                <div v-if="form.loginImage" class="thumb-wrap">
                                    <img :src="imgUrl('loginImage')" class="thumb" />
                                </div>
                                <el-tag size="small" :type="form.loginImage ? 'success' : 'info'" effect="light">
                                    {{ form.loginImage ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                                </el-tag>
                                <el-upload
                                    :show-file-list="false"
                                    :auto-upload="false"
                                    accept="image/*"
                                    :on-change="(f: any) => pick('loginImage', f)"
                                >
                                    <el-button size="small" type="primary" plain>{{ $t('pro.uisetting.upload') }}</el-button>
                                </el-upload>
                                <el-button size="small" :disabled="!form.loginImage" @click="reset('loginImage')">
                                    {{ $t('pro.uisetting.reset') }}
                                </el-button>
                                <span class="size-hint">{{ $t('pro.uisetting.sizeHint') }}</span>
                            </div>
                        </el-form-item>
                        <el-form-item :label="$t('pro.uisetting.loginBgType')">
                            <el-radio-group v-model="form.loginBgType">
                                <el-radio-button value="image">{{ $t('pro.uisetting.bgImage') }}</el-radio-button>
                                <el-radio-button value="color">{{ $t('pro.uisetting.bgColor') }}</el-radio-button>
                            </el-radio-group>
                        </el-form-item>
                        <el-form-item v-if="form.loginBgType === 'color'" :label="$t('pro.uisetting.loginBgColor')">
                            <div class="color-row">
                                <el-color-picker v-model="bgColor" />
                                <span class="mono color-hex">{{ bgColor }}</span>
                            </div>
                        </el-form-item>
                        <el-form-item v-else :label="$t('pro.uisetting.loginBackground')">
                            <div class="upload-row">
                                <div v-if="hasBgImage" class="thumb-wrap wide">
                                    <img :src="imgUrl('loginBackground')" class="thumb wide" />
                                </div>
                                <el-tag size="small" :type="hasBgImage ? 'success' : 'info'" effect="light">
                                    {{ hasBgImage ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                                </el-tag>
                                <el-upload
                                    :show-file-list="false"
                                    :auto-upload="false"
                                    accept="image/*"
                                    :on-change="(f: any) => pick('loginBackground', f)"
                                >
                                    <el-button size="small" type="primary" plain>{{ $t('pro.uisetting.upload') }}</el-button>
                                </el-upload>
                                <el-button size="small" :disabled="!hasBgImage" @click="reset('loginBackground')">
                                    {{ $t('pro.uisetting.reset') }}
                                </el-button>
                            </div>
                        </el-form-item>
                        <el-form-item :label="$t('pro.uisetting.linkColor')">
                            <div class="color-row">
                                <el-color-picker v-model="form.loginBtnLinkColor" />
                                <span v-if="form.loginBtnLinkColor" class="mono color-hex">{{ form.loginBtnLinkColor }}</span>
                            </div>
                        </el-form-item>
                    </el-form>
                    <div class="card-actions">
                        <el-button type="primary" @click="saveLogin">{{ $t('pro.uisetting.save') }}</el-button>
                    </div>
                </div>
            </el-card>
        </div>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { searchXSetting, updateXSettingByKey, uploadXImage } from '@/xpack/api/modules/setting';
import { getXpackSettingForTheme } from '@/utils/xpack';

const { t } = useI18n();
const brandKinds = ['logo', 'logoWithText', 'favicon'];
const loading = ref(false);
const stamp = ref(Date.now());
const bgColor = ref('#ffffff');
const hasBgImage = ref(false);
const form = reactive<Record<string, string>>({
    title: '',
    masterAlias: '',
    logo: '',
    logoWithText: '',
    favicon: '',
    loginImage: '',
    loginBgType: 'image',
    loginBackground: '',
    loginBtnLinkColor: '',
});

const imgUrl = (k: string) => `/api/v2/images/${k}?t=${stamp.value}`;

const probeBgImage = (): Promise<boolean> =>
    new Promise((resolve) => {
        const img = new Image();
        img.onload = () => resolve(true);
        img.onerror = () => resolve(false);
        img.src = imgUrl('loginBackground');
    });

const load = async () => {
    loading.value = true;
    try {
        const d = (await searchXSetting()).data || {};
        for (const k of Object.keys(form)) {
            form[k] = d[k] || (k === 'loginBgType' ? 'image' : '');
        }
        if (form.loginBackground && form.loginBackground !== 'loginBackground') {
            bgColor.value = form.loginBackground;
            hasBgImage.value = await probeBgImage();
        } else if (form.loginBackground === 'loginBackground') {
            hasBgImage.value = true;
        } else {
            hasBgImage.value = false;
        }
    } finally {
        loading.value = false;
    }
};
const refreshTheme = async () => {
    stamp.value = Date.now();
    await getXpackSettingForTheme();
};
const pick = async (kind: string, f: any) => {
    const file: File = f.raw;
    if (!file) return;
    if (file.size > 4 * 1024 * 1024) {
        ElMessage.warning(t('pro.uisetting.sizeHint'));
        return;
    }
    loading.value = true;
    try {
        await uploadXImage(kind, file);
        if (kind === 'loginBackground') {
            hasBgImage.value = true;
            await updateXSettingByKey('LoginBgType', 'image');
        }
        ElMessage.success(t('pro.uisetting.uploaded'));
        await load();
        await refreshTheme();
    } finally {
        loading.value = false;
    }
};
const keyOf = (k: string) => k.charAt(0).toUpperCase() + k.slice(1);
const reset = async (kind: string) => {
    await updateXSettingByKey(keyOf(kind), '');
    if (kind === 'loginBackground') {
        hasBgImage.value = false;
    }
    ElMessage.success(t('pro.uisetting.resetOk'));
    await load();
    await refreshTheme();
};
const saveText = async () => {
    await updateXSettingByKey('Title', form.title);
    await updateXSettingByKey('MasterAlias', form.masterAlias);
    ElMessage.success(t('pro.uisetting.saved'));
    await refreshTheme();
};
const saveLogin = async () => {
    await updateXSettingByKey('LoginBgType', form.loginBgType);
    if (form.loginBgType === 'color') {
        // Save solid hex colour without clearing the uploaded loginBackground file on disk
        await updateXSettingByKey('LoginBackground', bgColor.value || '#ffffff');
    } else if (hasBgImage.value && form.loginBackground !== 'loginBackground') {
        // Restore custom background image marker when switching back from solid color mode
        await updateXSettingByKey('LoginBackground', 'loginBackground');
    }
    await updateXSettingByKey('LoginBtnLinkColor', form.loginBtnLinkColor || '');
    ElMessage.success(t('pro.uisetting.saved'));
    await load();
    await refreshTheme();
};

onMounted(load);
</script>

<style scoped>
.settings-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    align-items: stretch;
}
.setting-card {
    height: 100%;
    display: flex;
    flex-direction: column;
}
.setting-card :deep(.el-card__body) {
    flex: 1;
    display: flex;
    flex-direction: column;
}
.card-body-inner {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
}
.setting-form {
    max-width: 100%;
}
.title {
    font-weight: 600;
    font-size: 15px;
    color: var(--el-text-color-primary);
}
.hint {
    margin-top: 4px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
    width: 100%;
}
.size-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.upload-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}
.thumb-wrap {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 4px 8px;
    border: 1px solid var(--el-border-color);
    border-radius: 6px;
    background-color: var(--el-fill-color-blank);
    background-image:
        linear-gradient(45deg, rgba(128, 128, 128, 0.16) 25%, transparent 25%),
        linear-gradient(-45deg, rgba(128, 128, 128, 0.16) 25%, transparent 25%),
        linear-gradient(45deg, transparent 75%, rgba(128, 128, 128, 0.16) 75%),
        linear-gradient(-45deg, transparent 75%, rgba(128, 128, 128, 0.16) 75%);
    background-size: 12px 12px;
    background-position: 0 0, 0 6px, 6px -6px, -6px 0;
}
.thumb {
    display: block;
    height: 32px;
    max-width: 120px;
    object-fit: contain;
}
.thumb.wide {
    height: 44px;
    max-width: 160px;
}
.color-row {
    display: flex;
    align-items: center;
    gap: 10px;
}
.color-hex {
    font-size: 12.5px;
    color: var(--el-text-color-secondary);
}
.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.card-actions {
    margin-top: 12px;
    padding-top: 14px;
    border-top: 1px solid var(--el-border-color-lighter);
    display: flex;
    justify-content: flex-end;
}

@media (max-width: 1279px) {
    .settings-grid {
        grid-template-columns: minmax(0, 1fr);
    }
}
</style>
