package ssr

import (
	"bytes"
	"encoding/json"
	"html/template"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// mdToHTML converts a markdown string to safe HTML for SSR rendering.
// Used for blueprint descriptions and blog posts (trusted content).
//
// Images are decorated on the way out (see decorateImages): every <img> gets
// real width/height plus decoding/loading hints, and the first image in the
// document gets eager+fetchpriority=high because it is the LCP element on
// article pages. Doing it here means one change covers every markdown surface
// (blog zh/en, blueprint descriptions, noscript bodies) without touching a
// single .md file.
func mdToHTML(md string) template.HTML {
	if md == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert([]byte(md), &buf); err != nil {
		return template.HTML(esc(md))
	}
	return decorateImages(buf.String())
}

// ── image dimensions ──

// imgDim is a resolved intrinsic size for one image URL.
type imgDim struct {
	W int `json:"w"`
	H int `json:"h"`
}

var (
	imgDimsMu     sync.RWMutex
	imgDimByURL   = map[string]imgDim{}
	imgDimsLoaded bool
)

// imageDimsCandidates are tried in order by the lazy loader. The env var comes
// first so production can point at an absolute path; the relative defaults keep
// `go test` and a plain `go run` working from backend-go/.
var imageDimsCandidates = []string{
	os.Getenv("IMAGE_DIMS_FILE"),
	"./content/image-dims.json",
	"../content/image-dims.json",
	"content/image-dims.json",
}

// LoadImageDims reads the URL → {w,h} manifest produced by tools/imgdims.py.
// Missing file is not an error: pages still render, images just lose the
// width/height pair (and the CLS win) until the manifest is deployed.
func LoadImageDims(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m := map[string]imgDim{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	imgDimsMu.Lock()
	imgDimByURL = m
	imgDimsLoaded = true
	imgDimsMu.Unlock()
	return nil
}

// ensureImageDims runs the candidate list once. Callers never need to care
// whether the manifest exists.
func ensureImageDims() {
	imgDimsMu.RLock()
	loaded := imgDimsLoaded
	imgDimsMu.RUnlock()
	if loaded {
		return
	}
	imgDimsMu.Lock()
	defer imgDimsMu.Unlock()
	if imgDimsLoaded {
		return
	}
	imgDimsLoaded = true // only one attempt; a missing file is not an error
	for _, p := range imageDimsCandidates {
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		m := map[string]imgDim{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		imgDimByURL = m
		return
	}
}

// imageSize returns the intrinsic size for an image URL. The COS host may
// appear with a different scheme/query in the wild, so look up the exact URL
// first and fall back to a delimiter-stripped key.
func imageSize(src string) (int, int, bool) {
	if src == "" {
		return 0, 0, false
	}
	ensureImageDims()
	imgDimsMu.RLock()
	defer imgDimsMu.RUnlock()
	if d, ok := imgDimByURL[src]; ok && d.W > 0 && d.H > 0 {
		return d.W, d.H, true
	}
	if i := strings.IndexAny(src, "?#"); i > 0 {
		if d, ok := imgDimByURL[src[:i]]; ok && d.W > 0 && d.H > 0 {
			return d.W, d.H, true
		}
	}
	return 0, 0, false
}

// ── <img> decoration ──

// decorateImages rewrites every <img> in an already-rendered markdown fragment:
//
//   - width/height from the dimension manifest (real intrinsic px, so the
//     browser reserves the box before the bytes arrive → CLS ~ 0)
//   - decoding="async" on all images
//   - first image: loading="eager" + fetchpriority="high" (LCP candidate)
//   - every other image: loading="lazy" (below-the-fold)
//
// HTML is round-tripped through x/net/html instead of a regexp so attribute
// quoting/escaping in alt text cannot be corrupted.
func decorateImages(fragment string) template.HTML {
	if !strings.Contains(fragment, "<img") {
		return template.HTML(fragment)
	}
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		return template.HTML(fragment)
	}
	first := true
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			decorateImg(n, first)
			first = false
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	var out bytes.Buffer
	for _, n := range nodes {
		if err := html.Render(&out, n); err != nil {
			return template.HTML(fragment)
		}
	}
	return template.HTML(out.String())
}

func decorateImg(n *html.Node, isFirst bool) {
	src := ""
	nativeLazy := false
	for _, a := range n.Attr {
		switch a.Key {
		case "src":
			src = a.Val
		case "loading":
			nativeLazy = nativeLazy || a.Val == "lazy"
		}
	}
	if w, h, ok := imageSize(src); ok {
		setAttr(n, "width", strconv.Itoa(w))
		setAttr(n, "height", strconv.Itoa(h))
	}
	setAttr(n, "decoding", "async")
	if isFirst {
		setAttr(n, "loading", "eager")
		setAttr(n, "fetchpriority", "high")
	} else if !nativeLazy {
		setAttr(n, "loading", "lazy")
	}
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}
