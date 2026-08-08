const api = require('../../utils/api')
const auth = require('../../utils/auth')
const util = require('../../utils/util')
const { mdToHtml } = require('../../utils/markdown')
const { difficultyText, categoryEmoji, REPORT_REASONS } = require('../../utils/config')

Page({
  data: {
    id: '',
    bp: null,
    images: [],     // image attachments (resolved absolute URLs) for the swiper
    pdfs: [],       // pdf attachments (resolved) -> "open in viewer" buttons
    descHtml: '',
    partList: null,
    related: [],
    comments: [],   // tree: [{...comment, replies: [...]}]
    loading: true,
    error: '',
    liking: false,
    favving: false,
    commentText: '',
    replyTo: null,  // {id, username}
    sendingComment: false,
    showReport: false,
    reportReason: '',
    reportDetail: '',
    reportReasons: REPORT_REASONS,
  },

  onLoad(opts) {
    this.setData({ id: opts.id || '' })
    this.load()
  },

  async load() {
    this.setData({ loading: true, error: '' })
    try {
      const [bp, comments, relatedRes] = await Promise.all([
        api.getBlueprint(this.data.id),
        api.listComments(this.data.id),
        api.getRelatedBlueprints(this.data.id),
      ])
      const images = (bp.images || []).filter((im) => im.file_type !== 'pdf')
        .map((im) => ({ ...im, absUrl: util.resolveUrl(im.url) }))
      const pdfs = (bp.images || []).filter((im) => im.file_type === 'pdf')
        .map((im) => ({ ...im, absUrl: util.resolveUrl(im.url) }))
      const descHtml = bp.description ? mdToHtml(bp.description) : ''
      // Precompute display-only fields (WXML can't call helper functions).
      bp.difficultyLabel = difficultyText(bp.difficulty)
      bp.catEmoji = categoryEmoji(bp.category)
      bp.viewText = util.formatCount(bp.view_count)
      bp.likeText = util.formatCount(bp.like_count)
      bp.favText = util.formatCount(bp.favorite_count)
      bp.createdText = util.formatDate(bp.created_at)
      bp.authorAvatarUrl = util.resolveUrl(bp.author && bp.author.avatar_url)
      let partList = null
      if (bp.part_list) {
        try { partList = typeof bp.part_list === 'string' ? JSON.parse(bp.part_list) : bp.part_list } catch (e) { partList = null }
      }
      this.setData({
        bp,
        images,
        pdfs,
        descHtml,
        partList,
        related: (relatedRes.items || []),
        comments: this.buildCommentTree(comments || []),
        loading: false,
      })
      wx.setNavigationBarTitle({ title: bp.title || '图纸详情' })
    } catch (e) {
      this.setData({ loading: false, error: (e && e.message) || '加载失败' })
    }
  },

  buildCommentTree(flat) {
    const byId = {}
    flat.forEach((c) => {
      byId[c.id] = Object.assign({}, c, {
        replies: [],
        avatarUrl: util.resolveUrl(c.user && c.user.avatar_url),
        timeText: util.timeAgo(c.created_at),
      })
    })
    const roots = []
    flat.forEach((c) => {
      if (c.parent_id && byId[c.parent_id]) byId[c.parent_id].replies.push(byId[c.id])
      else roots.push(byId[c.id])
    })
    return roots
  },

  // ── Like ──
  async onLike() {
    if (this.data.liking) return
    try { await auth.ensureLogin() } catch (e) { return }
    const bp = this.data.bp
    this.setData({ liking: true })
    try {
      if (bp.is_liked) {
        await api.unlikeBlueprint(bp.id)
        this.setData({ 'bp.is_liked': false, 'bp.like_count': Math.max(0, bp.like_count - 1), liking: false })
      } else {
        const res = await api.likeBlueprint(bp.id)
        this.setData({ 'bp.is_liked': true, 'bp.like_count': (res && res.like_count) || (bp.like_count + 1), liking: false })
      }
    } catch (e) {
      this.setData({ liking: false })
      wx.showToast({ title: (e && e.message) || '操作失败', icon: 'none' })
    }
  },

  // ── Favorite ──
  async onFavorite() {
    if (this.data.favving) return
    try { await auth.ensureLogin() } catch (e) { return }
    const bp = this.data.bp
    this.setData({ favving: true })
    try {
      if (bp.is_favorited) {
        await api.unfavoriteBlueprint(bp.id)
        this.setData({ 'bp.is_favorited': false, 'bp.favorite_count': Math.max(0, bp.favorite_count - 1), favving: false })
      } else {
        await api.favoriteBlueprint(bp.id)
        this.setData({ 'bp.is_favorited': true, 'bp.favorite_count': bp.favorite_count + 1, favving: false })
      }
    } catch (e) {
      this.setData({ favving: false })
      const msg = (e && e.status === 409) ? '已收藏' : ((e && e.message) || '操作失败')
      wx.showToast({ title: msg, icon: 'none' })
    }
  },

  // ── Comments ──
  onCommentInput(e) { this.setData({ commentText: e.detail.value }) },
  startReply(e) {
    const { id, name } = e.currentTarget.dataset
    this.setData({ replyTo: { id, username: name } })
  },
  cancelReply() { this.setData({ replyTo: null }) },
  async sendComment() {
    const content = (this.data.commentText || '').trim()
    if (!content || this.data.sendingComment) return
    try { await auth.ensureLogin() } catch (e) { return }
    this.setData({ sendingComment: true })
    try {
      const created = await api.createComment(this.data.id, {
        content,
        parent_id: this.data.replyTo ? this.data.replyTo.id : null,
      })
      const comments = this.data.comments.slice()
      const node = Object.assign({}, created, {
        replies: [],
        avatarUrl: util.resolveUrl(created.user && created.user.avatar_url),
        timeText: util.timeAgo(created.created_at),
      })
      if (this.data.replyTo) {
        const parent = comments.find((c) => c.id === this.data.replyTo.id)
        if (parent) parent.replies.push(node)
      } else {
        comments.push(node)
      }
      this.setData({ comments, commentText: '', replyTo: null, sendingComment: false })
    } catch (e) {
      this.setData({ sendingComment: false })
      wx.showToast({ title: (e && e.message) || '发送失败', icon: 'none' })
    }
  },

  // ── Report ──
  openReport() { this.setData({ showReport: true, reportReason: '', reportDetail: '' }) },
  closeReport() { this.setData({ showReport: false }) },
  onReportReason(e) { this.setData({ reportReason: e.currentTarget.dataset.value }) },
  onReportDetail(e) { this.setData({ reportDetail: e.detail.value }) },
  async submitReport() {
    if (!this.data.reportReason) { wx.showToast({ title: '请选择举报原因', icon: 'none' }); return }
    try { await auth.ensureLogin() } catch (e) { return }
    try {
      await api.createReport({
        blueprint_id: this.data.id,
        reason: this.data.reportReason,
        detail: this.data.reportDetail || undefined,
      })
      this.setData({ showReport: false })
      wx.showToast({ title: '举报已提交，感谢反馈', icon: 'success' })
    } catch (e) {
      const msg = (e && e.status === 409) ? '已举报过该内容' : ((e && e.message) || '提交失败')
      wx.showToast({ title: msg, icon: 'none' })
    }
  },

  // ── PDF (download + open in system viewer) ──
  openPdf(e) {
    const url = e.currentTarget.dataset.url
    if (!url) return
    wx.showLoading({ title: '下载 PDF...' })
    wx.downloadFile({
      url,
      success: (res) => {
        wx.hideLoading()
        if (res.statusCode === 200) {
          wx.openDocument({
            filePath: res.tempFilePath,
            showMenu: true,
            fail: () => wx.showToast({ title: '无法打开 PDF', icon: 'none' }),
          })
        } else {
          wx.showToast({ title: '下载失败', icon: 'none' })
        }
      },
      fail: () => { wx.hideLoading(); wx.showToast({ title: '下载失败', icon: 'none' }) },
    })
  },

  previewImage(e) {
    const { current } = e.currentTarget.dataset
    const urls = this.data.images.map((im) => im.absUrl)
    wx.previewImage({ current, urls })
  },

  goAuthor() {
    if (this.data.bp && this.data.bp.author && this.data.bp.author.id) {
      wx.navigateTo({ url: '/pages/user/index?id=' + this.data.bp.author.id })
    }
  },

  onShareAppMessage() {
    const bp = this.data.bp
    return {
      title: bp ? bp.title : 'BrickPlan',
      path: '/pages/detail/index?id=' + this.data.id,
    }
  },
})
