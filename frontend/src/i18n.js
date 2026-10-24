// ═══════════════════════════════════════════
// i18n — UI strings for the bilingual SPA
// ═══════════════════════════════════════════
//
// The site language is decided by the URL prefix alone (the same rule the SSR
// pass uses in backend-go/internal/ssr/i18n.go): "/en" or "/en/..." is English,
// everything else is Chinese. We never persist a language choice in localStorage
// — the URL stays the single source of truth, so SSR and SPA can never disagree
// about which language a page is in.
//
// Translation style: `tr(zh, en)` keeps the Chinese and English strings next to
// the markup that uses them. That is deliberately lighter than a key-based
// dictionary for a two-language site: a missing key is impossible (it would be a
// visible code smell) and page diffs stay local. The language REGISTRY is still
// list-driven in main.js (LANGS) — adding a third language there routes and
// labels automatically; UI strings would then move to a dictionary keyed by
// language code.

const EN_PREFIX = '/en';

export function isEnglishPath(path) {
  const p = path || (typeof location !== 'undefined' ? location.pathname : '/');
  return p === EN_PREFIX || p.startsWith(EN_PREFIX + '/');
}

// tr returns the string for the current page language.
export function tr(zh, en) {
  return isEnglishPath() ? en : zh;
}

// ── Category labels ────────────────────────────────────────────────────────
// Categories are stored in the database as Chinese strings. English pages show
// an English label but MUST send the stored Chinese value to the API — otherwise
// filtering silently returns nothing.
export const CATEGORIES = ['建筑', '车辆', '机甲', '奇幻', '科幻', '场景'];

const CATEGORY_EN = {
  建筑: 'Architecture',
  车辆: 'Vehicles',
  机甲: 'Mecha',
  奇幻: 'Fantasy',
  科幻: 'Sci-Fi',
  场景: 'Scenery',
};

export function categoryLabel(value) {
  if (!value) return value;
  if (!isEnglishPath()) return value;
  return CATEGORY_EN[value] || value;
}

// categoryOptions returns [storedValue, displayLabel] pairs for a <select>.
export function categoryOptions(includeAll = true) {
  const out = includeAll ? [['', tr('全部分类', 'All categories')]] : [];
  for (const c of CATEGORIES) out.push([c, categoryLabel(c)]);
  return out;
}

// ── Brand labels ───────────────────────────────────────────────────────────
// Brands are stored as the Chinese value picked from the curated dropdown
// (乐高/森宝/...). English pages show the brand's international name but MUST
// send the stored value to the API, exactly like categories.
const BRAND_EN = {
  乐高: 'LEGO',
  小米: 'Xiaomi',
  森宝: 'SEMBO',
  双鹰: 'Double Eagle',
  万格: 'Wange',
  邦宝: 'BanBao',
  沃马: 'Woma',
  开智: 'KAZI',
  启蒙: 'QMAN',
  布鲁可: 'Bloks',
  潘洛斯: 'Panlos',
  宇星模王: 'Mould King',
  星堡: 'Xingbao',
  奇积: 'Qiji',
  卫积: 'Weiji',
  咔搭: 'CaDA',
  双钻: 'Shuangzuan',
  得高: 'DECOOL',
  小鲁班: 'Sluban',
  高砖: 'Gobricks',
  星钻: 'Star Diamond',
  酷彼: 'Kubi',
  美高: 'Mega Bloks',
  其他: 'Other',
};

export function brandLabel(value) {
  if (!value) return value;
  if (!isEnglishPath()) return value;
  return BRAND_EN[value] || value;
}

// ── User-generated blueprint text ──────────────────────────────────────────
// title_en / description_en are optional translations the uploader provides.
// An empty value falls back to the Chinese field so the English tree never
// renders a blank heading (the API returns both fields).
export function blueprintTitle(bp) {
  if (!bp) return '';
  return isEnglishPath() && bp.title_en ? bp.title_en : bp.title;
}

export function blueprintDescription(bp) {
  if (!bp) return '';
  return isEnglishPath() && bp.description_en ? bp.description_en : bp.description;
}

// ── Difficulty ─────────────────────────────────────────────────────────────
const DIFFICULTY_EN = { 1: 'Easy', 2: 'Beginner', 3: 'Medium', 4: 'Hard', 5: 'Expert' };
const DIFFICULTY_ZH = { 1: '简单', 2: '初级', 3: '中等', 4: '困难', 5: '专家' };

export function difficultyLabel(n) {
  const key = n || 3;
  return (isEnglishPath() ? DIFFICULTY_EN : DIFFICULTY_ZH)[key] || (isEnglishPath() ? 'Medium' : '中等');
}

// ── API errors ─────────────────────────────────────────────────────────────
// The Go API returns Chinese messages. English users should never see them, so
// known messages are translated and anything unknown degrades to a neutral
// English sentence (the raw message is still logged for debugging).
const ERROR_EN = {
  '请先登录': 'Please sign in first.',
  '邮箱或密码错误': 'Incorrect email or password.',
  '该邮箱已注册': 'That email is already registered.',
  '用户名已存在': 'That username is already taken.',
  '密码至少6位': 'Password must be at least 6 characters.',
  '请填写所有字段': 'Please fill in all fields.',
  '文章不存在': 'Article not found.',
  '图纸不存在': 'Blueprint not found.',
  '用户不存在': 'User not found.',
  '没有权限': 'You do not have permission to do that.',
  'not found': 'Not found.',
};

export function errorMessage(err, fallback) {
  const raw = (err && err.message) || (typeof err === 'string' ? err : '');
  if (!isEnglishPath()) return raw || fallback || '操作失败';
  if (!raw) return fallback || 'Something went wrong. Please try again.';
  if (ERROR_EN[raw]) return ERROR_EN[raw];
  // Network/fetch failures already carry an English message.
  if (!/[一-鿿]/.test(raw)) return raw;
  console.warn('[i18n] untranslated API error:', raw);
  return fallback || 'Something went wrong. Please try again.';
}
