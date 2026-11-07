package client

import (
	"context"
	"errors"
	"slices"
	"time"

	"connectrpc.com/connect"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

type PlatformAccess struct {
	CanCurate      bool                    `json:"can_curate"`
	Supported      bool                    `json:"supported"`
	UserID         string                  `json:"user_id"`
	Roles          []taxonomy.PlatformRole `json:"roles"`
	CanManageUsers bool                    `json:"can_manage_users"`
	CanReadAudit   bool                    `json:"can_read_audit"`
	Administrator  bool                    `json:"administrator"`
}
type ScanFinding struct {
	Rule     string `json:"rule"`
	Path     string `json:"path"`
	Line     uint32 `json:"line"`
	Blocking bool   `json:"blocking"`
	Message  string `json:"message"`
}
type ScanInfo struct {
	Supported bool          `json:"supported"`
	Engine    string        `json:"engine"`
	Digest    string        `json:"digest"`
	Complete  bool          `json:"complete"`
	Blocking  bool          `json:"blocking"`
	Findings  []ScanFinding `json:"findings"`
	ScannedAt string        `json:"scanned_at"`
}
type Notification struct {
	ID         string `json:"id"`
	ReleaseID  string `json:"release_id"`
	Coordinate string `json:"coordinate"`
	Approved   bool   `json:"approved"`
	Read       bool   `json:"read"`
	CreatedAt  string `json:"created_at"`
}
type NotificationPage struct {
	Supported bool           `json:"supported"`
	Items     []Notification `json:"items"`
	NextPage  string         `json:"next_page"`
}

func scanInfo(r *governancev1.ScanReport) ScanInfo {
	out := ScanInfo{Supported: true, Findings: []ScanFinding{}}
	if r == nil {
		return out
	}
	out.Engine = r.Engine
	out.Digest = r.Digest
	out.Complete = r.Complete
	out.Blocking = r.Blocking
	if r.ScannedAt != nil {
		out.ScannedAt = r.ScannedAt.AsTime().UTC().Format(time.RFC3339)
	}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, ScanFinding{f.Rule, f.Path, f.Line, f.Blocking, f.Message})
	}
	return out
}
func (c *Client) PlatformAccess(ctx context.Context, source, id string) (PlatformAccess, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return PlatformAccess{}, err
	}
	if !slices.Contains(r.Capabilities, "governance.v1") {
		return PlatformAccess{Roles: []taxonomy.PlatformRole{}}, nil
	}
	v, err := r.Governance.GetPlatformAccess(ctx, connect.NewRequest(&governancev1.GetPlatformAccessRequest{UserId: id}))
	if err != nil {
		return PlatformAccess{}, err
	}
	return PlatformAccess{slices.Contains(r.Capabilities, "catalog.discovery.v1") && v.Msg.Administrator, true, v.Msg.UserId, v.Msg.Roles, v.Msg.CanManageUsers, v.Msg.CanReadAudit, v.Msg.Administrator}, nil
}
func (c *Client) SetPlatformRole(ctx context.Context, source, id string, role taxonomy.PlatformRole, granted bool, key string) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	if !slices.Contains(r.Capabilities, "governance.v1") {
		return errors.New("hub does not support governance")
	}
	_, err = r.Governance.SetPlatformRole(ctx, connect.NewRequest(&governancev1.SetPlatformRoleRequest{Mutation: &commonv1.Mutation{IdempotencyKey: key}, UserId: id, Role: role, Granted: granted}))
	return err
}
func (c *Client) ScanReport(ctx context.Context, source, id string) (ScanInfo, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return ScanInfo{}, err
	}
	if !slices.Contains(r.Capabilities, "governance.v1") {
		return ScanInfo{Findings: []ScanFinding{}}, nil
	}
	v, err := r.Governance.GetScanReport(ctx, connect.NewRequest(&governancev1.GetScanReportRequest{ReleaseId: id}))
	if err != nil {
		return ScanInfo{}, err
	}
	return scanInfo(v.Msg.Report), nil
}
func (c *Client) ScanRelease(ctx context.Context, source, id, key string) (ScanInfo, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return ScanInfo{}, err
	}
	if !slices.Contains(r.Capabilities, "governance.v1") {
		return ScanInfo{}, errors.New("hub does not support governance")
	}
	v, err := r.Governance.ScanRelease(ctx, connect.NewRequest(&governancev1.ScanReleaseRequest{ReleaseId: id, Mutation: &commonv1.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return ScanInfo{}, err
	}
	return scanInfo(v.Msg.Report), nil
}
func (c *Client) Notifications(ctx context.Context, source, token string) (NotificationPage, error) {
	out := NotificationPage{Items: []Notification{}}
	r, err := c.Remote(ctx, source)
	if err != nil {
		return out, err
	}
	if !slices.Contains(r.Capabilities, "governance.v1") {
		return out, nil
	}
	out.Supported = true
	v, err := r.Governance.ListNotifications(ctx, connect.NewRequest(&governancev1.ListNotificationsRequest{Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}}))
	if err != nil {
		return out, err
	}
	out.NextPage = v.Msg.GetPage().GetNextPageToken()
	for _, n := range v.Msg.Notifications {
		at := ""
		if n.CreatedAt != nil {
			at = n.CreatedAt.AsTime().UTC().Format(time.RFC3339)
		}
		out.Items = append(out.Items, Notification{n.NotificationId, n.ReleaseId, n.Coordinate, n.Approved, n.Read, at})
	}
	return out, nil
}
func (c *Client) MarkNotificationRead(ctx context.Context, source, id string) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	_, err = r.Governance.MarkNotificationRead(ctx, connect.NewRequest(&governancev1.MarkNotificationReadRequest{NotificationId: id}))
	return err
}

