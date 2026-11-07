package community

// Participation 为账号自身的投稿配额，不暴露其他用户用量。
type Participation struct {
	Supported         bool   `json:"supported"`
	CommunityMode     bool   `json:"community_mode"`
	PersonalNamespace string `json:"personal_namespace"`
	Versions          uint64 `json:"versions"`
	VersionLimit      uint64 `json:"version_limit"`
	SubmissionsToday  uint64 `json:"submissions_today"`
	SubmissionLimit   uint64 `json:"submission_limit"`
	StorageBytes      uint64 `json:"storage_bytes"`
	StorageLimit      uint64 `json:"storage_limit"`
}
type Report struct {
	ID         string `json:"id"`
	Reporter   string `json:"reporter"`
	ReleaseID  string `json:"release_id"`
	CommentID  string `json:"comment_id"`
	Reason     string `json:"reason"`
	Resolved   bool   `json:"resolved"`
	Resolution string `json:"resolution"`
	Coordinate string `json:"coordinate"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}
type Site struct {
	PrivacyURL string `yaml:"privacy_url" json:"privacy_url"`
	TermsURL   string `yaml:"terms_url" json:"terms_url"`
	ContactURL string `yaml:"contact_url" json:"contact_url"`
	FilingText string `yaml:"filing_text" json:"filing_text"`
	FilingURL  string `yaml:"filing_url" json:"filing_url"`
}
