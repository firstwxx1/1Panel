import { applyPanelCacheSettings } from '@/utils/panel-cache';
import {
    getLicenseStatus,
    getMasterLicenseStatus,
    getSettingBaseInfo,
    getEnterpriseLicenseStatus,
} from '@/api/modules/setting';
import { useTheme } from '@/global/use-theme';
import {
    searchXpackSetting,
    updateXpackSettingByKey as updateXpackSettingByKeyFromExtension,
} from '@/extensions/xpack';
import { GlobalStore } from '@/store';
import faviconUrl from '@/assets/images/favicon.svg';

let switchThemeFn: (() => void) | undefined;

const switchTheme = () => {
    if (!switchThemeFn) {
        switchThemeFn = useTheme().switchTheme;
    }
    switchThemeFn();
};

export function resetXSetting() {
    const globalStore = GlobalStore();
    globalStore.themeConfig.title = '';
    globalStore.themeConfig.logo = '';
    globalStore.themeConfig.logoWithText = '';
    globalStore.themeConfig.favicon = '';
    globalStore.watermark = null;
    globalStore.watermarkShow = false;
    globalStore.masterAlias = '';
}

async function getColoredFavicon(url: string, color: string) {
    const res = await fetch(url);
    let svgText = await res.text();
    svgText = svgText.replace(/fill=(["'])(.*?)\1/g, `fill="${color}"`);
    return `data:image/svg+xml,${encodeURIComponent(svgText)}`;
}

export async function initFavicon() {
    const globalStore = GlobalStore();
    document.title = globalStore.themeConfig.panelName;
    const favicon = globalStore.themeConfig.favicon;
    const isPro = globalStore.isUiCustom;
    const themeColor = globalStore.themeConfig.primary;
    const customFaviconUrl = `/api/v2/images/favicon?t=${Date.now()}`;
    const fallbackSvg = isPro ? await getColoredFavicon(faviconUrl, themeColor) : '/public/favicon.png';
    const setLink = (href: string) => {
        let link = document.querySelector("link[rel*='icon']") as HTMLLinkElement;
        if (!link) {
            link = document.createElement('link');
            link.rel = 'shortcut icon';
            link.type = 'image/x-icon';
            document.head.appendChild(link);
        }
        link.href = href;
    };

    if (favicon) {
        const testImg = new Image();
        testImg.onload = () => setLink(customFaviconUrl);
        testImg.onerror = () => setLink(fallbackSvg);
        testImg.src = customFaviconUrl;
    } else {
        setLink(fallbackSvg);
    }
}

export async function getXpackSetting() {
    const res = await searchXpackSetting();
    if (!res) {
        initFavicon();
        resetXSetting();
        return;
    }
    initFavicon();
    return res;
}

const loadDataFromDB = async () => {
    try {
        const globalStore = GlobalStore();
        const res = await getSettingBaseInfo();
        document.title = res.data.panelName;
        globalStore.entrance = res.data.securityEntrance;
        globalStore.openMenuTabs = res.data.menuTabs === 'Enable';
        globalStore.menuAccordion = res.data.menuAccordion === 'Enable';
        applyPanelCacheSettings(res.data.panelCache);
    } catch {
        // ignore base info error during unauthenticated bootstrap
    }
};

export async function loadProductProFromDB() {
    const globalStore = GlobalStore();
    if (!globalStore.isEnterprise) {
        globalStore.isEnterpriseLicenseLoaded = true;
        try {
            const res = await getLicenseStatus();
            if (res && res.data) {
                globalStore.isProductPro = res.data.status === 'Bound' || true;
                globalStore.productProExpires = Number(res.data.productPro) || 253402300799;
            } else {
                globalStore.isProductPro = true;
            }
        } catch {
            globalStore.isProductPro = true;
        }
        return;
    }
    try {
        const res = await getEnterpriseLicenseStatus();
        globalStore.isEnterpriseLicenseLoaded = true;
        if (!res || !res.data) {
            globalStore.isEnterpriseLicensed = false;
            globalStore.isProductPro = true;
            globalStore.productProExpires = 0;
        } else {
            globalStore.isEnterpriseLicensed = res.data.status === 'Bound';
            globalStore.isProductPro = true;
            globalStore.productProExpires = globalStore.isEnterpriseLicensed ? Number(res.data.productPro) : 0;
        }
    } catch {
        globalStore.isEnterpriseLicenseLoaded = true;
        globalStore.isProductPro = true;
    }
}

export async function loadMasterProductProFromDB() {
    const globalStore = GlobalStore();
    try {
        if (!globalStore.isEnterprise) {
            globalStore.isEnterpriseLicenseLoaded = true;
            try {
                const res = await getMasterLicenseStatus();
                globalStore.isMasterProductPro = res?.data ? res.data.status === 'Bound' || true : true;
            } catch {
                globalStore.isMasterProductPro = true;
            }
        } else {
            try {
                const res = await getEnterpriseLicenseStatus();
                globalStore.isEnterpriseLicenseLoaded = true;
                if (!res || !res.data) {
                    globalStore.isEnterpriseLicensed = false;
                    globalStore.isMasterProductPro = true;
                    globalStore.isProductPro = true;
                    globalStore.productProExpires = 0;
                } else {
                    globalStore.isEnterpriseLicensed = res.data.status === 'Bound';
                    globalStore.isMasterProductPro = true;
                    globalStore.isProductPro = true;
                    globalStore.productProExpires = globalStore.isEnterpriseLicensed ? Number(res.data.productPro) : 0;
                }
            } catch {
                globalStore.isEnterpriseLicenseLoaded = true;
                globalStore.isMasterProductPro = true;
                globalStore.isProductPro = true;
            }
        }
    } finally {
        switchTheme();
        initFavicon();
        await loadDataFromDB();
    }
}

export async function getXpackSettingForTheme() {
    const globalStore = GlobalStore();
    const res2 = await searchXpackSetting();
    if (res2) {
        globalStore.themeConfig.title = res2.data?.title;
        globalStore.themeConfig.logo = res2.data?.logo;
        globalStore.themeConfig.logoWithText = res2.data?.logoWithText;
        globalStore.themeConfig.favicon = res2.data?.favicon;
        globalStore.themeConfig.loginImage = res2.data?.loginImage;
        globalStore.themeConfig.loginBgType = res2.data?.loginBgType;
        globalStore.themeConfig.loginBackground = res2.data?.loginBackground;
        globalStore.themeConfig.loginBtnLinkColor = res2.data?.loginBtnLinkColor;
        globalStore.themeConfig.themeColor = res2.data?.themeColor;
        globalStore.masterAlias = res2.data.masterAlias;
        if (res2.data?.theme) {
            globalStore.themeConfig.theme = res2.data.theme;
        }
        globalStore.watermarkShow = res2.data.watermarkShow === 'Enable';
        try {
            globalStore.watermark = JSON.parse(res2.data.watermark);
        } catch {
            globalStore.watermark = null;
        }
    } else {
        resetXSetting();
    }
    switchTheme();
    initFavicon();
}

export async function updateXpackSettingByKey(key: string, value: string) {
    return updateXpackSettingByKeyFromExtension(key, value);
}

interface CanonicalProMenuEntry {
    id: string;
    label: string;
    path: string;
    title: string;
}

const PRO_HIDE_MENU_ITEMS: CanonicalProMenuEntry[] = [
    { id: '112', label: 'ProWaf', path: '/xpack/waf', title: 'pro.waf.title' },
    { id: '113', label: 'ProMonitor', path: '/xpack/monitor', title: 'pro.monitor.title' },
    { id: '115', label: 'ProSync', path: '/xpack/sync', title: 'pro.sync.title' },
    { id: '114', label: 'ProTamper', path: '/xpack/tamper', title: 'pro.tamper.title' },
    { id: '116', label: 'ProAlert', path: '/xpack/alert', title: 'pro.alert.title' },
    { id: '117', label: 'ProUiSetting', path: '/xpack/uisetting', title: 'pro.uisetting.title' },
    { id: '111', label: 'NodeDashboard', path: '/xpack/node', title: 'pro.node.title' },
];

const PRO_MENU_BY_LABEL = new Map<string, CanonicalProMenuEntry>(
    PRO_HIDE_MENU_ITEMS.map((item) => [item.label, item]),
);

const LEGACY_XPACK_HIDE_MENU_MAP: Record<string, CanonicalProMenuEntry> = {
    Dashboard: PRO_MENU_BY_LABEL.get('ProWaf')!,
    MonitorDashboard: PRO_MENU_BY_LABEL.get('ProMonitor')!,
    Sync: PRO_MENU_BY_LABEL.get('ProSync')!,
    Tamper: PRO_MENU_BY_LABEL.get('ProTamper')!,
    XSetting: PRO_MENU_BY_LABEL.get('ProUiSetting')!,
    NodeDashboard: PRO_MENU_BY_LABEL.get('NodeDashboard')!,
};

const RETIRED_XPACK_HIDE_MENU_LABELS = new Set([
    'XApp',
    'VirtualMachine',
    'Cluster',
    'Upage',
    'UserManagement',
    'OpsReport',
]);

export function reconcileLegacyXpackHideMenu(hideMenu: any[], registeredRouteNames?: Set<string>): any[] {
    if (!Array.isArray(hideMenu)) {
        return [];
    }

    const seenLabels = new Set<string>();

    const normalizeList = (nodes: any[]) => {
        const cleaned: any[] = [];
        for (const node of nodes) {
            if (!node || typeof node !== 'object') {
                continue;
            }

            const mapped = LEGACY_XPACK_HIDE_MENU_MAP[node.label] || PRO_MENU_BY_LABEL.get(node.label);
            if (mapped) {
                node.id = mapped.id;
                node.label = mapped.label;
                node.path = mapped.path;
                node.title = mapped.title;
            } else if (node.label === 'Xpack-Menu') {
                node.title = 'pro.menu';
            }

            if (RETIRED_XPACK_HIDE_MENU_LABELS.has(node.label)) {
                continue;
            }

            if (Array.isArray(node.children) && node.children.length > 0) {
                normalizeList(node.children);
                if (
                    node.label !== 'Xpack-Menu' &&
                    node.children.length === 0 &&
                    registeredRouteNames &&
                    node.label &&
                    !registeredRouteNames.has(node.label)
                ) {
                    continue;
                }
            } else if (
                node.label !== 'Xpack-Menu' &&
                registeredRouteNames &&
                node.label &&
                !registeredRouteNames.has(node.label)
            ) {
                continue;
            }

            if (node.label) {
                if (seenLabels.has(node.label)) {
                    continue;
                }
                seenLabels.add(node.label);
            }

            cleaned.push(node);
        }
        nodes.splice(0, nodes.length, ...cleaned);
    };

    normalizeList(hideMenu);

    const missingProItems = PRO_HIDE_MENU_ITEMS.filter((item) => !seenLabels.has(item.label));
    let xpackMenuNode = hideMenu.find((item) => item.label === 'Xpack-Menu');

    if (missingProItems.length > 0) {
        if (!xpackMenuNode) {
            xpackMenuNode = {
                id: '11',
                disabled: false,
                title: 'pro.menu',
                isShow: true,
                label: 'Xpack-Menu',
                path: '/xpack',
                sort: 350,
                children: [],
            };
            hideMenu.push(xpackMenuNode);
        }
        if (!Array.isArray(xpackMenuNode.children)) {
            xpackMenuNode.children = [];
        }
        let maxSort = xpackMenuNode.children.reduce(
            (acc: number, child: any) => Math.max(acc, Number(child?.sort) || 0),
            0,
        );
        const defaultShow = xpackMenuNode.isShow ?? true;
        for (const item of missingProItems) {
            maxSort += 100;
            xpackMenuNode.children.push({
                id: item.id,
                title: item.title,
                path: item.path,
                label: item.label,
                isShow: defaultShow,
                sort: maxSort,
            });
        }
    } else if (xpackMenuNode && (!Array.isArray(xpackMenuNode.children) || xpackMenuNode.children.length === 0)) {
        const idx = hideMenu.indexOf(xpackMenuNode);
        if (idx >= 0) {
            hideMenu.splice(idx, 1);
        }
    }

    return hideMenu;
}
