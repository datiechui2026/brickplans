// BrickPlan Mini Program - app entry.
// Browse-only WeChat Mini Program for the BrickPlan brick/MOC blueprint
// community. Auth is WeChat login (wx.login -> backend jscode2session -> JWT);
// tokens are stored in wx storage and sent via the Authorization header (mini
// programs can't rely on httpOnly cookies like the web frontend does).

const api = require('./utils/api')
const auth = require('./utils/auth')

App({
  globalData: {
    user: null,
  },

  onLaunch() {
    // Best-effort session restore: if we have a (possibly stale) access token,
    // ask /api/auth/me for the fresh user. api.request auto-refreshes on 401.
    this.restoreSession()
  },

  async restoreSession() {
    if (!auth.isLoggedIn()) return
    try {
      const me = await api.getMe()
      api.setUser(me)
      this.globalData.user = me
    } catch (e) {
      // Token invalid + refresh failed -> api already cleared auth. Stay logged-out.
      this.globalData.user = null
    }
  },
})
