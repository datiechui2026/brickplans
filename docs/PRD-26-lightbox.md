# PRD-26：作品图片灯箱预览

> **目标**：详情页图片画廊支持点击放大查看，可缩放/拖动/切换。

---

## 一、触发入口

**主图** `mainImgEl` 添加 `onclick` → 打开灯箱，传当前图片索引。

**缩略图**也加灯箱触发（和主图共用）。

---

## 二、灯箱组件

### 2.1 全屏遮罩

```css
.lightbox-overlay {
  position: fixed; inset: 0; z-index: 1000;
  background: rgba(0,0,0,.92);
  display: flex; align-items: center; justify-content: center;
}
```

### 2.2 图片行为

| 操作 | 行为 |
|------|------|
| 初始状态 | fit-to-screen（`max-width:90vw; max-height:85vh; object-fit:contain`） |
| 单击图片 | 切换 1x / fit 模式 |
| 滚轮 | 缩放（0.5x ~ 5x） |
| 拖拽 | 缩放后可用鼠标拖拽平移 |
| 左右箭头 | 切换上一张/下一张（仅多图时显示） |
| 键盘 ←→ | 切换图片 |
| Esc / 点遮罩 | 关闭 |
| 关闭按钮 | 右上角 ✕ |

### 2.3 实现方案

```
openLightbox(images, index)
├── 创建 .lightbox-overlay
├── 渲染当前图片 + 左右箭头 + ✕ 按钮 + 页码 (3/12)
├── 绑定事件：click toggle zoom、wheel zoom、mousedown drag、keydown、click overlay close
└── 移动端：touchstart/touchmove 双指缩放 + 单指拖拽
```

**缩放/平移用 CSS transform**：`transform: scale(${zoom}) translate(${dx}px, ${dy}px); transform-origin: center;`

### 2.4 CSS 关键样式

```css
.lightbox-overlay { ... }
.lightbox-close {
  position: fixed; top: 16px; right: 16px;
  width: 44px; height: 44px; font-size: 28px;
  color: #fff; background: rgba(0,0,0,.4); border: none;
  border-radius: 50%; cursor: pointer; z-index: 1001;
}
.lightbox-img-wrap {
  display: flex; align-items: center; justify-content: center;
  width: 100%; height: 100%;
}
.lightbox-img {
  max-width: 90vw; max-height: 85vh;
  object-fit: contain; cursor: zoom-in;
  transition: transform .2s;
}
.lightbox-img.zoomed { cursor: grab; }
.lightbox-img.zoomed.dragging { cursor: grabbing; }
.lightbox-nav {
  position: fixed; top: 50%; transform: translateY(-50%);
  width: 44px; height: 44px; font-size: 24px;
  color: #fff; background: rgba(0,0,0,.4); border: none;
  border-radius: 50%; cursor: pointer; z-index: 1001;
}
.lightbox-nav.prev { left: 16px; }
.lightbox-nav.next { right: 16px; }
.lightbox-counter {
  position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%);
  color: rgba(255,255,255,.7); font-size: 13px; font-weight: 600;
  z-index: 1001;
}
```

---

## 三、验收标准

- [ ] 点击详情页主图 → 全屏灯箱，显示当前图片
- [ ] 单击切换 fit/1x 缩放
- [ ] 鼠标滚轮缩放（0.5x~5x）
- [ ] 缩放后可拖拽平移
- [ ] 多图时显示 ← → 箭头，可切换
- [ ] 键盘 ←→ 切换，Esc 关闭
- [ ] 点遮罩背景关闭
- [ ] 右上角 ✕ 关闭
- [ ] 底部显示当前页/总页数
- [ ] 移动端双指缩放 + 单指拖拽
