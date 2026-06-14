// API client — configurable base URL (works on Vercel + local dev)
const API_BASE = (typeof import.meta !== 'undefined' && import.meta.env?.VITE_API_BASE_URL) || '';

function getToken() {
  try {
    const auth = JSON.parse(localStorage.getItem('bp_auth') || '{}');
    return auth.access_token || auth.token || auth.user?.access_token || null;
  }
  catch { return null; }
}

function getRefreshToken() {
  try { return JSON.parse(localStorage.getItem('bp_auth') || '{}').refresh_token || null; }
  catch { return null; }
}

function setAuth(data) {
  localStorage.setItem('bp_auth', JSON.stringify(data));
}

function clearAuth() {
  localStorage.removeItem('bp_auth');
}

function getAuth() {
  try { return JSON.parse(localStorage.getItem('bp_auth') || '{}'); }
  catch { return {}; }
}

// ── Token refresh ──
let refreshPromise = null; // prevent concurrent refresh attempts

async function tryRefreshToken() {
  if (refreshPromise) return refreshPromise;

  const rt = getRefreshToken();
  if (!rt) return null;

  refreshPromise = (async () => {
    try {
      const res = await fetch(`${API_BASE}/api/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: rt }),
      });

      if (!res.ok) {
        clearAuth();
        return null;
      }

      const data = await res.json();
      const current = getAuth();
      const updated = { ...current, ...data };
      setAuth(updated);
      return data.access_token;
    } catch {
      clearAuth();
      return null;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

async function request(path, options = {}) {
  const token = getToken();
  const method = (options.method || 'GET').toUpperCase();
  const needsAuth = options.auth !== false && (options.requireAuth || !['GET', 'HEAD', 'OPTIONS'].includes(method));
  if (needsAuth && !token) {
    throw new Error('请先登录');
  }
  const headers = { 'Content-Type': 'application/json', ...options.headers };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const { auth: _auth, ...fetchOptions } = options;
  const res = await fetch(`${API_BASE}${path}`, { ...fetchOptions, headers });

  if (res.status === 204) return null;

  // Auto-refresh on 401/403 auth failures (skip refresh endpoint itself to avoid infinite loop)
  if ((res.status === 401 || res.status === 403) && token && getRefreshToken() && path !== '/api/auth/refresh') {
    const newToken = await tryRefreshToken();
    if (newToken) {
      headers['Authorization'] = `Bearer ${newToken}`;
      const retryRes = await fetch(`${API_BASE}${path}`, { ...options, headers });

      if (retryRes.status === 204) return null;

      const retryData = await retryRes.json().catch(() => null);

      if (retryRes.ok) return retryData;

      const retryMsg = retryData?.detail || `Request failed (${retryRes.status})`;
      throw new Error(retryMsg);
    }
    // Refresh failed — force re-login
    throw new Error('登录已过期，请重新登录');
  }

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    if ((res.status === 401 || res.status === 403) && token) {
      clearAuth();
      throw new Error('登录已过期，请重新登录');
    }
    const msg = data?.detail || `Request failed (${res.status})`;
    throw new Error(msg);
  }

  return data;
}

async function formRequest(path, form, errorPrefix = 'Upload failed') {
  let token = getToken();
  if (!token) throw new Error('请先登录');

  const send = (accessToken) => fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    body: form,
  });

  let res = await send(token);
  if (res.status === 401 && getRefreshToken()) {
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

export function logout() {
  clearAuth();
}

export function isLoggedIn() {
  return !!getToken();
}

export function getCurrentUser() {
  const auth = getAuth();
  return auth?.user || auth;
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
