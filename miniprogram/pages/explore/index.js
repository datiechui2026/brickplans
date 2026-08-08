const api = require('../../utils/api')
const { CATEGORIES } = require('../../utils/config')

Page({
  data: {
    q: '',
    category: '',
    tag: '',
    sort: 'latest', // 'latest' | 'popular'
    tags: [],
    categories: CATEGORIES,
    items: [],
    total: 0,
    page: 1,
    pageSize: 12,
    loading: false,
    loadingMore: false,
    hasMore: true,
    error: '',
  },

  onLoad() {
    this.loadTags()
    this.resetAndLoad()
  },

  onShow() {
    // Home's category quick-nav hands us a filter via globalData (switchTab can't
    // carry query params).
    const app = getApp()
    if (app.globalData && app.globalData.pendingFilter) {
      const f = app.globalData.pendingFilter
      app.globalData.pendingFilter = null
      this.setData({ category: f.category || '', q: '' })
      this.resetAndLoad()
    }
  },

  async loadTags() {
    try {
      const tags = await api.listTags()
      this.setData({ tags: (tags || []).slice(0, 15) })
    } catch (e) { /* non-fatal */ }
  },

  onSearch(e) { this.setData({ q: e.detail.value }) },
  onSearchConfirm() { this.resetAndLoad() },
  clearSearch() { this.setData({ q: '' }); this.resetAndLoad() },

  selectCategory(e) {
    this.setData({ category: e.currentTarget.dataset.cat })
    this.resetAndLoad()
  },

  selectTag(e) {
    const tag = e.currentTarget.dataset.tag
    this.setData({ tag: this.data.tag === tag ? '' : tag })
    this.resetAndLoad()
  },

  toggleSort(e) {
    this.setData({ sort: e.currentTarget.dataset.sort })
    this.resetAndLoad()
  },

  resetAndLoad() {
    this.setData({ page: 1, items: [], hasMore: true, error: '' })
    return this.load()
  },

  async load() {
    if (this.data.loading || this.data.loadingMore) return
    this.setData({
      loading: this.data.page === 1,
      loadingMore: this.data.page > 1,
    })
    try {
      const res = await api.listBlueprints({
        page: this.data.page,
        size: this.data.pageSize,
        q: this.data.q,
        category: this.data.category,
        tag: this.data.tag,
        sort: this.data.sort === 'popular' ? 'popular' : '',
      })
      const items = res.items || []
      this.setData({
        items: this.data.page === 1 ? items : this.data.items.concat(items),
        total: res.total || 0,
        hasMore: this.data.page * this.data.pageSize < (res.total || 0),
        loading: false,
        loadingMore: false,
      })
    } catch (e) {
      this.setData({ loading: false, loadingMore: false, error: (e && e.message) || '加载失败' })
    }
  },

  onReachBottom() {
    if (!this.data.hasMore || this.data.loadingMore || this.data.loading) return
    this.setData({ page: this.data.page + 1 })
    this.load()
  },

  onPullDownRefresh() {
    this.resetAndLoad().then(() => wx.stopPullDownRefresh())
  },
})
