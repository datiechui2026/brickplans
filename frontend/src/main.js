import * as api from './api.js';
import './styles/main.css';
import { marked } from 'marked';
import { tr, CATEGORIES, categoryLabel, categoryOptions, difficultyLabel, errorMessage, isEnglishPath } from './i18n.js';

// Configure marked: disable mangle/email autolink for safety
marked.setOptions({ breaks: true, gfm: true });

// Strip markdown to plain text for meta tags
function stripMarkdown(md) {
  if (!md) return '';
  return md
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/`(.+?)`/g, '$1')
    .replace(/\[(.+?)\]\(.+?\)/g, '$1')
    .replace(/^[-*]\s+/gm, '')
    .replace(/\n+/g, ' ')
    .trim();
}

// ═══════════════════════════════════════════
// Toast notification system
// ═══════════════════════════════════════════
let _toastId = 0;
function showToast(message, type = 'error', duration = 5000) {
  let container = document.querySelector('.toast-container');
  if (!container) {
    container = document.createElement('div');
    container.className = 'toast-container';
    document.body.appendChild(container);
  }
  const id = ++_toastId;
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  toast.onclick = () => toast.remove();
  container.appendChild(toast);
  setTimeout(() => { if (toast.parentNode) toast.remove(); }, duration);
}

// ═══════════════════════════════════════════
// State
// ═══════════════════════════════════════════
const state = {
  page: 'home',
  detailId: null,
  editId: null,
  notificationUnreadCount: 0,
  flashMessage: null,
  explore: { page: 1, q: '', category: '', sort: 'new', tag: '', items: [], total: 0 },
  user: api.getCurrentUser(),
  userLoaded: !api.isLoggedIn(),
  userProfile: { id: null, username: null, tab: 'blueprints', page: 1 },
};

// headLocked is true while an English page still shows the SSR-rendered head.
// It is lifted the first time the user navigates inside the SPA, after which the
// SPA owns title/meta/JSON-LD for the new (client-rendered) URL.
let headLocked = isEnglishPath();

// ═══════════════════════════════════════════
// Brand list (图纸属性: 品牌)
// ═══════════════════════════════════════════
// Curated brick-brand list rendered as the 品牌 dropdown on upload/edit. The backend
// stores `brand` as a plain (indexed) string, so adding or renaming an entry here is
// the only change needed — no schema migration. "其他" is the escape hatch for brands
// not listed yet; 型号 (product_code) accepts any manufacturer set number.
const BRANDS = [
  '乐高', '小米', '森宝', '双鹰', '万格', '邦宝', '沃马', '开智',
  '启蒙', '布鲁可', '潘洛斯', '宇星模王', '星堡', '奇积', '卫积',
  '咔搭', '双钻', '得高', '小鲁班', '高砖', '星钻', '酷彼', '美高', '其他',
];

// ═══════════════════════════════════════════
// Cover image helpers
// ═══════════════════════════════════════════
function getCoverImage(images) {
  if (!images || images.length === 0) return null;
  // Prefer the marked cover, but skip PDFs (can't render as <img>)
  const cover = images.find(img => img.is_cover && (img.file_type || 'image') !== 'pdf');
  if (cover) return cover.url;
  // Fallback: last non-PDF image
  const lastImage = [...images].reverse().find(img => (img.file_type || 'image') !== 'pdf');
  if (lastImage) return lastImage.url;
  // All are PDFs — return first URL (won't render as image, but better than null)
  return images[0].url;
}

function getCoverIndex(images) {
  if (!images || images.length === 0) return 0;
  // Prefer the marked cover, but skip PDFs
  const idx = images.findIndex(img => img.is_cover && (img.file_type || 'image') !== 'pdf');
  if (idx >= 0) return idx;
  // Fallback: last non-PDF image
  const lastIdx = [...images].reverse().findIndex(img => (img.file_type || 'image') !== 'pdf');
  if (lastIdx >= 0) return images.length - 1 - lastIdx;
  // All are PDFs — return last index
  return images.length - 1;
}

// ═══════════════════════════════════════════
// Difficulty formatter
// ═══════════════════════════════════════════
function formatDifficulty(n) {
  const map = {
    1: { stars: '⭐', color: '#22c55e', bg: 'rgba(34,197,94,.75)' },
    2: { stars: '⭐⭐', color: '#3b82f6', bg: 'rgba(59,130,246,.75)' },
    3: { stars: '⭐⭐⭐', color: '#f59e0b', bg: 'rgba(245,158,11,.75)' },
    4: { stars: '⭐⭐⭐⭐', color: '#ef4444', bg: 'rgba(239,68,68,.75)' },
    5: { stars: '⭐⭐⭐⭐⭐', color: '#a855f7', bg: 'rgba(168,85,247,.75)' },
  };
  const level = n || 3;
  return { ...(map[level] || map[3]), label: difficultyLabel(level) };
}

// ═══════════════════════════════════════════
// SEO helpers — dynamically update meta tags
// ═══════════════════════════════════════════
function setMeta(property, content) {
  let meta = document.querySelector(`meta[property="${property}"]`);
  if (!meta) {
    meta = document.createElement('meta');
    meta.setAttribute('property', property);
    document.head.appendChild(meta);
  }
  meta.setAttribute('content', content || '');
}

function setMetaName(name, content) {
  let meta = document.querySelector(`meta[name="${name}"]`);
  if (!meta) {
    meta = document.createElement('meta');
    meta.setAttribute('name', name);
    document.head.appendChild(meta);
  }
  meta.setAttribute('content', content || '');
}

function resetMeta() {
  // On an EN page's first render the Go SSR already wrote a correct English
  // <head>. Do not touch it: Googlebot indexes the rendered DOM, so rewriting
  // title/meta/JSON-LD here is exactly how a bilingual SPA loses its English
  // signals. The lock lifts on the first in-app navigation (see navigate()).
  if (headLocked) return;
  if (isEnglishPath()) {
    document.title = 'BrickPlan — MOC Blueprints and Building Guides';
    setMeta('og:title', 'BrickPlan — MOC Blueprints and Building Guides');
    setMeta('og:description', 'Discover and share LEGO MOC blueprints, part guides, brand comparisons and building tutorials.');
    setMeta('og:image', '/og-default.png');
    setMeta('og:url', location.href);
    setMeta('og:type', 'website');
    setMetaName('description', 'BrickPlan is a blueprint-sharing community for brick and MOC builders.');
    return;
  }
  document.title = 'BrickPlan — 积木图纸分享';
  setMeta('og:title', 'BrickPlan — 积木图纸分享社区');
  setMeta('og:description', '分享你的乐高MOC创意，探索海量积木图纸');
  setMeta('og:image', '/og-default.png');
  setMeta('og:url', '/');
  setMeta('og:type', 'website');
  setMetaName('description', 'BrickPlan 积木图纸分享社区，发现和分享乐高MOC创意作品');
  // Remove any injected JSON-LD
  document.querySelectorAll('script[type="application/ld+json"]').forEach(el => el.remove());
}

// ═══════════════════════════════════════════
// Router — history API (real URLs for SEO; native back/forward via popstate)
// ═══════════════════════════════════════════
// neutralPath builds the language-free path; buildPath puts the current
// language prefix back on. Keeping the two separate means a page never has to
// know which language tree it is being rendered into.
function neutralPath(page, params = {}) {
  if (!page || page === 'home') return '/';
  if (page === 'detail' && params.id) return `/detail/${encodeURIComponent(params.id)}`;
  if (page === 'user' && params.id) return `/user/${encodeURIComponent(params.id)}`;
  if (page === 'edit' && params.id) return `/edit/${encodeURIComponent(params.id)}`;
  if (page === 'blog-detail' && params.slug) return `/blog/${encodeURIComponent(params.slug)}`;
  if (page === 'blog') {
    const q = new URLSearchParams();
    if (params.category) q.set('category', params.category);
    if (params.tag) q.set('tag', params.tag);
    // Only keep ?page= for deep links (SSR pager / shared URLs). Page 1 stays
    // canonical /blog so the SPA never pushes /blog?page=1 into history.
    const pg = parseInt(params.page, 10);
    if (pg > 1) q.set('page', pg);
    const qs = q.toString();
    return '/blog' + (qs ? '?' + qs : '');
  }
  if (page === 'explore') {
    const q = new URLSearchParams();
    if (params.q) q.set('q', params.q);
    if (params.category) q.set('category', params.category);
    if (params.tag) q.set('tag', params.tag);
    if (params.sort) q.set('sort', params.sort);
    if (params.page) q.set('page', params.page);
    const qs = q.toString();
    return '/explore' + (qs ? '?' + qs : '');
  }
  return '/' + page;
}

function buildPath(page, params = {}) {
  return withLangPrefix(neutralPath(page, params));
}

// ═══════════════════════════════════════════
// i18n — 站点语言
// ═══════════════════════════════════════════
// 加一门语言 = 往 LANGS 加一项 + 补 NAV_LABELS 的标签。不要写成 if/else 两分支。
// 服务端同构实现: backend-go/internal/ssr/i18n.go —— 两处必须一起改。
const LANGS = [
  { code: 'zh', label: '中文', prefix: '', hreflang: 'zh-CN' },
  { code: 'en', label: 'English', prefix: '/en', hreflang: 'en' },
];
const DEFAULT_LANG = 'zh';
const NAV_PATHS = ['/', '/explore', '/blog', '/about', '/faq', '/privacy'];
const NAV_LABELS = {
  zh: { '/': '首页', '/explore': '发现', '/blog': '博客', '/about': '关于' },
  en: { '/': 'Home', '/explore': 'Explore', '/blog': 'Blog', '/about': 'About' },
};

function langByCode(code) {
  return LANGS.find((l) => l.code === code) || LANGS.find((l) => l.code === DEFAULT_LANG);
}

// currentLang() reads the real path (not SPA state) — the SSR pass already
// decided the language from the same path, so both stay in agreement.
function currentLang() {
  const p = location.pathname;
  for (const l of LANGS) {
    if (!l.prefix) continue;
    if (p === l.prefix || p.startsWith(l.prefix + '/')) return l;
  }
  return langByCode(DEFAULT_LANG);
}

function langURL(lang, path) {
  if (!lang.prefix) return path;
  return path === '/' ? lang.prefix + '/' : lang.prefix + path;
}

function navLabel(lang, path) {
  return (NAV_LABELS[lang.code] || {})[path] || path;
}

// STATIC_ENGLISH_PATHS are the only /en pages whose SSR body stays final.
// about/faq/privacy carry original English copy written in Go; hydrating them
// with the SPA would only duplicate that copy in JavaScript. Every other /en
// path now hydrates exactly like its Chinese twin (home, explore, detail, user,
// blog, upload, edit, notifications, admin) and renders the same layout with
// English UI strings.
const STATIC_ENGLISH_PATHS = ['/en/about', '/en/faq', '/en/privacy'];
function isSsrOnlyPath(p) {
  return STATIC_ENGLISH_PATHS.includes(p);
}

// stripLangPrefix maps "/en/explore" → "/explore" and "/en/" → "/".
// The current language is read from the real URL, never from SPA state, so the
// router and the SSR pass always agree.
function stripLangPrefix(path) {
  const cur = currentLang();
  if (!cur.prefix) return path;
  if (path === cur.prefix) return '/';
  if (path.startsWith(cur.prefix + '/')) return path.slice(cur.prefix.length);
  return path;
}

// withLangPrefix is the inverse: it puts the current language back in front of a
// language-neutral path. Used by buildPath() so every in-app navigation stays
// inside the language tree the user is browsing.
function withLangPrefix(path) {
  const cur = currentLang();
  if (!cur.prefix) return path;
  return path === '/' ? cur.prefix + '/' : cur.prefix + path;
}

// The SSR nav already resolved the switcher target per language — including the
// blog case, which needs the content store to know whether a translation exists.
// Capture it before #app is replaced, and use it while we are still on the page
// the server rendered.
const SSR_LANG_TARGETS = (() => {
  const out = {};
  const sw = document.querySelector('.lang-switch');
  if (sw) sw.querySelectorAll('a[hreflang]').forEach((a) => { out[a.getAttribute('hreflang')] = a.getAttribute('href'); });
  return out;
})();
const SSR_PATH = location.pathname;

// switchTarget mirrors the server rule: a counterpart exists → link to it;
// otherwise fall back to that language's blog list (always non-empty).
function switchTarget(lang) {
  if (location.pathname === SSR_PATH && SSR_LANG_TARGETS[lang.hreflang]) {
    return SSR_LANG_TARGETS[lang.hreflang];
  }
  const cur = currentLang();
  let neutral = location.pathname;
  if (cur.prefix && (neutral === cur.prefix || neutral.startsWith(cur.prefix + '/'))) {
    neutral = neutral.slice(cur.prefix.length) || '/';
  }
  if (NAV_PATHS.includes(neutral)) return langURL(lang, neutral);
  // detail/user/tags have a UI counterpart in both trees. These are switcher
  // targets, not hreflang alternates (the body language is the same).
  if (/^\/(detail|user|tags)\//.test(neutral)) return langURL(lang, neutral);
  return langURL(lang, '/blog');
}

function renderLangSwitch() {
  const cur = currentLang();
  return h('span', { className: 'lang-switch', role: 'group', 'aria-label': '语言 / Language' },
    ...LANGS.map((l) => (
      l.code === cur.code
        ? h('span', { className: 'lang-opt is-current', lang: l.hreflang, 'aria-current': 'true' }, l.label)
        : h('a', {
            className: 'lang-opt',
            href: switchTarget(l),
            hreflang: l.hreflang,
            lang: l.hreflang,
            rel: 'alternate',
          }, l.label)
    )),
  );
}

function parsePath() {
  // Match against the language-neutral path, so /en/detail/x and /detail/x
  // resolve to the same SPA page (the renderer's language comes from the URL).
  const path = stripLangPrefix(location.pathname);
  const params = {};
  new URLSearchParams(location.search).forEach((v, k) => { params[k] = v; });
  if (path === '/' || path === '') return { page: 'home', params };
  const blogM = path.match(/^\/blog\/(.+?)\/?$/);
  if (blogM) return { page: 'blog-detail', params: { ...params, slug: decodeURIComponent(blogM[1]) } };
  if (path === '/blog' || path === '/blog/') return { page: 'blog', params };
  const m = path.match(/^\/(detail|user|edit)\/(.+?)\/?$/);
  if (m) return { page: m[1], params: { ...params, id: decodeURIComponent(m[2]) } };
  const page = path.replace(/^\//, '').replace(/\/$/, '');
  return { page: page || 'home', params };
}

// parseLegacyHash parses old '#/page?...' URLs for one-time redirect on boot.
function parseLegacyHash() {
  const raw = location.hash.slice(2) || 'home';
  const [path, query] = raw.split('?');
  const params = {};
  if (query) new URLSearchParams(query).forEach((v, k) => { params[k] = v; });
  const parts = path.split('/');
  if (parts[0] === 'user' && parts[1]) return { page: 'user', params: { ...params, id: decodeURIComponent(parts[1]) } };
  if (parts[0] === 'detail' && params.id) return { page: 'detail', params };
  if (parts[0] === 'edit' && params.id) return { page: 'edit', params };
  return { page: path || 'home', params };
}

function applyState(page, params = {}) {
  state.page = page;
  if (params.id) state.detailId = params.id;
  if (page === 'edit' && params.id) state.editId = params.id;
  if (page === 'blog-detail' && params.slug) state.blogSlug = params.slug;
  if (page === 'blog') {
    state.blogFilter = {
      category: params.category || '',
      tag: params.tag || '',
      // Deep link from the SSR pager: the SPA walks pages 1..page so the list
      // stays continuous. Clamped to >= 1; garbage falls back to 1.
      page: Math.max(1, parseInt(params.page, 10) || 1),
    };
  }
  if (page === 'user' && params.id) {
    state.userProfile = { ...state.userProfile, id: params.id };
  } else if (page === 'user' && params.username) {
    state.userProfile = { ...state.userProfile, id: params.username, username: params.username };
  }
  if (params.q !== undefined) { state.explore.q = params.q; }
  else if (page !== 'explore') { state.explore.q = ''; }
  if (params.category) state.explore.category = params.category;
  else if (page !== 'explore') { state.explore.category = ''; }
  if (params.tag) state.explore.tag = params.tag;
  else if (page !== 'explore') { state.explore.tag = ''; }
  if (params.sort) state.explore.sort = params.sort;
  else if (page !== 'explore') { state.explore.sort = 'new'; }
  if (params.page) state.explore.page = parseInt(params.page) || 1;
}

function navigate(page, params = {}) {
  // From here on the page is client-rendered: the SSR <head> describes the URL
  // the user landed on, not the one they are navigating to.
  headLocked = false;
  history.pushState({ page, params }, '', buildPath(page, params));
  onRouteChange();
}

// Handle back/forward (popstate) and post-pushState renders.
function onRouteChange() {
  const { page, params } = parsePath();
  applyState(page, params);
  render();
  window.scrollTo(0, 0);
}
window.addEventListener('popstate', onRouteChange);

async function refreshCurrentUser() {
  if (!api.isLoggedIn()) {
    state.userLoaded = true;
    return;
  }
  try {
    const user = await api.getMe();
    state.user = user;
    localStorage.setItem('bp_user', JSON.stringify(user));
  } catch (error) {
    console.warn('Refresh current user failed:', error);
  } finally {
    state.userLoaded = true;
    renderNavbarIntoDOM();
    if (state.page === 'admin') render();
  }
}

// ═══════════════════════════════════════════
// Render Engine
// ═══════════════════════════════════════════
function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'className') el.className = v;
    else if (k === 'innerHTML') el.innerHTML = v;
    else if (k.startsWith('on') && typeof v === 'function') {
      el.addEventListener(k.slice(2).toLowerCase(), v);
    } else if (k === 'style' && typeof v === 'object') {
      Object.assign(el.style, v);
    } else if (k === 'dataset') {
      Object.assign(el.dataset, v);
    } else if (typeof v === 'boolean') {
      // Boolean attributes (disabled, checked, …) are true by PRESENCE — the
      // value is ignored, so setAttribute(k, false) yields disabled="false",
      // which is still disabled. Set/remove the attribute instead. Without this
      // every `disabled: page <= 1` button was permanently dead.
      if (v) el.setAttribute(k, '');
      else el.removeAttribute(k);
    } else el.setAttribute(k, v);
  }
  for (const child of children.flat()) {
    if (child == null) continue;
    el.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return el;
}

// ── PDF.js viewer ──
// Renders PDFs to <canvas> via PDF.js instead of an <iframe>. The browser's
// native PDF viewer would execute JavaScript embedded in the PDF under the same
// origin; PDF.js does not. Falls back to a download link if PDF.js can't load.
let _pdfjsPromise = null;
function loadPdfjs() {
  if (_pdfjsPromise) return _pdfjsPromise;
  _pdfjsPromise = new Promise((resolve, reject) => {
    if (window.pdfjsLib) { resolve(window.pdfjsLib); return; }
    const s = document.createElement('script');
    s.src = '/lib/pdfjs/pdf.min.js';
    s.onload = () => {
      window.pdfjsLib.GlobalWorkerOptions.workerSrc =
        '/lib/pdfjs/pdf.worker.min.js';
      resolve(window.pdfjsLib);
    };
    s.onerror = () => reject(new Error('PDF.js 加载失败'));
    document.head.appendChild(s);
  });
  return _pdfjsPromise;
}

async function renderPDF(container, url) {
  try {
    const pdfjsLib = await loadPdfjs();
    // disableAutoFetch: don't pull the whole file up front; fetch page bytes on
    // demand (relies on COS range requests, enabled by the CORS Range header).
    const pdf = await pdfjsLib.getDocument({ url, disableAutoFetch: true }).promise;
    container.innerHTML = '';
    const width = Math.min(container.clientWidth || 800, 900);
    const total = pdf.numPages;
    const placeholders = [];

    const renderPage = async (i) => {
      const page = await pdf.getPage(i);
      const base = page.getViewport({ scale: 1 });
      const scale = width / base.width;
      const viewport = page.getViewport({ scale });
      const canvas = document.createElement('canvas');
      canvas.width = viewport.width;
      canvas.height = viewport.height;
      canvas.style.cssText = 'max-width:100%;display:block;margin:0 auto 8px;background:#fff';
      await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise;
      const ph = placeholders[i - 1];
      if (ph) ph.replaceWith(canvas);
    };

    // One placeholder per page so the scrollbar reflects the full length.
    for (let i = 1; i <= total; i++) {
      const ph = h('div', { style: { minHeight: '500px', background: '#f5f5f5', margin: '0 auto 8px', borderRadius: '4px' } });
      ph.dataset.page = String(i);
      placeholders.push(ph);
      container.appendChild(ph);
    }
    container.appendChild(h('a', {
      href: url, download: '',
      style: { display: 'block', textAlign: 'center', padding: '8px', color: 'var(--accent)' },
    }, tr('下载完整 PDF', 'Download the full PDF')));

    // Render the first 3 pages immediately; lazy-render the rest on scroll.
    for (let i = 1; i <= Math.min(3, total); i++) renderPage(i);
    if (total > 3) {
      const io = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            const i = parseInt(entry.target.dataset.page, 10);
            io.unobserve(entry.target);
            renderPage(i);
          }
        });
      }, { root: container, rootMargin: '300px' });
      for (let i = 4; i <= total; i++) io.observe(placeholders[i - 1]);
    }
  } catch (e) {
    container.innerHTML = '';
    container.appendChild(h('div', { style: { padding: '24px', textAlign: 'center' } },
      h('p', { style: { color: 'var(--text-sec)' } }, tr('PDF 预览不可用', 'PDF preview is unavailable')),
      h('a', { href: url, download: '', style: { color: 'var(--accent)' } }, tr('下载 PDF', 'Download PDF')),
    ));
  }
}

function pdfContainer(url) {
  const container = h('div', {
    style: {
      width: '100%', minHeight: '400px', maxHeight: '70vh', overflow: 'auto',
      background: '#f5f5f5', borderRadius: '8px', padding: '12px',
    },
  });
  container.appendChild(h('div', { style: { padding: '24px', color: 'var(--text-sec)', textAlign: 'center' } }, tr('加载 PDF…', 'Loading PDF…')));
  renderPDF(container, url);
  return container;
}

function $id(id) { return document.getElementById(id); }

// ═══════════════════════════════════════════
// Navbar
// ═══════════════════════════════════════════
async function refreshNotificationBadge() {
  if (!state.user?.id) return;
  try {
    const data = await api.getUnreadNotificationCount();
    state.notificationUnreadCount = data.unread_count || 0;
    renderNavbarIntoDOM();
  } catch { /* ignore */ }
}

function renderNavbar() {
  const brandEl = h('a', {
    className: 'nav-brand',
    href: langURL(currentLang(), '/'),
    onclick: (e) => { e.preventDefault(); navigate('home'); },
  }, '🧱 BrickPlan');

  const actions = [];

  // Upload button — shows login modal if not logged in
  if (state.user?.id) {
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => navigate('notifications') },
      tr('🔔 通知', '🔔 Notifications'),
      state.notificationUnreadCount > 0 ? h('span', { className: 'nav-dot' }, String(Math.min(state.notificationUnreadCount, 99))) : null,
    ));
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => navigate('upload') }, tr('📤 上传图纸', '📤 Upload')));
  } else {
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => showModal('login') }, tr('📤 上传图纸', '📤 Upload')));
  }

  // User avatar / name
  if (state.user?.id) {
    const avatarUrl = state.user.avatar_url || `https://api.dicebear.com/7.x/thumbs/svg?seed=${encodeURIComponent(state.user.username || 'user')}`;
    actions.push(h('img', {
      className: 'user-avatar',
      src: avatarUrl,
      alt: state.user.username || tr('用户', 'User'),
      title: tr('个人主页', 'Profile'),
      onclick: () => navigate('user', { id: state.user.id }),
    }));
  } else {
    actions.push(h('button', { className: 'btn btn-primary btn-sm', onclick: () => showModal('login') }, tr('登录', 'Sign in')));
    actions.push(h('button', { className: 'btn btn-ghost btn-sm', onclick: () => showModal('register') }, tr('注册', 'Sign up')));
  }

  const lang = currentLang();
  // Labels AND hrefs follow the language list, so adding a language needs no
  // change here. about/faq/privacy on a prefixed language stay server-rendered,
  // so those links are plain <a> full page loads; every other page hydrates in
  // the SPA just like its default-language twin.
  const navLink = (path, page) => h('a', {
    href: langURL(lang, path),
    ...(isSsrOnlyPath(langURL(lang, path)) ? {} : { onclick: (e) => { e.preventDefault(); navigate(page); } }),
    className: 'nav-link',
  }, navLabel(lang, path));

  return h('nav', { className: 'navbar' },
    brandEl,
    h('div', { className: 'nav-links' },
      navLink('/explore', 'explore'),
      navLink('/blog', 'blog'),
      navLink('/about', 'about'),
    ),
    h('div', { className: 'nav-actions' }, ...actions, renderLangSwitch()),
  );
}

// ═══════════════════════════════════════════
// Modal
// ═══════════════════════════════════════════
let modalMode = 'login';

function showModal(mode) {
  modalMode = mode;
  renderModal();
}

function hideModal() {
  const overlay = document.querySelector('.modal-overlay');
  if (overlay) {
    overlay.remove();
    state.user = api.getCurrentUser();
    renderNavbarIntoDOM();
  }
  // Also hide report modal
  const reportOverlay = document.querySelector('.report-modal-overlay');
  if (reportOverlay) reportOverlay.remove();
}

function _syncUserAvatar(avatarUrl) {
  // Update in-memory state
  if (state.user) state.user.avatar_url = avatarUrl;
  if (state.userProfile?.avatar_url) state.userProfile.avatar_url = avatarUrl;
  // Persist to localStorage so navbar stays in sync across reloads
  try {
    const u = JSON.parse(localStorage.getItem('bp_user') || 'null');
    if (u) {
      u.avatar_url = avatarUrl;
      localStorage.setItem('bp_user', JSON.stringify(u));
    }
  } catch(e) { /* ignore */ }
  renderNavbarIntoDOM();
}

// ── Report Modal ──

let _reportTargetId = null;

