// API client — configurable base URL (works on Vercel + local dev)
const API_BASE = (typeof import.meta !== 'undefined' && import.meta.env?.VITE_API_BASE_URL) || '';

// ── Auth state ──
// Access token lives only in memory (not localStorage) so an XSS cannot exfiltrate
// it from storage. The refresh token lives in an httpOnly cookie set by the backend
// (Path=/api/auth), so JS can never read it. On page reload we restore the session
// by calling /api/auth/refresh, which uses the cookie.
let accessToken = null;
let refreshPromise = null; // de-dupes concurrent refresh attempts

function getUser() {
  try { return JSON.parse(localStorage.getItem('bp_user') || 'null'); }
  catch { return null; }
}
function setUser(u) {
  if (u) localStorage.setItem('bp_user', JSON.stringify(u));
  else localStorage.removeItem('bp_user');
}

function setAuth(data) {
  accessToken = data.access_token || null;
  if (data.user) setUser(data.user);
}

function clearAuth() {
  accessToken = null;
  setUser(null);
}

export function isLoggedIn() {
  return !!accessToken || !!getUser();
}

export function getCurrentUser() {
  return getUser();
}

// ── Token refresh (uses the httpOnly cookie; no body, no Authorization header) ──
async function tryRefreshToken() {
  if (refreshPromise) return refreshPromise;
  refreshPromise = (async () => {
    try {
      const res = await fetch(`${API_BASE}/api/auth/refresh`, {
        method: 'POST',
        credentials: 'include', // send bp_refresh cookie
        headers: { 'Content-Type': 'application/json' },
      });
      if (!res.ok) { clearAuth(); return null; }
      const data = await res.json();
      accessToken = data.access_token || null;
      if (data.user) setUser(data.user);
      return accessToken;
    } catch {
      clearAuth();
      return null;
    } finally {
      refreshPromise = null;
    }
  })();
  return refreshPromise;
}

// ensureSession restores the access token on page load if the user was logged in.
// Fire-and-forget; requests that need auth will 401→refresh as a fallback.
export function ensureSession() {
  if (accessToken || !getUser()) return;
  tryRefreshToken();
}

async function request(path, options = {}) {
  const token = accessToken;
  const method = (options.method || 'GET').toUpperCase();
  const needsAuth = options.auth !== false && (options.requireAuth || !['GET', 'HEAD', 'OPTIONS'].includes(method));
  if (needsAuth && !token && !getUser()) {
    throw new Error('请先登录');
  }
  const headers = { 'Content-Type': 'application/json', ...options.headers };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const { auth: _auth, ...fetchOptions } = options;
  const res = await fetch(`${API_BASE}${path}`, { ...fetchOptions, headers, credentials: 'include' });

  if (res.status === 204) return null;

  // Auto-refresh on 401/403 auth failures (skip the refresh endpoint itself).
  if ((res.status === 401 || res.status === 403) && path !== '/api/auth/refresh') {
    const newToken = await tryRefreshToken();
    if (newToken) {
      headers['Authorization'] = `Bearer ${newToken}`;
      const retryRes = await fetch(`${API_BASE}${path}`, { ...options, headers, credentials: 'include' });
      if (retryRes.status === 204) return null;
      const retryData = await retryRes.json().catch(() => null);
      if (retryRes.ok) return retryData;
      throw new Error(retryData?.detail || `Request failed (${retryRes.status})`);
    }
    clearAuth();
    throw new Error('登录已过期，请重新登录');
  }

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    if ((res.status === 401 || res.status === 403) && token) {
      clearAuth();
      throw new Error('登录已过期，请重新登录');
    }
    throw new Error(data?.detail || `Request failed (${res.status})`);
  }

  return data;
}

async function formRequest(path, form, errorPrefix = 'Upload failed') {
  if (!accessToken && !getUser()) throw new Error('请先登录');

  const send = (tok) => fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: tok ? { Authorization: `Bearer ${tok}` } : {},
    body: form,
    credentials: 'include',
  });

  let res = await send(accessToken);
  if (res.status === 401) {
    const newToken = await tryRefreshToken();
    if (newToken) res = await send(newToken);
  }

  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new Error(data?.detail || `${errorPrefix} (${res.status})`);
  }
  return data;
}

// ── Auth ──
export async function register(username, email, password) {
  const data = await request('/api/auth/register', {
    method: 'POST',
    auth: false,
    body: JSON.stringify({ username, email, password }),
  });
  setAuth(data);
  return data;
}

export async function login(email, password) {
  const data = await request('/api/auth/login', {
    method: 'POST',
    auth: false,
    body: JSON.stringify({ email, password }),
  });
  setAuth(data);
  return data;
}

export async function logout() {
  try {
    await fetch(`${API_BASE}/api/auth/logout`, { method: 'POST', credentials: 'include' });
  } catch { /* ignore — clear local state regardless */ }
  clearAuth();
}

// ── Blueprints ──
export async function listBlueprints({ page = 1, size = 12, q, category, sort, tag } = {}) {
  const params = new URLSearchParams({ page, size });
  if (q) params.set('q', q);
  if (category) params.set('category', category);
  if (sort) params.set('sort', sort);
  if (tag) params.set('tag', tag);
  return request(`/api/blueprints?${params}`);
}

export async function getBlueprint(id) {
  return request(`/api/blueprints/${id}`);
}

