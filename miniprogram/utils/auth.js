// WeChat login + auth-state helpers.
//
// wx.login() is silent (no user-consent prompt) and returns a short-lived code;
// the backend exchanges it for the openid via jscode2session and issues JWTs.
// WeChat no longer returns nickname/avatar via that API, so first-time users get
// an auto-generated 微信用户_<hex> name + a preset avatar (editable later on the
// web, which is out of scope for this browse-only mini program).

const api = require('./api')

function isLoggedIn() { return !!api.getAccessToken() }

function wxLogin() {
  return new Promise((resolve, reject) => {
    wx.login({
      success: async (res) => {
        if (!res.code) { reject(new Error('微信 login 未返回 code')); return }
        try {
          const data = await api.wechatLogin(res.code)
          api.setTokens(data.access_token, data.refresh_token)
          if (data.user) api.setUser(data.user)
          const app = getApp()
          if (app) app.globalData.user = data.user
          resolve(data)
        } catch (e) {
          reject(e)
        }
      },
      fail: (err) => reject(err),
    })
  })
}

// If logged in, resolve with the current user. Otherwise show a modal offering
// WeChat login; resolves after a successful login, rejects if cancelled/failed.
function ensureLogin() {
  return new Promise((resolve, reject) => {
    if (isLoggedIn()) { resolve(api.getUser()); return }
    wx.showModal({
      title: '登录',
      content: '该操作需要登录，是否使用微信登录？',
      confirmText: '微信登录',
      cancelText: '取消',
      success: async (res) => {
        if (!res.confirm) { reject(new Error('cancelled')); return }
        wx.showLoading({ title: '登录中...' })
        try {
          const data = await wxLogin()
          wx.hideLoading()
          resolve(data.user)
        } catch (e) {
          wx.hideLoading()
          const msg = (e && e.message) || '登录失败'
          wx.showToast({ title: msg, icon: 'none' })
          reject(e)
        }
      },
    })
  })
}

async function logout() {
  try { await api.logout() } catch (e) { /* best-effort */ }
  api.clearTokens()
  const app = getApp()
  if (app) app.globalData.user = null
}

module.exports = {
  isLoggedIn,
  wxLogin,
  ensureLogin,
  logout,
}