function showReportModal(blueprintId) {
  _reportTargetId = blueprintId;
  hideModal(); // close any existing modals

  const overlay = h('div', { className: 'modal-overlay', onclick: (e) => { if (e.target === overlay) hideModal(); } },
    h('div', { className: 'modal' },
      h('button', { className: 'close', onclick: hideModal, style: { float: 'right' } }, '✕'),
      h('h2', {}, tr('🚩 举报内容', '🚩 Report content')),
      h('p', { style: { color: 'var(--text-sec)', fontWeight: 600, marginBottom: '16px', fontSize: '0.9rem' } }, tr('请选择举报原因：', 'Choose a reason:')),
      h('div', { id: 'report-error' }),
      h('div', { className: 'form-group' },
        ...['inappropriate', 'copyright', 'incomplete', 'spam', 'other'].map(r =>
          h('label', { style: { display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 0', cursor: 'pointer', fontWeight: 600 } },
            h('input', { type: 'radio', name: 'report-reason', value: r }),
            {
              inappropriate: tr('内容不当', 'Inappropriate content'),
              copyright: tr('版权问题', 'Copyright issue'),
              incomplete: tr('图纸不完整', 'Incomplete blueprint'),
              spam: tr('垃圾广告', 'Spam'),
              other: tr('其他', 'Other'),
            }[r],
          )
        ),
        h('input', { type: 'radio', name: 'report-reason', value: 'other', style: { display: 'none' }, id: 'report-reason-other-hidden' }),
      ),
      h('div', { className: 'form-group' },
        h('label', {}, tr('补充说明（可选）', 'Additional details (optional)')),
        h('textarea', {
          id: 'report-detail',
          placeholder: tr('请补充更多细节...', 'Add any details that help us review this...'),
          rows: 3,
          style: { width: '100%', padding: '10px', borderRadius: '8px', border: '2px solid #e5e5e5', fontFamily: 'inherit', fontSize: '0.9rem', resize: 'vertical', background: '#f5f5f7', boxSizing: 'border-box' },
        }),
      ),
      h('button', {
        className: 'btn btn-primary btn-lg',
        style: { width: '100%', marginTop: '8px' },
        onclick: handleReportSubmit,
      }, tr('提交举报', 'Submit report')),
    ),
  );
  document.body.appendChild(overlay);
}

async function handleReportSubmit() {
  const errEl = $id('report-error');
  const reasonRadio = document.querySelector('input[name="report-reason"]:checked');
  if (!reasonRadio) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('请选择举报原因', 'Please choose a reason')}</div>`;
    return;
  }
  const reason = reasonRadio.value;
  const detail = $id('report-detail')?.value.trim() || undefined;

  try {
    await api.createReport(_reportTargetId, reason, detail);
    hideModal();
    showToast(tr('举报已提交，感谢您的反馈！', 'Report submitted. Thanks for the feedback!'), 'success');
    // Update the report button to "已举报"
    const reportStatusEl = $id('report-status');
    if (reportStatusEl) {
      reportStatusEl.innerHTML = '';
      reportStatusEl.appendChild(
        h('span', { style: { color: '#ef4444', fontWeight: 700, fontSize: '0.9rem' } }, tr('✅ 已举报', '✅ Reported'))
      );
    }
  } catch (e) {
    if (e.message && e.message.includes('already reported')) {
      hideModal();
      showToast(tr('您已经举报过该内容', 'You have already reported this item'), 'error');
      const reportStatusEl = $id('report-status');
      if (reportStatusEl) {
        reportStatusEl.innerHTML = '';
        reportStatusEl.appendChild(
          h('span', { style: { color: '#ef4444', fontWeight: 700, fontSize: '0.9rem' } }, tr('✅ 已举报', '✅ Reported'))
        );
      }
    } else {
      if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('举报提交失败', 'Could not submit the report. Please try again.'))}</div>`;
    }
  }
}

function renderModal() {
  hideModal();

  const isLogin = modalMode === 'login';
  const overlay = h('div', { className: 'modal-overlay', onclick: (e) => { if (e.target === overlay) hideModal(); } },
    h('div', { className: 'modal' },
      h('button', { className: 'close', onclick: hideModal, style: { float: 'right' } }, '✕'),
      h('h2', {}, isLogin ? tr('🔑 登录 BrickPlan', '🔑 Sign in to BrickPlan') : tr('🧱 注册 BrickPlan', '🧱 Create a BrickPlan account')),
      h('div', { id: 'modal-error' }),
      h('div', { className: 'form-group' },
        isLogin ? null : h('label', { className: 'form-label' }, tr('用户名', 'Username')),
        ...(isLogin ? [] : [h('input', { type: 'text', id: 'modal-username', className: 'form-input', placeholder: tr('你的用户名', 'Your username') })]),
      ),
      h('div', { className: 'form-group' },
        h('label', { className: 'form-label' }, tr('邮箱', 'Email')),
        h('input', { type: 'email', id: 'modal-email', className: 'form-input', placeholder: 'your@email.com' }),
      ),
      h('div', { className: 'form-group' },
        h('label', { className: 'form-label' }, tr('密码', 'Password')),
        h('input', { type: 'password', id: 'modal-password', className: 'form-input', placeholder: tr('至少6位密码', 'At least 6 characters') }),
      ),
      ...(isLogin ? [] : [
        h('div', { style: { marginTop: '12px', display: 'flex', alignItems: 'center', gap: '8px', fontSize: '0.85rem' } },
          h('input', { type: 'checkbox', id: 'modal-privacy', checked: true }),
          h('span', {}, `${tr('我已阅读并同意 ', 'I have read and agree to the ')}`, h('a', { href: langURL(currentLang(), '/privacy'), style: { color: 'var(--accent)', fontWeight: 700 } }, tr('隐私策略', 'Privacy Policy'))),
        ),
      ]),
      h('div', { style: { display: 'flex', gap: '12px', marginTop: '20px' } },
        h('button', {
          className: `btn ${isLogin ? 'btn-primary' : 'btn-primary'} btn-lg`,
          style: { flex: 1 },
          onclick: isLogin ? handleLogin : handleRegister,
        }, isLogin ? tr('登录', 'Sign in') : tr('注册', 'Sign up')),
      ),
      h('div', {
        style: { textAlign: 'center', marginTop: '16px', fontSize: '0.85rem', fontWeight: 600 },
      },
        isLogin ? tr('还没有账号？', 'No account yet?') : tr('已有账号？', 'Already have an account?'),
        h('button', {
          style: { background: 'none', border: 'none', color: 'var(--accent)', fontWeight: 700, cursor: 'pointer', fontSize: '0.85rem', fontFamily: 'inherit', textDecoration: 'underline' },
          onclick: () => { modalMode = isLogin ? 'register' : 'login'; renderModal(); },
        }, isLogin ? tr('去注册', 'Sign up') : tr('去登录', 'Sign in')),
      ),
    ),
  );
  document.body.appendChild(overlay);
}

async function handleLogin() {
  const email = $id('modal-email')?.value.trim();
  const password = $id('modal-password')?.value.trim();
  const errEl = $id('modal-error');
  if (!email || !password) { errEl.innerHTML = `<div class="msg msg-error">${tr('请填写邮箱和密码', 'Please enter your email and password.')}</div>`; return; }
  try {
    const data = await api.login(email, password);
    state.user = data.user;
    hideModal();
    render();
  } catch (e) {
    errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('登录失败', 'Sign-in failed.'))}</div>`;
  }
}

async function handleRegister() {
  const username = $id('modal-username')?.value.trim();
  const email = $id('modal-email')?.value.trim();
  const password = $id('modal-password')?.value.trim();
  const errEl = $id('modal-error');
  if (!username || !email || !password) { errEl.innerHTML = `<div class="msg msg-error">${tr('请填写所有字段', 'Please fill in all fields.')}</div>`; return; }
  if (password.length < 6) { errEl.innerHTML = `<div class="msg msg-error">${tr('密码至少6位', 'Password must be at least 6 characters.')}</div>`; return; }
  const privacyAccepted = $id('modal-privacy')?.checked;
  if (!privacyAccepted) { errEl.innerHTML = `<div class="msg msg-error">${tr('请先阅读并同意隐私策略', 'Please read and accept the Privacy Policy first.')}</div>`; return; }
  try {
    const data = await api.register(username, email, password);
    state.user = data.user;
    hideModal();
    render();
  } catch (e) {
    errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('注册失败', 'Sign-up failed.'))}</div>`;
  }
}

// ── Lightbox ──
function openLightbox(images, curIdx) {
  const overlay = h('div', { className: 'lightbox-overlay' });
  const imgEl = h('img', { className: 'lightbox-img', src: images[curIdx].url });
  const wrap = h('div', { className: 'lightbox-img-wrap' });
  wrap.appendChild(imgEl);
  overlay.appendChild(wrap);

  // closeLightbox must be declared before it's referenced by onclick/onKey
  const closeLightbox = () => {
    document.removeEventListener('keydown', onKey);
    overlay.remove();
  };

  // Close button
  overlay.appendChild(h('button', { className: 'lightbox-close', onclick: closeLightbox }, '✕'));

  // Counter
  const counterEl = h('div', { className: 'lightbox-counter' }, `${curIdx + 1} / ${images.length}`);
  overlay.appendChild(counterEl);

  // Nav arrows (multi-image only)
  if (images.length > 1) {
    overlay.appendChild(h('button', { className: 'lightbox-nav prev', onclick: () => changeImage(-1) }, '‹'));
    overlay.appendChild(h('button', { className: 'lightbox-nav next', onclick: () => changeImage(1) }, '›'));
  }

  let zoom = 1, dx = 0, dy = 0, isDragging = false, dragStartX = 0, dragStartY = 0, startDx = 0, startDy = 0;

  const applyTransform = () => {
    imgEl.style.transform = `scale(${zoom}) translate(${dx}px, ${dy}px)`;
  };

  const resetTransform = () => { zoom = 1; dx = 0; dy = 0; imgEl.classList.remove('zoomed', 'dragging'); applyTransform(); };

  const changeImage = (delta) => {
    const newIdx = (curIdx + delta + images.length) % images.length;
    closeLightbox();
    openLightbox(images, newIdx);
  };

  // Click: toggle 1x / fit
  imgEl.onclick = (e) => {
    if (isDragging) return;
    e.stopPropagation();
    if (zoom > 1) { resetTransform(); }
    else { zoom = 2; imgEl.classList.add('zoomed'); applyTransform(); }
  };

  // Wheel zoom
  imgEl.addEventListener('wheel', (e) => {
    e.preventDefault();
    const delta = e.deltaY > 0 ? 0.85 : 1.15;
    zoom = Math.max(0.5, Math.min(5, zoom * delta));
    if (zoom <= 1) resetTransform();
    else { imgEl.classList.add('zoomed'); applyTransform(); }
  }, { passive: false });

  // Mouse drag
  imgEl.addEventListener('mousedown', (e) => {
    if (zoom <= 1) return;
    e.preventDefault();
    isDragging = true;
    dragStartX = e.clientX; dragStartY = e.clientY;
    startDx = dx; startDy = dy;
    imgEl.classList.add('dragging');
  });
  window.addEventListener('mousemove', (e) => {
    if (!isDragging) return;
    dx = startDx + (e.clientX - dragStartX) / zoom;
    dy = startDy + (e.clientY - dragStartY) / zoom;
    applyTransform();
  });
  window.addEventListener('mouseup', () => {
    if (isDragging) { isDragging = false; imgEl.classList.remove('dragging'); }
  });

  // Touch events (mobile: pinch + drag)
  let lastDist = 0, lastZoom = zoom;
  imgEl.addEventListener('touchstart', (e) => {
    if (e.touches.length === 2) {
      lastDist = Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY);
      lastZoom = zoom;
    } else if (e.touches.length === 1 && zoom > 1) {
      isDragging = true;
      dragStartX = e.touches[0].clientX; dragStartY = e.touches[0].clientY;
      startDx = dx; startDy = dy;
    }
  });
  imgEl.addEventListener('touchmove', (e) => {
    if (e.touches.length === 2) {
      e.preventDefault();
      const dist = Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY);
      zoom = Math.max(0.5, Math.min(5, lastZoom * (dist / lastDist)));
      if (zoom <= 1) resetTransform();
      else { imgEl.classList.add('zoomed'); applyTransform(); }
    } else if (e.touches.length === 1 && isDragging) {
      dx = startDx + (e.touches[0].clientX - dragStartX) / zoom;
      dy = startDy + (e.touches[0].clientY - dragStartY) / zoom;
      applyTransform();
    }
  }, { passive: false });
  imgEl.addEventListener('touchend', () => { isDragging = false; });

  // Keyboard
  const onKey = (e) => {
    if (e.key === 'Escape') closeLightbox();
    else if (e.key === 'ArrowLeft' && images.length > 1) changeImage(-1);
    else if (e.key === 'ArrowRight' && images.length > 1) changeImage(1);
  };
  document.addEventListener('keydown', onKey);

  // Click overlay background to close
  overlay.onclick = (e) => { if (e.target === overlay) closeLightbox(); };

  document.body.appendChild(overlay);
}

// ── Auth handlers ──
function handleLogout() {
  api.logout();
  state.user = {};
  navigate('home');
}

// ═══════════════════════════════════════════
// Pages
// ═══════════════════════════════════════════

// ── Privacy Page ──
function renderPrivacyPage() {
  const container = document.getElementById('page-privacy');
  if (!container) return;
  container.innerHTML = '';
  container.appendChild(
    h('div', { className: 'main', style: { maxWidth: '720px' } },
      h('div', { className: 'form-card', style: { lineHeight: '1.8' } },
        h('h1', {}, '📜 隐私策略'),
        h('p', { style: { color: 'var(--text-sec)', marginBottom: '24px' } }, '最后更新：2026年6月7日'),
        ...[
          ['一、数据收集范围',
            '为提供积木图纸分享服务，我们收集以下数据：',
            ['<strong>账户信息</strong>：用户名、邮箱、密码（加密存储）—— 登录鉴权必需',
             '<strong>个人资料</strong>：头像、简介 —— 社区展示，可选',
             '<strong>作品内容</strong>：图片、标题、描述、标签、分类、难度、零件数 —— 社区核心功能必需',
             '<strong>互动数据</strong>：点赞、收藏、评论 —— 自动记录',
             '<strong>日志数据</strong>：IP、访问时间、User-Agent —— 安全审计、反滥用']],
          ['二、数据使用原则',
            '我们承诺：',
            ['<strong>最小化原则</strong>：只收集产品功能必需的数据',
             '<strong>透明原则</strong>：用户可随时查看/导出自己的全部数据',
             '<strong>可控原则</strong>：用户可随时编辑/删除自己的作品和个人资料',
             '<strong>无追踪原则</strong>：不使用第三方追踪脚本',
             '<strong>无广告定向</strong>：不做用户画像用于广告投放']],
          ['三、内容所有权',
            '您上传的作品版权归您所有，BrickPlan 仅获得展示权限。',
            ['删除作品后，服务器副本在30天内彻底清除',
             '注销账号后，所有个人数据在30天内匿名化或删除']],
          ['四、第三方共享',
            '我们不会向任何第三方出售或共享您的数据。',
            ['图片使用自托管存储，不经过第三方CDN',
             '法律要求时（如公安机关依法调查），依法提供必要数据']],
          ['五、未成年人保护',
            '注册时不要求年龄，但建议用户在个人资料中标注。',
            ['发现未成年人不当内容可举报，管理后台优先处理']],
          ['六、策略更新',
            '修改隐私策略时，通过站内通知 + 邮箱告知。',
            ['重大变更需要用户重新确认同意']],
          ['七、联系方式',
            '如有隐私相关问题，请联系：',
            ['邮箱：ghostjeky@hotmail.com',
             '举报入口：每个作品详情页底部有小字举报链接']],
        ].flatMap(([title, intro, items]) => [
          h('h2', { style: { marginTop: '32px', fontSize: '1.2rem' } }, title),
          h('p', {}, intro),
          ...items.map(item => typeof item === 'string'
            ? h('p', { style: { paddingLeft: '16px', borderLeft: '3px solid var(--border)', fontSize: '0.92rem', color: 'var(--text)' }, innerHTML: item })
            : item),
        ]),
      ),
    ),
  );
}

// ── Error Page (404 / 500) ──
const FAQ_ITEMS = [
  { q: '什么是 MOC？', a: 'MOC（My Own Creation）是乐高玩家自己设计的原创作品，区别于乐高官方套装。BrickPlan 是分享 MOC 图纸的社区。' },
  { q: '如何上传我的作品？', a: '注册登录后，点击右上角「上传」按钮，填写标题、分类、难度、零件数等信息，并上传图片或 PDF 图纸。' },
  { q: '支持什么文件格式？', a: '支持 JPG/PNG/WebP 图片和 PDF 文件，单文件最大 20MB，一次最多 10 个文件。图片会自动压缩转码。' },
  { q: '图纸版权归谁？', a: '作品版权归创作者所有。请勿上传他人受版权保护的作品；发现侵权可在作品页底部「举报」反馈。' },
  { q: '怎样让作品更受欢迎？', a: '完善描述、添加标签、上传清晰封面图的作品更易被发现。浏览量高的作品会出现在首页热门区。' },
  { q: '支持哪些分类？', a: '建筑、车辆、机甲、奇幻、科幻、场景六大分类，可配合标签进一步细化。' },
];

function renderFAQ() {
  const container = $id('page-faq');
  if (!container) return;
  container.innerHTML = '';
  container.appendChild(
    h('div', { className: 'main', style: { maxWidth: '720px' } },
      h('article', { className: 'form-card', style: { lineHeight: '1.8' } },
        h('h1', {}, '❓ 常见问题'),
        h('p', { style: { color: 'var(--text-sec)', marginBottom: '24px' } }, '关于 BrickPlan 积木图纸社区的常见问题解答。'),
        ...FAQ_ITEMS.flatMap(item => [
          h('h2', { style: { marginTop: '24px', fontSize: '1.15rem' } }, item.q),
          h('p', {}, item.a),
        ]),
        h('p', { style: { marginTop: '32px' } },
          '还有疑问？前往 ', h('a', { href: '/explore', onclick: (e) => { e.preventDefault(); navigate('explore'); }, style: { color: 'var(--accent)' } }, '发现页'), ' 浏览作品，或注册后上传你的 MOC。'),
      ),
    ),
  );
}

function renderErrorPage(code, message, detail) {
  const app = document.getElementById('app');
  // Remove all existing page containers and nav
  const existingNav = app.querySelector('.nav');
  if (existingNav) existingNav.remove();
  // Create a fresh error page container
  const pages = ['home', 'explore', 'detail', 'upload', 'user'];
  pages.forEach(p => {
    const el = document.getElementById(`page-${p}`);
    if (el) el.remove();
  });

  const container = document.createElement('div');
  container.id = 'page-error';
  container.className = 'page active';
  app.appendChild(container);

  const icons = { 404: '🔍', 500: '💥' };
  const titles = { 404: '404', 500: '500' };
  const messages = {
    404: tr('你要找的图纸可能被拆掉了...', 'The page you are looking for may have been taken apart...'),
    500: tr('服务器出了点问题，积木塔倒了！', 'Something went wrong on our side — the brick tower fell over!'),
  };

  container.innerHTML = '';
  const page = document.createElement('div');
  page.className = 'error-page';
  page.innerHTML = `
    <div class="error-icon">${icons[code] || '❓'}</div>
    <h1>${titles[code] || code}</h1>
    <p>${message || messages[code] || tr('未知错误', 'Unknown error')}</p>
    ${detail ? `<p class="error-detail">${detail}</p>` : ''}
    <button class="btn btn-primary" onclick="location.href='${langURL(currentLang(), '/')}'">🧱 ${tr('返回首页', 'Back to home')}</button>
  `;
  container.appendChild(page);
}

// ── Home ──
// Auto-rotate handle for the homepage slideshow. Cleared at the start of every
// renderHome() so navigating away (or re-rendering) never leaves a live timer
// mutating detached DOM.
let _homeSliderTimer = null;

// Homepage slideshow — featured works. Slides are 16:9 with an overlay caption;
// auto-advances every 4.5s, pauses on hover, and supports arrows + dots.
function renderHomeSlider(items) {
  const root = $id('home-slider');
  if (!root) return;
  root.innerHTML = '';
  if (!items || !items.length) { root.style.display = 'none'; return; }

  const emojis = { '建筑': '🏰', '车辆': '🚗', '机甲': '🤖', '奇幻': '🐉', '科幻': '🛸', '场景': '🎨' };
  const count = items.length;
  let idx = 0;

  const track = h('div', { className: 'home-slider-track' });
  const dotsWrap = h('div', { className: 'home-slider-dots' });

  const go = (n) => {
    idx = (n % count + count) % count;
    track.style.transform = `translateX(${-idx * 100}%)`;
    [...dotsWrap.children].forEach((d, i) => { d.className = 'home-slider-dot' + (i === idx ? ' active' : ''); });
  };

  // Declared here (not inside the `if (count > 1)` block below) so the dot and
  // arrow click handlers can all close over them.
  const start = () => { _homeSliderTimer = setInterval(() => go(idx + 1), 4500); };
  const restart = () => { if (count < 2) return; clearInterval(_homeSliderTimer); start(); };
  const stop = () => clearInterval(_homeSliderTimer);

  items.forEach((bp, i) => {
    const cover = getCoverImage(bp.images);
    const emoji = emojis[bp.category] || '🧱';
    track.appendChild(h('a', {
      className: 'home-slide',
      href: buildPath('detail', { id: bp.id }),
      onclick: (e) => { e.preventDefault(); navigate('detail', { id: bp.id }); },
    },
      cover
        ? h('img', {
            className: 'home-slide-img',
            src: cover,
            alt: bp.title,
            // First slide is the LCP element → eager + high priority; the rest
            // load lazily so the initial paint isn't blocked by extra images.
            loading: i === 0 ? 'eager' : 'lazy',
            fetchpriority: i === 0 ? 'high' : 'auto',
            decoding: 'async',
            onerror: "this.style.display='none'",
          })
        : h('div', { className: 'home-slide-noimg' }, emoji),
      h('div', { className: 'home-slide-overlay' },
        h('span', { className: 'home-slide-cat' }, `${emoji} ${categoryLabel(bp.category) || tr('积木', 'Bricks')}`),
        h('h3', { className: 'home-slide-title' }, bp.title),
        h('p', { className: 'home-slide-meta' },
          h('span', {}, bp.author?.username || tr('匿名', 'Anonymous')),
          h('span', {}, `👁 ${bp.view_count || 0}`),
          h('span', {}, `⭐ ${bp.favorite_count || 0}`),
        ),
      ),
    ));
  });

  items.forEach((_, i) => {
    dotsWrap.appendChild(h('button', {
      className: 'home-slider-dot' + (i === 0 ? ' active' : ''),
      'aria-label': tr(`第 ${i + 1} 张`, `Slide ${i + 1}`),
      onclick: () => { go(i); restart(); },
    }));
  });

  root.appendChild(track);
  if (count > 1) {
    root.appendChild(h('button', { className: 'home-slider-arrow prev', 'aria-label': tr('上一张', 'Previous slide'), onclick: () => { go(idx - 1); restart(); } }, '‹'));
    root.appendChild(h('button', { className: 'home-slider-arrow next', 'aria-label': tr('下一张', 'Next slide'), onclick: () => { go(idx + 1); restart(); } }, '›'));
    root.appendChild(dotsWrap);

    start();
    root.addEventListener('mouseenter', stop);
    root.addEventListener('mouseleave', restart);
  }
}

