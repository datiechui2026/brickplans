const api = require('../../utils/api')
const { CATEGORIES } = require('../../utils/config')

Page({
  data: {
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
      const bpRes = await api.getFeatured(8)
      this.setData({
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