type ReviewTask struct {
	ReleaseID  string `json:"release_id"`
	Coordinate string `json:"coordinate"`
	CreatedAt  string `json:"created_at"`
}
type ReviewTaskPage struct {
	Supported bool         `json:"supported"`
	Tasks     []ReviewTask `json:"tasks"`
	Total     uint32       `json:"total"`
	OldestAt  string       `json:"oldest_at"`
	NextPage  string       `json:"next_page"`
}

func (c *Client) ReviewTasks(ctx context.Context, source, token string) (ReviewTaskPage, error) {
	out := ReviewTaskPage{Tasks: []ReviewTask{}}
	r, err := c.Remote(ctx, source)
	if err != nil {
		return out, err
	}
	if !slices.Contains(r.Capabilities, "governance.tasks.v1") {
		return out, nil
	}
	v, err := r.Governance.ListReviewTasks(ctx, connect.NewRequest(&governancev1.ListReviewTasksRequest{Page: &commonv1.PageRequest{PageSize: 20, PageToken: token}}))
	if err != nil {
		return out, err
	}
	out.Supported = true
	out.Total = v.Msg.Total
	out.NextPage = v.Msg.GetPage().GetNextPageToken()
	if v.Msg.OldestAt != nil {
		out.OldestAt = v.Msg.OldestAt.AsTime().UTC().Format(time.RFC3339)
	}
	for _, t := range v.Msg.Tasks {
		at := ""
		if t.CreatedAt != nil {
			at = t.CreatedAt.AsTime().UTC().Format(time.RFC3339)
		}
		out.Tasks = append(out.Tasks, ReviewTask{t.ReleaseId, t.Coordinate, at})
	}
	return out, nil
}
func (c *Client) SetFeaturedSkill(ctx context.Context, source, namespace, name string, featured bool, key string) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	if !slices.Contains(r.Capabilities, "catalog.discovery.v1") {
		return errors.New("hub does not support curated discovery")
	}
	_, err = r.Governance.SetFeaturedSkill(ctx, connect.NewRequest(&governancev1.SetFeaturedSkillRequest{Mutation: &commonv1.Mutation{IdempotencyKey: key}, Namespace: namespace, Name: name, Featured: featured}))
	return err
}