function renderHome() {
  // Stop any slideshow timer left over from a previous render/navigation.
  if (_homeSliderTimer) { clearInterval(_homeSliderTimer); _homeSliderTimer = null; }

  const container = $id('page-home');
  container.innerHTML = '';

  // Hero section
  container.appendChild(
    h('section', { className: 'hero' },
      h('h1', {}, '🧱 BrickPlan'),
      h('p', {}, tr('发现和分享乐高MOC创意的积木图纸社区', 'Discover and share LEGO MOC blueprints with builders worldwide')),
    ),
  );

  // Main content wrapper
  const mainWrap = h('div', { className: 'main' });
  container.appendChild(mainWrap);

  // ① Slideshow — current 推荐作品 (featured), skeleton until data arrives.
  mainWrap.appendChild(
    h('section', { id: 'home-slider', className: 'home-slider' },
      h('div', { className: 'home-slider-skeleton' }, h('div', { className: 'spinner' })),
    ),
  );

  // ② Category pills
  const categories = ['全部', '建筑', '机甲', '车辆', '科幻', '场景', '奇幻'];
  const emojis = { '全部': '🏠', '建筑': '🏰', '机甲': '🤖', '车辆': '🚗', '科幻': '🛸', '场景': '🎨', '奇幻': '🐉' };
  mainWrap.appendChild(
    h('div', { className: 'category-bar' },
      ...categories.map(cat =>
        h('button', {
          className: 'cat-pill' + (cat === '全部' ? ' active' : ''),
          onclick: () => {
            if (cat === '全部') {
              state.explore.category = ''; state.explore.page = 1; state.explore.q = ''; navigate('explore');
            } else {
              state.explore.category = cat; state.explore.page = 1; state.explore.q = ''; navigate('explore');
            }
          },
        }, `${emojis[cat]} ${cat === '全部' ? tr('全部', 'All') : categoryLabel(cat)}`),
      ),
    ),
  );

  // ③ 最新作品 section (was 热门推荐)
  mainWrap.appendChild(
    h('div', { className: 'section-header' },
      h('h2', { className: 'section-title' }, tr('🆕 最新作品', '🆕 Latest blueprints')),
      h('button', { className: 'btn btn-ghost', onclick: () => navigate('explore') }, tr('查看全部 →', 'View all →')),
    ),
  );
  mainWrap.appendChild(
    h('div', { id: 'home-latest', className: 'card-grid' },
      h('div', { className: 'loading', style: { gridColumn: '1/-1' } }, h('div', { className: 'spinner' })),
    ),
  );

  // ③bis 最新博客 — 首页是权重最高的已收录页，这里给文章页提供入口链接
  // （SEO 引荐来源）。真正的爬虫入口在 SSR noscript 镜像里，这里服务用户。
  mainWrap.appendChild(
    h('div', { className: 'section-header' },
      h('h2', { className: 'section-title' }, tr('📝 最新博客', '📝 Latest articles')),
      h('button', { className: 'btn btn-ghost', onclick: () => navigate('blog') }, tr('查看全部 →', 'View all →')),
    ),
  );
  mainWrap.appendChild(
    h('div', { id: 'home-blog', className: 'home-blog-list' },
      h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
    ),
  );

  // ④ Platform stats — single row, at the very end of the homepage content.
  mainWrap.appendChild(
    h('div', { id: 'home-stats', className: 'stats-bar stats-bar-end' },
      ...[tr('图纸作品', 'Blueprints'), tr('创作者', 'Creators'), tr('总浏览', 'Views'), tr('总收藏', 'Favorites')].map(label =>
        h('div', { className: 'stat-item' },
          h('div', { className: 'stat-value' }, '...'),
          h('div', { className: 'stat-label' }, label),
        ),
      ),
    ),
  );

  // ⑤ Footer
  mainWrap.appendChild(
    h('footer', { className: 'footer' },
      h('div', { style: { fontWeight: 800, fontSize: '1.3rem', marginBottom: '4px' } }, 'BrickPlan'),
      h('div', {}, tr('© 2026 BrickPlan 积木图纸分享社区', '© 2026 BrickPlan — brick blueprint community'),
        h('span', {}, ' · '),
        h('a', { href: buildPath('blog'), style: { color: 'var(--text-sec)', textDecoration: 'underline' } }, tr('博客', 'Blog')),
        h('span', {}, ' · '),
        h('a', { href: langURL(currentLang(), '/about'), style: { color: 'var(--text-sec)', textDecoration: 'underline' } }, tr('关于我们', 'About us')),
        h('span', {}, ' · '),
        h('a', { href: langURL(currentLang(), '/privacy'), style: { color: 'var(--text-sec)', textDecoration: 'underline' } }, tr('隐私策略', 'Privacy Policy')),
      ),
    ),
  );

  // One aggregated request renders the slideshow, the latest grid, and the
  // stats bar. Fewer round-trips = faster first meaningful paint.
  // 最新博客列表（失败时降级为指向 /blog 的静态链接，链接不会因为接口异常而消失）
  api.getBlogPosts({ page: 1, pageSize: 12 }).then(data => {
    const el = $id('home-blog');
    if (!el) return;
    const items = (data && data.items) || [];
    if (!items.length) {
      el.innerHTML = `<p class="loading">${tr('暂无文章', 'No articles yet')} <a href="${buildPath('blog')}">${tr('进入博客', 'Visit the blog')}</a></p>`;
      return;
    }
    el.innerHTML = '';
    items.forEach(p => {
      el.appendChild(
        h('a', {
          className: 'home-blog-card',
          href: buildPath('blog-detail', { slug: p.slug }),
          onclick: (e) => { e.preventDefault(); navigate('blog-detail', { slug: p.slug }); },
        },
          h('div', { className: 'home-blog-title' }, p.title),
          h('div', { className: 'home-blog-meta' }, `${p.date} · ${p.category || tr('博客', 'Blog')}`),
        ),
      );
    });
  }).catch(() => {
    const el = $id('home-blog');
    if (el) el.innerHTML = `<p class="loading">${tr('加载失败', 'Failed to load')} <a href="${buildPath('blog')}">${tr('直接访问博客', 'Open the blog')}</a></p>`;
  });


  api.getHome({ featured: 8, latest: 8 }).then(data => {
    renderHomeSlider(data.featured || []);

    const grid = $id('home-latest');
    if (grid) {
      const items = data.latest || [];
      grid.innerHTML = '';
      if (!items.length) {
        grid.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="empty-icon">📭</div><p>${tr('还没有图纸，快来上传第一个吧！', 'No blueprints yet — be the first to upload one!')}</p></div>`;
      } else {
        items.forEach(bp => grid.appendChild(renderBlueprintCard(bp)));
      }
    }

    const s = data.stats;
    const el = $id('home-stats');
    if (el && s) {
      const fmt = (n) => {
        n = n || 0;
        if (n >= 10000) return (n / 10000).toFixed(1).replace(/\.0$/, '') + 'w';
        if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
        return String(n);
      };
      el.innerHTML = '';
      [
        [fmt(s.total_blueprints), tr('图纸作品', 'Blueprints')],
        [fmt(s.total_users), tr('创作者', 'Creators')],
        [fmt(s.total_views), tr('总浏览', 'Views')],
        [fmt(s.total_favorites), tr('总收藏', 'Favorites')],
      ].forEach(([num, label]) => {
        el.appendChild(h('div', { className: 'stat-item' },
          h('div', { className: 'stat-value' }, num),
          h('div', { className: 'stat-label' }, label),
        ));
      });
    }
  }).catch(() => {
    const grid = $id('home-latest');
    if (grid) grid.innerHTML = `<div class="error-block" style="grid-column:1/-1"><div class="error-icon">🔌</div><p>${tr('加载失败，请检查后端服务', 'Failed to load. Please try again.')}</p><button class="btn btn-primary btn-sm" onclick="window._retryHome?.()">🔄 ${tr('重试', 'Retry')}</button></div>`;
    const slider = $id('home-slider');
    if (slider) slider.style.display = 'none';
    window._retryHome = () => navigate('home');
    showToast(tr('加载首页内容失败', 'Failed to load the homepage'), 'error');
  });
}


// ── Explore ──
function renderExplore() {
  const container = $id('page-explore');
  container.innerHTML = '';

  container.appendChild(
    h('div', { className: 'main' },
      // Page header
      h('div', { className: 'page-header' },
        h('h1', {}, tr('🔍 发现图纸', '🔍 Explore blueprints')),
        h('p', {}, tr('浏览社区中的积木创作，发现精彩作品', 'Browse brick builds from the community and find your next project')),
      ),

      // Filter bar
      h('div', { className: 'filter-bar' },
        h('div', { className: 'filter-group' },
          h('input', { type: 'text', id: 'explore-search', className: 'form-input', value: state.explore.q, placeholder: tr('🔍 搜索关键词...', '🔍 Search blueprints...'),
            onkeydown: (e) => { if (e.key === 'Enter') { state.explore.q = e.target.value; state.explore.category = $id('explore-category')?.value || ''; state.explore.sort = $id('explore-sort')?.value || 'new'; state.explore.page = 1; loadExploreResults(); } },
          }),
        ),
        h('div', { className: 'filter-group' },
          h('select', { id: 'explore-category', className: 'form-select' },
            // Category VALUES stay Chinese (that is what the API stores); only
            // the label is translated, or filtering would silently return 0.
            ...categoryOptions(true)
              .map(([v, t]) => h('option', { value: v, ...(state.explore.category === v ? { selected: '' } : {}) }, t)),
          ),
        ),
        h('div', { className: 'filter-group' },
          h('select', { id: 'explore-sort', className: 'form-select' },
            h('option', { value: 'new', ...(state.explore.sort === 'new' ? { selected: '' } : {}) }, tr('🆕 最新', '🆕 Newest')),
            h('option', { value: 'popular', ...(state.explore.sort === 'popular' ? { selected: '' } : {}) }, tr('🔥 最热门', '🔥 Most popular')),
          ),
        ),
        h('button', {
          className: 'btn btn-primary',
          onclick: () => {
            state.explore.q = $id('explore-search')?.value || '';
            state.explore.category = $id('explore-category')?.value || '';
            state.explore.sort = $id('explore-sort')?.value || 'new';
            state.explore.page = 1;
            loadExploreResults();
          },
        }, tr('筛选', 'Filter')),
      ),

      // Hot tags
      h('div', { id: 'explore-tags', className: 'tags-section' },
        h('span', { className: 'tags-title' }, tr('🏷️ 热门标签', '🏷️ Popular tags')),
        h('div', { id: 'explore-tags-list', style: { display: 'flex', gap: '6px', flexWrap: 'wrap' } },
          h('span', { style: { color: 'var(--text-sec)', fontSize: '0.8rem' } }, tr('加载中...', 'Loading...')),
        ),
      ),

      // Result count
      h('div', { className: 'result-info' },
        h('span', { id: 'explore-count', className: 'result-count' }, tr('加载中...', 'Loading...')),
      ),

      // Card grid
      h('div', { id: 'explore-results', className: 'card-grid' },
        h('div', { className: 'loading', style: { gridColumn: '1 / -1' } },
          h('div', { className: 'spinner' }),
        ),
      ),
      h('div', { id: 'explore-pagination', className: 'pagination' }),
    ),
  );

  loadExploreResults();
  loadExploreTags();
}

async function loadExploreResults() {
  const resultsEl = $id('explore-results');
  const countEl = $id('explore-count');
  const pagEl = $id('explore-pagination');

  try {
    const data = await api.listBlueprints({
      page: state.explore.page,
      size: 12,
      q: state.explore.q || undefined,
      category: state.explore.category || undefined,
      tag: state.explore.tag || undefined,
      sort: state.explore.sort,
    });

    if (countEl) countEl.textContent = tr(`共找到 ${data.total} 个作品`, `${data.total} blueprints found`);

    if (resultsEl) {
      resultsEl.innerHTML = '';
      if (!data.items?.length) {
        resultsEl.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="empty-icon">📭</div><p>${tr('没有找到匹配的图纸', 'No blueprints match your filters')}</p></div>`;
      } else {
        data.items.forEach(bp => resultsEl.appendChild(renderBlueprintCard(bp)));
      }
    }

    if (pagEl) {
      pagEl.innerHTML = '';
      const totalPages = Math.ceil(data.total / 12);
      if (totalPages <= 1) return;

      pagEl.appendChild(h('button', {
        className: 'page-btn',
        disabled: state.explore.page <= 1,
        style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
        onclick: () => { state.explore.page--; loadExploreResults(); },
      }, tr('‹ 上一页', '‹ Previous')));

      for (let i = 1; i <= Math.min(totalPages, 7); i++) {
        pagEl.appendChild(h('button', {
          className: `page-btn${i === state.explore.page ? ' active' : ''}`,
          onclick: () => { state.explore.page = i; loadExploreResults(); },
        }, String(i)));
      }

      if (totalPages > 7) {
        pagEl.appendChild(h('span', { style: { padding: '8px 6px' } }, '...'));
        pagEl.appendChild(h('button', {
          className: 'page-btn',
          onclick: () => { state.explore.page = totalPages; loadExploreResults(); },
        }, String(totalPages)));
      }

      pagEl.appendChild(h('button', {
        className: 'page-btn',
        disabled: state.explore.page >= totalPages,
        style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
        onclick: () => { state.explore.page++; loadExploreResults(); },
      }, tr('下一页 ›', 'Next ›')));
    }
  } catch (e) {
    if (resultsEl) resultsEl.innerHTML = `<div class="error-block" style="grid-column:1/-1">
      <div class="error-icon">🔌</div>
      <p>${tr('加载失败', 'Failed to load')}: ${errorMessage(e, '')}</p>
      <button class="btn btn-primary btn-sm retry-btn" onclick="window._retryExplore?.()">🔄 ${tr('重试', 'Retry')}</button>
    </div>`;
    window._retryExplore = loadExploreResults;
    if (countEl) countEl.textContent = '';
    showToast(tr('加载图纸列表失败，请检查网络连接', 'Failed to load blueprints. Please check your connection.'), 'error');
  }
}

async function loadExploreTags() {
  try {
    const tags = await api.listAllTags();
    const tagsListEl = $id('explore-tags-list');
    if (!tagsListEl) return;
    tagsListEl.innerHTML = '';
    if (!tags || !tags.length) {
      tagsListEl.appendChild(h('span', { style: { color: 'var(--text-sec)', fontSize: '0.8rem' } }, tr('暂无标签', 'No tags yet')));
      return;
    }
    tags.forEach(t => {
      tagsListEl.appendChild(
        h('button', {
          className: `tag-pill${state.explore.tag === t.name ? ' active' : ''}`,
          onclick: () => {
            state.explore.tag = state.explore.tag === t.name ? '' : t.name;
            state.explore.page = 1;
            loadExploreResults();
            loadExploreTags(); // re-render active state
          },
        },
          t.name,
          h('span', { className: 'tag-count' }, String(t.count || '')),
        ),
      );
    });
  } catch {
    const tagsListEl = $id('explore-tags-list');
    if (tagsListEl) tagsListEl.innerHTML = `<span style="color:var(--text-sec);font-size:0.8rem">${tr('加载标签失败', 'Failed to load tags')}</span>`;
  }
}

// ── Detail ──
function renderDetail() {
  const container = $id('page-detail');
  container.innerHTML = '';

  container.appendChild(
    h('div', { className: 'main' },
      state.flashMessage ? h('div', {
        id: 'flash-banner',
        style: { background: '#22c55e', color: 'white', padding: '12px 20px', borderRadius: '8px', textAlign: 'center', fontSize: '1rem', fontWeight: 700, marginBottom: '16px', opacity: '1', transition: 'opacity 0.5s' },
      }, state.flashMessage) : null,
      h('a', { className: 'back-link', href: buildPath('explore'), onclick: (e) => { e.preventDefault(); history.back(); } }, tr('← 返回列表', '← Back to list')),
      h('div', { id: 'detail-content', className: 'loading' },
        h('div', { className: 'spinner' }),
        h('p', { style: { marginTop: '12px' } }, tr('加载图纸详情...', 'Loading blueprint...')),
      ),
      h('section', { id: 'detail-related', className: 'detail-related' },
        h('h2', { style: { marginBottom: '16px' } }, tr('相关作品', 'Related blueprints')),
        h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
      ),
    ),
  );

  // Auto-dismiss flash message after 3 seconds
  if (state.flashMessage) {
    const msg = state.flashMessage;
    state.flashMessage = null;
    setTimeout(() => {
      const banner = document.getElementById('flash-banner');
      if (banner) { banner.style.opacity = '0'; setTimeout(() => banner.remove(), 500); }
    }, 3000);
  }

  if (state.detailId) {
    loadDetail(state.detailId);
    loadRelated(state.detailId);
  }
}

async function loadRelated(id) {
  try {
    const data = await api.getRelatedBlueprints(id);
    const el = $id('detail-related');
    if (!el) return;
    el.innerHTML = '';
    if (!data.items || data.items.length === 0) { el.style.display = 'none'; return; }
    el.appendChild(h('h2', { style: { marginBottom: '16px' } }, tr('相关作品', 'Related blueprints')));
    const grid = h('div', { className: 'card-grid' });
    data.items.forEach(bp => grid.appendChild(renderBlueprintCard(bp)));
    el.appendChild(grid);
  } catch {
    const el = $id('detail-related');
    if (el) el.style.display = 'none';
  }
}

async function loadDetail(id) {
  const contentEl = $id('detail-content');
  if (!contentEl) return;

  try {
    const bp = await api.getBlueprint(id);

    // ── Dynamic SEO: update title, OG tags, and inject JSON-LD ──
    // On the first render of an English page headLocked is true and the SSR
    // <head> stays untouched (it already describes this exact URL). After an
    // in-app navigation we own the head again and rewrite it in English.
    if (!headLocked) {
      document.title = `${bp.title} — BrickPlan`;
      setMeta('og:title', bp.title);
      const plainDesc = stripMarkdown(bp.description || tr('BrickPlan 积木图纸分享社区', 'BrickPlan blueprint community'));
      setMeta('og:description', plainDesc);
      setMeta('og:image', getCoverImage(bp.images) || '/og-default.png');
      setMeta('og:url', isEnglishPath() ? `${window.location.origin}${location.pathname}` : `${window.location.origin}/detail/${bp.id}`);
      setMeta('og:type', 'article');
      setMetaName('description', plainDesc.substring(0, 160));

      // Inject JSON-LD structured data
      const coverImg = getCoverImage(bp.images) || '';
      const ldScripts = [
        {
          '@context': 'https://schema.org',
          '@type': 'CreativeWork',
          name: bp.title,
          description: bp.description || '',
          image: coverImg,
          author: bp.author ? { '@type': 'Person', name: bp.author.username } : undefined,
          datePublished: bp.created_at || undefined,
        },
        {
          '@context': 'https://schema.org',
          '@type': 'Product',
          name: bp.title,
          description: bp.description || '',
          image: coverImg,
          brand: { '@type': 'Brand', name: bp.brand || 'BrickPlan' },
          offers: {
            '@type': 'Offer',
            price: '0',
            priceCurrency: 'CNY',
            availability: 'https://schema.org/InStock',
            url: isEnglishPath() ? `${window.location.origin}${location.pathname}` : `${window.location.origin}/detail/${bp.id}`,
          },
        },
      ];
      if (bp.category) {
        ldScripts[1].category = bp.category;
      }
      if (bp.product_code) {
        // mpn = the manufacturer's set/model number people actually search for.
        ldScripts[1].mpn = bp.product_code;
      }
      if (bp.piece_count != null) {
        ldScripts[1].additionalProperty = { '@type': 'PropertyValue', name: tr('零件数', 'Piece count'), value: bp.piece_count };
      }
      if (bp.difficulty != null) {
        ldScripts[1].aggregateRating = { '@type': 'AggregateRating', ratingValue: bp.difficulty, bestRating: 5, worstRating: 1, ratingCount: 1 };
      }
      injectJsonLd(ldScripts);
    }

    const stars = [];
    for (let i = 1; i <= 5; i++) {
      stars.push(h('span', { className: `dot${i <= (bp.difficulty || 0) ? ' filled' : ' empty'}` }));
    }

    const tags = (bp.tags || []).map(t => h('span', {
      className: 'tag',
      style: { marginRight: '6px', cursor: 'pointer' },
      onclick: () => navigate('explore', { tag: t }),
      title: tr(`查看所有 "${t}" 标签的图纸`, `View all blueprints tagged "${t}"`),
    }, t));

    const handleFavorite = async () => {
      if (!state.user?.id) { showModal('login'); return; }
      try {
        if (bp.is_favorited) {
          await api.unfavoriteBlueprint(id);
          bp.is_favorited = false;
          bp.favorite_count = Math.max(0, (bp.favorite_count || 1) - 1);
        } else {
          await api.favoriteBlueprint(id);
          bp.is_favorited = true;
          bp.favorite_count = (bp.favorite_count || 0) + 1;
        }
        loadDetail(id); // re-render
      } catch (e) {
        showToast(e.message, 'error');
      }
    };

    const diff = formatDifficulty(bp.difficulty || 0);

    contentEl.innerHTML = '';
    contentEl.appendChild(
      h('div', { className: 'detail-layout' },
        // LEFT: Gallery
        h('div', { className: 'gallery-wrap' },
          ...(bp.images && bp.images.length > 0 ? [
            (() => {
              const imgs = bp.images;
              let carouselIdx = getCoverIndex(imgs);
              const multi = imgs.length > 1;
              const currentItem = imgs[carouselIdx];
              const isCurrentPdf = (currentItem.file_type || 'image') === 'pdf';

              // Pre-compute non-PDF images for lightbox (PDFs can't be zoomed)
              const nonPdfImages = imgs.filter(i => (i.file_type || 'image') !== 'pdf');
              // Map: original index → non-PDF index
              const nonPdfIndexMap = {};
              imgs.forEach((img, i) => {
                if ((img.file_type || 'image') !== 'pdf') {
                  nonPdfIndexMap[i] = Object.keys(nonPdfIndexMap).length;
                }
              });
              const getLightboxIdx = (origIdx) => {
                // Find the correct index in nonPdfImages for the lightbox
                if (nonPdfImages.length === 0) return 0;
                if (nonPdfIndexMap[origIdx] !== undefined) return nonPdfIndexMap[origIdx];
                // If current is PDF, find nearest non-PDF
                for (let d = 1; d < imgs.length; d++) {
                  if (origIdx + d < imgs.length && nonPdfIndexMap[origIdx + d] !== undefined) return nonPdfIndexMap[origIdx + d];
                  if (origIdx - d >= 0 && nonPdfIndexMap[origIdx - d] !== undefined) return nonPdfIndexMap[origIdx - d];
                }
                return 0;
              };

              // Main content: image or PDF iframe
              let mainContentEl;
              if (isCurrentPdf) {
                mainContentEl = pdfContainer(currentItem.url);
              } else {
                mainContentEl = h('img', {
                  src: currentItem.url,
                  alt: bp.title,
                  loading: 'lazy',
                  onclick: () => {
                    if (nonPdfImages.length === 0) return;
                    openLightbox(nonPdfImages, getLightboxIdx(carouselIdx));
                  },
                });
              }

              const dots = imgs.map((_, i) =>
                h('button', {
                  className: `gallery-dot${i === carouselIdx ? ' active' : ''}`,
                  onclick: () => updateCarousel(i),
                })
              );

              const thumbs = imgs.map((img, i) => {
                const isPdf = (img.file_type || 'image') === 'pdf';
                if (isPdf) {
                  return h('div', {
                    className: `gallery-thumb${i === carouselIdx ? ' active' : ''}`,
                    style: {
                      width: '60px', height: '60px', display: 'flex', alignItems: 'center', justifyContent: 'center',
                      background: '#fef2f2', borderRadius: '6px', cursor: 'pointer', fontSize: '1.5rem',
                      border: i === carouselIdx ? '2px solid var(--accent)' : '2px solid transparent',
                    },
                    onclick: (e) => { e.stopPropagation(); updateCarousel(i); },
                    title: 'PDF 文件',
                  }, '📄');
                }
                return h('img', {
                  src: img.url,
                  loading: 'lazy',
                  className: `gallery-thumb${i === carouselIdx ? ' active' : ''}`,
                  onclick: (e) => {
                    e.stopPropagation();
                    updateCarousel(i);
                    if (nonPdfImages.length > 0) {
                      openLightbox(nonPdfImages, getLightboxIdx(i));
                    }
                  },
                });
              });

              const updateCarousel = (newIdx) => {
                if (newIdx < 0) newIdx = imgs.length - 1;
                if (newIdx >= imgs.length) newIdx = 0;
                carouselIdx = newIdx;
                const item = imgs[carouselIdx];
                const isPdf = (item.file_type || 'image') === 'pdf';
                // Replace main content
                galleryMain.innerHTML = '';
                if (isPdf) {
                  galleryMain.appendChild(pdfContainer(item.url));
                } else {
                  const newImg = h('img', {
                    src: item.url,
                    alt: bp.title,
                    loading: 'lazy',
                    onclick: () => {
                      if (nonPdfImages.length > 0) {
                        openLightbox(nonPdfImages, getLightboxIdx(carouselIdx));
                      }
                    },
                  });
                  galleryMain.appendChild(newImg);
                }
                if (prevBtn) galleryMain.appendChild(prevBtn);
                if (nextBtn) galleryMain.appendChild(nextBtn);
                if (dotsStrip) galleryMain.appendChild(dotsStrip);
                dots.forEach((d, i) => {
                  d.className = `gallery-dot${i === carouselIdx ? ' active' : ''}`;
                });
                thumbs.forEach((t, i) => {
                  t.className = `gallery-thumb${i === carouselIdx ? ' active' : ''}`;
                });
              };

              const prevBtn = multi ? h('button', {
                className: 'nav-arrow prev',
                onclick: (e) => { e.stopPropagation(); updateCarousel(carouselIdx - 1); },
              }, '‹') : null;

              const nextBtn = multi ? h('button', {
                className: 'nav-arrow next',
                onclick: (e) => { e.stopPropagation(); updateCarousel(carouselIdx + 1); },
              }, '›') : null;

              const dotsStrip = multi ? h('div', { className: 'gallery-dots' }, ...dots) : null;

              // gallery-main is the CONTAINER for img/iframe + arrows + dots
              const galleryMain = h('div', { className: 'gallery-main' },
                mainContentEl, prevBtn, nextBtn, dotsStrip,
              );

              // Touch / swipe
              let touchStartX = 0;
              if (multi) {
                galleryMain.addEventListener('touchstart', (e) => { touchStartX = e.touches[0].clientX; });
                galleryMain.addEventListener('touchend', (e) => {
                  const dx = e.changedTouches[0].clientX - touchStartX;
                  if (Math.abs(dx) > 50) updateCarousel(carouselIdx + (dx < 0 ? 1 : -1));
                });
              }

              // Keyboard arrows
              if (multi) {
                if (window._carouselKeyHandler) {
                  document.removeEventListener('keydown', window._carouselKeyHandler);
                }
                const keyHandler = (e) => {
                  if (state.page !== 'detail') return;
                  if (document.querySelector('.lightbox-overlay')) return;
                  if (e.key === 'ArrowLeft') updateCarousel(carouselIdx - 1);
                  if (e.key === 'ArrowRight') updateCarousel(carouselIdx + 1);
                };
                document.addEventListener('keydown', keyHandler);
                window._carouselKeyHandler = keyHandler;
              }

              const thumbsStrip = multi ? h('div', { className: 'gallery-thumbs' }, ...thumbs) : null;

              return [galleryMain, thumbsStrip].filter(Boolean);
            })(),
          ] : []),
        ),
        // RIGHT: Sidebar
        h('div', { className: 'detail-sidebar' },
          h('div', { className: 'info-card' },
            h('h1', { className: 'card-title', style: { fontSize: '1.5rem', marginBottom: '12px' } }, bp.title),
            (state.user?.id && bp.author?.id === state.user.id) ? h('div', { style: { marginBottom: '12px' } },
              h('button', { className: 'btn btn-ghost btn-sm', onclick: (e) => { e.preventDefault(); navigate('edit', { id: bp.id }); } }, tr('✏️ 编辑', '✏️ Edit')),
            ) : null,
            bp.author ? h('div', { className: 'author-row',
              onclick: () => navigate('user', { id: bp.author.id }),
            },
              h('img', { src: bp.author.avatar_url || `https://api.dicebear.com/7.x/thumbs/svg?seed=${encodeURIComponent(bp.author.username || 'anon')}`, alt: '' }),
              h('div', {},
                h('div', { className: 'author-name' }, bp.author.username),
                bp.created_at ? h('div', { className: 'author-date' }, tr(
                  `${new Date(bp.created_at).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long' })} 发布`,
                  `Published ${new Date(bp.created_at).toLocaleDateString('en-US', { year: 'numeric', month: 'long' })}`,
                )) : null,
              ),
            ) : null,
            h('div', { className: 'info-meta' },
              ...[
                h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('难度', 'Difficulty')),
                  h('span', { className: 'meta-value', style: { color: diff.color } }, `${diff.stars} ${diff.label}`),
                ),
                bp.piece_count ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('零件数', 'Pieces')),
                  h('span', { className: 'meta-value' }, String(bp.piece_count)),
                ) : null,
                bp.dimensions ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('尺寸', 'Dimensions')),
                  h('span', { className: 'meta-value' }, bp.dimensions),
                ) : null,
                bp.category ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('分类', 'Category')),
                  h('span', { className: 'meta-value' }, categoryLabel(bp.category)),
                ) : null,
                bp.brand ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('品牌', 'Brand')),
                  h('span', { className: 'meta-value' }, bp.brand),
                ) : null,
                bp.product_code ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, tr('编号', 'Set no.')),
                  h('span', { className: 'meta-value' }, bp.product_code),
                ) : null,
              ].filter(Boolean),
            ),
            h('div', { className: 'action-row' },
              h('button', {
                className: 'action-stat' + (bp.is_liked ? ' liked' : ''),
                onclick: async () => {
                  if (!state.user?.id) { showModal('login'); return; }
                  try {
                    if (bp.is_liked) {
                      await api.unlikeBlueprint(id);
                      bp.is_liked = false;
                      bp.like_count = Math.max(0, (bp.like_count || 1) - 1);
                    } else {
                      await api.likeBlueprint(id);
                      bp.is_liked = true;
                      bp.like_count = (bp.like_count || 0) + 1;
                    }
                    loadDetail(id);
                  } catch (e) { showToast(errorMessage(e), 'error'); }
                },
              },
                h('span', { className: 'stat-icon' }, bp.is_liked ? '❤️' : '🤍'),
                h('span', { className: 'stat-num' }, String(bp.like_count || 0)),
              ),
              h('button', {
                className: 'action-stat' + (bp.is_favorited ? ' faved' : ''),
                onclick: handleFavorite,
              },
                h('span', { className: 'stat-icon' }, bp.is_favorited ? '⭐' : '☆'),
                h('span', { className: 'stat-num' }, String(bp.favorite_count || 0)),
              ),
              h('div', { className: 'action-stat' },
                h('span', { className: 'stat-icon' }, '👁'),
                h('span', { className: 'stat-num' }, String(bp.view_count || 0)),
              ),
            ),
            bp.created_at ? h('div', { style: { fontSize: '0.8rem', color: 'var(--text-sec)', marginTop: '12px' } },
              tr(
                `发布于 ${new Date(bp.created_at).toLocaleDateString('zh-CN')}`,
                `Published ${new Date(bp.created_at).toLocaleDateString('en-US')}`,
              ),
            ) : null,
          ),
          // ── Parts List (PRD-07) ──
          ...(bp.part_list ? [
            (() => {
              let pl;
              try {
                pl = typeof bp.part_list === 'string' ? JSON.parse(bp.part_list) : bp.part_list;
              } catch (e) { pl = null; }
              if (!pl || !pl.parts || !pl.parts.length) return null;

              const rows = pl.parts.map(p =>
                h('tr', {},
                  h('td', {}, p.name || '—'),
                  h('td', { style: { textAlign: 'center' } }, String(p.count ?? p.qty ?? '—')),
                  h('td', {}, p.color || '—'),
                )
              );

              const totalRow = pl.total != null ? h('tr', { className: 'parts-total' },
                h('td', {}, tr('总计', 'Total')),
                h('td', { style: { textAlign: 'center', fontWeight: 800 } }, String(pl.total)),
                h('td', {}),
              ) : null;

              return h('div', { className: 'parts-section' },
                h('h3', {}, tr(`🧩 零件清单 (共 ${pl.parts.length} 种)`, `🧩 Parts list (${pl.parts.length} kinds)`)),
                h('table', { className: 'parts-table' },
                  h('thead', {},
                    h('tr', {},
                      h('th', {}, tr('零件名', 'Part')),
                      h('th', { style: { textAlign: 'center' } }, tr('数量', 'Qty')),
                      h('th', {}, tr('颜色', 'Color')),
                    ),
                  ),
                  h('tbody', {},
                    ...rows,
                    ...(totalRow ? [totalRow] : []),
                  ),
                ),
              );
            })(),
          ] : []),
          // ── Description inside sidebar (markdown rendered) ──
          ...(bp.description ? [
            h('div', { className: 'desc-card' },
              h('h3', {}, tr('📝 图纸介绍', '📝 About this blueprint')),
              h('div', { className: 'markdown-body', innerHTML: marked.parse(bp.description) }),
            ),
          ] : []),
          // ── Tags inside sidebar ──
          ...(tags.length ? [
            h('div', { style: { marginBottom: '16px' } },
              h('h3', { style: { fontWeight: 700, marginBottom: '8px' } }, tr('🏷️ 标签', '🏷️ Tags')),
              h('div', {}, ...tags),
            ),
          ] : []),
          // ── Comments inside sidebar ──
          h('div', { className: 'comments-card' },
            h('h3', {}, tr('💬 评论', '💬 Comments')),
            h('div', { id: 'detail-comments' },
              h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
            ),
          ),
        ),
      ),
    );

    // Load comments
    const renderComment = (c) => h('div', { className: `comment${c.parent_id ? ' comment-reply' : ''}` },
      h('div', { className: 'comment-header' },
        h('div', { className: 'comment-avatar' }, (c.user?.username || '?')[0].toUpperCase()),
        h('div', { className: 'comment-author' }, c.user?.username || tr('匿名', 'Anonymous')),
        c.is_pinned ? h('span', {
          style: { marginLeft: '6px', fontSize: '11px', fontWeight: 700, color: 'var(--accent, #2563eb)', background: 'var(--bg-sec, #f1f5f9)', borderRadius: '4px', padding: '1px 6px' },
        }, tr('📌 置顶', '📌 Pinned')) : null,
        c.created_at ? h('div', { className: 'comment-time' },
          new Date(c.created_at).toLocaleDateString(tr('zh-CN', 'en-US')),
        ) : null,
      ),
      h('div', { className: 'comment-text' }, c.content),
      state.user?.id ? h('button', {
        className: 'comment-reply-btn',
        onclick: () => showReplyBox(c.id, c.user?.username || tr('匿名', 'Anonymous')),
      }, tr('回复', 'Reply')) : null,
    );

    const showReplyBox = (parentId, username) => {
      const oldBox = document.querySelector('.reply-input-row');
      if (oldBox) oldBox.remove();
      const replyBox = h('div', { className: 'reply-input-row' },
        h('input', {
          type: 'text',
          className: 'comment-input',
          placeholder: tr(`回复 ${username}...`, `Reply to ${username}...`),
          onkeydown: async (e) => {
            if (e.key === 'Enter') {
              await submitReply(parentId, e.target);
            }
          },
        }),
        h('button', { className: 'btn btn-primary btn-sm', onclick: async (e) => submitReply(parentId, e.target.previousElementSibling) }, tr('发送', 'Send')),
        h('button', { className: 'btn btn-ghost btn-sm', onclick: () => replyBox.remove() }, tr('取消', 'Cancel')),
      );
      const commentsEl = $id('detail-comments');
      const target = commentsEl?.querySelector(`[data-comment-id="${parentId}"]`);
      if (target) target.insertAdjacentElement('afterend', replyBox);
    };

    const submitReply = async (parentId, input) => {
      const content = input?.value?.trim();
      if (!content) return;
      try {
        const newComment = await api.createComment(id, content, parentId);
        const commentsEl = $id('detail-comments');
        const target = commentsEl?.querySelector(`[data-comment-id="${parentId}"]`);
        const node = renderComment(newComment);
        node.dataset.commentId = newComment.id;
        if (target) target.insertAdjacentElement('afterend', node);
        input.closest('.reply-input-row')?.remove();
        showToast(tr('回复已发布', 'Reply posted'), 'success');
      } catch (e) {
        alert(tr('回复失败: ', 'Reply failed: ') + errorMessage(e, tr('请稍后重试', 'Please try again later.')));
      }
    };

    try {
      const comments = await api.listComments(id);
      const commentsEl = $id('detail-comments');
      if (commentsEl) {
        commentsEl.innerHTML = '';
        if (!comments.length) {
          commentsEl.appendChild(
            h('div', { className: 'empty', style: { padding: '24px' } },
              h('p', {}, tr('暂无评论，抢个沙发吧！', 'No comments yet — start the conversation!')),
            ),
          );
        } else {
          comments.forEach(c => {
            const node = renderComment(c);
            node.dataset.commentId = c.id;
            commentsEl.appendChild(node);
          });
        }
      }
    } catch { /* ignore */ }

    // Comment input box
    const commentsContainer = $id('detail-comments');
    if (commentsContainer) {
      if (state.user?.id) {
        const inputRow = h('div', { className: 'comment-input-row' },
          h('input', {
            type: 'text',
            id: 'comment-input',
            className: 'comment-input',
            placeholder: tr('写下你的评论...', 'Write a comment...'),
          }),
          h('button', {
            className: 'btn btn-primary btn-sm',
            onclick: async () => {
              const input = $id('comment-input');
              const content = input.value.trim();
              if (!content) return;
              const btn = input.nextElementSibling;
              btn.disabled = true;
              btn.textContent = tr('发表中...', 'Posting...');
              try {
                const newComment = await api.createComment(id, content);
                // Prepend new comment to the list
                const commentsEl = $id('detail-comments');
                // Remove empty state if present
                const emptyEl = commentsEl.querySelector('.empty');
                if (emptyEl) emptyEl.remove();
                const node = renderComment(newComment);
                node.dataset.commentId = newComment.id;
                commentsEl.insertBefore(node, commentsEl.firstChild);
                input.value = '';
              } catch (e) {
                alert(tr('发表失败: ', 'Could not post the comment: ') + errorMessage(e, tr('请稍后重试', 'Please try again later.')));
              } finally {
                btn.disabled = false;
                btn.textContent = tr('🚀 发表评论', '🚀 Post comment');
              }
            },
          }, tr('🚀 发表评论', '🚀 Post comment')),
        );
        commentsContainer.appendChild(inputRow);
      } else {
        const loginPrompt = h('div', { className: 'comment-login-prompt', style: {
          marginTop: '16px',
          borderTop: '2px solid #e5e5e5',
          paddingTop: '16px',
          textAlign: 'center',
        } },
          h('button', {
            className: 'btn btn-primary',
            onclick: () => showModal('login'),
          }, tr('🔑 登录后发表评论', '🔑 Sign in to comment')),
        );
        commentsContainer.appendChild(loginPrompt);
      }

      // Small report link at bottom of comments
      commentsContainer.appendChild(
        h('div', { style: { textAlign: 'center', marginTop: '16px', paddingTop: '12px', borderTop: '1px solid #eee' } },
          h('button', {
            className: 'report-link',
            style: { background: 'none', border: 'none', fontSize: '11px', color: 'var(--text-sec)', cursor: 'pointer', fontFamily: 'inherit' },
            onclick: () => showReportModal(id),
          }, tr('⚠ 举报不合适的内容', '⚠ Report inappropriate content')),
        ),
      );
    }
  } catch (e) {
    contentEl.innerHTML = `<div class="error-block">
      <div class="error-icon">😵</div>
      <p>${errorMessage(e, tr('加载失败', 'Failed to load'))}</p>
      <button class="btn btn-primary btn-sm retry-btn" onclick="window._retryDetail?.()">🔄 ${tr('重试', 'Retry')}</button>
    </div>`;
    window._retryDetail = () => loadDetail(id);
    showToast(tr('加载图纸详情失败', 'Failed to load the blueprint'), 'error');
  }
}

