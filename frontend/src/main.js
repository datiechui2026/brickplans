import * as api from './api.js';
import './styles/main.css';

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
    1: { stars: '⭐', label: '简单', color: '#22c55e', bg: 'rgba(34,197,94,.75)' },
    2: { stars: '⭐⭐', label: '初级', color: '#3b82f6', bg: 'rgba(59,130,246,.75)' },
    3: { stars: '⭐⭐⭐', label: '中等', color: '#f59e0b', bg: 'rgba(245,158,11,.75)' },
    4: { stars: '⭐⭐⭐⭐', label: '困难', color: '#ef4444', bg: 'rgba(239,68,68,.75)' },
    5: { stars: '⭐⭐⭐⭐⭐', label: '专家', color: '#a855f7', bg: 'rgba(168,85,247,.75)' },
  };
  return map[n] || map[3];
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
  document.title = 'BrickPlans — 积木图纸分享';
  setMeta('og:title', 'BrickPlans — 积木图纸分享社区');
  setMeta('og:description', '分享你的乐高MOC创意，探索海量积木图纸');
  setMeta('og:image', '/og-default.png');
  setMeta('og:url', '/');
  setMeta('og:type', 'website');
  setMetaName('description', 'BrickPlans 积木图纸分享社区，发现和分享乐高MOC创意作品');
  // Remove any injected JSON-LD
  document.querySelectorAll('script[type="application/ld+json"]').forEach(el => el.remove());
}

// ═══════════════════════════════════════════
// Router — uses hashchange (native back/forward support)
// ═══════════════════════════════════════════
function buildHash(page, params = {}) {
  let hash = `#/${page}`;
  if (page === 'user' && params.id) {
    hash += `/${encodeURIComponent(params.id)}`;
  } else if (page === 'user' && params.username) {
    hash += `/${encodeURIComponent(params.username)}`;
  }
  const qs = [];
  if (params.id && page !== 'user') qs.push(`id=${encodeURIComponent(params.id)}`);
  if (params.q) qs.push(`q=${encodeURIComponent(params.q)}`);
  if (params.category) qs.push(`category=${encodeURIComponent(params.category)}`);
  if (params.tag) qs.push(`tag=${encodeURIComponent(params.tag)}`);
  if (params.sort) qs.push(`sort=${encodeURIComponent(params.sort)}`);
  if (params.page) qs.push(`page=${params.page}`);
  return qs.length ? `${hash}?${qs.join('&')}` : hash;
}

function parseHash() {
  const raw = location.hash.slice(2) || 'home'; // remove '#/'
  const [path, query] = raw.split('?');
  const params = {};
  if (query) {
    new URLSearchParams(query).forEach((v, k) => { params[k] = v; });
  }
  // Parse user/{id} paths. Legacy user/{username} URLs are still accepted by the API.
  const parts = path.split('/');
  if (parts[0] === 'user' && parts[1]) {
    return { page: 'user', params: { ...params, id: decodeURIComponent(parts[1]) } };
  }
  return { page: path, params };
}

function applyState(page, params = {}) {
  state.page = page;
  if (params.id) state.detailId = params.id;
  if (page === 'edit' && params.id) state.editId = params.id;
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
  location.hash = buildHash(page, params);
  // hashchange handler fires automatically → calls applyState + render
}

// Handle hash changes (initial load + browser back/forward)
function onHashChange() {
  const { page, params } = parseHash();
  applyState(page, params);
  render();
}
window.addEventListener('hashchange', onHashChange);

