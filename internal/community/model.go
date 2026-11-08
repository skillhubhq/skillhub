// Package community 定义不依赖传输消息的社区领域投影。
package community

import (
	"time"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/provenance"
)

type Profile struct {
	Avatar      string `json:"avatar"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	Revision    uint64 `json:"revision"`
	CanEdit     bool   `json:"can_edit"`
}
type Team struct {
	Namespace      string `json:"namespace"`
	DisplayName    string `json:"display_name"`
	Bio            string `json:"bio"`
	Revision       uint64 `json:"revision"`
	CanEdit        bool   `json:"can_edit"`
	CanViewMembers bool   `json:"can_view_members"`
}
type Member struct {
	Profile Profile                  `json:"profile"`
	Roles   []taxonomy.NamespaceRole `json:"roles"`
}
type Contribution struct {
	Provenance    *provenance.Record `json:"provenance,omitempty"`
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	ReleaseID     string             `json:"release_id"`
	Namespace     string             `json:"namespace"`
	Name          string             `json:"name"`
	Version       string             `json:"version"`
	Description   string             `json:"description"`
	Author        Profile            `json:"author"`
}
type Comment struct {
	CommentID string    `json:"comment_id"`
	ReleaseID string    `json:"release_id"`
	ParentID  string    `json:"parent_id"`
	Author    Profile   `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Deleted   bool      `json:"deleted"`
	CanDelete bool      `json:"can_delete"`
}
type Discussion struct {
	LikeCount    uint64 `json:"like_count"`
	CommentCount uint64 `json:"comment_count"`
	Liked        bool   `json:"liked"`
	Bookmarked   bool   `json:"bookmarked"`
	CanInteract  bool   `json:"can_interact"`
}
type Page[T any] struct {
	Items    []T    `json:"items"`
	NextPage string `json:"next_page"`
}
type ProfileChange struct {
	DisplayName      string
	Bio              string
	ExpectedRevision uint64
}
type TeamChange struct {
	Namespace        string
	DisplayName      string
	Bio              string
	ExpectedRevision uint64
}
type CommentChange struct {
	ReleaseID string
	ParentID  string
	Body      string
}
type ReactionChange struct {
	ReleaseID string
	Bookmark  bool
	Active    bool
}

type AvatarChange struct {
	Image            []byte
	ExpectedRevision uint64
}