// ── Upload ──
async function renderNotifications() {
  const container = $id('page-notifications');
  container.innerHTML = '';

  if (!state.userLoaded) {
    container.appendChild(
      h('div', { className: 'main', style: { maxWidth: '760px' } },
        h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
      ),
    );
    return;
  }

  if (!state.user?.id) {
    container.appendChild(
      h('div', { className: 'main', style: { maxWidth: '760px' } },
        h('div', { className: 'page-header' },
          h('h1', {}, tr('🔔 站内通知', '🔔 Notifications')),
          h('p', {}, tr('登录后查看评论、回复、点赞和收藏提醒', 'Sign in to see comments, replies, likes and favorites')),
        ),
        h('div', { className: 'empty', style: { padding: '32px' } },
          h('div', { className: 'empty-icon' }, '🔐'),
          h('p', {}, tr('请先登录后查看通知', 'Please sign in to view your notifications')),
          h('button', { className: 'btn btn-primary', onclick: () => showModal('login') }, tr('去登录', 'Sign in')),
        ),
      ),
    );
    return;
  }
  container.appendChild(
    h('div', { className: 'main', style: { maxWidth: '760px' } },
      h('div', { className: 'page-header' },
        h('h1', {}, tr('🔔 站内通知', '🔔 Notifications')),
        h('p', {}, tr('评论、回复、点赞和收藏都会在这里提醒你', 'Comments, replies, likes and favorites all show up here')),
      ),
      h('div', { id: 'notifications-list', className: 'notifications-card' },
        h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
      ),
    ),
  );

  try {
    const data = await api.listNotifications({ size: 50 });
    const listEl = $id('notifications-list');
    if (!listEl) return;
    listEl.innerHTML = '';
    const items = data.items || [];
    if (!items.length) {
      listEl.appendChild(h('div', { className: 'empty', style: { padding: '32px' } },
        h('div', { className: 'empty-icon' }, '📭'),
        h('p', {}, tr('暂无通知', 'No notifications yet')),
      ));
    } else {
      items.forEach(item => listEl.appendChild(renderNotificationItem(item)));
    }
    if (data.unread_count > 0) {
      await api.markNotificationsRead();
      state.notificationUnreadCount = 0;
      renderNavbarIntoDOM();
    }
  } catch (e) {
    const listEl = $id('notifications-list');
    if (listEl) listEl.innerHTML = `<div class="error-block"><p>${tr('加载通知失败', 'Failed to load notifications')}: ${errorMessage(e, '')}</p></div>`;
  }
}

function renderNotificationItem(item) {
  const actor = item.actor?.username || tr('有人', 'Someone');
  const title = item.payload?.blueprint_title || tr('你的作品', 'your blueprint');
  const messageMap = isEnglishPath() ? {
    comment: `${actor} commented on "${title}"`,
    comment_reply: `${actor} replied to your comment`,
    like: `${actor} liked "${title}"`,
    favorite: `${actor} favorited "${title}"`,
  } : {
    comment: `${actor} 评论了《${title}》`,
    comment_reply: `${actor} 回复了你的评论`,
    like: `${actor} 点赞了《${title}》`,
    favorite: `${actor} 收藏了《${title}》`,
  };
  const message = messageMap[item.type] || tr(`${actor} 有新的互动`, `${actor} interacted with you`);
  const excerpt = item.payload?.comment_excerpt;
  return h('div', { className: `notification-item${item.is_read ? '' : ' unread'}` },
    h('div', { className: 'notification-main' },
      h('div', { className: 'notification-title' }, message),
      excerpt ? h('div', { className: 'notification-excerpt' }, excerpt) : null,
      h('div', { className: 'notification-time' }, item.created_at ? new Date(item.created_at).toLocaleString(tr('zh-CN', 'en-US')) : ''),
    ),
    item.blueprint_id ? h('button', {
      className: 'btn btn-ghost btn-sm',
      onclick: () => navigate('detail', { id: item.blueprint_id }),
    }, tr('查看', 'View')) : null,
  );
}

// ── Upload ──
function renderUpload() {
  if (!state.user?.id) {
    navigate('home');
    showModal('login');
    return;
  }

  const container = $id('page-upload');
  container.innerHTML = '';

  const categories = CATEGORIES;

  container.appendChild(
    h('div', { className: 'main' },
      h('div', { className: 'form-card' },
        h('h1', {}, tr('📤 上传图纸', '📤 Upload blueprint')),
        h('p', { className: 'subtitle' }, tr('分享你的积木创作', 'Share your brick build')),
        h('div', { id: 'upload-error' }),
        h('div', { id: 'upload-success' }),

        // Title
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('标题 *', 'Title *')),
          h('input', { type: 'text', id: 'upload-title', className: 'form-input', placeholder: tr('给你的作品起个名字', 'Give your build a name') }),
        ),

        // Files upload
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('图片 / PDF', 'Images / PDF')),
          h('div', {
            id: 'upload-images-area',
            className: 'upload-area',
            onclick: () => document.getElementById('upload-file-input')?.click(),
          },
            h('div', { className: 'icon' }, '🖼️'),
            h('div', { className: 'text' }, h('strong', {}, tr('点击选择图片或PDF', 'Click to choose images or PDFs'))),
            h('div', { className: 'hint' }, tr('支持 JPG / PNG / WebP / PDF，≤20MB，自动压缩', 'JPG / PNG / WebP / PDF, ≤20MB, compressed automatically')),
          ),
          h('input', {
            type: 'file', id: 'upload-file-input',
            accept: 'image/jpeg,image/png,image/webp,.pdf',
            multiple: true,
            style: { display: 'none' },
            onchange: handleImageSelect,
          }),
          h('div', { id: 'upload-preview', className: 'preview-grid' }),
        ),

        // Description
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('描述', 'Description')),
          h('textarea', { id: 'upload-desc', className: 'form-textarea', placeholder: tr('描述你的作品...', 'Describe your build...') }),
        ),

        // Category + Difficulty (2-col)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('分类', 'Category')),
            h('select', { id: 'upload-category', className: 'form-select' },
              h('option', { value: '' }, tr('选择分类', 'Choose a category')),
              ...categories.map(c => h('option', { value: c }, categoryLabel(c))),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('难度 (1-5)', 'Difficulty (1-5)')),
            h('select', { id: 'upload-difficulty', className: 'form-select' },
              h('option', { value: '' }, tr('选择难度', 'Choose difficulty')),
              ...[1, 2, 3, 4, 5].map(n => h('option', { value: String(n) }, `${'⭐'.repeat(n)} ${difficultyLabel(n)}`)),
            ),
          ),
        ),

        // Brand + Product code (2-col)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('品牌', 'Brand')),
            h('select', { id: 'upload-brand', className: 'form-select' },
              h('option', { value: '' }, tr('选择品牌', 'Choose a brand')),
              ...BRANDS.map(b => h('option', { value: b }, b)),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('型号', 'Set number')),
            h('input', { type: 'text', id: 'upload-product-code', className: 'form-input', placeholder: tr('例如: 42115', 'e.g. 42115') }),
          ),
        ),

        // Piece count + Dimensions (2-col)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('零件数', 'Piece count')),
            h('input', { type: 'text', id: 'upload-pieces', className: 'form-input', placeholder: tr('例如: 1500', 'e.g. 1500') }),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('尺寸', 'Dimensions')),
            h('input', { type: 'text', id: 'upload-dimensions', className: 'form-input', placeholder: 'e.g. 30x20x15 cm' }),
          ),
        ),

        // Tags
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('🏷️ 标签', '🏷️ Tags')),
          h('div', { id: 'upload-tags-chips', style: {
            display: 'flex', flexWrap: 'wrap', gap: '6px', marginBottom: '8px',
            minHeight: '0px',
          } }),
          h('div', { style: { display: 'flex', gap: '8px' } },
            h('input', {
              type: 'text', id: 'upload-tag-input',
              className: 'form-input',
              placeholder: tr('输入标签名，回车添加...', 'Type a tag and press Enter...'),
              style: { flex: 1 },
              onkeydown: (e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  addUploadTag();
                }
              },
            }),
            h('button', { className: 'btn btn-ghost btn-sm', onclick: addUploadTag }, tr('➕ 添加', '➕ Add')),
          ),
          h('p', { style: { fontSize: '0.75rem', color: 'var(--text-sec)', marginTop: '4px' } },
            tr('💡 按回车或点击「添加」按钮添加标签', '💡 Press Enter or click Add to attach a tag')),
        ),

        // Publish checkbox — hidden, always published
        h('input', { type: 'hidden', id: 'upload-published', value: 'true' }),

        // Submit buttons
        h('div', { style: { display: 'flex', gap: '12px', marginTop: '20px' } },
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { flex: 1 },
            onclick: handleUpload,
          }, tr('📤 上传图纸', '📤 Upload blueprint')),
          h('button', {
            className: 'btn btn-ghost',
            onclick: () => navigate('home'),
          }, tr('取消', 'Cancel')),
        ),
      ),
    ),
  );
}

// Store selected files for upload
let _selectedFiles = [];
let _selectedTags = [];

function addUploadTag() {
  const input = $id('upload-tag-input');
  const name = (input?.value || '').trim();
  if (!name) return;
  // Deduplicate
  if (_selectedTags.includes(name)) {
    input.value = '';
    return;
  }
  _selectedTags.push(name);
  input.value = '';
  renderTagsChips();
}

function removeUploadTag(idx) {
  _selectedTags = _selectedTags.filter((_, i) => i !== idx);
  renderTagsChips();
}

function renderTagsChips() {
  const chipsEl = $id('upload-tags-chips');
  if (!chipsEl) return;
  chipsEl.innerHTML = '';
  _selectedTags.forEach((t, idx) => {
    chipsEl.appendChild(
      h('span', {
        style: {
          display: 'inline-flex', alignItems: 'center', gap: '4px',
          background: 'var(--accent)', color: 'white',
          padding: '4px 10px', borderRadius: '20px',
          fontSize: '0.85rem', fontWeight: 700,
        },
      },
        t,
        h('span', {
          style: { cursor: 'pointer', marginLeft: '2px', fontSize: '1rem', lineHeight: '1' },
          onclick: () => removeUploadTag(idx),
        }, '×'),
      ),
    );
  });
}

let _coverIndex = 0; // cover index within _selectedFiles

function handleImageSelect(e) {
  const files = Array.from(e.target.files || []);
  if (files.length === 0) return;
  _selectedFiles = [..._selectedFiles, ...files];
  // Default cover: last non-PDF file
  _coverIndex = _selectedFiles.length - 1;
  for (let i = _selectedFiles.length - 1; i >= 0; i--) {
    if (!_selectedFiles[i].name.toLowerCase().endsWith('.pdf')) {
      _coverIndex = i;
      break;
    }
  }
  renderImagePreviews();
  // Reset the input so the same file can be re-selected if needed
  e.target.value = '';
}

function renderImagePreviews() {
  const preview = $id('upload-preview');
  if (!preview) return;
  preview.innerHTML = '';

  _selectedFiles.forEach((file, idx) => {
    const isPdf = file.name.toLowerCase().endsWith('.pdf');
    const wrapper = h('div', {
      'data-idx': idx,
      style: { position: 'relative', display: 'inline-block', cursor: 'grab' },
    });

    if (isPdf) {
      // PDF preview — show icon
      const pdfPreview = h('div', {
        style: {
          width: '100px', height: '100px', objectFit: 'cover',
          borderRadius: '8px', border: '3px solid #e5e5e5',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexDirection: 'column', background: '#fef2f2',
          fontSize: '0.7rem', color: '#dc2626', fontWeight: 700,
          textAlign: 'center', padding: '4px', boxSizing: 'border-box',
        },
      },
        h('div', { style: { fontSize: '1.8rem', marginBottom: '2px' } }, '📄'),
        h('div', { style: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '90px' } }, file.name),
      );
      wrapper.appendChild(pdfPreview);
    } else {
      // Image preview
      const reader = new FileReader();
      reader.onload = (ev) => {
        const imgEl = h('img', {
          src: ev.target.result,
          style: {
            width: '100px', height: '100px', objectFit: 'cover',
            borderRadius: '8px', border: '3px solid ' + (idx === _coverIndex ? '#FFD700' : '#e5e5e5'),
          },
          title: file.name,
        });
        wrapper.appendChild(imgEl);
      };
      reader.readAsDataURL(file);
    }

    // Star button (cover) — only for images
    if (!isPdf) {
      wrapper.appendChild(h('span', {
        style: {
          position: 'absolute', bottom: '-6px', right: '-6px',
          background: idx === _coverIndex ? '#FFD700' : '#aaa',
          color: idx === _coverIndex ? '#000' : '#fff',
          borderRadius: '50%', width: '24px', height: '24px',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: '14px', cursor: 'pointer', fontWeight: 'bold',
          border: '2px solid white', boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
        },
        title: tr('设为封面', 'Set as cover'),
        onclick: (ev2) => {
          ev2.stopPropagation();
          _coverIndex = idx;
          renderImagePreviews();
        },
      }, '⭐'));
    }

    // Delete button
    wrapper.appendChild(h('span', {
      style: {
        position: 'absolute', top: '-6px', right: '-6px',
        background: '#ef4444', color: 'white',
        borderRadius: '50%', width: '20px', height: '20px',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: '12px', cursor: 'pointer', fontWeight: 'bold',
      },
      onclick: (ev2) => {
        ev2.stopPropagation();
        _selectedFiles = _selectedFiles.filter((_, i) => i !== idx);
        if (_coverIndex >= _selectedFiles.length) _coverIndex = _selectedFiles.length - 1;
        // If cover is now a PDF, find last non-PDF
        if (_coverIndex >= 0 && _selectedFiles[_coverIndex]?.name?.toLowerCase().endsWith('.pdf')) {
          for (let i = _selectedFiles.length - 1; i >= 0; i--) {
            if (!_selectedFiles[i].name.toLowerCase().endsWith('.pdf')) {
              _coverIndex = i;
              break;
            }
          }
        }
        renderImagePreviews();
      },
    }, '×'));

    // Sort arrows (if multi-file)
    if (_selectedFiles.length > 1) {
      const arrowStyle = {
        position: 'absolute', left: '-4px',
        background: 'rgba(0,0,0,0.5)', color: 'white',
        width: '20px', height: '18px', display: 'flex',
        alignItems: 'center', justifyContent: 'center',
        cursor: 'pointer', fontSize: '12px', lineHeight: '1',
      };
      wrapper.appendChild(h('span', {
        style: { ...arrowStyle, top: '2px', borderRadius: '6px 6px 0 0' },
        title: tr('上移', 'Move up'),
        onclick: (ev2) => {
          ev2.stopPropagation();
          if (idx <= 0) return;
          [_selectedFiles[idx - 1], _selectedFiles[idx]] = [_selectedFiles[idx], _selectedFiles[idx - 1]];
          if (_coverIndex === idx) _coverIndex = idx - 1;
          else if (_coverIndex === idx - 1) _coverIndex = idx;
          renderImagePreviews();
        },
      }, '▲'));
      wrapper.appendChild(h('span', {
        style: { ...arrowStyle, bottom: '2px', borderRadius: '0 0 6px 6px' },
        title: tr('下移', 'Move down'),
        onclick: (ev2) => {
          ev2.stopPropagation();
          if (idx >= _selectedFiles.length - 1) return;
          [_selectedFiles[idx], _selectedFiles[idx + 1]] = [_selectedFiles[idx + 1], _selectedFiles[idx]];
          if (_coverIndex === idx) _coverIndex = idx + 1;
          else if (_coverIndex === idx + 1) _coverIndex = idx;
          renderImagePreviews();
        },
      }, '▼'));
    }

    preview.appendChild(wrapper);
  });
}

