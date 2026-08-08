const api = require('../../utils/api')
const { CATEGORIES } = require('../../utils/config')
const util = require('../../utils/util')

Page({
  data: {
    stats: null,
    popular: [],
    categories: CATEGORIES,
    loading: true,
    error: '',
  },

  onLoad() {
    this.load()
  },

  onPullDownRefresh() {
    this.load().then(() => wx.stopPullDownRefresh())
  },

  async load() {
    this.setData({ loading: true, error: '' })
    try {
      const [stats, bpRes] = await Promise.all([
        api.getStats(),
        api.listBlueprints({ size: 8, sort: 'popular' }),
      ])
      this.setData({
        stats: {
          blueprints: util.formatCount(stats.total_blueprints),
          users: util.formatCount(stats.total_users),
          views: util.formatCount(stats.total_views),
          favorites: util.formatCount(stats.total_favorites),
        },
        popular: bpRes.items || [],
        loading: false,
      })
    } catch (e) {
      this.setData({ loading: false, error: (e && e.message) || '加载失败' })
    }
  },

  goCategory(e) {
    const cat = e.currentTarget.dataset.cat
    const app = getApp()
    app.globalData.pendingFilter = cat ? { category: cat } : null
    wx.switchTab({ url: '/pages/explore/index' })
  },

  goExplore() {
    wx.switchTab({ url: '/pages/explore/index' })
  },
})
