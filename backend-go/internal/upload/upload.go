package upload

import (
	"bytes"
	"errors"

	"github.com/disintegration/imaging"

	// Register decoders for the image formats we accept.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	_ "golang.org/x/image/webp"
)

const (
	MaxAvatarSize   = 2 << 20    // 2 MB
	MaxImageSize    = 20 << 20   // 20 MB
	MaxPDFSize      = 20 << 20   // 20 MB
	MaxFilesPerBatch = 10        // per blueprint upload request
)

var (
	ErrEmpty      = errors.New("文件为空")
	ErrTooLarge   = errors.New("文件过大")
	ErrUnsupported = errors.New("不支持的文件格式，仅允许 JPG/PNG/WebP/PDF")
	ErrBadImage   = errors.New("无法解码图片")
)

// DetectImageType sniffs known image magic bytes. Returns the type and true on a match.
func DetectImageType(b []byte) (string, bool) {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png", true
	case len(b) >= 3 && bytes.Equal(b[:3], []byte("\xff\xd8\xff")):
		return "jpeg", true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp", true
	case len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a"))):
		return "gif", true
	}
	return "", false
}

// IsPDF reports whether the bytes begin with the PDF magic.
func IsPDF(b []byte) bool { return len(b) >= 4 && bytes.Equal(b[:4], []byte("%PDF")) }

// ReencodeImage decodes the image and re-encodes it as JPEG (quality 80, resized
// so the longest side ≤ maxDim). Re-encoding strips embedded metadata and any
// payload that is not pixel data — a PNG/SVG carrying an embedded script becomes
// inert pixels. Returns ErrBadImage if the bytes are not a decodable image.
func ReencodeImage(data []byte, maxDim, quality int) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, ErrBadImage
	}
	if maxDim > 0 {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		if w > maxDim || h > maxDim {
			if w >= h {
				img = imaging.Resize(img, maxDim, 0, imaging.Lanczos)
			} else {
				img = imaging.Resize(img, 0, maxDim, imaging.Lanczos)
			}
		}
	}
	buf := &bytes.Buffer{}
	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ProcessFile validates and normalizes an uploaded file for storage.
//   - PDF: stored as-is (Go has no lightweight JS-stripping lib; we rely on the
//     browser PDF sandbox in dev and on cross-origin COS delivery in prod).
//   - Image (PNG/JPEG/WebP/GIF by magic bytes): re-encoded to JPEG, stripping
//     any embedded payload. The caller stores it under a forced ".jpg" key.
//   - Anything else: rejected (ErrUnsupported). Decoding failure: rejected
//     (ErrBadImage) — never falls back to storing the raw bytes.
//
// Returns the bytes to store plus the file_type, Content-Type and extension to use.
func ProcessFile(data []byte, filename string) (content []byte, fileType, contentType, ext string, err error) {
	if len(data) == 0 {
		return nil, "", "", "", ErrEmpty
	}
	if len(data) > MaxImageSize {
		return nil, "", "", "", ErrTooLarge
	}
	if IsPDF(data) {
		return data, "pdf", "application/pdf", ".pdf", nil
	}
	if _, ok := DetectImageType(data); ok {
		reencoded, e := ReencodeImage(data, 2048, 80)
		if e != nil {
			return nil, "", "", "", ErrBadImage
		}
		return reencoded, "image", "image/jpeg", ".jpg", nil
	}
	return nil, "", "", "", ErrUnsupported
}
