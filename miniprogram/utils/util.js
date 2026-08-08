// Small formatting + URL helpers shared across pages.
const { API_BASE } = require('./config')

// resolveUrl turns a backend-relative URL ("/uploads/...") into an absolute one
// the mini program can load. Absolute URLs (COS CDN, https://...) pass through.
function resolveUrl(url) {
  if (!url) return ''
  if (/^https?:\/\//i.test(url)) return url
  if (url.startsWith('//')) return 'https:' + url
  return API_BASE + url
}

// formatDate: ISO -> "YYYY-MM-DD".
function formatDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const p = (n) => String(n).padStart(2, '0')
  return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate())
}

// timeAgo: ISO -> "3天前" / "2小时前" / "刚刚".
function timeAgo(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const diff = Date.now() - d.getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return min + '分钟前'
  const hr = Math.floor(min / 60)
  if (hr < 24) return hr + '小时前'
  const day = Math.floor(hr / 24)
  if (day < 30) return day + '天前'
  return formatDate(iso)
}

function formatCount(n) {
  n = Number(n) || 0
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}

module.exports = {
  resolveUrl,
  formatDate,
  timeAgo,
  formatCount,
}
