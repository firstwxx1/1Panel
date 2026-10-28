// Applies the custom primary colour by overriding the panel's colour variables on <html>.

type RGB = [number, number, number];

const parse = (color: string): RGB | null => {
    const c = color.trim();
    let m = /^#([0-9a-f]{3})$/i.exec(c);
    if (m) return m[1].split('').map((x) => parseInt(x + x, 16)) as RGB;
    m = /^#([0-9a-f]{6})([0-9a-f]{2})?$/i.exec(c);
    if (m) return [0, 2, 4].map((i) => parseInt(m![1].slice(i, i + 2), 16)) as RGB;
    m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)/i.exec(c);
    if (m) return [Number(m[1]), Number(m[2]), Number(m[3])];
    return null;
};

const mix = (a: RGB, b: RGB, weight: number): string => {
    const out = a.map((v, i) => Math.round(v * (1 - weight) + b[i] * weight));
    return `rgb(${out[0]}, ${out[1]}, ${out[2]})`;
};

export function setPrimaryColor(color: string) {
    const rgb = parse(color);
    if (!rgb) return;
    const dark = document.documentElement.classList.contains('dark');
    const base: RGB = dark ? [10, 10, 10] : [255, 255, 255];
    const style = document.documentElement.style;
    style.setProperty('--panel-color-primary', color);
    for (let i = 1; i <= 9; i++) {
        style.setProperty(`--panel-color-primary-light-${i}`, mix(rgb, base, i / 10));
    }
    style.setProperty('--el-color-primary-dark-2', mix(rgb, [0, 0, 0], 0.2));
}
