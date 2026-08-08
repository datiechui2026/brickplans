// Minimal Markdown -> HTML converter for the mini program's <rich-text>.
// WeChat mini programs can't run the web's `marked` + DOM pipeline, and <rich-text>
// accepts an HTML string. This handles the common subset used in blueprint
// descriptions: headings, bold/italic/inline-code, links, images, lists, and
// paragraphs. Relative image URLs are resolved against the backend origin so
// <img> can load them.
const { API_BASE } = require('./config')

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function resolve(url) {
  if (!url) return ''
  if (/^https?:\/\//i.test(url)) return url
  if (url.startsWith('//')) return 'https:' + url
  return API_BASE + url
}

// Inline formatting: images, links, bold, inline code, italic. Order matters.
function inline(s) {
  return escapeHtml(s)
    .replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (m, alt, url) =>
      `<img src="${resolve(url)}" alt="${alt}" style="max-width:100%;border-radius:8px"/>`)
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (m, text, url) =>
      `<a href="${resolve(url)}" style="color:#ea580c">${text}</a>`)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code style="background:#f3f4f6;padding:2px 6px;border-radius:4px">$1</code>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
}

function mdToHtml(md) {
  if (!md) return ''
  const lines = String(md).split(/\r?\n/)
  let html = ''
  let inUl = false
  let inOl = false
  const closeLists = () => {
    if (inUl) { html += '</ul>'; inUl = false }
    if (inOl) { html += '</ol>'; inOl = false }
  }
  for (const raw of lines) {
    const line = raw.replace(/\s+$/, '')
    if (!line.trim()) { closeLists(); continue }
    if (/^######\s/.test(line)) { closeLists(); html += `<h6>${inline(line.replace(/^######\s/, ''))}</h6>` }
    else if (/^#####\s/.test(line)) { closeLists(); html += `<h5>${inline(line.replace(/^#####\s/, ''))}</h5>` }
    else if (/^####\s/.test(line)) { closeLists(); html += `<h4>${inline(line.replace(/^####\s/, ''))}</h4>` }
    else if (/^###\s/.test(line)) { closeLists(); html += `<h3>${inline(line.replace(/^###\s/, ''))}</h3>` }
    else if (/^##\s/.test(line)) { closeLists(); html += `<h2>${inline(line.replace(/^##\s/, ''))}</h2>` }
    else if (/^#\s/.test(line)) { closeLists(); html += `<h1>${inline(line.replace(/^#\s/, ''))}</h1>` }
    else if (/^[-*+]\s+/.test(line)) {
      if (!inUl) { closeLists(); html += '<ul>'; inUl = true }
      html += `<li>${inline(line.replace(/^[-*+]\s+/, ''))}</li>`
    } else if (/^\d+\.\s+/.test(line)) {
      if (!inOl) { closeLists(); html += '<ol>'; inOl = true }
      html += `<li>${inline(line.replace(/^\d+\.\s+/, ''))}</li>`
    } else if (/^>\s?/.test(line)) {
      closeLists()
      html += `<p style="color:#6b7280;border-left:4px solid #e5e7eb;padding-left:12px">${inline(line.replace(/^>\s?/, ''))}</p>`
    } else if (/^---+$/.test(line)) {
      closeLists(); html += '<hr style="border:none;border-top:1px solid #e5e7eb;margin:16px 0"/>'
    } else {
      closeLists()
      html += `<p>${inline(line)}</p>`
    }
  }
  closeLists()
  return html
}

module.exports = { mdToHtml }