async function handleUpload() {
  const errEl = $id('upload-error');
  const okEl = $id('upload-success');
  if (errEl) errEl.innerHTML = '';
  if (okEl) okEl.innerHTML = '';

  const title = $id('upload-title')?.value.trim();
  const description = $id('upload-desc')?.value.trim();
  const category = $id('upload-category')?.value;
  const difficulty = parseInt($id('upload-difficulty')?.value) || undefined;
  const piece_count = parseInt($id('upload-pieces')?.value) || undefined;
  const dimensions = $id('upload-dimensions')?.value.trim();
  const brand = $id('upload-brand')?.value;
  const product_code = $id('upload-product-code')?.value.trim();
  const is_published = true; // always published

  if (!title) { if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('请输入标题', 'Please enter a title.')}</div>`; return; }

  try {
    // 1. Create blueprint
    if (okEl) okEl.innerHTML = `<div class="msg" style="color:var(--accent)">${tr('创建图纸中...', 'Creating the blueprint...')}</div>`;
    const data = await api.createBlueprint({
      title, description: description || undefined,
      category: category || undefined, difficulty,
      piece_count: piece_count || undefined,
      dimensions: dimensions || undefined,
      brand: brand || undefined,
      product_code: product_code || undefined,
      is_published,
    });

    // 2. Upload images if any
    if (_selectedFiles.length > 0) {
      if (okEl) okEl.innerHTML = `<div class="msg" style="color:var(--accent)">${tr(`上传图片中 (0/${_selectedFiles.length})...`, `Uploading files (0/${_selectedFiles.length})...`)}</div>`;
      for (let i = 0; i < _selectedFiles.length; i++) {
        try {
          await api.uploadBlueprintImage(data.id, _selectedFiles[i]);
          if (okEl) okEl.innerHTML = `<div class="msg" style="color:var(--accent)">${tr(`上传图片中 (${i + 1}/${_selectedFiles.length})...`, `Uploading files (${i + 1}/${_selectedFiles.length})...`)}</div>`;
        } catch (imgErr) {
          console.warn(`Image ${i + 1} upload failed:`, imgErr);
        }
      }
    }

    // Set cover image (skip if the target is a PDF)
    if (_selectedFiles.length > 0 && _coverIndex >= 0) {
      try {
        const bpDetail = await api.getBlueprint(data.id);
        if (bpDetail.images && bpDetail.images.length > 0) {
          const targetIdx = Math.min(_coverIndex, bpDetail.images.length - 1);
          const targetImg = bpDetail.images[targetIdx];
          // Only set cover if it's not a PDF
          if (!targetImg || (targetImg.file_type || 'image') !== 'pdf') {
            await api.setCover(data.id, targetImg.id);
          }
        }
      } catch (coverErr) {
        console.warn('Set cover failed:', coverErr);
      }
    }

    // 3. Bind tags
    if (_selectedTags.length > 0) {
      try {
        await api.bindTags(data.id, _selectedTags);
      } catch (tagErr) {
        console.warn('Tag binding failed:', tagErr);
      }
    }

    _selectedFiles = [];
    _selectedTags = [];
    if (okEl) okEl.innerHTML = `<div class="msg msg-success">${tr('🎉 上传成功！', '🎉 Upload complete!')}</div>`;
    state.flashMessage = tr('🎉 发布成功！可在个人主页管理你的作品', '🎉 Published! You can manage it from your profile.');
    setTimeout(() => navigate('detail', { id: data.id }), 800);
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('上传失败', 'Upload failed.'))}</div>`;
  }
}

// ── Blueprint Card Component ──
function renderBlueprintCard(bp, isOwnProfile = false) {
  const diff = formatDifficulty(bp.difficulty || 0);
  const coverUrl = getCoverImage(bp.images);
  const emojis = { '建筑': '🏰', '车辆': '🚗', '机甲': '🤖', '奇幻': '🐉', '科幻': '🛸', '场景': '🎨' };
  const emoji = emojis[bp.category] || '🧱';
  const bgColors = ['#6366f1', '#8b5cf6', '#ef4444', '#b91c1c', '#f59e0b', '#d97706', '#06b6d4', '#0891b2', '#a855f7', '#7c3aed', '#ec4899', '#be185d'];
  const bg = bgColors[Math.abs(bp.title?.length || 0) % bgColors.length];
  const bg2 = bgColors[(Math.abs(bp.title?.length || 0) + 1) % bgColors.length];

  const authorAvatar = bp.author?.avatar_url
    || `https://api.dicebear.com/7.x/thumbs/svg?seed=${encodeURIComponent(bp.author?.username || bp.id || 'anon')}`;

  const card = h('article', { className: `card${isOwnProfile ? ' card-own' : ''}`, onclick: isOwnProfile ? () => navigate('edit', { id: bp.id }) : () => navigate('detail', { id: bp.id }) },
    h('div', { className: 'card-img-wrap' },
      coverUrl
        ? h('img', {
            className: 'card-img',
            src: coverUrl,
            alt: bp.title,
            loading: 'lazy',
            decoding: 'async',
            onerror: "this.style.display='none';this.parentElement.style.background='linear-gradient(135deg," + bg + "," + bg2 + ")'",
          })
        : h('div', {
            className: 'card-noimg',
            style: { background: `linear-gradient(135deg, ${bg}, ${bg2})` },
          }, emoji),
      h('span', { className: 'card-diff', style: { background: diff.bg } }, `${diff.stars} ${diff.label}`),
      bp.piece_count ? h('span', { className: 'card-parts' }, tr(`🧩 ${bp.piece_count}片`, `🧩 ${bp.piece_count} pcs`)) : null,
    ),
    h('div', { className: 'card-body' },
      h('div', { className: 'card-title' }, bp.title),
      h('div', { className: 'card-author' },
        h('img', { src: authorAvatar, alt: '', loading: 'lazy', decoding: 'async' }),
        h('span', {}, bp.author?.username || tr('匿名', 'Anonymous')),
      ),
      h('div', { className: 'card-stats' },
        h('span', {}, `👁 ${bp.view_count || 0}`),
        h('span', {}, `❤️ ${bp.like_count || 0}`),
        h('span', {}, `⭐ ${bp.favorite_count || 0}`),
      ),
    ),
  );

  // Card actions for own profile
  if (isOwnProfile) {
    card.appendChild(h('div', { className: 'card-actions' },
      h('button', { className: 'btn btn-ghost btn-sm', onclick: (e) => { e.stopPropagation(); navigate('edit', { id: bp.id }); } }, tr('✏️ 编辑', '✏️ Edit')),
      h('button', { className: 'btn btn-ghost btn-sm', style: { color: '#ef4444' }, onclick: async (e) => {
        e.stopPropagation();
        if (!confirm(tr(`确定删除「${bp.title}」吗？此操作不可恢复。`, `Delete "${bp.title}"? This cannot be undone.`))) return;
        try {
          await api.deleteBlueprint(bp.id);
          showToast(tr('已删除', 'Deleted'), 'success');
          setTimeout(() => renderUserProfile(), 500);
        } catch (err) { showToast(errorMessage(err), 'error'); }
      } }, tr('🗑️ 删除', '🗑️ Delete')),
    ));
  }

  return card;
}

// ═══════════════════════════════════════════
// Render
// ═══════════════════════════════════════════

async function renderUserProfile() {
  const container = $id('page-user');
  container.innerHTML = '';
  const userId = state.userProfile.id;
  if (!userId) {
    container.innerHTML = `<div style="text-align:center;padding:80px 20px"><p style="font-size:3rem;margin-bottom:16px">🧱</p><p style="font-size:1.1rem;color:var(--text-sec);margin-bottom:24px">${tr('未指定用户', 'No user specified')}</p><button class="btn btn-primary" onclick="location.href='${langURL(currentLang(), '/')}'">${tr('返回首页', 'Back to home')}</button></div>`;
    return;
  }
  container.appendChild(h('p', {}, tr('Loading...', 'Loading...')));
  try {
    const profile = await api.getUserProfile(userId);
    state.userProfile = { ...state.userProfile, id: profile.id, username: profile.username };
    const items = state.userProfile.tab === 'favorites'
      ? await api.getUserFavorites(profile.id, { page: state.userProfile.page })
      : await api.getUserBlueprints(profile.id, { page: state.userProfile.page });
    container.innerHTML = '';
    const dateStr = profile.created_at ? new Date(profile.created_at).toLocaleDateString(tr('zh-CN', 'en-US'), { year: 'numeric', month: 'long' }) : '';
    const avatarUrl = profile.avatar_url;
    const avatarEl = avatarUrl
      ? h('img', { src: avatarUrl, className: 'profile-avatar' })
      : h('div', { className: 'profile-avatar', style: { background: 'var(--accent)', fontSize: '2rem', fontWeight: 800, display: 'flex', alignItems: 'center', justifyContent: 'center' } }, (profile.username || '?')[0].toUpperCase());

    const isOwnProfile = state.user && state.user.id === profile.id;

    container.appendChild(h('div', { className: 'main' },
      // Profile header
      h('div', { className: 'profile-header' },
        avatarEl,
        h('div', { className: 'profile-info' },
          h('div', { className: 'profile-name' }, profile.username),
          profile.bio ? h('div', { className: 'profile-bio' }, profile.bio) : null,
          h('div', { className: 'profile-meta' },
            dateStr ? h('span', {}, tr(`📅 ${dateStr} 加入`, `📅 Joined ${dateStr}`)) : null,
            h('span', {}, tr(`📐 ${profile.blueprint_count || 0} 作品`, `📐 ${profile.blueprint_count || 0} blueprints`)),
            h('span', {}, tr(`⭐ ${profile.favorite_count || 0} 收藏`, `⭐ ${profile.favorite_count || 0} favorites`)),
          ),
        ),
        isOwnProfile ? h('div', { className: 'profile-actions' },
          h('button', { className: 'btn btn-ghost btn-sm', onclick: () => openSettings(profile) }, tr('✏️ 编辑资料', '✏️ Edit profile')),
          h('button', { className: 'btn btn-primary btn-sm', onclick: () => navigate('upload') }, tr('📤 上传作品', '📤 Upload blueprint')),
          h('button', { className: 'btn btn-danger btn-sm', onclick: handleLogout }, tr('🚪 退出登录', '🚪 Sign out')),
        ) : null,
      ),

      // Stats row
      h('div', { className: 'stats-row' },
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.blueprint_count || 0)),
          h('div', { className: 'stat-label' }, tr('作品', 'Blueprints')),
        ),
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.favorite_count || 0)),
          h('div', { className: 'stat-label' }, tr('收藏', 'Favorites')),
        ),
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.total_views || 0)),
          h('div', { className: 'stat-label' }, tr('浏览量', 'Views')),
        ),
      ),

      // Tabs
      h('div', { className: 'tabs' },
        h('button', { className: `tab-btn${state.userProfile.tab === 'blueprints' ? ' active' : ''}`, onclick: () => { state.userProfile.tab = 'blueprints'; state.userProfile.page = 1; renderUserProfile(); } }, tr('我的作品', 'Blueprints')),
        h('button', { className: `tab-btn${state.userProfile.tab === 'favorites' ? ' active' : ''}`, onclick: () => { state.userProfile.tab = 'favorites'; state.userProfile.page = 1; renderUserProfile(); } }, tr('我的收藏', 'Favorites')),
      ),

      // Card grid
      items.items && items.items.length > 0
        ? h('div', { className: 'card-grid' },
            ...items.items.map(bp => renderBlueprintCard(bp, isOwnProfile)),
          )
        : h('div', { className: 'empty', style: { padding: '40px', textAlign: 'center' } }, h('p', {}, tr('暂无作品', 'No blueprints yet'))),

      // Pagination
      items.total > 12 ? h('div', { className: 'pagination', style: { justifyContent: 'center' } },
        ...Array.from({ length: Math.ceil(items.total / 12) }, (_, i) => h('button', { className: `page-btn${state.userProfile.page === i+1 ? ' active' : ''}`, onclick: () => { state.userProfile.page = i+1; renderUserProfile(); } }, String(i+1)))) : null,
    ));
  } catch (e) { container.innerHTML = `<div class="empty"><p>${tr('加载失败', 'Failed to load')}: ${errorMessage(e, '')}</p><button class="btn btn-primary btn-sm" onclick="location.reload()">${tr('重试', 'Retry')}</button></div>`; }
}

// ═══════════════════════════════════════════
// Settings Modal
// ═══════════════════════════════════════════
let _settingsTab = 'profile';
let _selectedAvatarUrl = null;
let _avatarPickerTab = 'presets';

function openAvatarPicker(profile) {
  // Close settings but we'll re-open it after avatar change
  const settingsOverlay = document.querySelector('.settings-overlay');
  if (settingsOverlay && settingsOverlay._escHandler) {
    document.removeEventListener('keydown', settingsOverlay._escHandler);
    settingsOverlay._escHandler = null;
  }

  _selectedAvatarUrl = null;
  _avatarPickerTab = 'presets';

  const overlay = h('div', {
    className: 'modal-overlay avatar-picker-overlay',
    onclick: (e) => { if (e.target === overlay) closeAvatarPicker(profile); },
  });

  const renderPickerContent = () => {
    const pickerEl = overlay.querySelector('.avatar-picker');
    if (!pickerEl) return;
    pickerEl.innerHTML = '';

    pickerEl.appendChild(
      h('button', { className: 'close', onclick: () => closeAvatarPicker(profile), style: { float: 'right' } }, '✕'),
      h('h2', {}, tr('🎨 选择头像', '🎨 Choose an avatar')),
      h('div', { className: 'avatar-picker-tabs' },
        h('button', {
          className: `avatar-picker-tab${_avatarPickerTab === 'presets' ? ' active' : ''}`,
          onclick: () => { _avatarPickerTab = 'presets'; _selectedAvatarUrl = null; renderPickerContent(); },
        }, tr('预设头像', 'Presets')),
        h('button', {
          className: `avatar-picker-tab${_avatarPickerTab === 'upload' ? ' active' : ''}`,
          onclick: () => { _avatarPickerTab = 'upload'; _selectedAvatarUrl = null; renderPickerContent(); },
        }, tr('上传图片', 'Upload image')),
      ),
    );

    if (_avatarPickerTab === 'presets') {
      const grid = h('div', { className: 'avatar-picker-grid' });
      pickerEl.appendChild(grid);

      api.getPresetAvatars().then(data => {
        const avatars = data.avatars || [];
        grid.innerHTML = '';
        avatars.forEach(av => {
          const img = h('img', {
            src: av.url,
            className: `avatar-option${_selectedAvatarUrl === av.url ? ' selected' : ''}`,
            onclick: () => {
              _selectedAvatarUrl = av.url;
              grid.querySelectorAll('.avatar-option').forEach(el => el.classList.remove('selected'));
              img.classList.add('selected');
            },
          });
          grid.appendChild(img);
        });
      }).catch(() => {
        grid.innerHTML = `<p style="grid-column:1/-1;text-align:center">${tr('加载预设头像失败', 'Failed to load preset avatars')}</p>`;
      });

      pickerEl.appendChild(
        h('div', { className: 'avatar-picker-actions' },
          h('button', { className: 'btn btn-white', onclick: () => closeAvatarPicker(profile) }, tr('取消', 'Cancel')),
          h('button', {
            className: 'btn btn-primary',
            onclick: async () => {
              if (!_selectedAvatarUrl) return;
              try {
                await api.updateProfile({ avatar_url: _selectedAvatarUrl });
                closeAvatarPicker(profile);
                showToast(tr('头像已更新', 'Avatar updated'), 'success');
                profile.avatar_url = _selectedAvatarUrl;
                _syncUserAvatar(_selectedAvatarUrl);
                openSettings(profile);
              } catch (e) {
                showToast(errorMessage(e), 'error');
              }
            },
          }, tr('确认', 'Confirm')),
        ),
      );
    } else {
      // Upload tab
      const uploadDiv = h('div', { className: 'avatar-picker-upload' },
        h('p', { style: { marginBottom: '12px', color: 'var(--text-sec)' } }, tr('选择图片文件上传（支持 jpg/png/webp/gif，最大 2MB）', 'Choose an image to upload (jpg/png/webp/gif, up to 2MB)')),
        h('input', {
          type: 'file',
          id: 'avatar-picker-file-input',
          accept: 'image/jpeg,image/png,image/webp,image/gif',
          style: { display: 'none' },
          onchange: async (e) => {
            const file = e.target.files?.[0];
            if (!file) return;
            if (file.size > 2 * 1024 * 1024) {
              showToast(tr('文件不能超过 2MB', 'The file must be 2MB or smaller'), 'error');
              return;
            }
            try {
              const result = await api.uploadAvatar(file);
              closeAvatarPicker(profile);
              showToast(tr('头像已更新', 'Avatar updated'), 'success');
              profile.avatar_url = result.avatar_url;
              _syncUserAvatar(result.avatar_url);
              openSettings(profile);
            } catch (e) {
              showToast(errorMessage(e), 'error');
            }
          },
        }),
        h('button', { className: 'btn btn-primary', onclick: () => document.getElementById('avatar-picker-file-input')?.click() }, tr('📁 选择文件', '📁 Choose file')),
      );
      pickerEl.appendChild(uploadDiv);

      pickerEl.appendChild(
        h('div', { className: 'avatar-picker-actions' },
          h('button', { className: 'btn btn-white', onclick: () => closeAvatarPicker(profile) }, tr('取消', 'Cancel')),
        ),
      );
    }
  };

  const picker = h('div', { className: 'avatar-picker' });
  overlay.appendChild(picker);
  document.body.appendChild(overlay);
  renderPickerContent();

  // Escape key to close
  const escHandler = (e) => {
    if (e.key === 'Escape') { closeAvatarPicker(profile); document.removeEventListener('keydown', escHandler); }
  };
  document.addEventListener('keydown', escHandler);
  overlay._escHandler = escHandler;
}

function closeAvatarPicker(profile) {
  const overlay = document.querySelector('.avatar-picker-overlay');
  if (overlay) {
    if (overlay._escHandler) document.removeEventListener('keydown', overlay._escHandler);
    overlay.remove();
  }
}

function openSettings(profile) {
  hideModal(); // close any existing modals
  _settingsTab = 'profile';

  const overlay = h('div', {
    className: 'modal-overlay settings-overlay',
    onclick: (e) => { if (e.target === overlay) closeSettings(); },
  });

  const renderSettingsContent = () => {
    const contentEl = overlay.querySelector('.settings-modal');
    if (!contentEl) return;
    contentEl.innerHTML = '';

    contentEl.appendChild(
      h('button', { className: 'close', onclick: closeSettings, style: { float: 'right' } }, '✕'),
      h('h2', {}, tr('⚙️ 账号设置', '⚙️ Account settings')),
      h('div', { className: 'settings-tabs' },
        h('button', {
          className: `settings-tab${_settingsTab === 'profile' ? ' active' : ''}`,
          onclick: () => { _settingsTab = 'profile'; renderSettingsContent(); },
        }, tr('个人资料', 'Profile')),
        h('button', {
          className: `settings-tab${_settingsTab === 'password' ? ' active' : ''}`,
          onclick: () => { _settingsTab = 'password'; renderSettingsContent(); },
        }, tr('修改密码', 'Password')),
      ),
      h('div', { id: 'settings-error' }),
    );

    if (_settingsTab === 'profile') {
      // Profile tab
      const avatarPreviewUrl = profile.avatar_url || null;
      const avatarPreview = h('div', { className: 'settings-avatar-upload', id: 'settings-avatar-preview' },
        avatarPreviewUrl
          ? h('img', { src: avatarPreviewUrl, style: { width: '80px', height: '80px', borderRadius: '50%', objectFit: 'cover', border: '3px solid #e5e5e5' } })
          : h('div', { style: { width: '80px', height: '80px', borderRadius: '50%', background: 'var(--accent)', fontSize: '2rem', fontWeight: 800, display: 'flex', alignItems: 'center', justifyContent: 'center', border: '3px solid #e5e5e5' } }, (profile.username || '?')[0].toUpperCase()),
        h('div', { style: { marginTop: '8px', display: 'flex', gap: '16px', justifyContent: 'center', fontSize: '0.8rem', fontWeight: 700 } },
          h('span', { style: { color: 'var(--text-sec)', cursor: 'pointer' }, onclick: (e) => { e.stopPropagation(); openAvatarPicker(profile); } }, tr('🎨 预设', '🎨 Presets')),
          h('span', { style: { color: 'var(--accent)', cursor: 'pointer' }, onclick: (e) => { e.stopPropagation(); document.getElementById('settings-avatar-input')?.click(); } }, tr('📁 本地上传', '📁 Upload')),
        ),
        h('input', {
          type: 'file',
          id: 'settings-avatar-input',
          accept: 'image/jpeg,image/png,image/webp,image/gif',
          style: { display: 'none' },
          onclick: (e) => e.stopPropagation(),
          onchange: handleAvatarUpload,
        }),
      );
      avatarPreview.onclick = () => openAvatarPicker(profile);

      contentEl.appendChild(
        h('div', { className: 'settings-form' },
          avatarPreview,
          h('div', { className: 'form-group' },
            h('label', {}, tr('昵称', 'Display name')),
            h('input', { type: 'text', id: 'settings-username', value: profile.username || '', placeholder: tr('你的用户名', 'Your username') }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, tr('个人简介', 'Bio')),
            h('textarea', { id: 'settings-bio', rows: 3, placeholder: tr('介绍一下自己...', 'Tell the community about yourself...'), style: { width: '100%', resize: 'vertical' } }, profile.bio || ''),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, tr('邮箱（不可更改）', 'Email (cannot be changed)')),
            h('input', { type: 'text', value: profile.email || '', disabled: true, style: { opacity: '0.6' } }),
          ),
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { width: '100%', marginTop: '8px' },
            onclick: handleSaveSettings,
          }, tr('💾 保存', '💾 Save')),
        ),
      );
    } else {
      // Password tab
      contentEl.appendChild(
        h('div', { className: 'settings-form' },
          h('div', { className: 'form-group' },
            h('label', {}, tr('当前密码', 'Current password')),
            h('input', { type: 'password', id: 'settings-current-password', placeholder: tr('输入当前密码', 'Enter your current password') }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, tr('新密码', 'New password')),
            h('input', { type: 'password', id: 'settings-new-password', placeholder: tr('至少6位', 'At least 6 characters') }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, tr('确认新密码', 'Confirm new password')),
            h('input', { type: 'password', id: 'settings-confirm-password', placeholder: tr('再次输入新密码', 'Enter the new password again') }),
          ),
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { width: '100%', marginTop: '8px' },
            onclick: handleChangePassword,
          }, tr('🔒 修改密码', '🔒 Change password')),
        ),
      );
    }
  };

  const modal = h('div', { className: 'modal settings-modal' });
  overlay.appendChild(modal);
  document.body.appendChild(overlay);
  renderSettingsContent();

  // Escape key to close
  const escHandler = (e) => {
    if (e.key === 'Escape') { closeSettings(); document.removeEventListener('keydown', escHandler); }
  };
  document.addEventListener('keydown', escHandler);
  overlay._escHandler = escHandler;
}

function closeSettings() {
  const overlay = document.querySelector('.settings-overlay');
  if (overlay) {
    if (overlay._escHandler) document.removeEventListener('keydown', overlay._escHandler);
    overlay.remove();
  }
}

async function handleSaveSettings() {
  const errEl = document.getElementById('settings-error');
  if (errEl) errEl.innerHTML = '';

  const username = document.getElementById('settings-username')?.value.trim();
  const bio = document.getElementById('settings-bio')?.value.trim();

  if (!username) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('昵称不能为空', 'Display name cannot be empty')}</div>`;
    return;
  }

  try {
    const data = await api.updateProfile({ username: username || undefined, bio: bio || undefined });
    state.user = data;
    state.userProfile = { ...state.userProfile, id: data.id, username: data.username };
    try {
      localStorage.setItem('bp_user', JSON.stringify(data));
    } catch { /* ignore */ }
    closeSettings();
    renderNavbarIntoDOM();
    showToast(tr('资料已更新', 'Profile updated'), 'success');
    // Refresh the profile page without changing the stable user id route
    renderUserProfile();
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('保存失败', 'Could not save your profile.'))}</div>`;
  }
}

async function handleAvatarUpload(e) {
  const file = e.target.files?.[0];
  if (!file) return;

  const errEl = document.getElementById('settings-error');
  if (errEl) errEl.innerHTML = '';

  // Validate size client-side
  if (file.size > 2 * 1024 * 1024) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('文件不能超过 2MB', 'The file must be 2MB or smaller')}</div>`;
    return;
  }

  try {
    const data = await api.uploadAvatar(file);
    // Update the preview
    const preview = document.getElementById('settings-avatar-preview');
    if (preview) {
      preview.innerHTML = '';
      const img = document.createElement('img');
      img.src = data.avatar_url;
      img.style.cssText = 'width:80px;height:80px;border-radius:50%;object-fit:cover;border:3px solid #e5e5e5';
      preview.appendChild(img);
      const label = document.createElement('p');
      label.style.cssText = 'margin-top:8px;font-size:0.8rem;font-weight:700;color:var(--text-sec)';
      label.textContent = tr('点击更换头像', 'Click to change your avatar');
      preview.appendChild(label);
      const input = document.getElementById('settings-avatar-input');
      if (input) {
        const newInput = input.cloneNode(true);
        newInput.onchange = handleAvatarUpload;
        input.replaceWith(newInput);
      }
      preview.onclick = () => openAvatarPicker(profile);
    }
    // Sync to navbar + localStorage
    _syncUserAvatar(data.avatar_url);
    if (data.user) {
      state.user = data.user;
      try {
        localStorage.setItem('bp_user', JSON.stringify(data.user));
      } catch { /* ignore */ }
      renderNavbarIntoDOM();
    }
    showToast(tr('头像已更新', 'Avatar updated'), 'success');
    // Re-render the profile behind the scenes
    setTimeout(() => renderUserProfile(), 300);
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('头像上传失败', 'Avatar upload failed.'))}</div>`;
  }
}

async function handleChangePassword() {
  const errEl = document.getElementById('settings-error');
  if (errEl) errEl.innerHTML = '';

  const current = document.getElementById('settings-current-password')?.value;
  const newPw = document.getElementById('settings-new-password')?.value;
  const confirm = document.getElementById('settings-confirm-password')?.value;

  if (!current || !newPw || !confirm) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('请填写所有密码字段', 'Please fill in all password fields.')}</div>`;
    return;
  }
  if (newPw.length < 6) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('新密码至少6位', 'The new password must be at least 6 characters.')}</div>`;
    return;
  }
  if (newPw !== confirm) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('两次输入的新密码不一致', 'The new passwords do not match.')}</div>`;
    return;
  }

  try {
    await api.changePassword(current, newPw);
    closeSettings();
    showToast(tr('密码已修改', 'Password changed'), 'success');
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('修改失败', 'Could not change your password.'))}</div>`;
  }
}

// ═══════════════════════════════════════════
// Delete Blueprint
// ═══════════════════════════════════════════
async function handleDeleteBlueprint(bp) {
  if (!confirm(tr(`确定要删除「${bp.title}」吗？此操作不可撤销！`, `Delete "${bp.title}"? This cannot be undone.`))) return;

  try {
    await api.deleteBlueprint(bp.id);
    showToast(tr('图纸已删除', 'Blueprint deleted'), 'success');
    renderUserProfile();
  } catch (e) {
    showToast(`${tr('删除失败', 'Delete failed')}: ${errorMessage(e, '')}`, 'error');
  }
}

// ═══════════════════════════════════════════
// Edit Blueprint Modal
// ═══════════════════════════════════════════
let _editImages = [];
let _editCoverIdx = 0;

