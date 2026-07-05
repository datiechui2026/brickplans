package storage

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/tencentyun/cos-go-sdk-v5"

	"brickplans/internal/config"
)

// StoredObject is the metadata persisted by callers alongside the DB record.
// ObjectKey is kept server-side only (never returned to the client) and is what
// Delete needs.
type StoredObject struct {
	URL       string
	ObjectKey string
}

type Storage interface {
	Upload(data []byte, filename, contentType, prefix string) (*StoredObject, error)
	Delete(urlOrKey string) error
}

// makeObjectKey builds a server-generated key. The extension is preserved but the
// caller is expected to pass a normalized, safe extension (e.g. ".jpg" / ".pdf");
// the random UUID prefix makes the path unpredictable and not user-controlled.
func makeObjectKey(filename, prefix string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "blueprints"
	}
	return fmt.Sprintf("%s/%s%s", prefix, uuid.NewString()[:12], ext)
}

func objectKeyFromURLOrKey(urlOrKey, publicBaseURL string) string {
	v := strings.TrimSpace(urlOrKey)
	if publicBaseURL != "" {
		base := strings.TrimRight(publicBaseURL, "/") + "/"
		if strings.HasPrefix(v, base) {
			return v[len(base):]
		}
	}
	if strings.HasPrefix(v, "/uploads/") {
		return v[len("/uploads/"):]
	}
	return strings.TrimLeft(v, "/")
}

// ── LocalStorage ─────────────────────────────────────

type LocalStorage struct {
	uploadDir  string
	publicBase string
}

func NewLocalStorage(cfg *config.Config) (*LocalStorage, error) {
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{uploadDir: cfg.UploadDir, publicBase: "/uploads"}, nil
}

func (s *LocalStorage) Upload(data []byte, filename, contentType, prefix string) (*StoredObject, error) {
	key := makeObjectKey(filename, prefix)
	full := filepath.Join(s.uploadDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return nil, err
	}
	return &StoredObject{URL: s.publicBase + "/" + key, ObjectKey: key}, nil
}

func (s *LocalStorage) Delete(urlOrKey string) error {
	key := objectKeyFromURLOrKey(urlOrKey, "")
	full := filepath.Join(s.uploadDir, filepath.FromSlash(key))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ServeFile serves an uploaded file by its URL path (e.g. "/avatars/xxx.jpg").
// PDFs are forced to Content-Disposition: inline; a strict nosniff header is
// applied so the browser honors the declared Content-Type (defeats MIME sniffing
// of uploaded .html/.svg as executable content). Path traversal is rejected.
func (s *LocalStorage) ServeFile(w http.ResponseWriter, r *http.Request, urlPath string) {
	rel := strings.TrimPrefix(urlPath, "/")
	uploadAbs, _ := filepath.Abs(s.uploadDir)
	abs, _ := filepath.Abs(filepath.Join(uploadAbs, filepath.FromSlash(rel)))
	if !strings.HasPrefix(abs, uploadAbs+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(abs))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ext == ".pdf" {
		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Type", "application/pdf")
	} else if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, abs)
}

// ── TencentCOSStorage ────────────────────────────────

type TencentCOSStorage struct {
	client        *cos.Client
	publicBaseURL string
}

func NewTencentCOSStorage(cfg *config.Config) (*TencentCOSStorage, error) {
	base := strings.TrimRight(cfg.TencentCOSPublicBaseURL, "/")
	if base == "" {
		base = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.TencentCOSBucket, cfg.TencentCOSRegion)
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.TencentCOSSecretID,
			SecretKey: cfg.TencentCOSSecretKey,
		},
	})
	return &TencentCOSStorage{client: client, publicBaseURL: base}, nil
}

func (s *TencentCOSStorage) Upload(data []byte, filename, contentType, prefix string) (*StoredObject, error) {
	key := makeObjectKey(filename, prefix)
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:        contentType,
			ContentDisposition: "",
		},
		ACLHeaderOptions: &cos.ACLHeaderOptions{XCosACL: "public-read"},
	}
	if contentType == "application/pdf" {
		opt.ObjectPutHeaderOptions.ContentDisposition = "inline"
	}
	if _, err := s.client.Object.Put(context.Background(), key, bytes.NewReader(data), opt); err != nil {
		return nil, err
	}
	return &StoredObject{URL: s.publicBaseURL + "/" + key, ObjectKey: key}, nil
}

func (s *TencentCOSStorage) Delete(urlOrKey string) error {
	key := objectKeyFromURLOrKey(urlOrKey, s.publicBaseURL)
	_, _ = s.client.Object.Delete(context.Background(), key)
	return nil
}

// ── Singleton ────────────────────────────────────────

var (
	instance Storage
	initOnce sync.Once
	initErr  error
)

// Get returns the singleton storage backend (local or tencent_cos).
func Get(cfg *config.Config) (Storage, error) {
	initOnce.Do(func() {
		if cfg.StorageBackend == "tencent_cos" {
			instance, initErr = NewTencentCOSStorage(cfg)
		} else {
			instance, initErr = NewLocalStorage(cfg)
		}
	})
	return instance, initErr
}

// Reset clears the storage singleton. Intended for tests only.
func Reset() {
	instance = nil
	initErr = nil
	initOnce = sync.Once{}
}
