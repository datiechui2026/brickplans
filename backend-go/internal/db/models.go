package db

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NewUUID returns a new UUID v4 string. Used as the primary key for most tables.
func NewUUID() string { return uuid.NewString() }

// User ───────────────────────────────────────────────
type User struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	Username      string     `gorm:"type:varchar(30);uniqueIndex;not null" json:"username"`
	Email         string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	PasswordHash  string     `gorm:"type:varchar(255);not null" json:"-"`
	AvatarURL     *string    `gorm:"type:varchar(500)" json:"avatar_url"`
	Bio           *string    `gorm:"type:text" json:"bio"`
	IsAdmin       bool       `gorm:"default:false" json:"is_admin"`
	TokenVersion  int        `gorm:"not null;default:0" json:"-"`
	EmailVerified bool       `gorm:"not null;default:false" json:"-"`
	Banned        bool       `gorm:"not null;default:false" json:"-"`
	BannedAt      *time.Time `json:"-"`
	// WeChat Mini Program identity. WeChatOpenID uniquely identifies a user within
	// one mini program; WeChatUnionID is set only when the app belongs to a WeChat
	// Open Platform account (shared across the brand's apps). Explicit column names
	// avoid GORM splitting "WeChat" into "we_chat". WeChatOpenID is a pointer so
	// non-WeChat users store NULL - a UNIQUE index allows many NULLs but not many
	// empty strings, so this keeps the uniqueness check correct for everyone.
	WeChatOpenID  *string   `gorm:"type:varchar(64);column:wechat_open_id;uniqueIndex" json:"-"`
	WeChatUnionID string    `gorm:"type:varchar(64);column:wechat_union_id;index" json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"-"`

	Blueprints    []Blueprint    `gorm:"foreignKey:AuthorID;constraint:OnDelete:CASCADE" json:"-"`
	Comments      []Comment      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Reports       []Report       `gorm:"foreignKey:ReporterID;constraint:OnDelete:CASCADE" json:"-"`
	Likes         []Like         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Notifications []Notification `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = NewUUID()
	}
	return nil
}

// Blueprint ──────────────────────────────────────────
type Blueprint struct {
	ID       string `gorm:"type:char(36);primaryKey" json:"id"`
	AuthorID string `gorm:"type:char(36);index;not null" json:"author_id"`
	Title    string `gorm:"type:varchar(100);not null" json:"title"`
	// TitleEN / DescriptionEN are the author-supplied English title and
	// description shown on /en/ pages. Both are optional: when empty the English
	// tree falls back to the Chinese fields, which is what every row uploaded
	// before they existed does.
	TitleEN       *string `gorm:"type:varchar(100)" json:"title_en"`
	Slug          string  `gorm:"type:varchar(120);index;not null" json:"slug"`
	Description   *string `gorm:"type:text" json:"description"`
	DescriptionEN *string `gorm:"type:text" json:"description_en"`
	// DescriptionLong / DescriptionLongEN are the 300-500 character editorial
	// bodies added for ticket 02 batch 2. They are separate columns on purpose:
	// Description stays the short author blurb shared by the card, the API and
	// the meta description, while the long body is per-page unique prose that
	// must never leak into list or search snippets.
	DescriptionLong   *string `gorm:"column:description_long;type:text" json:"description_long"`
	DescriptionLongEN *string `gorm:"column:description_long_en;type:text" json:"description_long_en"`
	Difficulty    *int    `json:"difficulty"`
	PieceCount    *int    `json:"piece_count"`
	Category      *string `gorm:"type:varchar(30);index" json:"category"`
	// Brand is the brick brand the design belongs to (乐高/森宝/双鹰/...). Stored as a
	// plain string so the curated list can grow without a schema change; the frontend
	// renders it as a dropdown, the backend only enforces the length.
	Brand *string `gorm:"type:varchar(30);index" json:"brand"`
	// ProductCode is the manufacturer's set/model number (e.g. 42115). Shown as
	// "编号" on the detail page and "型号" in the upload form.
	ProductCode *string         `gorm:"type:varchar(60);index" json:"product_code"`
	Dimensions  *string         `gorm:"type:varchar(50)" json:"dimensions"`
	PartList    json.RawMessage `gorm:"type:json" json:"part_list"`
	ViewCount   int             `gorm:"not null;default:0" json:"view_count"`
	LikeCount   int             `gorm:"not null;default:0" json:"like_count"`
	IsPublished bool            `gorm:"not null;default:false" json:"is_published"`
	CreatedAt   time.Time       `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`

	Author    *User            `gorm:"foreignKey:AuthorID" json:"-"`
	Images    []BlueprintImage `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"images"`
	Tags      []BlueprintTag   `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
	Comments  []Comment        `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
	Favorites []Favorite       `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
	Likes     []Like           `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
	Reports   []Report         `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
}