async function refreshCurrentUser() {
  if (!api.isLoggedIn()) {
    state.userLoaded = true;
    return;
  }
  try {
    const user = await api.getMe();
    state.user = user;
    const auth = JSON.parse(localStorage.getItem('bp_auth') || '{}');
    localStorage.setItem('bp_auth', JSON.stringify({ ...auth, user }));
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
    } else el.setAttribute(k, v);
  }
  for (const child of children.flat()) {
    if (child == null) continue;
    el.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return el;
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
    href: '#/home',
    onclick: (e) => { e.preventDefault(); navigate('home'); },
  }, '🧱 BrickPlans');

  const actions = [];

  // Upload button — shows login modal if not logged in
  if (state.user?.id) {
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => navigate('notifications') },
      '🔔 通知',
      state.notificationUnreadCount > 0 ? h('span', { className: 'nav-dot' }, String(Math.min(state.notificationUnreadCount, 99))) : null,
    ));
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => navigate('upload') }, '📤 上传图纸'));
  } else {
    actions.push(h('button', { className: 'btn btn-ghost', onclick: () => showModal('login') }, '📤 上传图纸'));
  }

  // User avatar / name
  if (state.user?.id) {
    const avatarUrl = state.user.avatar_url || `https://api.dicebear.com/7.x/thumbs/svg?seed=${encodeURIComponent(state.user.username || 'user')}`;
    actions.push(h('img', {
      className: 'user-avatar',
      src: avatarUrl,
      alt: state.user.username || '用户',
      title: '个人主页',
      onclick: () => navigate('user', { id: state.user.id }),
    }));
  } else {
    actions.push(h('button', { className: 'btn btn-primary btn-sm', onclick: () => showModal('login') }, '登录'));
    actions.push(h('button', { className: 'btn btn-ghost btn-sm', onclick: () => showModal('register') }, '注册'));
  }

  return h('nav', { className: 'navbar' },
    brandEl,
    h('div', { className: 'nav-actions' }, ...actions),
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
    const auth = JSON.parse(localStorage.getItem('bp_auth') || '{}');
    if (auth.user) {
      auth.user.avatar_url = avatarUrl;
      localStorage.setItem('bp_auth', JSON.stringify(auth));
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
      h('h2', {}, '🚩 举报内容'),
      h('p', { style: { color: 'var(--text-sec)', fontWeight: 600, marginBottom: '16px', fontSize: '0.9rem' } }, '请选择举报原因：'),
      h('div', { id: 'report-error' }),
      h('div', { className: 'form-group' },
        ...['inappropriate', 'copyright', 'incomplete', 'spam', 'other'].map(r =>
          h('label', { style: { display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 0', cursor: 'pointer', fontWeight: 600 } },
            h('input', { type: 'radio', name: 'report-reason', value: r }),
            { inappropriate: '内容不当', copyright: '版权问题', incomplete: '图纸不完整', spam: '垃圾广告', other: '其他' }[r],
          )
        ),
        h('input', { type: 'radio', name: 'report-reason', value: 'other', style: { display: 'none' }, id: 'report-reason-other-hidden' }),
      ),
      h('div', { className: 'form-group' },
        h('label', {}, '补充说明（可选）'),
        h('textarea', {
          id: 'report-detail',
          placeholder: '请补充更多细节...',
          rows: 3,
          style: { width: '100%', padding: '10px', borderRadius: '8px', border: '2px solid #e5e5e5', fontFamily: 'inherit', fontSize: '0.9rem', resize: 'vertical', background: '#f5f5f7', boxSizing: 'border-box' },
        }),
      ),
      h('button', {
        className: 'btn btn-primary btn-lg',
        style: { width: '100%', marginTop: '8px' },
        onclick: handleReportSubmit,
      }, '提交举报'),
    ),
  );
  document.body.appendChild(overlay);
}

async function handleReportSubmit() {
  const errEl = $id('report-error');
  const reasonRadio = document.querySelector('input[name="report-reason"]:checked');
  if (!reasonRadio) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">请选择举报原因</div>';
    return;
  }
  const reason = reasonRadio.value;
  const detail = $id('report-detail')?.value.trim() || undefined;

  try {
    await api.createReport(_reportTargetId, reason, detail);
    hideModal();
    showToast('举报已提交，感谢您的反馈！', 'success');
    // Update the report button to "已举报"
    const reportStatusEl = $id('report-status');
    if (reportStatusEl) {
      reportStatusEl.innerHTML = '';
      reportStatusEl.appendChild(
        h('span', { style: { color: '#ef4444', fontWeight: 700, fontSize: '0.9rem' } }, '✅ 已举报')
      );
    }
  } catch (e) {
    if (e.message && e.message.includes('already reported')) {
      hideModal();
      showToast('您已经举报过该内容', 'error');
      const reportStatusEl = $id('report-status');
      if (reportStatusEl) {
        reportStatusEl.innerHTML = '';
        reportStatusEl.appendChild(
          h('span', { style: { color: '#ef4444', fontWeight: 700, fontSize: '0.9rem' } }, '✅ 已举报')
        );
      }
    } else {
      if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message || '举报提交失败'}</div>`;
    }
  }
}

function renderModal() {
  hideModal();

  const isLogin = modalMode === 'login';
  const overlay = h('div', { className: 'modal-overlay', onclick: (e) => { if (e.target === overlay) hideModal(); } },
    h('div', { className: 'modal' },
      h('button', { className: 'close', onclick: hideModal, style: { float: 'right' } }, '✕'),
      h('h2', {}, isLogin ? '🔑 登录 BrickPlans' : '🧱 注册 BrickPlans'),
      h('div', { id: 'modal-error' }),
      h('div', { className: 'form-group' },
        isLogin ? null : h('label', { className: 'form-label' }, '用户名'),
        ...(isLogin ? [] : [h('input', { type: 'text', id: 'modal-username', className: 'form-input', placeholder: '你的用户名' })]),
      ),
      h('div', { className: 'form-group' },
        h('label', { className: 'form-label' }, '邮箱'),
        h('input', { type: 'email', id: 'modal-email', className: 'form-input', placeholder: 'your@email.com' }),
      ),
      h('div', { className: 'form-group' },
        h('label', { className: 'form-label' }, '密码'),
        h('input', { type: 'password', id: 'modal-password', className: 'form-input', placeholder: '至少6位密码' }),
      ),
      ...(isLogin ? [] : [
        h('div', { style: { marginTop: '12px', display: 'flex', alignItems: 'center', gap: '8px', fontSize: '0.85rem' } },
          h('input', { type: 'checkbox', id: 'modal-privacy', checked: true }),
          h('span', {}, '我已阅读并同意 ', h('a', { href: '#/privacy', style: { color: 'var(--accent)', fontWeight: 700 } }, '隐私策略')),
        ),
      ]),
      h('div', { style: { display: 'flex', gap: '12px', marginTop: '20px' } },
        h('button', {
          className: `btn ${isLogin ? 'btn-primary' : 'btn-primary'} btn-lg`,
          style: { flex: 1 },
          onclick: isLogin ? handleLogin : handleRegister,
        }, isLogin ? '登录' : '注册'),
      ),
      h('div', {
        style: { textAlign: 'center', marginTop: '16px', fontSize: '0.85rem', fontWeight: 600 },
      },
        isLogin ? '还没有账号？' : '已有账号？',
        h('button', {
          style: { background: 'none', border: 'none', color: 'var(--accent)', fontWeight: 700, cursor: 'pointer', fontSize: '0.85rem', fontFamily: 'inherit', textDecoration: 'underline' },
          onclick: () => { modalMode = isLogin ? 'register' : 'login'; renderModal(); },
        }, isLogin ? '去注册' : '去登录'),
      ),
    ),
  );
  document.body.appendChild(overlay);
}

async function handleLogin() {
  const email = $id('modal-email')?.value.trim();
  const password = $id('modal-password')?.value.trim();
  const errEl = $id('modal-error');
  if (!email || !password) { errEl.innerHTML = '<div class="msg msg-error">请填写邮箱和密码</div>'; return; }
  try {
    const data = await api.login(email, password);
    state.user = data.user;
    hideModal();
    render();
  } catch (e) {
    errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
  }
}

async function handleRegister() {
  const username = $id('modal-username')?.value.trim();
  const email = $id('modal-email')?.value.trim();
  const password = $id('modal-password')?.value.trim();
  const errEl = $id('modal-error');
  if (!username || !email || !password) { errEl.innerHTML = '<div class="msg msg-error">请填写所有字段</div>'; return; }
  if (password.length < 6) { errEl.innerHTML = '<div class="msg msg-error">密码至少6位</div>'; return; }
  const privacyAccepted = $id('modal-privacy')?.checked;
  if (!privacyAccepted) { errEl.innerHTML = '<div class="msg msg-error">请先阅读并同意隐私策略</div>'; return; }
  try {
    const data = await api.register(username, email, password);
    state.user = data.user;
    hideModal();
    render();
  } catch (e) {
    errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
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
            '您上传的作品版权归您所有，BrickPlans 仅获得展示权限。',
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
            ['邮箱：privacy@brickplans.com',
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
    404: '你要找的图纸可能被拆掉了...',
    500: '服务器出了点问题，积木塔倒了！',
  };

  container.innerHTML = '';
  const page = document.createElement('div');
  page.className = 'error-page';
  page.innerHTML = `
    <div class="error-icon">${icons[code] || '❓'}</div>
    <h1>${titles[code] || code}</h1>
    <p>${message || messages[code] || '未知错误'}</p>
    ${detail ? `<p class="error-detail">${detail}</p>` : ''}
    <button class="btn btn-primary" onclick="location.href='/'">🧱 返回首页</button>
  `;
  container.appendChild(page);
}

// ── Home ──
function renderHome() {
  const container = $id('page-home');
  container.innerHTML = '';

  // Hero section
  container.appendChild(
    h('section', { className: 'hero' },
      h('h1', {}, '🧱 BrickPlans'),
      h('p', {}, '发现和分享乐高MOC创意的积木图纸社区'),
    ),
  );

  // Main content wrapper
  const mainWrap = h('div', { className: 'main' });
  container.appendChild(mainWrap);

  // Stats bar — dynamic from API, fallback to placeholders
  const statsBar = h('div', { id: 'home-stats', className: 'stats-bar' },
    ...[
      ['...', '图纸作品'],
      ['...', '创作者'],
      ['...', '总浏览'],
      ['...', '总收藏'],
    ].map(([num, label]) =>
      h('div', { className: 'stat-item' },
        h('div', { className: 'stat-value' }, num),
        h('div', { className: 'stat-label' }, label),
      ),
    ),
  );
  mainWrap.appendChild(statsBar);

  // Fetch real stats
  api.getStats().then(s => {
    const el = $id('home-stats');
    if (!el) return;
    const fmt = (n) => n >= 10000 ? (n / 1000).toFixed(1) + 'k' : n >= 1000 ? (n / 1000).toFixed(1) + 'k' : String(n);
    el.innerHTML = '';
    const data = [
      [fmt(s.total_blueprints), '图纸作品'],
      [fmt(s.total_users), '创作者'],
      [fmt(s.total_views || 0), '总浏览'],
      [fmt(s.total_favorites), '总收藏'],
    ];
    data.forEach(([num, label]) => {
      el.appendChild(h('div', { className: 'stat-item' },
        h('div', { className: 'stat-value' }, num),
        h('div', { className: 'stat-label' }, label),
      ));
    });
  }).catch(() => {});

  // Category pills
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
        }, `${emojis[cat]} ${cat}`),
      ),
    ),
  );

  // Section header + card grid
  mainWrap.appendChild(
    h('div', { className: 'section-header' },
      h('h2', { className: 'section-title' }, '🔥 热门推荐'),
      h('button', { className: 'btn btn-ghost', onclick: () => navigate('explore', { sort: 'popular' }) }, '查看全部 →'),
    ),
  );

  const featuredGrid = h('div', { id: 'home-featured', className: 'card-grid' },
    h('div', { className: 'loading', style: { gridColumn: '1/-1' } }, h('div', { className: 'spinner' })),
  );
  mainWrap.appendChild(featuredGrid);

  // Load-more button container
  mainWrap.appendChild(h('div', { id: 'home-load-more', className: 'load-more' }));

  // Fetch featured — 8 items, popular
  const loadFeatured = () => {
    api.listBlueprints({ size: 8, sort: 'popular' }).then(data => {
      const grid = $id('home-featured');
      if (!grid) return;
      grid.innerHTML = '';
      if (!data?.items?.length) {
        grid.innerHTML = '<div class="empty" style="grid-column:1/-1"><div class="empty-icon">📭</div><p>还没有图纸，快来上传第一个吧！</p></div>';
        return;
      }
      data.items.forEach(bp => { grid.appendChild(renderBlueprintCard(bp)); });
      // Show load-more if there are more pages
      const totalPages = Math.ceil(data.total / 8);
      const moreEl = $id('home-load-more');
      if (moreEl && totalPages > 1) {
        moreEl.innerHTML = '';
        moreEl.appendChild(h('button', {
          className: 'btn btn-outline',
          onclick: () => navigate('explore', { sort: 'popular' }),
        }, '加载更多 →'));
      }
    }).catch(() => {
      const grid = $id('home-featured');
      if (grid) grid.innerHTML = '<div class="error-block" style="grid-column:1/-1"><div class="error-icon">🔌</div><p>加载失败，请检查后端服务</p><button class="btn btn-primary btn-sm" onclick="window._retryHome?.()">🔄 重试</button></div>';
      window._retryHome = () => {
        const g = $id('home-featured');
        if (g) { g.innerHTML = '<div class="loading" style="grid-column:1/-1"><div class="spinner"></div></div>'; }
        api.listBlueprints({ size: 8, sort: 'popular' }).then(data => {
          const g2 = $id('home-featured');
          if (!g2) return;
          g2.innerHTML = '';
          data.items.forEach(bp => { g2.appendChild(renderBlueprintCard(bp)); });
          const totalPages = Math.ceil(data.total / 8);
          const moreEl = $id('home-load-more');
          if (moreEl && totalPages > 1) {
            moreEl.innerHTML = '';
            moreEl.appendChild(h('button', {
              className: 'btn btn-outline',
              onclick: () => navigate('explore', { sort: 'popular' }),
            }, '加载更多 →'));
          }
        }).catch(() => {
          const g3 = $id('home-featured');
          if (g3) g3.innerHTML = '<div class="error-block" style="grid-column:1/-1"><div class="error-icon">🔌</div><p>加载失败</p><button class="btn btn-primary btn-sm" onclick="window._retryHome?.()">🔄 重试</button></div>';
        });
      };
      showToast('加载热门作品失败', 'error');
    });
  };
  loadFeatured();

  // Footer
  mainWrap.appendChild(
    h('footer', { className: 'footer' },
      h('div', { style: { fontWeight: 800, fontSize: '1.3rem', marginBottom: '4px' } }, 'BrickPlans'),
      h('div', {}, '© 2026 BrickPlans 积木图纸分享社区',
        h('span', {}, ' · '),
        h('a', { href: '#/privacy', style: { color: 'var(--text-sec)', textDecoration: 'underline' } }, '隐私策略'),
      ),
    ),
  );
}


// ── Explore ──
function renderExplore() {
  const container = $id('page-explore');
  container.innerHTML = '';

  container.appendChild(
    h('div', { className: 'main' },
      // Page header
      h('div', { className: 'page-header' },
        h('h1', {}, '🔍 发现图纸'),
        h('p', {}, '浏览社区中的积木创作，发现精彩作品'),
      ),

      // Filter bar
      h('div', { className: 'filter-bar' },
        h('div', { className: 'filter-group' },
          h('input', { type: 'text', id: 'explore-search', className: 'form-input', value: state.explore.q, placeholder: '🔍 搜索关键词...',
            onkeydown: (e) => { if (e.key === 'Enter') { state.explore.q = e.target.value; state.explore.category = $id('explore-category')?.value || ''; state.explore.sort = $id('explore-sort')?.value || 'new'; state.explore.page = 1; loadExploreResults(); } },
          }),
        ),
        h('div', { className: 'filter-group' },
          h('select', { id: 'explore-category', className: 'form-select' },
            ...[
              ['', '全部分类'], ['建筑', '建筑'], ['车辆', '车辆'], ['机甲', '机甲'],
              ['奇幻', '奇幻'], ['科幻', '科幻'], ['场景', '场景'],
            ].map(([v, t]) => h('option', { value: v, ...(state.explore.category === v ? { selected: '' } : {}) }, t)),
          ),
        ),
        h('div', { className: 'filter-group' },
          h('select', { id: 'explore-sort', className: 'form-select' },
            h('option', { value: 'new', ...(state.explore.sort === 'new' ? { selected: '' } : {}) }, '🆕 最新'),
            h('option', { value: 'popular', ...(state.explore.sort === 'popular' ? { selected: '' } : {}) }, '🔥 最热门'),
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
        }, '筛选'),
      ),

      // Hot tags
      h('div', { id: 'explore-tags', className: 'tags-section' },
        h('span', { className: 'tags-title' }, '🏷️ 热门标签'),
        h('div', { id: 'explore-tags-list', style: { display: 'flex', gap: '6px', flexWrap: 'wrap' } },
          h('span', { style: { color: 'var(--text-sec)', fontSize: '0.8rem' } }, '加载中...'),
        ),
      ),

      // Result count
      h('div', { className: 'result-info' },
        h('span', { id: 'explore-count', className: 'result-count' }, '加载中...'),
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

    if (countEl) countEl.textContent = `共找到 ${data.total} 个作品`;

    if (resultsEl) {
      resultsEl.innerHTML = '';
      if (!data.items?.length) {
        resultsEl.innerHTML = '<div class="empty" style="grid-column:1/-1"><div class="empty-icon">📭</div><p>没有找到匹配的图纸</p></div>';
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
      }, '‹ 上一页'));

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
      }, '下一页 ›'));
    }
  } catch (e) {
    if (resultsEl) resultsEl.innerHTML = `<div class="error-block" style="grid-column:1/-1">
      <div class="error-icon">🔌</div>
      <p>加载失败：${e.message}</p>
      <button class="btn btn-primary btn-sm retry-btn" onclick="window._retryExplore?.()">🔄 重试</button>
    </div>`;
    window._retryExplore = loadExploreResults;
    if (countEl) countEl.textContent = '';
    showToast('加载图纸列表失败，请检查网络连接', 'error');
  }
}

async function loadExploreTags() {
  try {
    const tags = await api.listAllTags();
    const tagsListEl = $id('explore-tags-list');
    if (!tagsListEl) return;
    tagsListEl.innerHTML = '';
    if (!tags || !tags.length) {
      tagsListEl.appendChild(h('span', { style: { color: 'var(--text-sec)', fontSize: '0.8rem' } }, '暂无标签'));
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
    if (tagsListEl) tagsListEl.innerHTML = '<span style="color:var(--text-sec);font-size:0.8rem">加载标签失败</span>';
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
      h('a', { className: 'back-link', href: '#/home', onclick: (e) => { e.preventDefault(); history.back(); } }, '← 返回列表'),
      h('div', { id: 'detail-content', className: 'loading' },
        h('div', { className: 'spinner' }),
        h('p', { style: { marginTop: '12px' } }, '加载图纸详情...'),
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
  }
}

async function loadDetail(id) {
  const contentEl = $id('detail-content');
  if (!contentEl) return;

  try {
    const bp = await api.getBlueprint(id);

    // ── Dynamic SEO: update title, OG tags, and inject JSON-LD ──
    document.title = `${bp.title} — BrickPlans`;
    setMeta('og:title', bp.title);
    setMeta('og:description', bp.description || 'BrickPlans 积木图纸分享社区');
    setMeta('og:image', getCoverImage(bp.images) || '/og-default.png');
    setMeta('og:url', `${window.location.origin}/#/detail?id=${bp.id}`);
    setMeta('og:type', 'article');
    setMetaName('description', (bp.description || 'BrickPlans 积木图纸').substring(0, 160));

    // Inject JSON-LD structured data
    document.querySelectorAll('script[type="application/ld+json"]').forEach(el => el.remove());
    const ldJson = {
      '@context': 'https://schema.org',
      '@type': 'CreativeWork',
      name: bp.title,
      description: bp.description || '',
      image: getCoverImage(bp.images) || '',
      author: bp.author ? { '@type': 'Person', name: bp.author.username } : undefined,
      datePublished: bp.created_at || undefined,
    };
    const ldScript = document.createElement('script');
    ldScript.type = 'application/ld+json';
    ldScript.textContent = JSON.stringify(ldJson, null, 2);
    document.head.appendChild(ldScript);

    const stars = [];
    for (let i = 1; i <= 5; i++) {
      stars.push(h('span', { className: `dot${i <= (bp.difficulty || 0) ? ' filled' : ' empty'}` }));
    }

    const tags = (bp.tags || []).map(t => h('span', {
      className: 'tag',
      style: { marginRight: '6px', cursor: 'pointer' },
      onclick: () => navigate('explore', { tag: t }),
      title: `查看所有 "${t}" 标签的图纸`,
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
                mainContentEl = h('iframe', {
                  src: currentItem.url + '#toolbar=0&navpanes=0',
                  style: { width: '100%', height: '600px', border: 'none', borderRadius: '8px' },
                  title: bp.title,
                });
              } else {
                mainContentEl = h('img', {
                  src: currentItem.url,
                  alt: bp.title,
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
                  galleryMain.appendChild(h('iframe', {
                    src: item.url + '#toolbar=0&navpanes=0',
                    style: { width: '100%', height: '600px', border: 'none', borderRadius: '8px' },
                    title: bp.title,
                  }));
                } else {
                  const newImg = h('img', {
                    src: item.url,
                    alt: bp.title,
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
              h('button', { className: 'btn btn-ghost btn-sm', onclick: (e) => { e.preventDefault(); navigate('edit', { id: bp.id }); } }, '✏️ 编辑'),
            ) : null,
            bp.author ? h('div', { className: 'author-row',
              onclick: () => navigate('user', { id: bp.author.id }),
            },
              h('img', { src: bp.author.avatar_url || `https://api.dicebear.com/7.x/thumbs/svg?seed=${encodeURIComponent(bp.author.username || 'anon')}`, alt: '' }),
              h('div', {},
                h('div', { className: 'author-name' }, bp.author.username),
                bp.created_at ? h('div', { className: 'author-date' }, `${new Date(bp.created_at).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long' })} 发布`) : null,
              ),
            ) : null,
            h('div', { className: 'info-meta' },
              ...[
                h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, '难度'),
                  h('span', { className: 'meta-value', style: { color: diff.color } }, `${diff.stars} ${diff.label}`),
                ),
                bp.piece_count ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, '零件数'),
                  h('span', { className: 'meta-value' }, String(bp.piece_count)),
                ) : null,
                bp.dimensions ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, '尺寸'),
                  h('span', { className: 'meta-value' }, bp.dimensions),
                ) : null,
                bp.category ? h('div', { className: 'meta-item' },
                  h('span', { className: 'meta-label' }, '分类'),
                  h('span', { className: 'meta-value' }, bp.category),
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
                  } catch (e) { showToast(e.message, 'error'); }
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
              `发布于 ${new Date(bp.created_at).toLocaleDateString('zh-CN')}`,
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
                h('td', {}, '总计'),
                h('td', { style: { textAlign: 'center', fontWeight: 800 } }, String(pl.total)),
                h('td', {}),
              ) : null;

              return h('div', { className: 'parts-section' },
                h('h3', {}, `🧩 零件清单 (共 ${pl.parts.length} 种)`),
                h('table', { className: 'parts-table' },
                  h('thead', {},
                    h('tr', {},
                      h('th', {}, '零件名'),
                      h('th', { style: { textAlign: 'center' } }, '数量'),
                      h('th', {}, '颜色'),
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
          // ── Description inside sidebar ──
          ...(bp.description ? [
            h('div', { className: 'desc-card' },
              h('h3', {}, '📝 描述'),
              h('div', {}, bp.description),
            ),
          ] : []),
          // ── Tags inside sidebar ──
          ...(tags.length ? [
            h('div', { style: { marginBottom: '16px' } },
              h('h3', { style: { fontWeight: 700, marginBottom: '8px' } }, '🏷️ 标签'),
              h('div', {}, ...tags),
            ),
          ] : []),
          // ── Comments inside sidebar ──
          h('div', { className: 'comments-card' },
            h('h3', {}, '💬 评论'),
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
        h('div', { className: 'comment-author' }, c.user?.username || '匿名'),
        c.created_at ? h('div', { className: 'comment-time' },
          new Date(c.created_at).toLocaleDateString('zh-CN'),
        ) : null,
      ),
      h('div', { className: 'comment-text' }, c.content),
      state.user?.id ? h('button', {
        className: 'comment-reply-btn',
        onclick: () => showReplyBox(c.id, c.user?.username || '匿名'),
      }, '回复') : null,
    );

    const showReplyBox = (parentId, username) => {
      const oldBox = document.querySelector('.reply-input-row');
      if (oldBox) oldBox.remove();
      const replyBox = h('div', { className: 'reply-input-row' },
        h('input', {
          type: 'text',
          className: 'comment-input',
          placeholder: `回复 ${username}...`,
          onkeydown: async (e) => {
            if (e.key === 'Enter') {
              await submitReply(parentId, e.target);
            }
          },
        }),
        h('button', { className: 'btn btn-primary btn-sm', onclick: async (e) => submitReply(parentId, e.target.previousElementSibling) }, '发送'),
        h('button', { className: 'btn btn-ghost btn-sm', onclick: () => replyBox.remove() }, '取消'),
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
        showToast('回复已发布', 'success');
      } catch (e) {
        alert('回复失败: ' + (e.message || '请稍后重试'));
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
              h('p', {}, '暂无评论，抢个沙发吧！'),
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
            placeholder: '写下你的评论...',
          }),
          h('button', {
            className: 'btn btn-primary btn-sm',
            onclick: async () => {
              const input = $id('comment-input');
              const content = input.value.trim();
              if (!content) return;
              const btn = input.nextElementSibling;
              btn.disabled = true;
              btn.textContent = '发表中...';
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
                alert('发表失败: ' + (e.message || '请稍后重试'));
              } finally {
                btn.disabled = false;
                btn.textContent = '🚀 发表评论';
              }
            },
          }, '🚀 发表评论'),
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
          }, '🔑 登录后发表评论'),
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
          }, '⚠ 举报不合适的内容'),
        ),
      );
    }
  } catch (e) {
    contentEl.innerHTML = `<div class="error-block">
      <div class="error-icon">😵</div>
      <p>${e.message}</p>
      <button class="btn btn-primary btn-sm retry-btn" onclick="window._retryDetail?.()">🔄 重试</button>
    </div>`;
    window._retryDetail = () => loadDetail(id);
    showToast('加载图纸详情失败', 'error');
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
          h('h1', {}, '🔔 站内通知'),
          h('p', {}, '登录后查看评论、回复、点赞和收藏提醒'),
        ),
        h('div', { className: 'empty', style: { padding: '32px' } },
          h('div', { className: 'empty-icon' }, '🔐'),
          h('p', {}, '请先登录后查看通知'),
          h('button', { className: 'btn btn-primary', onclick: () => showModal('login') }, '去登录'),
        ),
      ),
    );
    return;
  }
  container.appendChild(
    h('div', { className: 'main', style: { maxWidth: '760px' } },
      h('div', { className: 'page-header' },
        h('h1', {}, '🔔 站内通知'),
        h('p', {}, '评论、回复、点赞和收藏都会在这里提醒你'),
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
        h('p', {}, '暂无通知'),
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
    if (listEl) listEl.innerHTML = `<div class="error-block"><p>加载通知失败：${e.message}</p></div>`;
  }
}

function renderNotificationItem(item) {
  const actor = item.actor?.username || '有人';
  const title = item.payload?.blueprint_title || '你的作品';
  const messageMap = {
    comment: `${actor} 评论了《${title}》`,
    comment_reply: `${actor} 回复了你的评论`,
    like: `${actor} 点赞了《${title}》`,
    favorite: `${actor} 收藏了《${title}》`,
  };
  const message = messageMap[item.type] || `${actor} 有新的互动`;
  const excerpt = item.payload?.comment_excerpt;
  return h('div', { className: `notification-item${item.is_read ? '' : ' unread'}` },
    h('div', { className: 'notification-main' },
      h('div', { className: 'notification-title' }, message),
      excerpt ? h('div', { className: 'notification-excerpt' }, excerpt) : null,
      h('div', { className: 'notification-time' }, item.created_at ? new Date(item.created_at).toLocaleString('zh-CN') : ''),
    ),
    item.blueprint_id ? h('button', {
      className: 'btn btn-ghost btn-sm',
      onclick: () => navigate('detail', { id: item.blueprint_id }),
    }, '查看') : null,
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

  const categories = ['建筑', '车辆', '机甲', '奇幻', '科幻', '场景'];

  container.appendChild(
    h('div', { className: 'main' },
      h('div', { className: 'form-card' },
        h('h1', {}, '📤 上传图纸'),
        h('p', { className: 'subtitle' }, '分享你的积木创作'),
        h('div', { id: 'upload-error' }),
        h('div', { id: 'upload-success' }),

        // Title
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '标题 *'),
          h('input', { type: 'text', id: 'upload-title', className: 'form-input', placeholder: '给你的作品起个名字' }),
        ),

        // Files upload
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '图片 / PDF'),
          h('div', {
            id: 'upload-images-area',
            className: 'upload-area',
            onclick: () => document.getElementById('upload-file-input')?.click(),
          },
            h('div', { className: 'icon' }, '🖼️'),
            h('div', { className: 'text' }, h('strong', {}, '点击选择图片或PDF')),
            h('div', { className: 'hint' }, '支持 JPG / PNG / WebP / PDF，≤20MB，自动压缩'),
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
          h('label', { className: 'form-label' }, '描述'),
          h('textarea', { id: 'upload-desc', className: 'form-textarea', placeholder: '描述你的作品...' }),
        ),

        // Category + Difficulty (2-col)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '分类'),
            h('select', { id: 'upload-category', className: 'form-select' },
              h('option', { value: '' }, '选择分类'),
              ...categories.map(c => h('option', { value: c }, c)),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '难度 (1-5)'),
            h('select', { id: 'upload-difficulty', className: 'form-select' },
              h('option', { value: '' }, '选择难度'),
              ...[1, 2, 3, 4, 5].map(n => h('option', { value: String(n) }, '⭐'.repeat(n))),
            ),
          ),
        ),

        // Piece count + Dimensions (2-col)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '零件数'),
            h('input', { type: 'text', id: 'upload-pieces', className: 'form-input', placeholder: '例如: 1500' }),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '尺寸'),
            h('input', { type: 'text', id: 'upload-dimensions', className: 'form-input', placeholder: '例如: 30x20x15 cm' }),
          ),
        ),

        // Tags
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '🏷️ 标签'),
          h('div', { id: 'upload-tags-chips', style: {
            display: 'flex', flexWrap: 'wrap', gap: '6px', marginBottom: '8px',
            minHeight: '0px',
          } }),
          h('div', { style: { display: 'flex', gap: '8px' } },
            h('input', {
              type: 'text', id: 'upload-tag-input',
              className: 'form-input',
              placeholder: '输入标签名，回车添加...',
              style: { flex: 1 },
              onkeydown: (e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  addUploadTag();
                }
              },
            }),
            h('button', { className: 'btn btn-ghost btn-sm', onclick: addUploadTag }, '➕ 添加'),
          ),
          h('p', { style: { fontSize: '0.75rem', color: 'var(--text-sec)', marginTop: '4px' } },
            '💡 按回车或点击「添加」按钮添加标签'),
        ),

        // Publish checkbox — hidden, always published
        h('input', { type: 'hidden', id: 'upload-published', value: 'true' }),

        // Submit buttons
        h('div', { style: { display: 'flex', gap: '12px', marginTop: '20px' } },
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { flex: 1 },
            onclick: handleUpload,
          }, '📤 上传图纸'),
          h('button', {
            className: 'btn btn-ghost',
            onclick: () => navigate('home'),
          }, '取消'),
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
        title: '设为封面',
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
        title: '上移',
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
        title: '下移',
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
  const is_published = true; // always published

  if (!title) { if (errEl) errEl.innerHTML = '<div class="msg msg-error">请输入标题</div>'; return; }

  try {
    // 1. Create blueprint
    if (okEl) okEl.innerHTML = '<div class="msg" style="color:var(--accent)">创建图纸中...</div>';
    const data = await api.createBlueprint({
      title, description: description || undefined,
      category: category || undefined, difficulty,
      piece_count: piece_count || undefined,
      dimensions: dimensions || undefined, is_published,
    });

    // 2. Upload images if any
    if (_selectedFiles.length > 0) {
      if (okEl) okEl.innerHTML = `<div class="msg" style="color:var(--accent)">上传图片中 (0/${_selectedFiles.length})...</div>`;
      for (let i = 0; i < _selectedFiles.length; i++) {
        try {
          await api.uploadBlueprintImage(data.id, _selectedFiles[i]);
          if (okEl) okEl.innerHTML = `<div class="msg" style="color:var(--accent)">上传图片中 (${i + 1}/${_selectedFiles.length})...</div>`;
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
    if (okEl) okEl.innerHTML = '<div class="msg msg-success">🎉 上传成功！</div>';
    state.flashMessage = '🎉 发布成功！可在个人主页管理你的作品';
    setTimeout(() => navigate('detail', { id: data.id }), 800);
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
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

  const card = h('div', { className: `card${isOwnProfile ? ' card-own' : ''}`, onclick: isOwnProfile ? () => navigate('edit', { id: bp.id }) : () => navigate('detail', { id: bp.id }) },
    h('div', { className: 'card-img-wrap' },
      coverUrl
        ? h('img', {
            className: 'card-img',
            src: coverUrl,
            alt: bp.title,
            onerror: "this.style.display='none';this.parentElement.style.background='linear-gradient(135deg," + bg + "," + bg2 + ")'",
          })
        : h('div', {
            className: 'card-noimg',
            style: { background: `linear-gradient(135deg, ${bg}, ${bg2})` },
          }, emoji),
      h('span', { className: 'card-diff', style: { background: diff.bg } }, `${diff.stars} ${diff.label}`),
      bp.piece_count ? h('span', { className: 'card-parts' }, `🧩 ${bp.piece_count}片`) : null,
    ),
    h('div', { className: 'card-body' },
      h('div', { className: 'card-title' }, bp.title),
      h('div', { className: 'card-author' },
        h('img', { src: authorAvatar, alt: '' }),
        h('span', {}, bp.author?.username || '匿名'),
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
      h('button', { className: 'btn btn-ghost btn-sm', onclick: (e) => { e.stopPropagation(); navigate('edit', { id: bp.id }); } }, '✏️ 编辑'),
      h('button', { className: 'btn btn-ghost btn-sm', style: { color: '#ef4444' }, onclick: async (e) => {
        e.stopPropagation();
        if (!confirm(`确定删除「${bp.title}」吗？此操作不可恢复。`)) return;
        try {
          await api.deleteBlueprint(bp.id);
          showToast('已删除', 'success');
          setTimeout(() => renderUserProfile(), 500);
        } catch (err) { showToast(err.message, 'error'); }
      } }, '🗑️ 删除'),
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
    container.innerHTML = '<div style="text-align:center;padding:80px 20px"><p style="font-size:3rem;margin-bottom:16px">🧱</p><p style="font-size:1.1rem;color:var(--text-sec);margin-bottom:24px">未指定用户</p><button class="btn btn-primary" onclick="location.hash=\'#/home\'">返回首页</button></div>';
    return;
  }
  container.appendChild(h('p', {}, 'Loading...'));
  try {
    const profile = await api.getUserProfile(userId);
    state.userProfile = { ...state.userProfile, id: profile.id, username: profile.username };
    const items = state.userProfile.tab === 'favorites'
      ? await api.getUserFavorites(profile.id, { page: state.userProfile.page })
      : await api.getUserBlueprints(profile.id, { page: state.userProfile.page });
    container.innerHTML = '';
    const dateStr = profile.created_at ? new Date(profile.created_at).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long' }) : '';
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
            dateStr ? h('span', {}, `📅 ${dateStr} 加入`) : null,
            h('span', {}, `📐 ${profile.blueprint_count || 0} 作品`),
            h('span', {}, `⭐ ${profile.favorite_count || 0} 收藏`),
          ),
        ),
        isOwnProfile ? h('div', { className: 'profile-actions' },
          h('button', { className: 'btn btn-ghost btn-sm', onclick: () => openSettings(profile) }, '✏️ 编辑资料'),
          h('button', { className: 'btn btn-primary btn-sm', onclick: () => navigate('upload') }, '📤 上传作品'),
          h('button', { className: 'btn btn-danger btn-sm', onclick: handleLogout }, '🚪 退出登录'),
        ) : null,
      ),

      // Stats row
      h('div', { className: 'stats-row' },
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.blueprint_count || 0)),
          h('div', { className: 'stat-label' }, '作品'),
        ),
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.favorite_count || 0)),
          h('div', { className: 'stat-label' }, '收藏'),
        ),
        h('div', { className: 'stat-box' },
          h('div', { className: 'stat-value' }, String(profile.total_views || 0)),
          h('div', { className: 'stat-label' }, '浏览量'),
        ),
      ),

      // Tabs
      h('div', { className: 'tabs' },
        h('button', { className: `tab-btn${state.userProfile.tab === 'blueprints' ? ' active' : ''}`, onclick: () => { state.userProfile.tab = 'blueprints'; state.userProfile.page = 1; renderUserProfile(); } }, '我的作品'),
        h('button', { className: `tab-btn${state.userProfile.tab === 'favorites' ? ' active' : ''}`, onclick: () => { state.userProfile.tab = 'favorites'; state.userProfile.page = 1; renderUserProfile(); } }, '我的收藏'),
      ),

      // Card grid
      items.items && items.items.length > 0
        ? h('div', { className: 'card-grid' },
            ...items.items.map(bp => renderBlueprintCard(bp, isOwnProfile)),
          )
        : h('div', { className: 'empty', style: { padding: '40px', textAlign: 'center' } }, h('p', {}, '暂无作品')),

      // Pagination
      items.total > 12 ? h('div', { className: 'pagination', style: { justifyContent: 'center' } },
        ...Array.from({ length: Math.ceil(items.total / 12) }, (_, i) => h('button', { className: `page-btn${state.userProfile.page === i+1 ? ' active' : ''}`, onclick: () => { state.userProfile.page = i+1; renderUserProfile(); } }, String(i+1)))) : null,
    ));
  } catch (e) { container.innerHTML = '<div class="empty"><p>加载失败: '+e.message+'</p><button class="btn btn-primary btn-sm" onclick="location.reload()">重试</button></div>'; }
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
      h('h2', {}, '🎨 选择头像'),
      h('div', { className: 'avatar-picker-tabs' },
        h('button', {
          className: `avatar-picker-tab${_avatarPickerTab === 'presets' ? ' active' : ''}`,
          onclick: () => { _avatarPickerTab = 'presets'; _selectedAvatarUrl = null; renderPickerContent(); },
        }, '预设头像'),
        h('button', {
          className: `avatar-picker-tab${_avatarPickerTab === 'upload' ? ' active' : ''}`,
          onclick: () => { _avatarPickerTab = 'upload'; _selectedAvatarUrl = null; renderPickerContent(); },
        }, '上传图片'),
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
        grid.innerHTML = '<p style="grid-column:1/-1;text-align:center">加载预设头像失败</p>';
      });

      pickerEl.appendChild(
        h('div', { className: 'avatar-picker-actions' },
          h('button', { className: 'btn btn-white', onclick: () => closeAvatarPicker(profile) }, '取消'),
          h('button', {
            className: 'btn btn-primary',
            onclick: async () => {
              if (!_selectedAvatarUrl) return;
              try {
                await api.updateProfile({ avatar_url: _selectedAvatarUrl });
                closeAvatarPicker(profile);
                showToast('头像已更新', 'success');
                profile.avatar_url = _selectedAvatarUrl;
                _syncUserAvatar(_selectedAvatarUrl);
                openSettings(profile);
              } catch (e) {
                showToast(e.message, 'error');
              }
            },
          }, '确认'),
        ),
      );
    } else {
      // Upload tab
      const uploadDiv = h('div', { className: 'avatar-picker-upload' },
        h('p', { style: { marginBottom: '12px', color: 'var(--text-sec)' } }, '选择图片文件上传（支持 jpg/png/webp/gif，最大 2MB）'),
        h('input', {
          type: 'file',
          id: 'avatar-picker-file-input',
          accept: 'image/jpeg,image/png,image/webp,image/gif',
          style: { display: 'none' },
          onchange: async (e) => {
            const file = e.target.files?.[0];
            if (!file) return;
            if (file.size > 2 * 1024 * 1024) {
              showToast('文件不能超过 2MB', 'error');
              return;
            }
            try {
              const result = await api.uploadAvatar(file);
              closeAvatarPicker(profile);
              showToast('头像已更新', 'success');
              profile.avatar_url = result.avatar_url;
              _syncUserAvatar(result.avatar_url);
              openSettings(profile);
            } catch (e) {
              showToast(e.message, 'error');
            }
          },
        }),
        h('button', { className: 'btn btn-primary', onclick: () => document.getElementById('avatar-picker-file-input')?.click() }, '📁 选择文件'),
      );
      pickerEl.appendChild(uploadDiv);

      pickerEl.appendChild(
        h('div', { className: 'avatar-picker-actions' },
          h('button', { className: 'btn btn-white', onclick: () => closeAvatarPicker(profile) }, '取消'),
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
      h('h2', {}, '⚙️ 账号设置'),
      h('div', { className: 'settings-tabs' },
        h('button', {
          className: `settings-tab${_settingsTab === 'profile' ? ' active' : ''}`,
          onclick: () => { _settingsTab = 'profile'; renderSettingsContent(); },
        }, '个人资料'),
        h('button', {
          className: `settings-tab${_settingsTab === 'password' ? ' active' : ''}`,
          onclick: () => { _settingsTab = 'password'; renderSettingsContent(); },
        }, '修改密码'),
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
          h('span', { style: { color: 'var(--text-sec)', cursor: 'pointer' }, onclick: (e) => { e.stopPropagation(); openAvatarPicker(profile); } }, '🎨 预设'),
          h('span', { style: { color: 'var(--accent)', cursor: 'pointer' }, onclick: (e) => { e.stopPropagation(); document.getElementById('settings-avatar-input')?.click(); } }, '📁 本地上传'),
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
            h('label', {}, '昵称'),
            h('input', { type: 'text', id: 'settings-username', value: profile.username || '', placeholder: '你的用户名' }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, '个人简介'),
            h('textarea', { id: 'settings-bio', rows: 3, placeholder: '介绍一下自己...', style: { width: '100%', resize: 'vertical' } }, profile.bio || ''),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, '邮箱（不可更改）'),
            h('input', { type: 'text', value: profile.email || '', disabled: true, style: { opacity: '0.6' } }),
          ),
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { width: '100%', marginTop: '8px' },
            onclick: handleSaveSettings,
          }, '💾 保存'),
        ),
      );
    } else {
      // Password tab
      contentEl.appendChild(
        h('div', { className: 'settings-form' },
          h('div', { className: 'form-group' },
            h('label', {}, '当前密码'),
            h('input', { type: 'password', id: 'settings-current-password', placeholder: '输入当前密码' }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, '新密码'),
            h('input', { type: 'password', id: 'settings-new-password', placeholder: '至少6位' }),
          ),
          h('div', { className: 'form-group' },
            h('label', {}, '确认新密码'),
            h('input', { type: 'password', id: 'settings-confirm-password', placeholder: '再次输入新密码' }),
          ),
          h('button', {
            className: 'btn btn-primary btn-lg',
            style: { width: '100%', marginTop: '8px' },
            onclick: handleChangePassword,
          }, '🔒 修改密码'),
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
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">昵称不能为空</div>';
    return;
  }

  try {
    const data = await api.updateProfile({ username: username || undefined, bio: bio || undefined });
    state.user = data;
    state.userProfile = { ...state.userProfile, id: data.id, username: data.username };
    try {
      const auth = JSON.parse(localStorage.getItem('bp_auth') || '{}');
      localStorage.setItem('bp_auth', JSON.stringify({ ...auth, user: data }));
    } catch { /* ignore */ }
    closeSettings();
    renderNavbarIntoDOM();
    showToast('资料已更新', 'success');
    // Refresh the profile page without changing the stable user id route
    renderUserProfile();
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
  }
}

async function handleAvatarUpload(e) {
  const file = e.target.files?.[0];
  if (!file) return;

  const errEl = document.getElementById('settings-error');
  if (errEl) errEl.innerHTML = '';

  // Validate size client-side
  if (file.size > 2 * 1024 * 1024) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">文件不能超过 2MB</div>';
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
      label.textContent = '点击更换头像';
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
        const auth = JSON.parse(localStorage.getItem('bp_auth') || '{}');
        localStorage.setItem('bp_auth', JSON.stringify({ ...auth, user: data.user }));
      } catch { /* ignore */ }
      renderNavbarIntoDOM();
    }
    showToast('头像已更新', 'success');
    // Re-render the profile behind the scenes
    setTimeout(() => renderUserProfile(), 300);
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
  }
}

