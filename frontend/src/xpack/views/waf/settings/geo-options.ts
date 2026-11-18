import isoRaw from '@/assets/json/iso.json';
import { GeoRestrictConfig } from '@/xpack/api/waf';

export interface GeoCountryOption {
    iso: string;
    zh: string;
    en: string;
}

export interface GeoProvinceOption {
    value: string;
    zh: string;
    en: string;
}

const PRIORITY_COUNTRIES: GeoCountryOption[] = [
    { iso: 'CN', zh: '中国大陆', en: 'China Mainland' },
    { iso: 'HK', zh: '中国香港', en: 'Hong Kong' },
    { iso: 'MO', zh: '中国澳门', en: 'Macau' },
    { iso: 'TW', zh: '中国台湾', en: 'Taiwan' },
    { iso: 'US', zh: '美国', en: 'United States' },
    { iso: 'JP', zh: '日本', en: 'Japan' },
    { iso: 'SG', zh: '新加坡', en: 'Singapore' },
    { iso: 'KR', zh: '韩国', en: 'South Korea' },
    { iso: 'RU', zh: '俄罗斯', en: 'Russia' },
    { iso: 'DE', zh: '德国', en: 'Germany' },
    { iso: 'GB', zh: '英国', en: 'United Kingdom' },
    { iso: 'FR', zh: '法国', en: 'France' },
    { iso: 'NL', zh: '荷兰', en: 'Netherlands' },
    { iso: 'CA', zh: '加拿大', en: 'Canada' },
    { iso: 'AU', zh: '澳大利亚', en: 'Australia' },
    { iso: 'IN', zh: '印度', en: 'India' },
    { iso: 'VN', zh: '越南', en: 'Vietnam' },
    { iso: 'TH', zh: '泰国', en: 'Thailand' },
    { iso: 'MY', zh: '马来西亚', en: 'Malaysia' },
    { iso: 'ID', zh: '印度尼西亚', en: 'Indonesia' },
    { iso: 'PH', zh: '菲律宾', en: 'Philippines' },
    { iso: 'BR', zh: '巴西', en: 'Brazil' },
];

const seen = new Set<string>(PRIORITY_COUNTRIES.map((c) => c.iso));
const restCountries: GeoCountryOption[] = [];
for (const item of isoRaw as any[]) {
    const code = String(item.iso || '').trim().toUpperCase();
    if (!code || seen.has(code)) continue;
    seen.add(code);
    restCountries.push({
        iso: code,
        zh: item.zh || code,
        en: item.en || code,
    });
}

export const GEO_COUNTRIES: GeoCountryOption[] = [...PRIORITY_COUNTRIES, ...restCountries];

// Verified against 1pwaf GeoIP.mmdb English subdivision names
export const CN_PROVINCES: GeoProvinceOption[] = [
    { value: 'Beijing', zh: '北京', en: 'Beijing' },
    { value: 'Shanghai', zh: '上海', en: 'Shanghai' },
    { value: 'Guangdong', zh: '广东', en: 'Guangdong' },
    { value: 'Zhejiang', zh: '浙江', en: 'Zhejiang' },
    { value: 'Jiangsu', zh: '江苏', en: 'Jiangsu' },
    { value: 'Sichuan', zh: '四川', en: 'Sichuan' },
    { value: 'Hubei', zh: '湖北', en: 'Hubei' },
    { value: 'Fujian', zh: '福建', en: 'Fujian' },
    { value: 'Shandong', zh: '山东', en: 'Shandong' },
    { value: 'Henan', zh: '河南', en: 'Henan' },
    { value: 'Hunan', zh: '湖南', en: 'Hunan' },
    { value: 'Anhui', zh: '安徽', en: 'Anhui' },
    { value: 'Hebei', zh: '河北', en: 'Hebei' },
    { value: 'Shaanxi', zh: '陕西', en: 'Shaanxi' },
    { value: 'Liaoning', zh: '辽宁', en: 'Liaoning' },
    { value: 'Chongqing', zh: '重庆', en: 'Chongqing' },
    { value: 'Tianjin', zh: '天津', en: 'Tianjin' },
    { value: 'Yunnan', zh: '云南', en: 'Yunnan' },
    { value: 'Guangxi', zh: '广西', en: 'Guangxi' },
    { value: 'Jiangxi', zh: '江西', en: 'Jiangxi' },
    { value: 'Guizhou', zh: '贵州', en: 'Guizhou' },
    { value: 'Shanxi', zh: '山西', en: 'Shanxi' },
    { value: 'Heilongjiang', zh: '黑龙江', en: 'Heilongjiang' },
    { value: 'Jilin', zh: '吉林', en: 'Jilin' },
    { value: 'Gansu', zh: '甘肃', en: 'Gansu' },
    { value: 'Hainan', zh: '海南', en: 'Hainan' },
    { value: 'Inner Mongolia', zh: '内蒙古', en: 'Inner Mongolia' },
    { value: 'Ningxia', zh: '宁夏', en: 'Ningxia' },
    { value: 'Xinjiang', zh: '新疆', en: 'Xinjiang' },
    { value: 'Qinghai', zh: '青海', en: 'Qinghai' },
    { value: 'Tibet', zh: '西藏', en: 'Tibet' },
];

export function normalizeGeoRestrict(raw?: Record<string, any> | null): GeoRestrictConfig {
    const r = raw || {};
    const rules = Array.isArray(r.rules)
        ? r.rules.map((x: any) => String(x || '').trim().toUpperCase()).filter(Boolean)
        : [];
    const provinceRules = Array.isArray(r.provinceRules)
        ? r.provinceRules.map((x: any) => String(x || '').trim()).filter(Boolean)
        : [];
    return {
        state: r.state === 'on' ? 'on' : 'off',
        schemaVersion: 2,
        type: 'geoRestrict',
        res: 'geo',
        code: typeof r.code === 'number' ? r.code : 403,
        action: r.action === 'allow' ? 'allow' : 'deny',
        rules: [...new Set(rules)],
        provinceAction: r.provinceAction === 'allow' ? 'allow' : 'deny',
        provinceRules: [...new Set(provinceRules)],
    };
}
