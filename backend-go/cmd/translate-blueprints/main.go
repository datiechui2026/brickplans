// Command translate-blueprints fills the English columns (title_en,
// description_en) of every blueprint from its Chinese title/description.
//
// Why a command and not a one-off SQL file: the translation has to come from a
// model (machine translation services mangle brick terminology and brand
// names), the work must be resumable, and it has to be safe to re-run. Rows that
// already have both English fields are skipped, so the job can be resumed after
// an interruption without paying for the same text twice.
//
// Usage (on the server, from backend-go/, where .env lives):
//
//	go run ./cmd/translate-blueprints                  # translate everything missing
//	go run ./cmd/translate-blueprints --dry-run        # show what would be sent
//	go run ./cmd/translate-blueprints --limit 20       # next 20 rows only
//	go run ./cmd/translate-blueprints --force          # retranslate everything
//	go run ./cmd/translate-blueprints --batch 5        # smaller API batches
//
// Requires MYSQL_DSN (same .env as the backend) and ANTHROPIC_API_KEY.
// ANTHROPIC_BASE_URL / ANTHROPIC_MODEL are optional and match the settings the
// English blog rewrite pipeline already uses (scripts/en_rewrite.py).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

const (
	// titleLimit mirrors Blueprint.TitleEN's varchar(100).
	titleLimit = 100
	// descBytesLimit stays below MySQL TEXT's 65535-byte cap, leaving room for
	// multi-byte UTF-8 characters.
	descBytesLimit = 60000
	defaultBatch   = 10
	maxOutputToken = 8000
	requestTimeout = 4 * time.Minute
)

// row is the subset of blueprints this command reads and writes.
type row struct {
	ID            string  `gorm:"column:id"`
	Title         string  `gorm:"column:title"`
	Description   *string `gorm:"column:description"`
	TitleEN       *string `gorm:"column:title_en"`
	DescriptionEN *string `gorm:"column:description_en"`
}

type translation struct {
	TitleEN       string `json:"title_en"`
	DescriptionEN string `json:"description_en"`
}

type translationItem struct {
	ID string `json:"id"`
	translation
}

type translationResponse struct {
	Items []translationItem `json:"items"`
}