function renderEditImages() {
  const container = document.getElementById('edit-bp-images');
  if (!container) return;
  container.innerHTML = '';

  if (_editImages.length === 0) {
    container.innerHTML = `<p style="color:var(--text-sec);font-size:0.8rem;">${tr('暂无图片', 'No images yet')}</p>`;
    return;
  }

  const moveImage = (fromIdx, toIdx) => {
    if (toIdx < 0 || toIdx >= _editImages.length) return;
    [_editImages[fromIdx], _editImages[toIdx]] = [_editImages[toIdx], _editImages[fromIdx]];
    if (_editCoverIdx === fromIdx) _editCoverIdx = toIdx;
    else if (_editCoverIdx === toIdx) _editCoverIdx = fromIdx;
    renderEditImages();
  };

  _editImages.forEach((img, idx) => {
    const isPdf = (img.file_type || 'image') === 'pdf';
    const item = h('div', { className: 'preview-item' });

    if (isPdf) {
      // PDF preview
      item.appendChild(h('div', {
        style: {
          width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexDirection: 'column', background: '#fef2f2', borderRadius: '8px',
          fontSize: '0.75rem', color: '#dc2626', fontWeight: 700, textAlign: 'center',
          padding: '8px', boxSizing: 'border-box', minHeight: '120px',
        },
      },
        h('div', { style: { fontSize: '2.5rem', marginBottom: '4px' } }, '📄'),
        h('div', { style: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%' } }, tr('PDF 文件', 'PDF file')),
      ));
    } else {
      item.appendChild(h('img', {
        src: img.url,
      }));
    }

    // Cover badge (bottom-left) — only for images
    if (!isPdf) {
      if (idx === _editCoverIdx) {
        item.appendChild(h('span', {
          className: 'cover-badge',
          title: tr('当前封面', 'Current cover'),
          onclick: (ev2) => { ev2.stopPropagation(); },
        }, tr('封面', 'Cover')));
      } else {
        item.appendChild(h('span', {
          className: 'cover-badge',
          style: { opacity: '0.5', cursor: 'pointer' },
          title: tr('设为封面', 'Set as cover'),
          onclick: (ev2) => {
            ev2.stopPropagation();
            _editCoverIdx = idx;
            renderEditImages();
          },
        }, tr('封面', 'Cover')));
      }
    }

    if (_editImages.length > 1) {
      const arrowStyle = {
        position: 'absolute', left: '-4px',
        background: 'rgba(0,0,0,0.5)', color: 'white',
        width: '20px', height: '18px', display: 'flex',
        alignItems: 'center', justifyContent: 'center',
        cursor: 'pointer', fontSize: '12px', lineHeight: '1', zIndex: 2,
      };
      item.appendChild(h('span', {
        className: 'sort-arrow sort-arrow-up',
        style: { ...arrowStyle, top: '2px', borderRadius: '6px 6px 0 0', opacity: idx === 0 ? 0.35 : 1 },
        title: tr('上移', 'Move up'),
        onclick: (ev2) => {
          ev2.stopPropagation();
          moveImage(idx, idx - 1);
        },
      }, '▲'));
      item.appendChild(h('span', {
        className: 'sort-arrow sort-arrow-down',
        style: { ...arrowStyle, bottom: '2px', borderRadius: '0 0 6px 6px', opacity: idx === _editImages.length - 1 ? 0.35 : 1 },
        title: tr('下移', 'Move down'),
        onclick: (ev2) => {
          ev2.stopPropagation();
          moveImage(idx, idx + 1);
        },
      }, '▼'));
    }

    // Delete button (top-right)
    item.appendChild(h('span', {
      className: 'remove-btn',
      title: tr('删除图片', 'Delete image'),
      onclick: async (ev2) => {
        ev2.stopPropagation();
        if (!confirm(tr('确定删除这张图片吗？', 'Delete this image?'))) return;
        try {
          await api.deleteBlueprintImage(_editImages[idx].blueprint_id || img.blueprint_id || state.editId, img.id);
          _editImages = _editImages.filter((_, i) => i !== idx);
          if (_editCoverIdx >= _editImages.length) _editCoverIdx = _editImages.length - 1;
          // If cover is now a PDF, find last non-PDF
          if (_editCoverIdx >= 0 && (_editImages[_editCoverIdx]?.file_type || 'image') === 'pdf') {
            for (let i = _editImages.length - 1; i >= 0; i--) {
              if ((_editImages[i]?.file_type || 'image') !== 'pdf') {
                _editCoverIdx = i;
                break;
              }
            }
          }
          renderEditImages();
          showToast(tr('图片已删除', 'Image deleted'), 'success');
        } catch (e) {
          showToast(tr('删除失败', 'Delete failed') + ': ' + errorMessage(e, ''), 'error');
        }
      },
    }, '×'));

    container.appendChild(item);
  });
}

let _editTags = [];

function openEditBlueprint(bp) {
  // Redirect to edit page — legacy wrapper
  navigate('edit', { id: bp.id });
}

async function loadEditPage(id) {
  const container = $id('page-edit');
  container.innerHTML = '';
  container.appendChild(h('p', {}, 'Loading...'));

  let bp;
  try {
    bp = await api.getBlueprint(id);
  } catch (e) {
    container.innerHTML = '';
    container.appendChild(
      h('div', { className: 'main' },
        h('div', { className: 'form-card', style: { textAlign: 'center', padding: '60px 20px' } },
          h('p', { style: { fontSize: '3rem', marginBottom: '16px' } }, '🧱'),
          h('p', { style: { fontSize: '1.1rem', color: 'var(--text-sec)', marginBottom: '24px' } }, `${tr('加载失败', 'Failed to load')}: ${errorMessage(e, '')}`),
          h('button', { className: 'btn btn-primary', onclick: () => navigate('home') }, tr('返回首页', 'Back to home')),
        ),
      ),
    );
    return;
  }

  // Initialize edit state
  _editImages = [...(bp.images || [])];
  _editCoverIdx = getCoverIndex(_editImages);
  _editTags = [...(bp.tags || [])];

  const categories = CATEGORIES;

  // Keep a brand the visitor typed before it was in the curated list selectable.
  const editBrand = bp.brand || '';
  const editBrandOptions = BRANDS.slice();
  if (editBrand && !editBrandOptions.includes(editBrand)) editBrandOptions.push(editBrand);

  const renderEditTagsChips = () => {
    const chipsEl = document.getElementById('edit-tags-chips');
    if (!chipsEl) return;
    chipsEl.innerHTML = '';
    _editTags.forEach((t, idx) => {
      chipsEl.appendChild(
        h('span', {
          style: {
            display: 'inline-flex', alignItems: 'center', gap: '4px',
            background: 'var(--accent)', color: 'white',
            padding: '4px 10px', borderRadius: '20px',
            fontSize: '0.85rem', fontWeight: 700,
          },
        },
          t,
          h('span', {
            style: { cursor: 'pointer', marginLeft: '2px', fontSize: '1rem', lineHeight: '1' },
            onclick: () => { _editTags = _editTags.filter((_, i) => i !== idx); renderEditTagsChips(); },
          }, '\u00d7'),
        ),
      );
    });
  };

  const addEditTag = () => {
    const input = document.getElementById('edit-tag-input');
    const name = (input?.value || '').trim();
    if (!name) return;
    if (_editTags.includes(name)) { input.value = ''; return; }
    _editTags.push(name);
    input.value = '';
    renderEditTagsChips();
  };

  container.innerHTML = '';
  container.appendChild(
    h('div', { className: 'main' },
      h('div', { className: 'form-card' },
        h('h1', {}, '\u270f\ufe0f 编辑图纸'),
        h('p', { className: 'subtitle' }, tr('修改你的积木创作信息', 'Update your build details')),
        h('div', { id: 'edit-bp-error' }),

        // Title
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('标题', 'Title')),
          h('input', { type: 'text', id: 'edit-bp-title', className: 'form-input', value: bp.title || '', placeholder: tr('图纸标题', 'Blueprint title') }),
        ),

        // Description
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('描述', 'Description')),
          h('textarea', { id: 'edit-bp-desc', className: 'form-textarea', placeholder: tr('描述你的作品...', 'Describe your build...') }, bp.description || ''),
        ),

        // Category + Difficulty (form-row)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('分类', 'Category')),
            h('select', { id: 'edit-bp-category', className: 'form-select' },
              h('option', { value: '' }, tr('选择分类', 'Choose a category')),
              ...categories.map(c => h('option', { value: c, ...(bp.category === c ? { selected: '' } : {}) }, categoryLabel(c))),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('难度 (1-5)', 'Difficulty (1-5)')),
            h('select', { id: 'edit-bp-difficulty', className: 'form-select' },
              h('option', { value: '' }, tr('选择难度', 'Choose difficulty')),
              ...[1, 2, 3, 4, 5].map(n => h('option', { value: String(n), ...(bp.difficulty === n ? { selected: '' } : {}) }, `${
                '\u2b50'.repeat(n)} ${difficultyLabel(n)}`)),
            ),
          ),
        ),

        // Brand + Product code (form-row)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('品牌', 'Brand')),
            h('select', { id: 'edit-bp-brand', className: 'form-select' },
              h('option', { value: '' }, tr('选择品牌', 'Choose a brand')),
              ...editBrandOptions.map(b => h('option', { value: b, ...(editBrand === b ? { selected: '' } : {}) }, b)),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('型号', 'Set number')),
            h('input', { type: 'text', id: 'edit-bp-product-code', className: 'form-input', value: bp.product_code || '', placeholder: tr('例如: 42115', 'e.g. 42115') }),
          ),
        ),

        // Piece count + Dimensions (form-row)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('零件数', 'Piece count')),
            h('input', { type: 'text', id: 'edit-bp-pieces', className: 'form-input', value: bp.piece_count || '', placeholder: tr('例如: 250', 'e.g. 250') }),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, tr('尺寸', 'Dimensions')),
            h('input', { type: 'text', id: 'edit-bp-dimensions', className: 'form-input', value: bp.dimensions || '', placeholder: 'e.g. 30x20x15 cm' }),
          ),
        ),

        // Tags
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('🏷️ 标签', '🏷️ Tags')),
          h('div', { id: 'edit-tags-chips', style: { display: 'flex', flexWrap: 'wrap', gap: '6px', marginBottom: '8px', minHeight: '0px' } }),
          h('div', { style: { display: 'flex', gap: '8px' } },
            h('input', {
              type: 'text', id: 'edit-tag-input', className: 'form-input',
              placeholder: tr('输入标签名，回车添加...', 'Type a tag and press Enter...'),
              style: { flex: 1 },
              onkeydown: (e) => { if (e.key === 'Enter') { e.preventDefault(); addEditTag(); } },
            }),
            h('button', { className: 'btn btn-ghost btn-sm', onclick: addEditTag }, '\u2795 添加'),
          ),
        ),

        // File management
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, tr('🖼️ 文件管理 (点击⭐设为封面，▲▼排序)', '🖼️ Files (click ⭐ to set the cover, ▲▼ to reorder)')),
          h('div', {
            className: 'upload-area',
            onclick: () => document.getElementById('edit-file-input')?.click(),
          },
            h('div', { className: 'icon' }, '🖼️'),
            h('div', { className: 'text' }, h('strong', {}, tr('点击添加图片或PDF', 'Click to add images or PDFs'))),
            h('div', { className: 'hint' }, tr('支持 JPG / PNG / WebP / PDF，≤20MB，自动压缩', 'JPG / PNG / WebP / PDF, ≤20MB, compressed automatically')),
          ),
          h('input', {
            type: 'file', id: 'edit-file-input',
            accept: 'image/jpeg,image/png,image/webp,.pdf',
            multiple: true,
            style: { display: 'none' },
            onchange: async (ev) => {
              const files = [...ev.target.files];
              if (!files || files.length === 0) return;
              let success = 0, fail = 0;
              let lastResult = null;
              for (const f of files) {
                try {
                  lastResult = await api.uploadBlueprintImage(id, f);
                  success++;
                } catch { fail++; }
              }
              // API returns all images for the blueprint — use the last (most complete) result
              if (lastResult && Array.isArray(lastResult)) {
                _editImages = lastResult.map(img => ({
                  ...img,
                  blueprint_id: img.blueprint_id || id,
                }));
              }
              renderEditImages();
              const msg = [];
              if (success > 0) msg.push(tr(`${success} 个上传成功`, `${success} uploaded`));
              if (fail > 0) msg.push(tr(`${fail} 个失败`, `${fail} failed`));
              showToast(msg.join(tr('，', ', ')), fail > 0 ? 'error' : 'success');
              // Reset input
              ev.target.value = '';
            },
          }),
          h('div', { id: 'edit-bp-images', className: 'preview-grid' }),
        ),

        // Save button
        h('button', {
          className: 'btn btn-primary btn-lg',
          style: { width: '100%', marginTop: '16px' },
          onclick: () => handleSaveEditBlueprint(id),
        }, tr('💾 保存修改', '💾 Save changes')),
      ),
    ),
  );

  // Render tags chips and images after DOM is ready
  setTimeout(() => renderEditTagsChips(), 50);
  setTimeout(() => renderEditImages(), 50);
}

function closeEditBlueprint() {
  const overlay = document.querySelector('.edit-bp-overlay');
  if (overlay) {
    if (overlay._escHandler) document.removeEventListener('keydown', overlay._escHandler);
    overlay.remove();
  }
}

async function handleSaveEditBlueprint(id) {
  const errEl = document.getElementById('edit-bp-error');
  if (errEl) errEl.innerHTML = '';

  const title = document.getElementById('edit-bp-title')?.value.trim();
  const description = document.getElementById('edit-bp-desc')?.value.trim();
  const category = document.getElementById('edit-bp-category')?.value;
  const difficultyRaw = document.getElementById('edit-bp-difficulty')?.value;
  const difficulty = difficultyRaw ? parseInt(difficultyRaw) : null;
  const piecesRaw = document.getElementById('edit-bp-pieces')?.value;
  const piece_count = piecesRaw ? parseInt(piecesRaw) || undefined : undefined;
  const dimensions = document.getElementById('edit-bp-dimensions')?.value.trim() || undefined;
  const brand = document.getElementById('edit-bp-brand')?.value || '';
  const product_code = document.getElementById('edit-bp-product-code')?.value.trim() || '';

  if (!title) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${tr('标题不能为空', 'Title cannot be empty')}</div>`;
    return;
  }

  try {
    await api.updateBlueprint(id, {
      title,
      description: description || undefined,
      category: category || undefined,
      difficulty: difficulty || undefined,
      piece_count,
      dimensions,
      // brand / product_code are sent even when empty so clearing the dropdown
      // actually removes the attribute (the backend treats "" as "clear").
      brand,
      product_code,
      tags: _editTags,
    });

    // Save image reorder
    if (_editImages.length > 0) {
      try {
        const orderPayload = _editImages.map((img, i) => ({ id: img.id, sort_order: i }));
        await api.reorderImages(id, orderPayload);
      } catch (reorderErr) {
        console.warn('Reordering images failed:', reorderErr);
      }
    }

    // Save cover image (skip PDFs)
    if (_editImages.length > 0 && _editCoverIdx >= 0 && _editCoverIdx < _editImages.length) {
      const coverImg = _editImages[_editCoverIdx];
      if (!coverImg || (coverImg.file_type || 'image') !== 'pdf') {
        try {
          await api.setCover(id, coverImg.id);
        } catch (coverErr) {
          console.warn('Setting cover failed:', coverErr);
        }
      }
    }

    showToast(tr('图纸已更新', 'Blueprint updated'), 'success');
    navigate('user', { id: state.user?.id });
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${errorMessage(e, tr('保存失败', 'Could not save the blueprint.'))}</div>`;
  }
}


