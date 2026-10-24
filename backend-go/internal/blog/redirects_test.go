package blog

import (
	"os"
	"path/filepath"
	"testing"
)

// A 301 whose target does not exist is worse than no redirect at all: Google
// follows it into a 404 and drops both URLs. This test runs against the real
// shipped content so a rename cannot leave RetiredENSlugs pointing at a slug
// that the same content sync removed.
func TestRetiredENSlugsResolveToLiveEnglishArticles(t *testing.T) {
	enDir := filepath.Join("..", "..", "content", "en", "blog")
	if _, err := os.Stat(enDir); err != nil {
		t.Skipf("English content not present (%v)", err)
	}
	st, err := LoadDirs(t.TempDir(), enDir)
	if err != nil {
		t.Fatalf("load EN store: %v", err)
	}
	if st.CountByLang(LangEN) == 0 {
		t.Fatal("EN store is empty — fixture path wrong, test would silently pass")
	}

	for retired, target := range RetiredENSlugs {
		if st.GetByLang(retired, LangEN) != nil {
			t.Errorf("%q is listed as retired but still loaded (LoadDirs must skip it)", retired)
		}
		if st.GetByLang(target, LangEN) == nil {
			t.Errorf("%q 301s to %q, which does not exist in the EN corpus", retired, target)
		}
		if target == retired {
			t.Errorf("%q redirects to itself", retired)
		}
	}
}
