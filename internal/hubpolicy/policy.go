// Package hubpolicy 定义同一 Hub 的准入、投稿和资源策略。
package hubpolicy

import "errors"

const Enterprise = "enterprise"
const Community = "community"

type Limits struct {
	RequestsPerMinute int   `yaml:"requests_per_minute"`
	Users             int   `yaml:"users"`
	SubmissionsPerDay int   `yaml:"submissions_per_day"`
	VersionsPerUser   int   `yaml:"versions_per_user"`
	BytesPerUser      int64 `yaml:"bytes_per_user"`
	PackageBytes      int64 `yaml:"package_bytes"`
	CommentsPerDay    int   `yaml:"comments_per_day"`
	ReportsPerDay     int   `yaml:"reports_per_day"`
}

type Policy struct {
	Mode   string
	Limits Limits
}

func Defaults() Limits {
	return Limits{RequestsPerMinute: 600, Users: 10000, SubmissionsPerDay: 10, VersionsPerUser: 200, BytesPerUser: 200 << 20, PackageBytes: 5 << 30, CommentsPerDay: 100, ReportsPerDay: 10}
}
func (p Policy) IsCommunity() bool { return p.Mode == Community }
func (p Policy) Validate() error {
	if p.Mode != "" && p.Mode != Enterprise && p.Mode != Community {
		return errors.New("mode must be enterprise or community")
	}
	if !p.IsCommunity() {
		return nil
	}
	l := p.Limits
	if l.RequestsPerMinute < 1 || l.RequestsPerMinute > 100000 || l.Users < 1 || l.Users > 1000000 || l.SubmissionsPerDay < 1 || l.SubmissionsPerDay > 10000 || l.VersionsPerUser < 1 || l.VersionsPerUser > 1000000 || l.BytesPerUser < 1 || l.BytesPerUser > 1<<40 || l.PackageBytes < 1 || l.PackageBytes > 1<<50 || l.CommentsPerDay < 1 || l.CommentsPerDay > 10000 || l.ReportsPerDay < 1 || l.ReportsPerDay > 1000 {
		return errors.New("community limits must be positive and within supported bounds")
	}
	return nil
}