// TitleIn returns the title to display in one site language ("zh" | "en").
// English falls back to the Chinese title when the author left title_en empty,
// so the /en/ tree never shows a blank heading.
func (b *Blueprint) TitleIn(lang string) string {
	if lang == "en" && b.TitleEN != nil && strings.TrimSpace(*b.TitleEN) != "" {
		return *b.TitleEN
	}
	return b.Title
}

// DescriptionIn returns the description to display in one site language, with
// the same fallback rule as TitleIn.
func (b *Blueprint) DescriptionIn(lang string) *string {
	if lang == "en" && b.DescriptionEN != nil && strings.TrimSpace(*b.DescriptionEN) != "" {
		return b.DescriptionEN
	}
	return b.Description
}

// DescriptionLongIn returns the long editorial body for one site language.
// Unlike DescriptionIn it never falls back across languages: a Chinese body
// rendered on /en/ is mixed-language content, which is worse for the reader and
// for the index than an omitted block, so an untranslated row simply has no
// long body on the English tree.
func (b *Blueprint) DescriptionLongIn(lang string) *string {
	if lang == "en" {
		if b.DescriptionLongEN != nil && strings.TrimSpace(*b.DescriptionLongEN) != "" {
			return b.DescriptionLongEN
		}
		return nil
	}
	if b.DescriptionLong != nil && strings.TrimSpace(*b.DescriptionLong) != "" {
		return b.DescriptionLong
	}
	return nil
}

func (b *Blueprint) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = NewUUID()
	}
	return nil
}

// BlueprintImage ─────────────────────────────────────
type BlueprintImage struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	BlueprintID string    `gorm:"type:char(36);index;not null" json:"blueprint_id"`
	URL         string    `gorm:"type:varchar(500);not null" json:"url"`
	ObjectKey   string    `gorm:"type:varchar(500);index" json:"-"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	IsCover     bool      `gorm:"not null;default:false" json:"is_cover"`
	FileType    string    `gorm:"type:varchar(10);not null;default:image" json:"file_type"`
	CreatedAt   time.Time `json:"-"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
}

func (i *BlueprintImage) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = NewUUID()
	}
	return nil
}

// Tag ────────────────────────────────────────────────
type Tag struct {
	ID            string         `gorm:"type:char(36);primaryKey" json:"id"`
	Name          string         `gorm:"type:varchar(30);uniqueIndex;not null" json:"name"`
	BlueprintTags []BlueprintTag `gorm:"foreignKey:TagID" json:"-"`
	CreatedAt     time.Time      `json:"-"`
}

func (t *Tag) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = NewUUID()
	}
	return nil
}