async function handleChangePassword() {
  const errEl = document.getElementById('settings-error');
  if (errEl) errEl.innerHTML = '';

  const current = document.getElementById('settings-current-password')?.value;
  const newPw = document.getElementById('settings-new-password')?.value;
  const confirm = document.getElementById('settings-confirm-password')?.value;

  if (!current || !newPw || !confirm) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">请填写所有密码字段</div>';
    return;
  }
  if (newPw.length < 6) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">新密码至少6位</div>';
    return;
  }
  if (newPw !== confirm) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">两次输入的新密码不一致</div>';
    return;
  }

  try {
    await api.changePassword(current, newPw);
    closeSettings();
    showToast('密码已修改', 'success');
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
  }
}

// ═══════════════════════════════════════════
// Delete Blueprint
// ═══════════════════════════════════════════
async function handleDeleteBlueprint(bp) {
  if (!confirm(`确定要删除「${bp.title}」吗？此操作不可撤销！`)) return;

  try {
    await api.deleteBlueprint(bp.id);
    showToast('图纸已删除', 'success');
    renderUserProfile();
  } catch (e) {
    showToast(`删除失败: ${e.message}`, 'error');
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
    container.innerHTML = '<p style="color:var(--text-sec);font-size:0.8rem;">暂无图片</p>';
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
        h('div', { style: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%' } }, 'PDF 文件'),
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
          title: '当前封面',
          onclick: (ev2) => { ev2.stopPropagation(); },
        }, '封面'));
      } else {
        item.appendChild(h('span', {
          className: 'cover-badge',
          style: { opacity: '0.5', cursor: 'pointer' },
          title: '设为封面',
          onclick: (ev2) => {
            ev2.stopPropagation();
            _editCoverIdx = idx;
            renderEditImages();
          },
        }, '封面'));
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
        title: '上移',
        onclick: (ev2) => {
          ev2.stopPropagation();
          moveImage(idx, idx - 1);
        },
      }, '▲'));
      item.appendChild(h('span', {
        className: 'sort-arrow sort-arrow-down',
        style: { ...arrowStyle, bottom: '2px', borderRadius: '0 0 6px 6px', opacity: idx === _editImages.length - 1 ? 0.35 : 1 },
        title: '下移',
        onclick: (ev2) => {
          ev2.stopPropagation();
          moveImage(idx, idx + 1);
        },
      }, '▼'));
    }

    // Delete button (top-right)
    item.appendChild(h('span', {
      className: 'remove-btn',
      title: '删除图片',
      onclick: async (ev2) => {
        ev2.stopPropagation();
        if (!confirm('确定删除这张图片吗？')) return;
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
          showToast('图片已删除', 'success');
        } catch (e) {
          showToast('删除失败: ' + e.message, 'error');
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
          h('p', { style: { fontSize: '1.1rem', color: 'var(--text-sec)', marginBottom: '24px' } }, '加载失败: ' + e.message),
          h('button', { className: 'btn btn-primary', onclick: () => navigate('home') }, '返回首页'),
        ),
      ),
    );
    return;
  }

  // Initialize edit state
  _editImages = [...(bp.images || [])];
  _editCoverIdx = getCoverIndex(_editImages);
  _editTags = [...(bp.tags || [])];

  const categories = ['建筑', '车辆', '机甲', '奇幻', '科幻', '场景'];

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
        h('p', { className: 'subtitle' }, '修改你的积木创作信息'),
        h('div', { id: 'edit-bp-error' }),

        // Title
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '标题'),
          h('input', { type: 'text', id: 'edit-bp-title', className: 'form-input', value: bp.title || '', placeholder: '图纸标题' }),
        ),

        // Description
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '描述'),
          h('textarea', { id: 'edit-bp-desc', className: 'form-textarea', placeholder: '描述你的作品...' }, bp.description || ''),
        ),

        // Category + Difficulty (form-row)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '分类'),
            h('select', { id: 'edit-bp-category', className: 'form-select' },
              h('option', { value: '' }, '选择分类'),
              ...categories.map(c => h('option', { value: c, ...(bp.category === c ? { selected: '' } : {}) }, c)),
            ),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '难度 (1-5)'),
            h('select', { id: 'edit-bp-difficulty', className: 'form-select' },
              h('option', { value: '' }, '选择难度'),
              ...[1, 2, 3, 4, 5].map(n => h('option', { value: String(n), ...(bp.difficulty === n ? { selected: '' } : {}) }, '\u2b50'.repeat(n))),
            ),
          ),
        ),

        // Piece count + Dimensions (form-row)
        h('div', { className: 'form-row' },
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '零件数'),
            h('input', { type: 'text', id: 'edit-bp-pieces', className: 'form-input', value: bp.piece_count || '', placeholder: '例如: 250' }),
          ),
          h('div', { className: 'form-group' },
            h('label', { className: 'form-label' }, '尺寸'),
            h('input', { type: 'text', id: 'edit-bp-dimensions', className: 'form-input', value: bp.dimensions || '', placeholder: '例如: 30x20x15 cm' }),
          ),
        ),

        // Tags
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '\ud83c\udff7\ufe0f 标签'),
          h('div', { id: 'edit-tags-chips', style: { display: 'flex', flexWrap: 'wrap', gap: '6px', marginBottom: '8px', minHeight: '0px' } }),
          h('div', { style: { display: 'flex', gap: '8px' } },
            h('input', {
              type: 'text', id: 'edit-tag-input', className: 'form-input',
              placeholder: '输入标签名，回车添加...',
              style: { flex: 1 },
              onkeydown: (e) => { if (e.key === 'Enter') { e.preventDefault(); addEditTag(); } },
            }),
            h('button', { className: 'btn btn-ghost btn-sm', onclick: addEditTag }, '\u2795 添加'),
          ),
        ),

        // File management
        h('div', { className: 'form-group' },
          h('label', { className: 'form-label' }, '🖼️ 文件管理 (点击⭐设为封面，▲▼排序)'),
          h('div', {
            className: 'upload-area',
            onclick: () => document.getElementById('edit-file-input')?.click(),
          },
            h('div', { className: 'icon' }, '🖼️'),
            h('div', { className: 'text' }, h('strong', {}, '点击添加图片或PDF')),
            h('div', { className: 'hint' }, '支持 JPG / PNG / WebP / PDF，≤20MB，自动压缩'),
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
              if (success > 0) msg.push(`${success} 个上传成功`);
              if (fail > 0) msg.push(`${fail} 个失败`);
              showToast(msg.join('，'), fail > 0 ? 'error' : 'success');
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
        }, '\ud83d\udcbe 保存修改'),
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

  if (!title) {
    if (errEl) errEl.innerHTML = '<div class="msg msg-error">标题不能为空</div>';
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

    showToast('图纸已更新', 'success');
    navigate('user', { id: state.user?.id });
  } catch (e) {
    if (errEl) errEl.innerHTML = `<div class="msg msg-error">${e.message}</div>`;
  }
}


