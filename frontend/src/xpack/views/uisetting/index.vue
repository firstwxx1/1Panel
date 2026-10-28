<template>
    <div v-loading="loading">
        <el-card shadow="never" class="mb-3">
            <div class="desc">{{ $t('pro.uisetting.desc') }}</div>
        </el-card>

        <el-card shadow="never" class="mb-3">
            <div class="title">{{ $t('pro.uisetting.brand') }}</div>
            <el-form label-width="180px" class="mt-3" style="max-width: 760px">
                <el-form-item :label="$t('pro.uisetting.panelName')">
                    <el-input v-model="form.title" maxlength="60" />
                    <div class="hint">{{ $t('pro.uisetting.panelNameHint') }}</div>
                </el-form-item>
                <el-form-item :label="$t('pro.uisetting.masterAlias')">
                    <el-input v-model="form.masterAlias" maxlength="30" />
                </el-form-item>
                <el-form-item v-for="k in brandKinds" :key="k" :label="$t('pro.uisetting.' + k)">
                    <div class="row">
                        <img v-if="form[k]" :src="imgUrl(k)" class="thumb" />
                        <el-tag :type="form[k] ? 'success' : 'info'">
                            {{ form[k] ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                        </el-tag>
                        <el-upload :show-file-list="false" :auto-upload="false" accept="image/*" :on-change="(f: any) => pick(k, f)">
                            <el-button size="small">{{ $t('pro.uisetting.upload') }}</el-button>
                        </el-upload>
                        <el-button size="small" :disabled="!form[k]" @click="reset(k)">{{ $t('pro.uisetting.reset') }}</el-button>
                        <span class="hint">{{ $t('pro.uisetting.sizeHint') }}</span>
                    </div>
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" @click="saveText">{{ $t('pro.uisetting.save') }}</el-button>
                </el-form-item>
            </el-form>
        </el-card>

        <el-card shadow="never">
            <div class="title">{{ $t('pro.uisetting.loginPage') }}</div>
            <el-form label-width="180px" class="mt-3" style="max-width: 760px">
                <el-form-item :label="$t('pro.uisetting.loginImage')">
                    <div class="row">
                        <img v-if="form.loginImage" :src="imgUrl('loginImage')" class="thumb" />
                        <el-tag :type="form.loginImage ? 'success' : 'info'">
                            {{ form.loginImage ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                        </el-tag>
                        <el-upload :show-file-list="false" :auto-upload="false" accept="image/*" :on-change="(f: any) => pick('loginImage', f)">
                            <el-button size="small">{{ $t('pro.uisetting.upload') }}</el-button>
                        </el-upload>
                        <el-button size="small" :disabled="!form.loginImage" @click="reset('loginImage')">
                            {{ $t('pro.uisetting.reset') }}
                        </el-button>
                    </div>
                </el-form-item>
                <el-form-item :label="$t('pro.uisetting.loginBgType')">
                    <el-radio-group v-model="form.loginBgType">
                        <el-radio-button value="image">{{ $t('pro.uisetting.bgImage') }}</el-radio-button>
                        <el-radio-button value="color">{{ $t('pro.uisetting.bgColor') }}</el-radio-button>
                    </el-radio-group>
                </el-form-item>
                <el-form-item v-if="form.loginBgType === 'color'" :label="$t('pro.uisetting.loginBgColor')">
                    <el-color-picker v-model="bgColor" />
                </el-form-item>
                <el-form-item v-else :label="$t('pro.uisetting.loginBackground')">
                    <div class="row">
                        <img v-if="form.loginBackground === 'loginBackground'" :src="imgUrl('loginBackground')" class="thumb wide" />
                        <el-tag :type="form.loginBackground === 'loginBackground' ? 'success' : 'info'">
                            {{ form.loginBackground === 'loginBackground' ? $t('pro.uisetting.custom') : $t('pro.uisetting.isDefault') }}
                        </el-tag>
                        <el-upload :show-file-list="false" :auto-upload="false" accept="image/*" :on-change="(f: any) => pick('loginBackground', f)">
                            <el-button size="small">{{ $t('pro.uisetting.upload') }}</el-button>
                        </el-upload>
                        <el-button size="small" :disabled="form.loginBackground !== 'loginBackground'" @click="reset('loginBackground')">
                            {{ $t('pro.uisetting.reset') }}
                        </el-button>
                    </div>
                </el-form-item>
                <el-form-item :label="$t('pro.uisetting.linkColor')">
                    <el-color-picker v-model="form.loginBtnLinkColor" />
                </el-form-item>
                <el-form-item>
                    <el-button type="primary" @click="saveLogin">{{ $t('pro.uisetting.save') }}</el-button>
                </el-form-item>
            </el-form>
        </el-card>
    </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { ElMessage } from 'element-plus';
import { searchXSetting, updateXSettingByKey, uploadXImage } from '@/xpack/api/modules/setting';
import { getXpackSettingForTheme } from '@/utils/xpack';
import i18n from '@/lang';

const t = i18n.global.t;
const brandKinds = ['logo', 'logoWithText', 'favicon'];
const loading = ref(false);
const stamp = ref(Date.now());
const bgColor = ref('#ffffff');
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

const load = async () => {
    loading.value = true;
    try {
        const d = (await searchXSetting()).data;
        for (const k of Object.keys(form)) form[k] = d[k] || (k === 'loginBgType' ? 'image' : '');
        bgColor.value = form.loginBgType === 'color' && form.loginBackground ? form.loginBackground : '#ffffff';
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
        if (kind === 'loginBackground') await updateXSettingByKey('LoginBgType', 'image');
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
    if (form.loginBgType === 'color') await updateXSettingByKey('LoginBackground', bgColor.value || '');
    else if (form.loginBackground !== 'loginBackground') await updateXSettingByKey('LoginBackground', '');
    await updateXSettingByKey('LoginBtnLinkColor', form.loginBtnLinkColor || '');
    ElMessage.success(t('pro.uisetting.saved'));
    await load();
    await refreshTheme();
};

onMounted(load);
</script>

<style scoped>
.title {
    font-weight: 600;
}
.desc {
    color: var(--el-text-color-secondary);
    font-size: 13px;
    line-height: 1.6;
}
.hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.5;
    width: 100%;
}
.row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}
.thumb {
    height: 36px;
    max-width: 120px;
    object-fit: contain;
    border: 1px solid var(--el-border-color);
    border-radius: 4px;
    background: #fff;
}
.thumb.wide {
    height: 48px;
    max-width: 160px;
}
</style>