// BlueprintTag (composite PK) ────────────────────────
type BlueprintTag struct {
	BlueprintID string    `gorm:"type:char(36);primaryKey" json:"blueprint_id"`
	TagID       string    `gorm:"type:char(36);primaryKey" json:"tag_id"`
	CreatedAt   time.Time `json:"-"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
	Tag       *Tag       `gorm:"foreignKey:TagID" json:"tag"`
}

// Favorite (composite PK) ────────────────────────────
type Favorite struct {
	UserID      string    `gorm:"type:char(36);primaryKey" json:"user_id"`
	BlueprintID string    `gorm:"type:char(36);primaryKey" json:"blueprint_id"`
	CreatedAt   time.Time `json:"created_at"`

	User      *User      `gorm:"foreignKey:UserID" json:"-"`
	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
}

// Like ───────────────────────────────────────────────
type Like struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	UserID      string    `gorm:"type:char(36);index;not null" json:"user_id"`
	BlueprintID string    `gorm:"type:char(36);index;not null" json:"blueprint_id"`
	CreatedAt   time.Time `json:"created_at"`

	User      *User      `gorm:"foreignKey:UserID" json:"-"`
	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
}

func (l *Like) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = NewUUID()
	}
	return nil
}

// Comment (one-level replies via ParentID) ───────────
type Comment struct {
	ID          string  `gorm:"type:char(36);primaryKey" json:"id"`
	BlueprintID string  `gorm:"type:char(36);index;not null" json:"blueprint_id"`
	UserID      string  `gorm:"type:char(36);index;not null" json:"user_id"`
	ParentID    *string `gorm:"type:char(36);index" json:"parent_id"`
	Content     string  `gorm:"type:text;not null" json:"content"`
	// Status is the moderation state: "approved" (default, shown publicly) or
	// "hidden" (removed from public listings by an admin).
	Status    string    `gorm:"type:varchar(20);not null;default:approved" json:"status"`
	IsPinned  bool      `gorm:"not null;default:false" json:"is_pinned"`
	CreatedAt time.Time `json:"created_at"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
	User      *User      `gorm:"foreignKey:UserID" json:"user"`
	Parent    *Comment   `gorm:"foreignKey:ParentID" json:"-"`
	Replies   []Comment  `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (c *Comment) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = NewUUID()
	}
	return nil
}

// Notification ───────────────────────────────────────
type Notification struct {
	ID          string          `gorm:"type:char(36);primaryKey" json:"id"`
	UserID      string          `gorm:"type:char(36);index;not null" json:"user_id"`
	ActorID     *string         `gorm:"type:char(36)" json:"actor_id"`
	Type        string          `gorm:"type:varchar(30);not null" json:"type"`
	BlueprintID *string         `gorm:"type:char(36)" json:"blueprint_id"`
	CommentID   *string         `gorm:"type:char(36)" json:"comment_id"`
	Payload     json.RawMessage `gorm:"type:json" json:"payload"`
	IsRead      bool            `gorm:"not null;default:false" json:"is_read"`
	CreatedAt   time.Time       `json:"created_at"`
	ReadAt      *time.Time      `json:"read_at"`

	User      *User      `gorm:"foreignKey:UserID" json:"-"`
	Actor     *User      `gorm:"foreignKey:ActorID;constraint:OnDelete:SET NULL" json:"actor"`
	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
	Comment   *Comment   `gorm:"foreignKey:CommentID" json:"-"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.ID == "" {
		n.ID = NewUUID()
	}
	return nil
}

// Report ─────────────────────────────────────────────
type Report struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	ReporterID  string    `gorm:"type:char(36);index;not null" json:"reporter_id"`
	BlueprintID string    `gorm:"type:char(36);index;not null" json:"blueprint_id"`
	Reason      string    `gorm:"type:varchar(20);not null" json:"reason"`
	Detail      *string   `gorm:"type:text" json:"detail"`
	Status      string    `gorm:"type:varchar(20);not null;default:pending" json:"status"`
	CreatedAt   time.Time `json:"created_at"`

	Reporter  *User      `gorm:"foreignKey:ReporterID" json:"reporter"`
	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"-"`
}

func (r *Report) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = NewUUID()
	}
	return nil
}

// FeaturedBlueprint is an admin-curated entry in the home page's "热门推荐"
// section. SortOrder is ascending; the home falls back to latest published
// blueprints when no entries are curated (never to view_count).
type FeaturedBlueprint struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	BlueprintID string    `gorm:"type:char(36);uniqueIndex;not null" json:"blueprint_id"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"-"`
}

func (f *FeaturedBlueprint) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = NewUUID()
	}
	return nil
}

// AllModels returns every model for AutoMigrate.
func AllModels() []interface{} {
	return []interface{}{
		&User{}, &Blueprint{}, &BlueprintImage{}, &Tag{}, &BlueprintTag{},
		&Favorite{}, &Like{}, &Comment{}, &Notification{}, &Report{},
		&FeaturedBlueprint{},
	}
}
