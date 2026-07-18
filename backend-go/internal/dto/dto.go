package dto

import (
	"encoding/json"
	"time"

	"brickplans/internal/db"
)

// ISO formats a time as an RFC3339 (UTC) string, matching the Python backend's
// isoformat() output so existing frontends parse it unchanged.
func ISO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// UserOut is the public user projection (author, commenter, notification actor).
// It deliberately omits email and is_admin — those are private to the user.
type UserOut struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url"`
	Bio       *string `json:"bio"`
	CreatedAt string  `json:"created_at"`
}

// MeOut is the authenticated user's own projection — includes email and is_admin.
type MeOut struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	AvatarURL *string `json:"avatar_url"`
	Bio       *string `json:"bio"`
	IsAdmin   bool    `json:"is_admin"`
	CreatedAt string  `json:"created_at"`
}

func FromUser(u *db.User) *UserOut {
	if u == nil {
		return nil
	}
	return &UserOut{
		ID:        u.ID,
		Username:  u.Username,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
		CreatedAt: ISO(u.CreatedAt),
	}
}

func FromMe(u *db.User) *MeOut {
	if u == nil {
		return nil
	}
	return &MeOut{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
		IsAdmin:   u.IsAdmin,
		CreatedAt: ISO(u.CreatedAt),
	}
}

// AdminUserOut is the admin-only user projection - includes email, is_admin,
// email_verified, banned and blueprint_count. Private to admins (never returned
// by public endpoints).
type AdminUserOut struct {
	ID             string  `json:"id"`
	Username       string  `json:"username"`
	Email          string  `json:"email"`
	AvatarURL      *string `json:"avatar_url"`
	IsAdmin        bool    `json:"is_admin"`
	EmailVerified  bool    `json:"email_verified"`
	Banned         bool    `json:"banned"`
	BlueprintCount int     `json:"blueprint_count"`
	CreatedAt      string  `json:"created_at"`
}

// TokenResponse matches the Python TokenResponse shape used by the frontend.
// RefreshToken is delivered via httpOnly cookie (not in the body) in the cookie
// auth scheme, so it's omitempty here.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	User         *MeOut `json:"user"`
}

// ImageOut is the public blueprint-image projection (object_key intentionally omitted).
type ImageOut struct {
	ID          string `json:"id"`
	BlueprintID string `json:"blueprint_id"`
	URL         string `json:"url"`
	SortOrder   int    `json:"sort_order"`
	IsCover     bool   `json:"is_cover"`
	FileType    string `json:"file_type"`
}

// BlueprintOut is the list/card projection.
type BlueprintOut struct {
	ID                string          `json:"id"`
	AuthorID          string          `json:"author_id"`
	Title             string          `json:"title"`
	Slug              string          `json:"slug"`
	Description       *string         `json:"description"`
	Difficulty        *int            `json:"difficulty"`
	PieceCount        *int            `json:"piece_count"`
	Category          *string         `json:"category"`
	Dimensions        *string         `json:"dimensions"`
	PartList          json.RawMessage `json:"part_list"`
	ViewCount         int             `json:"view_count"`
	LikeCount         int             `json:"like_count"`
	FavoriteCount     int             `json:"favorite_count"`
	IsLiked           bool            `json:"is_liked"`
	CoverURL          *string         `json:"cover_url"`
	IsPublished       bool            `json:"is_published"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	Author            *UserOut        `json:"author"`
	Images            []ImageOut      `json:"images"`
	Tags              []string        `json:"tags"`
	ModerationStatus  *string         `json:"moderation_status"`
}

// BlueprintDetail adds the viewer's favorite state on top of BlueprintOut.
type BlueprintDetail struct {
	BlueprintOut
	IsFavorited bool `json:"is_favorited"`
}

// BlueprintListOut is the paginated list envelope.
type BlueprintListOut struct {
	Items    []BlueprintOut `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// CommentOut is the comment projection (one-level replies via parent_id).
type CommentOut struct {
	ID          string   `json:"id"`
	BlueprintID string   `json:"blueprint_id"`
	UserID      string   `json:"user_id"`
	ParentID    *string  `json:"parent_id"`
	Content     string   `json:"content"`
	CreatedAt   string   `json:"created_at"`
	User        *UserOut `json:"user"`
}

// NotificationOut is the notification projection. Actor uses the public UserOut
// (no email / is_admin) — the Python version leaked the actor's email here.
type NotificationOut struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	ActorID     *string         `json:"actor_id"`
	Type        string          `json:"type"`
	BlueprintID *string         `json:"blueprint_id"`
	CommentID   *string         `json:"comment_id"`
	Payload     json.RawMessage `json:"payload"`
	IsRead      bool            `json:"is_read"`
	CreatedAt   string          `json:"created_at"`
	ReadAt      *string         `json:"read_at"`
	Actor       *UserOut        `json:"actor"`
}

type NotificationListOut struct {
	Items      []NotificationOut `json:"items"`
	Total      int               `json:"total"`
	UnreadCount int              `json:"unread_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
}