func main() {
	var (
		dryRun  = flag.Bool("dry-run", false, "print the rows that would be translated, call no API and write nothing")
		force   = flag.Bool("force", false, "retranslate rows that already have English fields")
		limit   = flag.Int("limit", 0, "translate at most N blueprints (0 = all)")
		batch   = flag.Int("batch", defaultBatch, "blueprints per API request")
		dsnFlag = flag.String("dsn", "", "MySQL DSN (defaults to MYSQL_DSN from .env/environment)")
		model   = flag.String("model", "", "model id (defaults to ANTHROPIC_MODEL or "+defaultModel+")")
	)
	flag.Parse()

	if *batch < 1 {
		log.Fatal("--batch must be >= 1")
	}

	cfg := config.Load()
	dsn := cfg.MySQLDSN
	if *dsnFlag != "" {
		dsn = *dsnFlag
	}
	if dsn == "" {
		log.Fatal("MYSQL_DSN is empty - run this on the server (backend-go/.env) or pass --dsn")
	}

	gdb, err := db.Open(dsn, cfg.AppEnv)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	// The command may run before the new binary has started once; make sure the
	// two columns exist so it cannot fail on a database that predates them.
	if err := gdb.AutoMigrate(&db.Blueprint{}); err != nil {
		log.Fatalf("auto-migrate: %v", err)
	}

	var all []row
	if err := gdb.Model(&db.Blueprint{}).
		Select("id, title, description, title_en, description_en").
		Order("created_at ASC").
		Find(&all).Error; err != nil {
		log.Fatalf("load blueprints: %v", err)
	}

	pending := selectPending(all, *force)
	if *limit > 0 && len(pending) > *limit {
		pending = pending[:*limit]
	}
	log.Printf("blueprints: %d total, %d need translation%s", len(all), len(pending), forceSuffix(*force))
	if len(pending) == 0 {
		return
	}

	if *dryRun {
		for i, r := range pending {
			if i >= 20 {
				log.Printf("... and %d more", len(pending)-i)
				break
			}
			log.Printf("[dry-run] %s | %s", r.ID, r.Title)
		}
		return
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set")
	}
	modelID := *model
	if modelID == "" {
		modelID = envOr("ANTHROPIC_MODEL", defaultModel)
	}
	client := newClient(envOr("ANTHROPIC_BASE_URL", "https://api.anthropic.com"), apiKey, modelID)

	translated, skipped, failed := 0, 0, 0
	for start := 0; start < len(pending); start += *batch {
		end := start + *batch
		if end > len(pending) {
			end = len(pending)
		}
		chunk := pending[start:end]
		log.Printf("[%d/%d] translating %d blueprint(s)", end, len(pending), len(chunk))

		results, err := client.translateRows(chunk)
		if err != nil {
			log.Printf("[ERROR] batch %d-%d: %v (will retry on the next run)", start+1, end, err)
			failed += len(chunk)
			continue
		}

		for _, r := range chunk {
			tr, ok := results[r.ID]
			if !ok {
				log.Printf("[WARN] %s: model returned no translation", r.ID)
				failed++
				continue
			}
			title, ok := normalizeTitle(tr.TitleEN)
			if !ok {
				log.Printf("[WARN] %s: empty English title, left untranslated", r.ID)
				failed++
				continue
			}
			updates := map[string]any{}
			if needsTitle(r, *force) {
				updates["title_en"] = title
			}
			if needsDescription(r, *force) {
				desc, ok := normalizeDescription(tr.DescriptionEN)
				if !ok {
					log.Printf("[WARN] %s: empty English description, left untranslated", r.ID)
					failed++
					continue
				}
				updates["description_en"] = desc
			}
			if len(updates) == 0 {
				skipped++
				continue
			}
			if err := gdb.Model(&db.Blueprint{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
				log.Printf("[ERROR] %s: update: %v", r.ID, err)
				failed++
				continue
			}
			translated++
			log.Printf("[OK] %s -> %s", r.Title, title)
		}
	}

	log.Printf("done: %d translated, %d skipped, %d failed", translated, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

const defaultModel = "claude-sonnet-5-5"

// selectPending returns the rows that still need at least one English field.
// A row whose Chinese description is blank does not need description_en; a row
// with both fields already filled is skipped unless force is set.
func selectPending(rows []row, force bool) []row {
	out := make([]row, 0, len(rows))
	for _, r := range rows {
		if force || needsTitle(r, force) || needsDescription(r, force) {
			out = append(out, r)
		}
	}
	return out
}

func needsTitle(r row, force bool) bool {
	return force || blank(r.TitleEN)
}

func needsDescription(r row, force bool) bool {
	if blank(r.Description) {
		return false
	}
	return force || blank(r.DescriptionEN)
}

func blank(s *string) bool {
	return s == nil || strings.TrimSpace(*s) == ""
}

func forceSuffix(force bool) string {
	if force {
		return " (--force: all rows)"
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// normalizeTitle trims the model output and enforces the DB's 100-character
// limit. Oversized titles are cut at a word boundary instead of failing the
// whole row.
func normalizeTitle(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", false
	}
	if utf8.RuneCountInString(s) <= titleLimit {
		return s, true
	}
	runes := []rune(s)
	cut := string(runes[:titleLimit])
	if i := strings.LastIndexByte(cut, ' '); i >= titleLimit/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut), true
}

// normalizeDescription trims outer whitespace and guards the MySQL TEXT limit.
// Inner line breaks and markdown are preserved.
func normalizeDescription(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if len(s) > descBytesLimit {
		// Cut on a rune boundary and add an ellipsis so the text stays valid
		// UTF-8; a translation this long is pathological anyway.
		cut := s[:descBytesLimit]
		for len(cut) > 0 && !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
		s = strings.TrimSpace(cut) + "…"
	}
	return s, true
}

// ── Anthropic client ────────────────────────────────────────────────────────

type client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func newClient(baseURL, apiKey, model string) *client {
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

const systemPrompt = `You are a professional Chinese-to-English translator for BrickPlan, a brick/MOC blueprint sharing community. You translate blueprint titles and descriptions. Output only a single JSON object; never add commentary or code fences.`

func buildUserPrompt(rows []row) string {
	type input struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	in := make([]input, 0, len(rows))
	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		in = append(in, input{ID: r.ID, Title: r.Title, Description: desc})
	}
	payload, _ := json.Marshal(in)

	return `Translate the Chinese fields of every item below into natural, accurate English.

Rules:
- Translate faithfully: keep every fact, number, piece count, dimension, set/model number, URL and image link unchanged. Do not add, omit, or turn the text into marketing copy.
- Preserve the markdown structure of the description exactly (headings, lists, bold, blank lines). Translate the prose inside it.
- Use official English brick-brand names: 乐高=LEGO, 森宝=SEMBO, 双鹰=Double Eagle, 万格=Wange, 邦宝=BanBao, 沃马=Woma, 开智=KAZI, 启蒙=QMAN, 布鲁可=Bloks, 潘洛斯=Panlos, 宇星模王=Mould King, 星堡=Xingbao, 奇积=Qiji, 卫积=Weiji, 咔搭=CaDA, 双钻=Shuangzuan, 得高=DECOOL, 小鲁班=Sluban, 高砖=Gobricks, 星钻=Star Diamond, 酷彼=Kubi, 美高=Mega Bloks, 小米=Xiaomi. For any other Chinese brand name use its common English name if it has one, otherwise keep the original name.
- title_en: one line, at most 100 characters, no surrounding quotes.
- description_en: keep the original markdown line breaks. If the input description is empty, return an empty string.
- Return every input id exactly once and nothing else.

Input JSON:
` + string(payload) + `

Output schema:
{"items":[{"id":"<input id>","title_en":"<English title>","description_en":"<English description>"}]}`
}

// translateRows translates one batch, splitting it in half when the model hits
// the output-token ceiling so a long description cannot fail its neighbours.
func (c *client) translateRows(rows []row) (map[string]translation, error) {
	if len(rows) == 0 {
		return map[string]translation{}, nil
	}
	text, stop, err := c.complete(systemPrompt, buildUserPrompt(rows))
	if err != nil {
		return nil, err
	}
	if stop == "max_tokens" {
		if len(rows) == 1 {
			return nil, errors.New("model output truncated on a single blueprint; raise maxOutputToken")
		}
		mid := len(rows) / 2
		left, err := c.translateRows(rows[:mid])
		if err != nil {
			return nil, err
		}
		right, err := c.translateRows(rows[mid:])
		if err != nil {
			return nil, err
		}
		for id, tr := range right {
			left[id] = tr
		}
		return left, nil
	}

	parsed, err := parseTranslationResponse(text)
	if err != nil {
		return nil, err
	}
	out := make(map[string]translation, len(parsed.Items))
	for _, item := range parsed.Items {
		out[item.ID] = item.translation
	}
	return out, nil
}

func parseTranslationResponse(text string) (*translationResponse, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	start, end := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in model response: %.200s", text)
	}
	var resp translationResponse
	if err := json.Unmarshal([]byte(text[start:end+1]), &resp); err != nil {
		return nil, fmt.Errorf("parse model response: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, errors.New("model returned no items")
	}
	return &resp, nil
}

type apiRequest struct {
	Model     string   `json:"model"`
	MaxTokens int      `json:"max_tokens"`
	System    string   `json:"system"`
	Messages  []apiMsg `json:"messages"`
}

type apiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// complete calls the Messages API with retries for rate limits and transient
// server errors. It returns the concatenated text blocks and the stop reason.
func (c *client) complete(system, user string) (string, string, error) {
	payload, err := json.Marshal(apiRequest{
		Model:     c.model,
		MaxTokens: maxOutputToken,
		System:    system,
		Messages:  []apiMsg{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", "", err
	}

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}
		req, err := http.NewRequest(http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(payload))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("content-type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %.300s", resp.StatusCode, body)
			continue
		}
		if resp.StatusCode != 200 {
			return "", "", fmt.Errorf("HTTP %d: %.300s", resp.StatusCode, body)
		}

		var out apiResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return "", "", fmt.Errorf("decode API response: %w", err)
		}
		var sb strings.Builder
		for _, block := range out.Content {
			if block.Type == "text" {
				sb.WriteString(block.Text)
			}
		}
		log.Printf("    tokens: in=%d out=%d stop=%s", out.Usage.InputTokens, out.Usage.OutputTokens, out.StopReason)
		return sb.String(), out.StopReason, nil
	}
	return "", "", fmt.Errorf("API request failed after retries: %w", lastErr)
}
