// Central config for the BrickPlan mini program.
//
// API_BASE: the backend's public origin (the same server the web SPA uses).
//   - In dev, set WeChat DevTools to "不校验合法域名" (details -> local settings).
//   - In production, register this domain under "request合法域名" AND
//     "downloadFile合法域名" in the WeChat console (mp.weixin.qq.com).
const API_BASE = 'https://brickplan.cn'

// Category list mirrors the web frontend (search/explore filters + emojis).
const CATEGORIES = [
  { value: '', label: '全部', emoji: '🏠' },
  { value: '建筑', label: '建筑', emoji: '🏰' },
  { value: '车辆', label: '车辆', emoji: '🚗' },
  { value: '机甲', label: '机甲', emoji: '🤖' },
  { value: '奇幻', label: '奇幻', emoji: '🐉' },
  { value: '科幻', label: '科幻', emoji: '🛸' },
  { value: '场景', label: '场景', emoji: '🎨' },
]

const DIFFICULTY_LABELS = ['', '简单', '初级', '中等', '困难', '专家']

function difficultyText(d) {
  if (!d) return '未设置'
  return (DIFFICULTY_LABELS[d] || '未设置') + ' ' + '⭐'.repeat(d)
}

function categoryEmoji(cat) {
  const c = CATEGORIES.find((x) => x.value === cat)
  return c ? c.emoji : '📦'
}

// Report reasons (must match the backend's reports handler enum).
const REPORT_REASONS = [
  { value: 'inappropriate', label: '内容不当' },
  { value: 'copyright', label: '版权问题' },
  { value: 'incomplete', label: '图纸不完整' },
  { value: 'spam', label: '垃圾广告' },
  { value: 'other', label: '其他' },
]

module.exports = {
  API_BASE,
  CATEGORIES,
  DIFFICULTY_LABELS,
  difficultyText,
  categoryEmoji,
  REPORT_REASONS,
}
