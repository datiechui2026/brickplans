const util = require('../../utils/util')
const { difficultyText, categoryEmoji } = require('../../utils/config')

// blueprint-card: presentational card for a BlueprintOut. Computes display
// fields (absolute cover URL, difficulty label, short counts) from the raw
// blueprint so pages can pass items straight through.
Component({
  properties: {
    blueprint: { type: Object, value: null },
  },
  data: {
    cover: '',
    emoji: '📦',
    difficulty: '',
    views: '0',
    likes: '0',
  },
  observers: {
    blueprint(bp) {
      if (!bp) return
      this.setData({
        cover: util.resolveUrl(bp.cover_url),
        emoji: categoryEmoji(bp.category),
        difficulty: difficultyText(bp.difficulty),
        views: util.formatCount(bp.view_count),
        likes: util.formatCount(bp.like_count),
      })
    },
  },
  methods: {
    onTap() {
      const bp = this.data.blueprint
      if (bp && bp.id) wx.navigateTo({ url: '/pages/detail/index?id=' + bp.id })
    },
  },
})
