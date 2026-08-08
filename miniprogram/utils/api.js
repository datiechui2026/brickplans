// The only API client for the BrickPlan mini program.
//
// Auth model (differs from the web SPA): mini programs can't rely on httpOnly
// cookies, so both the access AND refresh tokens live in wx storage. Every
// request sends `Authorization: Bearer <access_token>`; on 401 we call
// /api/auth/refresh with the refresh token in the body (the backend accepts
// body refresh as a cookieless fallback) and retry once.

const { API_BASE } = require('./config')

// ── token storage ──
function getAccessToken() { return wx.getStorageSync('access_token') || '' }
function getRefreshToken() { return wx.getStorageSync('refresh_token') || '' }
function setTokens(at, rt) {
  wx.setStorageSync('access_token', at)
  if (rt) wx.setStorageSync('refresh_token', rt)
}
function clearTokens() {
  wx.removeStorageSync('access_token')
  wx.removeStorageSync('refresh_token')
  wx.removeStorageSync('user')
}
function getUser() { return wx.getStorageSync('user') || null }
function setUser(u) { wx.setStorageSync('user', u) }

// ── refresh (singleton promise de-dupes concurrent 401s) ──
let refreshing = null
function refresh() {
  if (refreshing) return refreshing
  const rt = getRefreshToken()
  if (!rt) return Promise.reject(new Error('no refresh token'))
  refreshing = new Promise((resolve, reject) => {
    wx.request({
      url: API_BASE + '/api/auth/refresh',
      method: 'POST',
      data: { refresh_token: rt },
      header: { 'Content-Type': 'application/json' },
      success: (res) => {
        if (res.statusCode === 200 && res.data && res.data.access_token) {
          setTokens(res.data.access_token, res.data.refresh_token)
          if (res.data.user) setUser(res.data.user)
          resolve(res.data.access_token)
        } else {
          clearTokens()
          reject(new Error('refresh failed'))
        }
      },
      fail: () => { clearTokens(); reject(new Error('network')) },
    })
  })
  refreshing.then(() => { refreshing = null }, () => { refreshing = null })
  return refreshing
}

// ── core request ──
function request(method, path, data, opts) {
  opts = opts || {}
  return new Promise((resolve, reject) => {
    const fire = () => {
      const header = { 'Content-Type': 'application/json' }
      const at = getAccessToken()
      if (at) header.Authorization = 'Bearer ' + at
      Object.assign(header, opts.header || {})
      wx.request({
        url: API_BASE + path,
        method,
        data,
        header,
        success: async (res) => {
          // 401 => token expired/invalid; try one refresh + retry.
          if (res.statusCode === 401 && !opts._retried && getRefreshToken()) {
            try {
              await refresh()
              resolve(await request(method, path, data, Object.assign({}, opts, { _retried: true })))
            } catch (e) {
              clearTokens()
              reject(errOf(401, '登录已过期，请重新登录', res.data))
            }
            return
          }
          if (res.statusCode >= 400) {
            reject(errOf(res.statusCode, (res.data && (res.data.detail || res.data.message)) || '请求失败', res.data))
          } else {
            resolve(res.data)
          }
        },
        fail: () => reject(errOf(0, '网络错误，请检查连接')),
      })
    }
    fire()
  })
}

function errOf(status, message, data) {
  return { status, message, data }
}

// ── Auth ──
function wechatLogin(code) { return request('POST', '/api/auth/wechat-login', { code }) }
function getMe() { return request('GET', '/api/auth/me') }
function logout() { return request('POST', '/api/auth/logout') }

// ── Blueprints ──
function listBlueprints(params) {
  const q = query(params)
  return request('GET', '/api/blueprints' + (q ? '?' + q : ''))
}
function getFeatured(size) {
  return request('GET', '/api/blueprints/featured' + (size ? '?size=' + size : ''))
}
function getBlueprint(id) { return request('GET', '/api/blueprints/' + id) }
function getRelatedBlueprints(id) { return request('GET', '/api/blueprints/' + id + '/related') }
function favoriteBlueprint(id) { return request('POST', '/api/blueprints/' + id + '/favorite') }
function unfavoriteBlueprint(id) { return request('DELETE', '/api/blueprints/' + id + '/favorite') }
function likeBlueprint(id) { return request('POST', '/api/blueprints/' + id + '/like') }
function unlikeBlueprint(id) { return request('DELETE', '/api/blueprints/' + id + '/like') }

// ── Comments ──
function listComments(id) { return request('GET', '/api/blueprints/' + id + '/comments') }
function createComment(id, payload) { return request('POST', '/api/blueprints/' + id + '/comments', payload) }

// ── Tags ──
function listTags() { return request('GET', '/api/tags') }

// ── Reports ──
function createReport(payload) { return request('POST', '/api/reports', payload) }

// ── Users ──
function getUserProfile(id) { return request('GET', '/api/users/' + id) }
function getUserBlueprints(id, params) {
  const q = query(params)
  return request('GET', '/api/users/' + id + '/blueprints' + (q ? '?' + q : ''))
}
function getUserFavorites(id, params) {
  const q = query(params)
  return request('GET', '/api/users/' + id + '/favorites' + (q ? '?' + q : ''))
}

// ── Stats ──
function getStats() { return request('GET', '/api/stats') }

// ── helper: build a query string from an object, skipping null/empty values ──
function query(params) {
  if (!params) return ''
  const parts = []
  Object.keys(params).forEach((k) => {
    const v = params[k]
    if (v !== undefined && v !== null && v !== '') parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(v))
  })
  return parts.join('&')
}

module.exports = {
  request,
  // token storage (also used by utils/auth.js)
  getAccessToken, getRefreshToken, setTokens, clearTokens, getUser, setUser,
  refresh,
  // endpoints
  wechatLogin, getMe, logout,
  listBlueprints, getFeatured, getBlueprint, getRelatedBlueprints,
  favoriteBlueprint, unfavoriteBlueprint, likeBlueprint, unlikeBlueprint,
  listComments, createComment,
  listTags, createReport,
  getUserProfile, getUserBlueprints, getUserFavorites,
  getStats,
}