export async function getRelatedBlueprints(id) {
  return request(`/api/blueprints/${id}/related`);
}

export async function createBlueprint(payload) {
  return request('/api/blueprints', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateBlueprint(id, payload) {
  return request(`/api/blueprints/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export async function deleteBlueprint(id) {
  return request(`/api/blueprints/${id}`, { method: 'DELETE' });
}

// ── Favorites ──
export async function favoriteBlueprint(id) {
  return request(`/api/blueprints/${id}/favorite`, { method: 'POST' });
}

export async function unfavoriteBlueprint(id) {
  return request(`/api/blueprints/${id}/favorite`, { method: 'DELETE' });
}

// ── Comments ──
export async function listComments(blueprintId) {
  return request(`/api/blueprints/${blueprintId}/comments`);
}

export async function createComment(blueprintId, content, parentId = null) {
  const payload = { content };
  if (parentId) payload.parent_id = parentId;
  return request(`/api/blueprints/${blueprintId}/comments`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

// ── Images ──
export async function uploadBlueprintImage(blueprintId, file) {
  const form = new FormData();
  form.append('files', file);
  return formRequest(`/api/blueprints/${blueprintId}/images`, form, '上传失败');
}

export async function deleteBlueprintImage(blueprintId, imageId) {
  return request(`/api/blueprints/${blueprintId}/images/${imageId}`, { method: 'DELETE' });
}

export async function reorderImages(blueprintId, images) {
  return request(`/api/blueprints/${blueprintId}/images/reorder`, {
    method: 'PUT',
    body: JSON.stringify({ images }),
  });
}

export async function setCover(blueprintId, imageId) {
  return request(`/api/blueprints/${blueprintId}/images/${imageId}/cover`, {
    method: 'PUT',
  });
}

// ── Tags ──
export async function listAllTags() {
  return request('/api/tags');
}

export async function getBlueprintTags(blueprintId) {
  return request(`/api/blueprints/${blueprintId}/tags`);
}

export async function bindTags(blueprintId, tags) {
  return request(`/api/blueprints/${blueprintId}/tags`, {
    method: 'POST',
    body: JSON.stringify({ tags }),
  });
}

// ── Reports ──
export async function createReport(blueprintId, reason, detail) {
  return request('/api/reports', {
    method: 'POST',
    body: JSON.stringify({ blueprint_id: blueprintId, reason, detail }),
  });
}
export async function getUserProfile(userId) {
  return request(`/api/users/${encodeURIComponent(userId)}`);
}

export async function getUserBlueprints(userId, { page = 1, size = 12 } = {}) {
  const params = new URLSearchParams({ page, size });
  return request(`/api/users/${encodeURIComponent(userId)}/blueprints?${params}`);
}

export async function getUserFavorites(userId, { page = 1, size = 12 } = {}) {
  const params = new URLSearchParams({ page, size });
  return request(`/api/users/${encodeURIComponent(userId)}/favorites?${params}`);
}

// ── User Profile & Settings ──
export async function getMe() {
  return request('/api/auth/me');
}

export async function updateProfile(data) {
  return request('/api/auth/me', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function changePassword(currentPassword, newPassword) {
  return request('/api/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

export async function uploadAvatar(file) {
  const form = new FormData();
  form.append('file', file);
  return formRequest('/api/auth/avatar', form, 'Upload failed');
}

export async function getPresetAvatars() {
  return request('/api/auth/avatars');
}

export async function resendVerification() {
  return request('/api/auth/verify-email/resend', { method: 'POST' });
}

// ── Likes ──
export async function likeBlueprint(id) {
  return request(`/api/blueprints/${id}/like`, { method: 'POST' });
}

export async function unlikeBlueprint(id) {
  return request(`/api/blueprints/${id}/like`, { method: 'DELETE' });
}

// ── Stats ──
export async function getStats() {
  return request('/api/stats');
}

// ── Notifications ──
export async function listNotifications({ page = 1, size = 20 } = {}) {
  const params = new URLSearchParams({ page, size });
  return request(`/api/notifications?${params}`, { requireAuth: true });
}

export async function getUnreadNotificationCount() {
  return request('/api/notifications/unread-count', { requireAuth: true });
}

export async function markNotificationsRead() {
  return request('/api/notifications/mark-read', { method: 'POST' });
}

// ── Admin ──
export async function adminListBlueprints({ page = 1, q = '' } = {}) {
  const params = new URLSearchParams({ page, size: '20' });
  if (q) params.set('q', q);
  return request(`/api/admin/blueprints?${params}`);
}

export async function adminPendingBlueprints({ page = 1 } = {}) {
  const params = new URLSearchParams({ page, size: '20' });
  return request(`/api/admin/blueprints/pending?${params}`);
}

export async function adminPublish(id) {
  return request(`/api/admin/blueprints/${id}/publish`, { method: 'PUT' });
}

export async function adminUnpublish(id) {
  return request(`/api/admin/blueprints/${id}/unpublish`, { method: 'PUT' });
}

export async function adminDelete(id) {
  return request(`/api/admin/blueprints/${id}`, { method: 'DELETE' });
}

export async function adminListReports({ page = 1 } = {}) {
  const params = new URLSearchParams({ page, size: '20' });
  return request(`/api/admin/reports?${params}`);
}

// Restore session on load if the user was logged in (refresh cookie is httpOnly).
ensureSession();