async function renderAdminPage() {
  const container = $id('page-admin');
  container.innerHTML = '';
  let tab = 'pending'; // defaults
  let page = 1;
  let searchQ = '';
  let featuredAddQ = '';   // search query for the "add to featured" box
  let featuredItems = [];  // current featured list (for reorder)

  const buildUI = () => {
    container.innerHTML = '';
    container.appendChild(
      h('div', { className: 'main' },
        h('a', { className: 'back-link', href: '/', onclick: (e) => { e.preventDefault(); navigate('home'); } }, '← 返回前台'),
        h('div', { className: 'page-header' },
          h('h1', {}, '🔧 管理后台'),
          h('p', {}, '审核作品、管理全部内容'),
        ),

        // Stats row
        h('div', { id: 'admin-stats', className: 'stats-row' },
          h('div', { className: 'stat-box warn' },
            h('div', { className: 'stat-value' }, '...'),
            h('div', { className: 'stat-label' }, '待审核'),
          ),
          h('div', { className: 'stat-box' },
            h('div', { className: 'stat-value' }, '...'),
            h('div', { className: 'stat-label' }, '总作品'),
          ),
          h('div', { className: 'stat-box' },
            h('div', { className: 'stat-value' }, '...'),
            h('div', { className: 'stat-label' }, '用户数'),
          ),
          h('div', { className: 'stat-box' },
            h('div', { className: 'stat-value' }, '...'),
            h('div', { className: 'stat-label' }, '总浏览'),
          ),
          h('div', { className: 'stat-box warn' },
            h('div', { className: 'stat-value' }, '...'),
            h('div', { className: 'stat-label' }, '被举报'),
          ),
        ),

        // Tabs
        h('div', { className: 'tabs' },
          h('button', { className: `tab-btn${tab === 'pending' ? ' active' : ''}`, onclick: () => { tab = 'pending'; page = 1; buildUI(); } },
            '⏳ 待审核',
          ),
          h('button', { className: `tab-btn${tab === 'all' ? ' active' : ''}`, onclick: () => { tab = 'all'; page = 1; buildUI(); } }, '📋 全部作品'),
          h('button', { className: `tab-btn${tab === 'reports' ? ' active' : ''}`, onclick: () => { tab = 'reports'; page = 1; buildUI(); } }, '🚩 举报管理'),
          h('button', { className: `tab-btn${tab === 'comments' ? ' active' : ''}`, onclick: () => { tab = 'comments'; page = 1; buildUI(); } }, '💬 评论审核'),
          h('button', { className: `tab-btn${tab === 'users' ? ' active' : ''}`, onclick: () => { tab = 'users'; page = 1; buildUI(); } }, '👤 用户管理'),
          h('button', { className: `tab-btn${tab === 'featured' ? ' active' : ''}`, onclick: () => { tab = 'featured'; page = 1; buildUI(); } }, '🔥 热门推荐'),
        ),

        // Search (for "all" and "users")
        ...((tab === 'all' || tab === 'users' || tab === 'comments') ? [h('div', { style: { display: 'flex', gap: '8px', marginBottom: '16px' } },
          h('input', { type: 'text', id: 'admin-search', className: 'form-input', placeholder: tab === 'users' ? '搜索用户名或邮箱...' : (tab === 'comments' ? '搜索评论内容、作者或作品...' : '搜索标题或作者...'), value: searchQ, style: { flex: '1', maxWidth: '320px' },
            onkeydown: (e) => { if (e.key === 'Enter') { searchQ = e.target.value; page = 1; buildUI(); } },
          }),
          h('button', { className: 'btn btn-primary btn-sm', onclick: () => { searchQ = $id('admin-search')?.value || ''; page = 1; buildUI(); } }, '搜索'),
        )] : []),

        // Table card
        h('div', { id: 'admin-table', className: 'table-card', style: { minHeight: '200px' } },
          h('div', { className: 'loading' }, h('div', { className: 'spinner' })),
        ),

        // Pagination
        h('div', { id: 'admin-pagination', className: 'pagination' }),
      ),
    );

    loadTable();
    loadAdminStats();
  };

  const loadAdminStats = async () => {
    try {
      const stats = await api.getStats();
      const statsEl = $id('admin-stats');
      if (!statsEl) return;
      const fmt = (n) => n >= 10000 ? (n / 1000).toFixed(1) + 'k' : String(n);
      statsEl.innerHTML = '';
      const data = [
        [fmt(stats.pending_count || 0), '待审核', true],
        [fmt(stats.total_blueprints), '总作品', false],
        [fmt(stats.total_users), '用户数', false],
        [fmt(stats.total_views || 0), '总浏览', false],
        [fmt(stats.report_count || 0), '被举报', true],
      ];
      data.forEach(([num, label, warn]) => {
        statsEl.appendChild(h('div', { className: `stat-box${warn ? ' warn' : ''}` },
          h('div', { className: 'stat-value' }, num),
          h('div', { className: 'stat-label' }, label),
        ));
      });
    } catch { /* ignore */ }
  };

  // ── Featured (home 热门推荐) curation ──
  const renderFeaturedManager = async (tableEl, pagEl) => {
    if (pagEl) pagEl.innerHTML = '';
    tableEl.innerHTML = '';
    tableEl.appendChild(
      h('div', { style: { marginBottom: '24px' } },
        h('h3', { style: { margin: '0 0 8px' } }, '➕ 添加推荐'),
        h('div', { style: { display: 'flex', gap: '8px', marginBottom: '8px' } },
          h('input', { type: 'text', id: 'featured-add-search', className: 'form-input',
            placeholder: '搜索标题或作者...', value: featuredAddQ,
            style: { flex: '1', maxWidth: '320px' },
            onkeydown: (e) => { if (e.key === 'Enter') { featuredAddQ = e.target.value; searchFeaturedAdd(); } },
          }),
          h('button', { className: 'btn btn-primary btn-sm', onclick: () => { featuredAddQ = $id('featured-add-search')?.value || ''; searchFeaturedAdd(); } }, '搜索'),
        ),
        h('div', { id: 'featured-add-results' }),
      ),
    );
    tableEl.appendChild(
      h('div', {},
        h('h3', { style: { margin: '0 0 8px' } }, '🔥 当前推荐（↑↓ 排序，✕ 移除）'),
        h('div', { id: 'featured-list' }, h('div', { className: 'loading' }, h('div', { className: 'spinner' }))),
      ),
    );
    await loadFeaturedList();
    if (featuredAddQ.trim()) searchFeaturedAdd();
  };

  const loadFeaturedList = async () => {
    const el = $id('featured-list');
    if (!el) return;
    el.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
    try {
      const data = await api.adminListFeatured();
      featuredItems = data.items || [];
      renderFeaturedList();
    } catch {
      el.innerHTML = '<div class="empty"><p>加载失败</p></div>';
    }
  };

  const renderFeaturedList = () => {
    const el = $id('featured-list');
    if (!el) return;
    el.innerHTML = '';
    if (!featuredItems.length) {
      el.innerHTML = '<div class="empty"><div class="empty-icon">📭</div><p>暂无推荐作品，在上方搜索并添加</p></div>';
      return;
    }
    featuredItems.forEach((f, i) => {
      const bp = f.blueprint;
      const cover = getCoverImage(bp.images) || '';
      el.appendChild(
        h('div', { style: { display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 0', borderBottom: '1px solid var(--border)' } },
          h('span', { style: { color: 'var(--text-sec)', width: '24px', textAlign: 'center', fontWeight: 700 } }, String(i + 1)),
          cover ? h('img', { src: cover, className: 'cell-thumb' }) : h('div', { className: 'cell-thumb', style: { display: 'flex', alignItems: 'center', justifyContent: 'center' } }, '-'),
          h('div', { style: { flex: '1', minWidth: '0' } },
            h('a', { href: `#/detail?id=${bp.id}`, style: { fontWeight: 700 } }, bp.title),
            h('div', { style: { fontSize: '0.8rem', color: 'var(--text-sec)' } }, bp.author ? bp.author.username : '-'),
          ),
          h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '4px' }, disabled: i === 0, onclick: () => handleMoveFeatured(i, -1) }, '↑'),
          h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '4px' }, disabled: i === featuredItems.length - 1, onclick: () => handleMoveFeatured(i, 1) }, '↓'),
          h('button', { className: 'btn btn-danger btn-sm', onclick: () => handleRemoveFeatured(f.id) }, '✕'),
        ),
      );
    });
  };

  const searchFeaturedAdd = async () => {
    const el = $id('featured-add-results');
    if (!el) return;
    if (!featuredAddQ.trim()) { el.innerHTML = ''; return; }
    el.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
    try {
      const data = await api.adminListBlueprints({ q: featuredAddQ, page: 1 });
      const items = data.items || [];
      const featuredIds = new Set(featuredItems.map(f => f.blueprint_id));
      el.innerHTML = '';
      if (!items.length) {
        el.innerHTML = '<div class="empty" style="padding:16px"><p>没有找到作品</p></div>';
        return;
      }
      items.forEach(bp => {
        const cover = getCoverImage(bp.images) || '';
        const already = featuredIds.has(bp.id);
        el.appendChild(
          h('div', { style: { display: 'flex', alignItems: 'center', gap: '10px', padding: '8px 0', borderBottom: '1px solid var(--border)' } },
            cover ? h('img', { src: cover, className: 'cell-thumb' }) : h('div', { className: 'cell-thumb', style: { display: 'flex', alignItems: 'center', justifyContent: 'center' } }, '-'),
            h('div', { style: { flex: '1', minWidth: '0' } },
              h('span', { style: { fontWeight: 700 } }, bp.title),
              h('span', { style: { fontSize: '0.8rem', color: 'var(--text-sec)', marginLeft: '8px' } }, bp.author ? bp.author.username : '-'),
            ),
            already
              ? h('span', { style: { color: 'var(--text-sec)', fontSize: '0.85rem' } }, '已推荐')
              : h('button', { className: 'btn btn-primary btn-sm', onclick: () => handleAddFeatured(bp.id) }, '添加'),
          ),
        );
      });
    } catch {
      el.innerHTML = '<div class="empty"><p>搜索失败</p></div>';
    }
  };

  const handleAddFeatured = async (bpId) => {
    try {
      await api.adminAddFeatured(bpId);
      showToast('已添加到推荐', 'success');
      await loadFeaturedList();
      searchFeaturedAdd();
    } catch (e) {
      showToast(e.message || '添加失败', 'error');
    }
  };

  const handleRemoveFeatured = async (id) => {
    try {
      await api.adminRemoveFeatured(id);
      showToast('已移除', 'success');
      await loadFeaturedList();
    } catch (e) {
      showToast(e.message || '移除失败', 'error');
    }
  };

  const handleMoveFeatured = async (index, dir) => {
    const newIndex = index + dir;
    if (newIndex < 0 || newIndex >= featuredItems.length) return;
    const arr = featuredItems;
    [arr[index], arr[newIndex]] = [arr[newIndex], arr[index]];
    try {
      await api.adminReorderFeatured(arr.map((f, i) => ({ id: f.id, sort_order: i })));
      renderFeaturedList();
    } catch (e) {
      showToast(e.message || '排序失败', 'error');
      await loadFeaturedList();
    }
  };

  // Toggle featured state from the "全部作品" tab row. fRec is the existing
  // featured record (undefined when not yet featured). Reloads the table so the
  // button lights/unlights immediately.
  const handleToggleFeaturedAll = async (bp, fRec) => {
    try {
      if (fRec) {
        await api.adminRemoveFeatured(fRec.id);
        showToast('已取消推荐', 'success');
      } else {
        await api.adminAddFeatured(bp.id);
        showToast('已设为推荐', 'success');
      }
      await loadTable();
    } catch (e) {
      showToast(e.message || '操作失败', 'error');
    }
  };

  const loadTable = async () => {
    const tableEl = $id('admin-table');
    const pagEl = $id('admin-pagination');
    if (!tableEl) return;

    try {
      if (tab === 'featured') {
        await renderFeaturedManager(tableEl, pagEl);
        return;
      }
      if (tab === 'reports') {
        const data = await api.adminListReports({ page });
        const items = data.items || [];

        if (!items.length) {
          tableEl.innerHTML = '<div class="empty"><div class="empty-icon">✅</div><p>没有被举报的作品</p></div>';
          if (pagEl) pagEl.innerHTML = '';
          return;
        }

        const reasonLabels = { inappropriate: '不当内容', copyright: '侵权', incomplete: '图纸不完整', spam: '垃圾信息', other: '其他' };

        const rows = items.map(item => {
          const bp = item.blueprint;
          const cover = getCoverImage(bp.images) || '';

          // Build reports detail rows
          const reportRows = item.reports.map(r => {
            return h('tr', { style: { background: 'var(--bg-sec)' } },
              h('td', { colSpan: '7', style: { padding: '6px 12px', fontSize: '0.85rem' } },
                h('span', { style: { fontWeight: 700, color: 'var(--danger)' } }, `⚠ ${reasonLabels[r.reason] || r.reason}`),
                r.detail ? h('span', { style: { color: 'var(--text-sec)', marginLeft: '8px' } }, `— ${r.detail}`) : null,
                h('span', { style: { color: 'var(--text-sec)', marginLeft: '12px', fontSize: '0.8rem' } },
                  `举报人: ${r.reporter ? r.reporter.username : '—'} · ${new Date(r.created_at).toLocaleDateString('zh-CN')}`,
                ),
              ),
            );
          });

          const actions = [];
          if (bp.is_published) {
            actions.push(h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '6px' }, onclick: () => handleUnpublish(bp.id) }, '🚫 下架'));
          } else {
            actions.push(h('button', { className: 'btn btn-success btn-sm', style: { marginRight: '6px' }, onclick: () => handleApprove(bp.id) }, '✅ 通过'));
          }
          actions.push(h('button', { className: 'btn btn-danger btn-sm', onclick: () => confirmDelete(bp.id, 'delete') }, '🗑 删除'));

          const diff = formatDifficulty(bp.difficulty || 0);

          return [
            h('tr', { style: { borderLeft: '3px solid var(--danger)' } },
              h('td', {}, cover ? h('img', { src: cover, className: 'cell-thumb' }) : '—'),
              h('td', {},
                h('a', { href: `#/detail?id=${bp.id}`, style: { fontWeight: 700 } }, bp.title),
                h('div', { style: { fontSize: '0.8rem', color: 'var(--danger)', fontWeight: 700 } }, `🚩 ${item.report_count} 次举报`),
              ),
              h('td', {}, bp.author ? bp.author.username : '—'),
              h('td', {}, bp.category || '—'),
              h('td', {}, `${diff.stars}`),
              h('td', {}, bp.created_at ? new Date(bp.created_at).toLocaleDateString('zh-CN') : '—'),
              h('td', { style: { whiteSpace: 'nowrap' } }, ...actions),
            ),
            ...reportRows,
          ];
        }).flat();

        tableEl.innerHTML = '';
        tableEl.appendChild(
          h('div', { style: { overflowX: 'auto' } },
            h('table', { style: { width: '100%', borderCollapse: 'collapse' } },
              h('thead', {},
                h('tr', {},
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '封面'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '标题'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '作者'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '分类'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '难度'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '时间'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '操作'),
                ),
              ),
              h('tbody', {}, ...rows),
            ),
          ),
        );

        // Pagination
        const totalPages = Math.ceil(data.total / 20);
        if (pagEl && totalPages > 1) {
          pagEl.innerHTML = '';
          pagEl.appendChild(h('button', {
            className: 'page-btn',
            disabled: page <= 1,
            style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
            onclick: () => { page--; buildUI(); },
          }, tr('‹ 上一页', '‹ Previous')));
          for (let i = 1; i <= Math.min(totalPages, 7); i++) {
            pagEl.appendChild(h('button', {
              className: `page-btn${i === page ? ' active' : ''}`,
              onclick: () => { page = i; buildUI(); },
            }, String(i)));
          }
          if (totalPages > 7) {
            pagEl.appendChild(h('span', { style: { padding: '4px 8px' } }, '...'));
            pagEl.appendChild(h('button', { className: 'page-btn', onclick: () => { page = totalPages; buildUI(); } }, String(totalPages)));
          }
          pagEl.appendChild(h('button', {
            className: 'page-btn',
            disabled: page >= totalPages,
            style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
            onclick: () => { page++; buildUI(); },
          }, tr('下一页 ›', 'Next ›')));
        } else if (pagEl) {
          pagEl.innerHTML = '';
        }
        return;
      }

      if (tab === 'comments') {
        const data = await api.adminListComments({ page, q: searchQ });
        const items = data.items || [];
        if (!items.length) {
          tableEl.innerHTML = '<div class="empty"><div class="empty-icon">💬</div><p>' + (searchQ ? '没有找到评论' : '暂无评论') + '</p></div>';
          if (pagEl) pagEl.innerHTML = '';
          return;
        }

        const rows = items.map(cm => {
          const actions = [];
          actions.push(h('button', {
            className: `btn btn-sm${cm.is_pinned ? ' btn-primary' : ' btn-ghost'}`,
            style: { marginRight: '6px' },
            onclick: () => handleTogglePinComment(cm.id, !cm.is_pinned),
          }, cm.is_pinned ? '📌 取消置顶' : '📌 置顶'));
          if (cm.status === 'hidden') {
            actions.push(h('button', { className: 'btn btn-success btn-sm', style: { marginRight: '6px' }, onclick: () => handleSetCommentStatus(cm.id, 'approved') }, '通过'));
          } else {
            actions.push(h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '6px' }, onclick: () => handleSetCommentStatus(cm.id, 'hidden') }, '隐藏'));
          }
          actions.push(h('button', { className: 'btn btn-danger btn-sm', onclick: () => confirmDeleteComment(cm.id) }, '🗑 删除'));

          const excerpt = cm.content.length > 60 ? cm.content.slice(0, 60) + '…' : cm.content;
          return h('tr', {},
            h('td', { style: { maxWidth: '320px' } }, h('div', {}, excerpt)),
            h('td', {}, cm.user ? cm.user.username : '—'),
            h('td', {}, cm.blueprint_id
              ? h('a', {
                href: `/detail/${cm.blueprint_id}`,
                style: { fontWeight: 700 },
                onclick: (e) => { e.preventDefault(); navigate('detail', { id: cm.blueprint_id }); },
              }, cm.blueprint_title || '（作品已删除）')
              : '—'),
            h('td', {}, cm.status === 'hidden'
              ? h('span', { style: { color: '#dc2626', fontWeight: 700 } }, '已隐藏')
              : h('span', { style: { color: '#16a34a' } }, '已通过')),
            h('td', {}, cm.is_pinned ? '📌' : '—'),
            h('td', {}, cm.created_at ? new Date(cm.created_at).toLocaleDateString('zh-CN') : '—'),
            h('td', { style: { whiteSpace: 'nowrap' } }, ...actions),
          );
        });

        tableEl.innerHTML = '';
        tableEl.appendChild(
          h('div', { style: { overflowX: 'auto' } },
            h('table', { style: { width: '100%', borderCollapse: 'collapse' } },
              h('thead', {},
                h('tr', {},
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '评论内容'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '作者'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '所属作品'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '状态'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '置顶'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '时间'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '操作'),
                ),
              ),
              h('tbody', {}, ...rows),
            ),
          ),
        );

        const totalPages = Math.ceil(data.total / 20);
        if (pagEl && totalPages > 1) {
          pagEl.innerHTML = '';
          pagEl.appendChild(h('button', { className: 'page-btn', disabled: page <= 1, style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' }, onclick: () => { page--; buildUI(); } }, '‹ 上一页'));
          for (let i = 1; i <= Math.min(totalPages, 7); i++) {
            pagEl.appendChild(h('button', { className: `page-btn${i === page ? ' active' : ''}`, onclick: () => { page = i; buildUI(); } }, String(i)));
          }
          if (totalPages > 7) {
            pagEl.appendChild(h('span', { style: { padding: '4px 8px' } }, '...'));
            pagEl.appendChild(h('button', { className: 'page-btn', onclick: () => { page = totalPages; buildUI(); } }, String(totalPages)));
          }
          pagEl.appendChild(h('button', { className: 'page-btn', disabled: page >= totalPages, style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' }, onclick: () => { page++; buildUI(); } }, '下一页 ›'));
        } else if (pagEl) {
          pagEl.innerHTML = '';
        }
        return;
      }

      if (tab === 'users') {
        const data = await api.adminListUsers({ page, q: searchQ });
        const items = data.items || [];
        if (!items.length) {
          tableEl.innerHTML = '<div class="empty"><div class="empty-icon">👤</div><p>没有找到用户</p></div>';
          if (pagEl) pagEl.innerHTML = '';
          return;
        }

        const currentUserId = state.user?.id;
        const rows = items.map(u => {
          const tags = [];
          if (u.is_admin) tags.push(h('span', { style: { color: 'var(--accent)', fontWeight: 700, marginRight: '6px' } }, '管理员'));
          if (u.banned) tags.push(h('span', { style: { color: 'var(--danger)', fontWeight: 700, marginRight: '6px' } }, '已禁用'));
          if (!u.email_verified) tags.push(h('span', { style: { color: 'var(--text-sec)', marginRight: '6px' } }, '未验证'));
          const actions = [];
          if (u.id === currentUserId) {
            actions.push(h('span', { style: { color: 'var(--text-sec)', fontSize: '0.8rem' } }, '当前账号'));
          } else {
            actions.push(h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '6px' }, onclick: () => handleSetAdmin(u.id, !u.is_admin) }, u.is_admin ? '取消管理员' : '设为管理员'));
            actions.push(h('button', { className: `btn btn-sm${u.banned ? ' btn-success' : ' btn-ghost'}`, style: { marginRight: '6px' }, onclick: () => handleSetBanned(u.id, !u.banned) }, u.banned ? '启用' : '禁用'));
            actions.push(h('button', { className: 'btn btn-danger btn-sm', onclick: () => confirmDeleteUser(u.id, u.username) }, '🗑 删除'));
          }
          return h('tr', {},
            h('td', {}, u.avatar_url
              ? h('img', { src: u.avatar_url, className: 'cell-thumb', style: { borderRadius: '50%', objectFit: 'cover' } })
              : h('div', { className: 'cell-thumb', style: { display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-sec)', borderRadius: '50%', fontWeight: 700 } }, (u.username || '?').charAt(0).toUpperCase())),
            h('td', {}, h('strong', {}, u.username)),
            h('td', { style: { color: 'var(--text-sec)', fontSize: '0.85rem' } }, u.email),
            h('td', {}, String(u.blueprint_count || 0)),
            h('td', {}, u.created_at ? new Date(u.created_at).toLocaleDateString('zh-CN') : '-'),
            h('td', {}, ...tags),
            h('td', { style: { whiteSpace: 'nowrap' } }, ...actions),
          );
        });

        tableEl.innerHTML = '';
        tableEl.appendChild(
          h('div', { style: { overflowX: 'auto' } },
            h('table', { style: { width: '100%', borderCollapse: 'collapse' } },
              h('thead', {},
                h('tr', {},
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '头像'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '用户名'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '邮箱'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '作品数'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '注册时间'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '状态'),
                  h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '操作'),
                ),
              ),
              h('tbody', {}, ...rows),
            ),
          ),
        );

        const totalPages = Math.ceil(data.total / 20);
        if (pagEl && totalPages > 1) {
          pagEl.innerHTML = '';
          pagEl.appendChild(h('button', { className: 'page-btn', disabled: page <= 1, style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' }, onclick: () => { page--; buildUI(); } }, '‹ 上一页'));
          for (let i = 1; i <= Math.min(totalPages, 7); i++) {
            pagEl.appendChild(h('button', { className: `page-btn${i === page ? ' active' : ''}`, onclick: () => { page = i; buildUI(); } }, String(i)));
          }
          if (totalPages > 7) {
            pagEl.appendChild(h('span', { style: { padding: '4px 8px' } }, '...'));
            pagEl.appendChild(h('button', { className: 'page-btn', onclick: () => { page = totalPages; buildUI(); } }, String(totalPages)));
          }
          pagEl.appendChild(h('button', { className: 'page-btn', disabled: page >= totalPages, style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' }, onclick: () => { page++; buildUI(); } }, '下一页 ›'));
        } else if (pagEl) {
          pagEl.innerHTML = '';
        }
        return;
      }

      const data = tab === 'pending'
        ? await api.adminPendingBlueprints({ page })
        : await api.adminListBlueprints({ page, q: searchQ });

      const items = data.items || [];
      if (!items.length) {
        tableEl.innerHTML = '<div class="empty"><div class="empty-icon">📭</div><p>' + (tab === 'pending' ? '没有待审核的作品' : '没有找到作品') + '</p></div>';
        if (pagEl) pagEl.innerHTML = '';
        return;
      }

      // For the "全部作品" tab, preload which blueprints are featured (and their
      // featured record ids) so each row's 推荐 button can light up + toggle.
      let featuredMap = {};
      if (tab === 'all') {
        try {
          const fd = await api.adminListFeatured();
          (fd.items || []).forEach(f => { featuredMap[f.blueprint_id] = f; });
        } catch { /* ignore - buttons just default to off */ }
      }

      const coverUrl = (bp) => {
        const img = getCoverImage(bp.images);
        return img || '';
      };

      const rows = items.map(bp => {
        const actions = [];
        if (tab === 'pending') {
          actions.push(h('button', { className: 'btn btn-success btn-sm', style: { marginRight: '6px' }, onclick: () => handleApprove(bp.id) }, '✅ 通过'));
          actions.push(h('button', { className: 'btn btn-danger btn-sm', onclick: () => confirmDelete(bp.id, 'reject') }, '❌ 拒绝'));
        } else {
          const fRec = featuredMap[bp.id];
          const isFeatured = !!fRec;
          actions.push(h('button', {
            className: `btn btn-sm${isFeatured ? ' btn-primary' : ' btn-ghost'}`,
            style: { marginRight: '6px' },
            onclick: () => handleToggleFeaturedAll(bp, fRec),
          }, isFeatured ? '🔥 已推荐' : '🔥 推荐'));
          if (bp.is_published) {
            actions.push(h('button', { className: 'btn btn-ghost btn-sm', style: { marginRight: '6px' }, onclick: () => handleUnpublish(bp.id) }, '🚫 下架'));
          } else {
            actions.push(h('button', { className: 'btn btn-success btn-sm', style: { marginRight: '6px' }, onclick: () => handleApprove(bp.id) }, '✅ 通过'));
          }
          actions.push(h('button', { className: 'btn btn-danger btn-sm', onclick: () => confirmDelete(bp.id, 'delete') }, '🗑 删除'));
        }

        const diff = formatDifficulty(bp.difficulty || 0);

        return h('tr', {},
          h('td', {}, coverUrl(bp) ? h('img', { src: coverUrl(bp), className: 'cell-thumb' }) : '—'),
          h('td', {}, h('a', { href: `#/detail?id=${bp.id}`, style: { fontWeight: 700 } }, bp.title)),
          h('td', {}, bp.author ? bp.author.username : '—'),
          h('td', {}, bp.category || '—'),
          h('td', {}, `${diff.stars}`),
          h('td', {}, bp.created_at ? new Date(bp.created_at).toLocaleDateString('zh-CN') : '—'),
          h('td', { style: { whiteSpace: 'nowrap' } }, ...actions),
        );
      });

      tableEl.innerHTML = '';
      tableEl.appendChild(
        h('div', { style: { overflowX: 'auto' } },
          h('table', { style: { width: '100%', borderCollapse: 'collapse' } },
            h('thead', {},
              h('tr', {},
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '封面'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '标题'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '作者'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '分类'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '难度'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '时间'),
                h('th', { style: { textAlign: 'left', padding: '8px', borderBottom: '2px solid var(--border)' } }, '操作'),
              ),
            ),
            h('tbody', {}, ...rows),
          ),
        ),
      );

      // Pagination
      const totalPages = Math.ceil(data.total / 20);
      if (pagEl && totalPages > 1) {
        pagEl.innerHTML = '';
        pagEl.appendChild(h('button', {
          className: 'page-btn',
          disabled: page <= 1,
          style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
          onclick: () => { page--; buildUI(); },
        }, tr('‹ 上一页', '‹ Previous')));
        for (let i = 1; i <= Math.min(totalPages, 7); i++) {
          pagEl.appendChild(h('button', {
            className: `page-btn${i === page ? ' active' : ''}`,
            onclick: () => { page = i; buildUI(); },
          }, String(i)));
        }
        if (totalPages > 7) {
          pagEl.appendChild(h('span', { style: { padding: '4px 8px' } }, '...'));
          pagEl.appendChild(h('button', { className: 'page-btn', onclick: () => { page = totalPages; buildUI(); } }, String(totalPages)));
        }
        pagEl.appendChild(h('button', {
          className: 'page-btn',
          disabled: page >= totalPages,
          style: { width: 'auto', padding: '0 12px', whiteSpace: 'nowrap' },
          onclick: () => { page++; buildUI(); },
        }, tr('下一页 ›', 'Next ›')));
      } else if (pagEl) {
        pagEl.innerHTML = '';
      }
    } catch (e) {
      if (tableEl) tableEl.innerHTML = '<div class="error-block"><p>加载失败：' + e.message + '</p><button class="btn btn-primary btn-sm" onclick="buildUI()">🔄 重试</button></div>';
    }
  };

  const handleApprove = async (id) => {
    try {
      await api.adminPublish(id);
      showToast('审核通过 ✅', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  const handleUnpublish = async (id) => {
    try {
      await api.adminUnpublish(id);
      showToast('已下架 🚫', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  // Delete with confirmation modal
  const confirmDelete = (id, type) => {
    const modal = h('div', {
      id: 'confirm-modal',
      style: { position: 'fixed', inset: '0', background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: '9999' },
    },
      h('div', { style: { background: 'white', borderRadius: '12px', padding: '32px', maxWidth: '400px', width: '90%', textAlign: 'center', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' } },
        h('div', { style: { fontSize: '3rem', marginBottom: '12px' } }, '⚠️'),
        h('h3', { style: { marginBottom: '8px' } }, type === 'delete' ? '确认删除' : '确认拒绝'),
        h('p', { style: { color: 'var(--text-sec)', marginBottom: '24px' } }, type === 'delete' ? '此操作不可撤销，确定删除该作品？' : '确定拒绝该作品？'),
        h('div', { style: { display: 'flex', gap: '12px', justifyContent: 'center' } },
          h('button', { className: 'btn btn-ghost', onclick: () => { const el = $id('confirm-modal'); if (el) el.remove(); } }, '取消'),
          h('button', { className: 'btn btn-danger', onclick: async () => {
            try {
              if (type === 'delete') {
                await api.adminDelete(id);
              } else {
                await api.adminDelete(id);
              }
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast(type === 'delete' ? '已删除 🗑' : '已拒绝 ❌', 'success');
              buildUI();
            } catch (e) {
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast('操作失败：' + e.message, 'error');
            }
          } }, type === 'delete' ? '确认删除' : '确认拒绝'),
        ),
      ),
    );
    document.body.appendChild(modal);
  };

  const handleSetAdmin = async (id, isAdmin) => {
    try {
      await api.adminSetAdmin(id, isAdmin);
      showToast(isAdmin ? '已设为管理员' : '已取消管理员', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  const handleSetBanned = async (id, banned) => {
    try {
      await api.adminSetBanned(id, banned);
      showToast(banned ? '已禁用' : '已启用', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  const handleTogglePinComment = async (id, pinned) => {
    try {
      await api.adminSetCommentPinned(id, pinned);
      showToast(pinned ? '已置顶 📌' : '已取消置顶', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  const handleSetCommentStatus = async (id, status) => {
    try {
      await api.adminSetCommentStatus(id, status);
      showToast(status === 'hidden' ? '已隐藏评论' : '已通过评论 ✅', 'success');
      buildUI();
    } catch (e) {
      showToast('操作失败：' + e.message, 'error');
    }
  };

  const confirmDeleteComment = (id) => {
    const modal = h('div', {
      id: 'confirm-modal',
      style: { position: 'fixed', inset: '0', background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: '9999' },
    },
      h('div', { style: { background: 'white', borderRadius: '12px', padding: '32px', maxWidth: '400px', width: '90%', textAlign: 'center', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' } },
        h('div', { style: { fontSize: '3rem', marginBottom: '12px' } }, '⚠️'),
        h('h3', { style: { marginBottom: '8px' } }, '确认删除评论'),
        h('p', { style: { color: 'var(--text-sec)', marginBottom: '24px' } }, '将删除该评论及其所有回复，此操作不可撤销。确定？'),
        h('div', { style: { display: 'flex', gap: '12px', justifyContent: 'center' } },
          h('button', { className: 'btn btn-ghost', onclick: () => { const el = $id('confirm-modal'); if (el) el.remove(); } }, '取消'),
          h('button', { className: 'btn btn-danger', onclick: async () => {
            try {
              await api.adminDeleteComment(id);
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast('已删除评论 🗑', 'success');
              buildUI();
            } catch (e) {
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast('操作失败：' + e.message, 'error');
            }
          } }, '确认删除'),
        ),
      ),
    );
    document.body.appendChild(modal);
  };

  const confirmDeleteUser = (id, username) => {
    const modal = h('div', {
      id: 'confirm-modal',
      style: { position: 'fixed', inset: '0', background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: '9999' },
    },
      h('div', { style: { background: 'white', borderRadius: '12px', padding: '32px', maxWidth: '400px', width: '90%', textAlign: 'center', boxShadow: '0 20px 60px rgba(0,0,0,0.3)' } },
        h('div', { style: { fontSize: '3rem', marginBottom: '12px' } }, '⚠️'),
        h('h3', { style: { marginBottom: '8px' } }, '确认删除用户'),
        h('p', { style: { color: 'var(--text-sec)', marginBottom: '24px' } }, `将删除用户「${username}」及其所有作品，此操作不可撤销。确定？`),
        h('div', { style: { display: 'flex', gap: '12px', justifyContent: 'center' } },
          h('button', { className: 'btn btn-ghost', onclick: () => { const el = $id('confirm-modal'); if (el) el.remove(); } }, '取消'),
          h('button', { className: 'btn btn-danger', onclick: async () => {
            try {
              await api.adminDeleteUser(id);
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast('已删除用户 🗑', 'success');
              buildUI();
            } catch (e) {
              const el = $id('confirm-modal');
              if (el) el.remove();
              showToast('操作失败：' + e.message, 'error');
            }
          } }, '确认删除'),
        ),
      ),
    );
    document.body.appendChild(modal);
  };

  buildUI();
}
function renderNavbarIntoDOM() {
  // English pages are SSR-only (see isSsrOnlyPath) — leave the server's navbar
  // (and the rest of #app) alone so the English shell is never replaced.
  if (isSsrOnlyPath(location.pathname)) return;
  const existing = document.querySelector('.navbar');
  if (existing) existing.replaceWith(renderNavbar());
  else document.getElementById('app').prepend(renderNavbar());
}

// ═══════════════════════════════════════════
// Blog Pages
// ═══════════════════════════════════════════

// Module-level pagination state for the blog index.
// `page` is the last page successfully appended. There is no generic page
// cleanup hook, so renderBlogList() must reset this at its top (same pattern as
// _homeSliderTimer) or the counter would carry over between navigations.
let _blogPage = 1;
let _blogTotalPages = 1;
let _blogItems = 0;
let _blogTotal = 0;
let _blogLoading = false;

function buildBlogCard(post) {
  return h('div', {
    className: 'blog-card',
    onclick: () => navigate('blog-detail', { slug: post.slug }),
  },
    h('h2', {}, post.title),
    h('div', { className: 'blog-meta' },
      h('span', {}, post.date),
      post.category ? h('span', { className: 'blog-category' }, post.category) : null,
      post.author ? h('span', {}, post.author) : null,
    ),
    post.description ? h('div', { className: 'blog-desc' }, post.description) : null,
    ...(post.tags && post.tags.length ? [
      h('div', { className: 'blog-tags' },
        ...post.tags.map(t => h('span', { className: 'blog-tag' }, t)),
      ),
    ] : []),
  );
}

// Appends one page of cards and refreshes the load-more control so it always
// reflects how many posts are left.
function appendBlogCards(el, items) {
  items.forEach(post => {
    el.appendChild(buildBlogCard(post));
    _blogItems += 1;
  });
  updateBlogMore(el);
}

// Renders/updates/removes the "加载更多" button in one place, so the click
// handler can never be left pointing at a stale page number.
function updateBlogMore(el) {
  let more = $id('blog-load-more');
  const hasMore = _blogPage < _blogTotalPages;
  if (!hasMore) {
    if (more) more.remove();
    return;
  }
  if (!more) {
    more = h('div', { id: 'blog-load-more', className: 'load-more' });
    el.appendChild(more);
  }
  more.innerHTML = '';
  more.appendChild(h('button', {
    className: 'btn btn-outline',
    disabled: _blogLoading,
    onclick: () => loadMoreBlog(),
  }, _blogLoading
    ? tr('加载中…', 'Loading…')
    : tr(`加载更多（已显示 ${_blogItems} / ${_blogTotal} 篇）`, `Load more (${_blogItems} / ${_blogTotal} shown)`)));
}

async function loadMoreBlog() {
  if (_blogLoading || _blogPage >= _blogTotalPages) return;
  _blogLoading = true;
  const el = $id('blog-content');
  if (!el) { _blogLoading = false; return; }
  updateBlogMore(el);
  const filter = state.blogFilter || {};
  try {
    const data = await api.getBlogPosts({
      category: filter.category,
      tag: filter.tag,
      page: _blogPage + 1,
    });
    // The user may have navigated away while the request was in flight; writing
    // into a detached container would silently leak cards into nothing.
    if (!document.body.contains(el)) { _blogLoading = false; return; }
    _blogPage += 1;
    _blogTotalPages = data.total_pages || _blogTotalPages;
    _blogTotal = data.total || _blogTotal;
    appendBlogCards(el, data.items || []);
  } catch (e) {
    // Leave the button in place so the user can retry.
    console.error('loadMoreBlog failed', e);
  } finally {
    _blogLoading = false;
    const still = $id('blog-content');
    if (still) updateBlogMore(still);
  }
}

async function renderBlogList() {
  const container = $id('page-blog');
  if (!container) return;
  container.innerHTML = '';

  // Reset pagination state on every entry.
  _blogPage = 1;
  _blogTotalPages = 1;
  _blogItems = 0;
  _blogTotal = 0;
  _blogLoading = false;

  const filter = state.blogFilter || {};
  // The SSR handler serves /blog?page=N for crawlers; when a human lands on one
  // of those URLs the SPA walks pages 1..N so the list stays continuous.
  const targetPage = Math.max(1, parseInt(filter.page, 10) || 1);

  const main = h('div', { className: 'main' },
    h('div', { className: 'blog-list' },
      h('h1', {}, tr('📖 BrickPlan 博客', '📖 BrickPlan Blog')),
      h('p', { className: 'blog-subtitle' }, tr('积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容', 'Beginner guides, parts knowledge, brand comparisons and building tutorials for brick MOC builders')),
      h('div', { id: 'blog-content', className: 'loading' },
        h('div', { className: 'spinner' }),
      ),
    ),
  );
  container.appendChild(main);

  try {
    for (let p = 1; p <= targetPage; p++) {
      const data = await api.getBlogPosts({ category: filter.category, tag: filter.tag, page: p });
      const el = $id('blog-content');
      if (!el) return;
      if (p === 1) {
        el.innerHTML = '';
        el.className = '';
        if (data.categories && data.categories.length > 0) {
          const filterBar = h('div', { className: 'blog-filter' });
          filterBar.appendChild(h('button', {
            className: !filter.category && !filter.tag ? 'active' : '',
            onclick: () => navigate('blog'),
          }, tr('全部', 'All')));
          data.categories.forEach(cat => {
            filterBar.appendChild(h('button', {
              className: filter.category === cat ? 'active' : '',
              onclick: () => navigate('blog', { category: cat }),
            }, cat));
          });
          el.appendChild(filterBar);
        }
        if (!data.items || data.items.length === 0) {
          el.appendChild(h('div', { className: 'empty' },
            h('div', { className: 'empty-icon' }, '📭'),
            h('p', {}, tr('暂无文章', 'No articles yet')),
          ));
          return;
        }
      }
      _blogTotalPages = data.total_pages || 1;
      _blogTotal = data.total || 0;
      _blogPage = p;
      appendBlogCards(el, data.items || []);
      if (p >= _blogTotalPages) break;
    }
  } catch (e) {
    const el = $id('blog-content');
    if (el) {
      el.innerHTML = '';
      el.appendChild(h('div', { className: 'error-block' },
        h('div', { className: 'error-icon' }, '🔌'),
        h('p', {}, tr('加载失败，请检查后端服务', 'Failed to load. Please try again.')),
      ));
    }
  }
}

async function renderBlogDetail() {
  const container = $id('page-blog-detail');
  if (!container) return;
  container.innerHTML = '';

  const slug = state.blogSlug;
  if (!slug) { navigate('blog'); return; }

  container.appendChild(h('div', { className: 'main' },
    h('div', { className: 'blog-detail' },
      h('a', { className: 'back-link', href: buildPath('blog'), onclick: (e) => { e.preventDefault(); navigate('blog'); } }, tr('← 返回博客', '← Back to blog')),
      h('div', { id: 'blog-detail-content', className: 'loading' },
        h('div', { className: 'spinner' }),
      ),
    ),
  ));

  try {
    const post = await api.getBlogPost(slug);
    const el = $id('blog-detail-content');
    if (!el) return;
    el.innerHTML = '';
    el.className = '';

    // SEO: update title and meta. First render of an SSR page keeps the
    // server's <head> (headLocked); after in-app navigation we own it again.
    if (!headLocked) {
      document.title = tr(`${post.title} - BrickPlan 博客`, `${post.title} - BrickPlan Blog`);
      const plainDesc = stripMarkdown(post.description || post.title);
      setMeta('og:title', post.title);
      setMeta('og:description', plainDesc);
      setMeta('og:url', `${window.location.origin}${location.pathname}`);
      setMeta('og:type', 'article');
      setMetaName('description', plainDesc.substring(0, 160));
    }

    // Header
    el.appendChild(h('div', { className: 'blog-header' },
      h('h1', {}, post.title),
      h('div', { className: 'blog-meta' },
        h('span', {}, post.date),
        post.author ? h('span', {}, tr(`作者：${post.author}`, `By ${post.author}`)) : null,
        post.category ? h('span', { className: 'blog-category' }, post.category) : null,
      ),
    ));

    // Content (rendered markdown)
    el.appendChild(h('div', {
      className: 'blog-content markdown-body',
      innerHTML: marked.parse(post.body || ''),
    }));

    // Tags
    if (post.tags && post.tags.length) {
      el.appendChild(h('div', { className: 'blog-tags', style: { marginTop: '24px' } },
        ...post.tags.map(t => h('span', {
          className: 'blog-tag',
          style: { cursor: 'pointer' },
          onclick: () => navigate('blog', { tag: t }),
        }, t)),
      ));
    }

    // Prev/Next navigation
    if (post.prev_slug || post.next_slug) {
      const nav = h('div', { className: 'blog-nav' });
      if (post.prev_slug) {
        nav.appendChild(h('div', { className: 'nav-prev' },
          h('div', { className: 'nav-label' }, tr('上一篇', 'Previous')),
          h('a', { href: buildPath('blog-detail', { slug: post.prev_slug }), onclick: (e) => { e.preventDefault(); navigate('blog-detail', { slug: post.prev_slug }); } }, post.prev_title),
        ));
      } else {
        nav.appendChild(h('div', {}));
      }
      if (post.next_slug) {
        nav.appendChild(h('div', { className: 'nav-next', style: { textAlign: 'right' } },
          h('div', { className: 'nav-label' }, tr('下一篇', 'Next')),
          h('a', { href: buildPath('blog-detail', { slug: post.next_slug }), onclick: (e) => { e.preventDefault(); navigate('blog-detail', { slug: post.next_slug }); } }, post.next_title),
        ));
      }
      el.appendChild(nav);
    }

    // Related posts ("你可能还喜欢")
    if (post.related && post.related.length > 0) {
      const relSection = h('section', { className: 'blog-related', style: { marginTop: '40px', padding: '24px', background: 'var(--bg-card, #f9fafb)', borderRadius: '12px' } },
        h('h2', { style: { fontSize: '1.25rem', marginBottom: '16px' } }, tr('你可能还喜欢', 'You may also like')),
        h('div', { className: 'blog-related-grid', style: { display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(250px, 1fr))', gap: '16px' } },
          ...post.related.map(rp => h('a', {
            href: buildPath('blog-detail', { slug: rp.slug }),
            onclick: (e) => { e.preventDefault(); navigate('blog-detail', { slug: rp.slug }); },
            className: 'blog-related-card',
            style: { display: 'block', padding: '16px', background: 'white', borderRadius: '8px', border: '1px solid var(--border, #e5e7eb)', transition: 'box-shadow 0.2s' },
          },
            h('div', { style: { fontWeight: '600', fontSize: '0.95rem', marginBottom: '6px', color: 'var(--text-primary, #1f2937)' } }, rp.title),
            h('div', { style: { fontSize: '0.8rem', color: 'var(--text-secondary, #6b7280)' } },
              rp.category ? `${rp.category} · ${rp.date}` : rp.date,
            ),
          )),
        ),
      );
      el.appendChild(relSection);
    }

    // Inject structured data: site-wide nodes + BlogPosting/Article + GEO extras
    // (mirrors BlogDetail() in backend-go/internal/ssr/blog.go).
    const blogLdScripts = [
      organizationJsonLd(),
      websiteJsonLd(),
      {
        '@context': 'https://schema.org',
        '@type': 'BlogPosting',
        headline: post.title,
        description: post.description || '',
        datePublished: post.date,
        author: { '@type': 'Person', name: post.author || 'BrickPlan' },
        publisher: { '@id': `${window.location.origin}/#organization` },
        mainEntityOfPage: { '@type': 'WebPage', '@id': window.location.href },
        url: window.location.href,
      },
      {
        '@context': 'https://schema.org',
        '@type': 'Article',
        headline: post.title,
        description: post.description || '',
        datePublished: post.date,
        dateModified: post.date,
        author: { '@type': 'Person', name: post.author || 'BrickPlan' },
        publisher: { '@id': `${window.location.origin}/#organization` },
        mainEntityOfPage: { '@type': 'WebPage', '@id': window.location.href },
        url: window.location.href,
        inLanguage: tr('zh-CN', 'en'),
      },
    ];
    if (post.tags && post.tags.length) {
      blogLdScripts[3].keywords = post.tags.join(', ');
    }

    // GEO: FAQPage derived from the article's own H2/H3 sections
    const faqPairs = buildFaqFromBody(post.body || '', 6);
    if (faqPairs.length) {
      blogLdScripts.push({
        '@context': 'https://schema.org',
        '@type': 'FAQPage',
        mainEntity: faqPairs.map(x => ({
          '@type': 'Question',
          name: x.q,
          acceptedAnswer: { '@type': 'Answer', text: x.a },
        })),
      });
    }

    // GEO: tutorial-style posts expose their ordered steps as HowTo
    const howToLd = buildHowToJsonLd(post);
    if (howToLd) blogLdScripts.push(howToLd);

    injectJsonLd(blogLdScripts);
  } catch (e) {
    const el = $id('blog-detail-content');
    if (el) {
      el.innerHTML = '';
      el.appendChild(h('div', { className: 'error-block' },
        h('div', { className: 'error-icon' }, '📄'),
        h('p', {}, errorMessage(e, tr('文章不存在', 'Article not found.'))),
        h('button', { className: 'btn btn-primary btn-sm', onclick: () => navigate('blog') }, tr('返回博客列表', 'Back to the blog')),
      ));
    }
  }
}

// ═══════════════════════════════════════════
// GEO / structured data helpers
// ═══════════════════════════════════════════

// injectJsonLd replaces the page's JSON-LD blocks with the given ones.
//
// On the first render of an English page it does nothing: the SSR nodes are the
// authoritative structured data for the URL the crawler fetched, and deleting
// them was the single biggest SEO regression risk of hydrating /en/*. Once the
// user navigates in-app the SSR nodes describe the previous URL, so they are
// replaced by fresh, language-correct blocks.
function injectJsonLd(blocks) {
  if (headLocked) return;
  document.querySelectorAll('script[type="application/ld+json"]').forEach(el => el.remove());
  blocks.filter(Boolean).forEach(ld => {
    const s = document.createElement('script');
    s.type = 'application/ld+json';
    s.setAttribute('data-spa-ld', '1');
    s.textContent = JSON.stringify(ld, null, 2);
    document.head.appendChild(s);
  });
}

// organizationJsonLd / websiteJsonLd mirror siteJSONLD() in
// backend-go/internal/ssr/jsonld.go — both layers must stay in sync.
function injectSiteJsonLd() {
  injectJsonLd([organizationJsonLd(), websiteJsonLd()]);
}

function organizationJsonLd() {
  const origin = window.location.origin;
  const en = isEnglishPath();
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    '@id': `${origin}/#organization`,
    name: 'BrickPlan',
    alternateName: en ? 'Brick building blueprint community' : '积木图纸社区',
    url: origin,
    description: en
      ? 'BrickPlan is a blueprint-sharing community for brick and MOC builders, covering LEGO and Chinese brick brands such as Double Eagle, Mould King, Gobricks, SEMBO and Qiji.'
      : 'BrickPlan 是面向积木/MOC 爱好者的图纸分享社区，收录乐高及国产积木（双鹰、宇星、高砖、森宝、奇积等）的 MOC 图纸、零件知识、搭建教程与作品评测。',
    slogan: en ? 'Discover and share brick MOC blueprints' : '发现和分享积木 MOC 图纸',
    logo: { '@type': 'ImageObject', url: `${origin}/og-default.png`, width: 1200, height: 630 },
    image: `${origin}/og-default.png`,
    inLanguage: en ? 'en' : 'zh-CN',
    areaServed: { '@type': 'Country', name: 'China' },
    knowsAbout: en
      ? ['MOC blueprints', 'LEGO MOC', 'brick building tutorials', 'brick parts', 'Chinese brick brands']
      : ['积木图纸', 'MOC', '乐高 MOC', '积木搭建教程', '积木零件', '国产积木品牌'],
    contactPoint: [{
      '@type': 'ContactPoint',
      contactType: 'customer support',
      email: 'ghostjeky@hotmail.com',
      url: en ? `${origin}/en/about` : `${origin}/about`,
      availableLanguage: en ? ['en', 'zh-CN'] : ['zh-CN'],
    }],
  };
}

function websiteJsonLd() {
  const origin = window.location.origin;
  const en = isEnglishPath();
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    '@id': `${origin}/#website`,
    url: origin,
    name: 'BrickPlan',
    alternateName: en ? 'Brick building blueprint community' : '积木图纸社区',
    description: en ? 'Brick building and MOC blueprint community' : '积木/MOC 图纸分享社区',
    inLanguage: en ? 'en' : 'zh-CN',
    publisher: { '@id': `${origin}/#organization` },
    potentialAction: {
      '@type': 'SearchAction',
      target: { '@type': 'EntryPoint', urlTemplate: `${origin}${en ? '/en/explore' : '/explore'}?q={search_term_string}` },
      'query-input': 'required name=search_term_string',
    },
  };
}

// plainMdText reduces markdown syntax to readable text (Go: ssr.plainMDText).
function plainMdText(s) {
  return String(s || '')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/[*_`]+/g, '')
    .replace(/^\s*>\s*/, '')
    .replace(/\s+/g, ' ')
    .trim();
}

// mdSectionsFromBody splits markdown into H2/H3 sections with the text that
// follows each heading (Go: ssr.mdSections). `raw` keeps line breaks for
// line-oriented parsing; `text` is markdown-stripped.
function mdSectionsFromBody(body) {
  const out = [];
  let cur = null;
  let buf = [];
  const flush = () => {
    if (!cur) return;
    cur.raw = buf.join('\n');
    cur.text = plainMdText(cur.raw);
    out.push(cur);
    cur = null;
    buf = [];
  };
  String(body || '').split('\n').forEach(line => {
    const m = line.match(/^(#{2,3})[ \t]+(.+?)[ \t]*#*[ \t]*$/);
    if (m) {
      flush();
      cur = { level: m[1].length, heading: plainMdText(m[2]), text: '', raw: '' };
      return;
    }
    if (cur) buf.push(line);
  });
  flush();
  return out;
}

// isFAQHeading mirrors ssr.isFAQHeading in Go.
function isFAQHeading(heading) {
  const h = String(heading || '');
  const lower = h.toLowerCase();
  return h.includes('常见问题') || h.includes('问答') || lower.includes('faq') || lower.includes('q&a');
}

// explicitFAQ parses hand-written "**Q：…**" / "A：…" pairs out of a 常见问题
// section (Go: ssr.explicitFAQ). Curated pairs beat auto-derived questions.
function explicitFAQ(body) {
  for (const s of mdSectionsFromBody(body)) {
    if (!isFAQHeading(s.heading) || !s.raw) continue;
    const pairs = [];
    let cur = -1;
    s.raw.split('\n').forEach(line => {
      const qm = line.match(/^\s*\**\s*(?:Q|问|问题)\s*[：:]\s*\**\s*(.*)$/);
      if (qm) {
        const q = toQuestionText(plainMdText(qm[1]));
        if (!q) return;
        pairs.push({ q, a: '' });
        cur = pairs.length - 1;
        return;
      }
      if (cur < 0) return;
      let ans = '';
      const am = line.match(/^\s*\**\s*(?:A|答|答案)\s*[：:]\s*\**\s*(.*)$/);
      if (am) ans = plainMdText(am[1]);
      else if (pairs[cur].a) ans = plainMdText(line); // continuation line
      if (!ans) return;
      pairs[cur].a = pairs[cur].a ? `${pairs[cur].a} ${ans}` : ans;
    });
    const complete = pairs
      .filter(p => p.q && Array.from(p.a).length >= 8)
      .map(p => ({ q: p.q, a: clipText(p.a, 300) }));
    if (complete.length >= 2) return complete;
  }
  return [];
}

function toQuestionText(heading) {
  const s = String(heading || '')
    .replace(/^第\s*[0-9一二三四五六七八九十]+\s*[步节部分]\s*[:：.、]?\s*/, '')
    .replace(/^(?:[0-9]+|[一二三四五六七八九十]+)\s*[、.．)）:：]\s*/, '')
    .replace(/[？?。.：:]+$/, '')
    .trim();
  return s ? `${s}？` : '';
}

function clipText(s, n) {
  const r = Array.from(String(s || ''));
  return r.length > n ? `${r.slice(0, n).join('')}...` : String(s || '');
}

// buildFaqFromBody derives FAQPage Q&A pairs for a post (Go: ssr.buildFAQFromBody).
// Hand-written 常见问题 sections win; otherwise H2/H3 sections become questions.
function buildFaqFromBody(body, max) {
  const explicit = explicitFAQ(body);
  if (explicit.length) return explicit.slice(0, max);

  const sections = mdSectionsFromBody(body);
  const pick = (level) => {
    const qa = [];
    for (const s of sections) {
      if (s.level !== level || !s.heading || isFAQHeading(s.heading)) continue;
      if (!s.text) continue;
      if (Array.from(s.text).length < 12) continue;
      const q = toQuestionText(s.heading);
      if (!q) continue;
      qa.push({ q, a: clipText(s.text, 260) });
      if (qa.length >= max) break;
    }
    return qa;
  };
  let qa = pick(2);
  if (qa.length < 2) qa = pick(3);
  return qa.length >= 2 ? qa : [];
}

function isTutorialPost(post) {
  const hay = [post.category, post.title, post.slug, (post.tags || []).join(' ')].join(' ');
  return ['教程', '指南', '入门', 'tutorial', 'guide', 'how-to', 'howto'].some(k => hay.includes(k));
}

// buildHowToJsonLd mirrors howToJSONLD() in Go: H2 sections become ordered steps.
function buildHowToJsonLd(post) {
  if (!isTutorialPost(post)) return null;
  const steps = mdSectionsFromBody(post.body)
    .filter(s => s.level === 2 && s.heading && s.text && !isFAQHeading(s.heading))
    .map((s, i) => ({ '@type': 'HowToStep', position: i + 1, name: s.heading, text: clipText(s.text, 300) }));
  if (steps.length < 2) return null;
  return {
    '@context': 'https://schema.org',
    '@type': 'HowTo',
    name: post.title,
    description: post.description || '',
    inLanguage: 'zh-CN',
    step: steps,
  };
}

// ═══════════════════════════════════════════
// About page — mirrors backend-go/internal/ssr/about.go
// ═══════════════════════════════════════════
function aboutSectionsData() {
  return [
    {
      id: 'background',
      heading: '项目背景',
      paras: [
        'BrickPlan（积木图纸社区）是一个面向积木与 MOC 爱好者的图纸分享社区，主站为 brickplan.cn。',
        '玩家找图纸时最大的痛点是：图纸散落在论坛、网盘和社交平台，搜索困难、格式混乱、链接还常常失效。BrickPlan 把这些内容集中到一个可检索、可分类、带完整元信息（零件数、难度、尺寸、推荐品牌）的站点上。',
        '目前站内收录建筑、车辆、机甲、奇幻、科幻、场景六大分类的积木图纸，覆盖乐高以及双鹰、宇星、高砖、森宝、奇积等国产积木品牌，并提供零件知识、品牌对比与搭建教程类文章。',
      ],
      bullets: [],
    },
    {
      id: 'what-we-offer',
      heading: '我们提供什么',
      paras: [],
      bullets: [
        '图纸库：按分类、标签、关键词检索的 MOC 图纸，支持图片与 PDF 图纸在线预览。',
        '元信息：每份图纸标注零件数、难度、尺寸与推荐品牌，方便在动手前评估可行性。',
        '教程与评测：博客栏目持续更新 MOC 搭建教程、零件分类知识、品牌评测与选购建议。',
        '社区互动：点赞、收藏、评论与作者主页，创作者的作品可以被直接关注。',
      ],
    },
    {
      id: 'team',
      heading: '作者团队',
      paras: ['BrickPlan 由一个小型独立团队维护：'],
      bullets: [
        '内容与运营：负责图纸整理、分类标注、教程与评测撰写。',
        '工程：负责站点、微信小程序与检索体验的开发与运维。',
        '社区创作者：图纸与作品的版权归各自创作者所有，BrickPlan 仅提供展示与分享平台。',
      ],
    },
    {
      id: 'contact',
      heading: '联系方式',
      paras: [],
      bullets: [
        '邮箱：ghostjeky@hotmail.com（隐私与内容相关事务）',
        '侵权与下架：在作品页底部点击「举报」，或直接发送邮件，我们会在核实后处理。',
        '商务与合作：同样通过上述邮箱联系，标题请注明「合作」。',
      ],
    },
    {
      id: 'inclusion',
      heading: '收录说明',
      paras: [],
      bullets: [
        '收录范围：站内收录玩家自荐或公开渠道可获取的积木 MOC 图纸与原创文章，图纸版权归原作者所有。',
        '收录方式：注册用户可自行上传；非注册用户可通过邮箱推荐，我们整理后入库。',
        '更新频率：图纸库与博客持续更新，站点地图（/sitemap.xml）实时反映最新内容。',
        '展示信息：每份图纸的元信息（零件数、难度等）来自原作者提供或公开资料整理，如有出入欢迎指正。',
        '版权与下架：如果你是版权方且不希望作品被收录，请通过举报入口或邮箱联系我们，核实后会尽快下架。',
        '抓取与引用：欢迎搜索引擎与 AI 助手抓取、引用本站公开内容，引用时请保留原文链接（https://brickplan.cn）。站内提供 sitemap.xml、llms.txt 与结构化数据便于机器读取。',
      ],
    },
  ];
}

function renderAboutPage() {
  const container = $id('page-about');
  if (!container) return;
  container.innerHTML = '';

  const title = '关于我们 — BrickPlan 积木图纸社区';
  const desc = 'BrickPlan 积木图纸社区的项目背景、作者团队、联系方式与内容收录说明。';
  document.title = title;
  setMeta('og:title', title);
  setMeta('og:description', desc);
  setMeta('og:url', `${window.location.origin}/about`);
  setMeta('og:type', 'website');
  setMetaName('description', desc);

  const body = h('div', { className: 'markdown-body' },
    h('h1', {}, '关于 BrickPlan'),
    h('p', {}, 'BrickPlan（积木图纸社区）是面向积木/MOC 爱好者的图纸分享社区，本文介绍项目背景、团队、联系方式与内容收录说明。'),
    ...aboutSectionsData().map(s => h('section', { id: s.id },
      h('h2', {}, s.heading),
      ...s.paras.map(p => h('p', {}, p)),
      s.bullets.length
        ? h('ul', {}, ...s.bullets.map(li => h('li', {}, li)))
        : null,
    )),
    h('p', {},
      h('a', { href: '/blog', onclick: (e) => { e.preventDefault(); navigate('blog'); } }, '阅读积木搭建教程与评测'),
      ' · ',
      h('a', { href: '/explore', onclick: (e) => { e.preventDefault(); navigate('explore'); } }, '浏览全部图纸'),
      ' · ',
      h('a', { href: '/faq', onclick: (e) => { e.preventDefault(); navigate('faq'); } }, '常见问题'),
    ),
  );

  container.appendChild(h('div', { className: 'main' },
    h('div', { className: 'blog-detail' }, body),
  ));

  injectJsonLd([
    organizationJsonLd(),
    websiteJsonLd(),
    {
      '@context': 'https://schema.org',
      '@type': 'AboutPage',
      '@id': `${window.location.origin}/about#aboutpage`,
      url: `${window.location.origin}/about`,
      name: '关于 BrickPlan 积木图纸社区',
      description: desc,
      inLanguage: 'zh-CN',
      isPartOf: { '@type': 'WebSite', '@id': `${window.location.origin}/#website` },
      about: { '@id': `${window.location.origin}/#organization` },
      mainEntity: { '@id': `${window.location.origin}/#organization` },
    },
  ]);
}

function render() {
  // about/faq/privacy in English are final SSR documents (see isSsrOnlyPath);
  // every other page — CN or EN — is rendered by the SPA.
  if (isSsrOnlyPath(location.pathname)) return;
  const app = document.getElementById('app');

  // Ensure page containers exist
  const pages = ['home', 'explore', 'detail', 'upload', 'user', 'admin', 'edit', 'privacy', 'notifications', 'faq', 'blog', 'blog-detail', 'about'];
  if (!app.querySelector('#page-home')) {
    app.innerHTML = '';
    renderNavbarIntoDOM();
    pages.forEach(p => {
      const div = document.createElement('div');
      div.id = `page-${p}`;
      div.className = 'page';
      app.appendChild(div);
    });
  } else {
    renderNavbarIntoDOM();
  }

  // Activate correct page
  pages.forEach(p => {
    const el = document.getElementById(`page-${p}`);
    if (el) el.className = `page${state.page === p ? ' active' : ''}`;
  });

  // After the first in-app navigation on an English page, restore the
  // site-wide entities in the page language. Page renderers that have their own
  // structured data (detail/blog) replace these a moment later.
  if (!headLocked && isEnglishPath()) injectSiteJsonLd();

  // Render page content
  switch (state.page) {
    case 'home': resetMeta(); renderHome(); break;
    case 'explore': resetMeta(); renderExplore(); break;
    case 'detail': renderDetail(); break;
    case 'faq': resetMeta(); renderFAQ(); break;
    case 'upload': resetMeta(); renderUpload(); break;
    case 'user':
      resetMeta();
      if (!state.userProfile.id && state.user?.id) {
        navigate('user', { id: state.user.id });
        break;
      }
      renderUserProfile(); break;
    case 'admin':
      resetMeta();
      if (!state.userLoaded) {
        const container = $id('page-admin');
        if (container) container.innerHTML = '<div class="main"><div class="loading"><div class="spinner"></div></div></div>';
        break;
      }
      if (!state.user?.is_admin) {
        renderErrorPage(403);
        break;
      }
      renderAdminPage(); break;
    case 'edit':
      resetMeta();
      if (!state.editId) { navigate('home'); break; }
      loadEditPage(state.editId);
      break;
    case 'privacy':
      resetMeta();
      renderPrivacyPage();
      break;
    case 'about':
      resetMeta();
      renderAboutPage();
      break;
    case 'notifications':
      resetMeta();
      renderNotifications();
      break;
    case 'blog':
      resetMeta();
      renderBlogList();
      break;
    case 'blog-detail':
      renderBlogDetail();
      break;
    default: resetMeta(); renderErrorPage(404); break;
  }

  if (state.user?.id && state.page !== 'notifications') {
    refreshNotificationBadge();
  }
}

// ═══════════════════════════════════════════
// Boot — trigger initial render from current URL
// ═══════════════════════════════════════════
refreshCurrentUser();
// Redirect legacy '#/...' URLs to real paths, then render.
if (location.hash && location.hash.startsWith('#/')) {
  const { page, params } = parseLegacyHash();
  history.replaceState({ page, params }, '', buildPath(page, params));
}
onRouteChange();
