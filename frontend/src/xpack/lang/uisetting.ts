import i18n from '@/lang';

const messages = {
    zh: {
        title: '界面设置',
        desc: '自定义面板的图标和登录页外观。面板名称、主题色、水印请在「面板设置」中修改。',
        brand: '品牌',
        panelName: '面板简介',
        panelNameHint: '显示在「关于」页，留空使用默认简介',
        masterAlias: '主控节点别名',
        logo: '菜单图标（折叠时）',
        logoWithText: '菜单 Logo（带文字）',
        favicon: '浏览器图标 Favicon',
        loginPage: '登录页',
        loginImage: '登录页插图',
        loginBgType: '背景类型',
        bgImage: '图片',
        bgColor: '纯色',
        loginBackground: '登录页背景图',
        loginBgColor: '背景颜色',
        linkColor: '登录按钮/链接颜色',
        upload: '上传',
        reset: '恢复默认',
        custom: '已自定义',
        isDefault: '默认',
        save: '保存',
        saved: '已保存',
        uploaded: '上传成功',
        resetOk: '已恢复默认',
        sizeHint: '图片不超过 4MB',
        preview: '预览',
    },
};

for (const [locale, msg] of Object.entries(messages)) {
    i18n.global.mergeLocaleMessage(locale, { pro: { uisetting: msg } });
}
