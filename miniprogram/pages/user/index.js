const api = require('../../utils/api')
const auth = require('../../utils/auth')
const util = require('../../utils/util')

Page({
  data: {
    userId: '',
    isSelf: false,
    notLoggedIn: false,
    agreedPrivacy: false,
    profile: null,
    avatarUrl: '',
    createdText: '',
    tab: 'works', // 'works' | 'favorites'
    items: [],
    page: 1,
    pageSize: 12,
    loading: false,
    loadingMore: false,
    hasMore: true,
    error: '',
  },

  onLoad(opts) {
    if (opts.id) {
      this.setData({ userId: opts.id })
      this.loadProfile()
    } else {
      const user = api.getUser()
      if (user) {
        this.setData({ userId: user.id, isSelf: true })
        this.loadProfile()
      } else {
        this.setData({ notLoggedIn: true })
      }
    }
  },

  onShow() {
    // After returning from a successful WeChat login, reload as self.
    if (this.data.notLoggedIn && auth.isLoggedIn()) {
      const user = api.getUser()
      this.setData({ userId: user.id, isSelf: true, notLoggedIn: false })
      this.loadProfile()
    }
  },

  async loadProfile() {
    try {
      const p = await api.getUserProfile(this.data.userId)
      const me = api.getUser()
      this.setData({
        profile: p,
        avatarUrl: util.resolveUrl(p.avatar_url),
        createdText: util.formatDate(p.created_at),
        isSelf: !!(me && me.id === p.id),
        tab: (me && me.id === p.id) ? 'favorites' : 'works',
        items: [],
        page: 1,
        hasMore: true,
        error: '',
      })
      wx.setNavigationBarTitle({ title: p.username + ' 的主页' })
      this.loadItems()
    } catch (e) {
      this.setData({ error: (e && e.message) || '加载失败' })
    }
  },

  async loadItems() {
    if (this.data.loading || this.data.loadingMore) return
    this.setData({ loading: this.data.page === 1, loadingMore: this.data.page > 1 })
    try {
      const params = { page: this.data.page, size: this.data.pageSize }
      // Favorites are owner-only (403 otherwise) - the tab is only shown for self.
      const res = this.data.tab === 'works'
        ? await api.getUserBlueprints(this.data.userId, params)
        : await api.getUserFavorites(this.data.userId, params)
      const items = res.items || []
      this.setData({
        items: this.data.page === 1 ? items : this.data.items.concat(items),
        hasMore: this.data.page * this.data.pageSize < (res.total || 0),
        loading: false,
        loadingMore: false,
      })
    } catch (e) {
      this.setData({ loading: false, loadingMore: false, error: (e && e.message) || '加载失败' })
    }
  },

  switchTab(e) {
    const tab = e.currentTarget.dataset.tab
    if (tab === this.data.tab) return
    this.setData({ tab, items: [], page: 1, hasMore: true, error: '' })
    this.loadItems()
  },

  onReachBottom() {
    if (!this.data.hasMore || this.data.loadingMore || this.data.loading) return
    this.setData({ page: this.data.page + 1 })
    this.loadItems()
  },

  togglePrivacy() {
    this.setData({ agreedPrivacy: !this.data.agreedPrivacy })
  },

  async doLogin() {
    if (!this.data.agreedPrivacy) {
      wx.showToast({ title: '请先阅读并同意隐私策略', icon: 'none' })
      return
    }
    wx.showLoading({ title: '登录中...' })
    try {
      await auth.wxLogin()
      wx.hideLoading()
      const user = api.getUser()
      this.setData({ userId: user.id, isSelf: true, notLoggedIn: false })
      this.loadProfile()
    } catch (e) {
      wx.hideLoading()
      wx.showToast({ title: (e && e.message) || '登录失败', icon: 'none' })
    }
  },

  doLogout() {
    wx.showModal({
      title: '退出登录',
      content: '确定退出登录吗？',
      success: async (res) => {
        if (!res.confirm) return
        await auth.logout()
        this.setData({ notLoggedIn: true, profile: null, items: [], userId: '', isSelf: false })
      },
    })
  },
})
