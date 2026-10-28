import isoList from '@/assets/json/iso.json';

const lookup = isoList as any[];

// Names used by the world geo json that differ from the plain country name.
const worldAlias: Record<string, string> = {
    'United States': 'United States of America',
    'Tanzania': 'United Republic of Tanzania',
    'Republic of the Congo': 'Republic of Congo',
    'Democratic Republic of the Congo': 'Democratic Republic of the Congo',
    'Ivory Coast': "Cote d'Ivoire",
    "Côte d'Ivoire": "Cote d'Ivoire",
    'Czech Republic': 'Czechia',
    'North Korea': 'North Korea',
    'South Korea': 'South Korea',
};

// map a Chinese country name from the 1pwaf geo db to the world map feature name
export function countryToWorld(zhName: string): string {
    if (!zhName) return '';
    if (zhName === '内网' || zhName === '局域网') return '';
    const hit = lookup.find((x) => x.zh === zhName || x.tw === zhName || x.en === zhName);
    const en = hit ? hit.en : zhName;
    return worldAlias[en] || en;
}

// the china map is drawn per province; the geo db stores short names (广东) while
// echarts/china.json uses the full official names (广东省)
const cnSuffix: Record<string, string> = {
    北京: '北京市',
    天津: '天津市',
    上海: '上海市',
    重庆: '重庆市',
    内蒙古: '内蒙古自治区',
    广西: '广西壮族自治区',
    西藏: '西藏自治区',
    宁夏: '宁夏回族自治区',
    新疆: '新疆维吾尔自治区',
    香港: '香港特别行政区',
    澳门: '澳门特别行政区',
    台湾: '台湾省',
};

export function provinceToChina(name: string): string {
    if (!name) return '';
    if (cnSuffix[name]) return cnSuffix[name];
    if (/(省|市|自治区|特别行政区)$/.test(name)) return name;
    return name + '省';
}