async function renderAdminPage() {
  const container = $id('page-admin');
  container.innerHTML = '';
  let tab = 'pending'; // defaults
  let page = 1;
  let searchQ = '';

  const buildUI = () => {
    container.innerHTML = '';
    container.appendChild(
      h('div', { className: 'main' },
        h('a', { className: 'back-link', href: '#/home', onclick: (e) => { e.preventDefault(); navigate('home'); } }, '← 返回前台'),
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
        ),

        // Search (only for "all")
        ...(tab === 'all' ? [h('div', { style: { display: 'flex', gap: '8px', marginBottom: '16px' } },
          h('input', { type: 'text', id: 'admin-search', className: 'form-input', placeholder: '搜索标题或作者...', value: searchQ, style: { flex: '1', maxWidth: '320px' },
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

  const loadTable = async () => {
    const tableEl = $id('admin-table');
    const pagEl = $id('admin-pagination');
    if (!tableEl) return;

    try {
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
          }, '‹ 上一页'));
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
          }, '下一页 ›'));
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
        }, '‹ 上一页'));
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
        }, '下一页 ›'));
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

  buildUI();
}
function renderNavbarIntoDOM() {
  const existing = document.querySelector('.navbar');
  if (existing) existing.replaceWith(renderNavbar());
  else document.getElementById('app').prepend(renderNavbar());
}

function render() {
  const app = document.getElementById('app');

  // Ensure page containers exist
  const pages = ['home', 'explore', 'detail', 'upload', 'user', 'admin', 'edit', 'privacy', 'notifications'];
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

  // Render page content
  switch (state.page) {
    case 'home': resetMeta(); renderHome(); break;
    case 'explore': resetMeta(); renderExplore(); break;
    case 'detail': renderDetail(); break;
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
    case 'notifications':
      resetMeta();
      renderNotifications();
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
if (location.hash) {
  onHashChange();  // restore state from deep link
} else {
  // Set initial hash so first back-button press works
  location.hash = '#/home';
  // hashchange fires → calls onHashChange → render
}
