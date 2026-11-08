package client

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	common "github.com/skillhubhq/skillhub/gen/api/common/v1"
	api "github.com/skillhubhq/skillhub/gen/api/community/v1"
	registry "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"slices"
	"time"
)

func (client *Client) CommunityParticipation(ctx context.Context, source string) (c.Participation, error) {
	r, e := client.Remote(ctx, source)
	if e != nil {
		return c.Participation{}, e
	}
	if !slices.Contains(r.Capabilities, "community.moderation.v1") {
		return c.Participation{}, nil
	}
	v, e := r.Community.GetParticipation(ctx, connect.NewRequest(&api.GetParticipationRequest{}))
	if e != nil {
		return c.Participation{}, e
	}
	p := v.Msg
	return c.Participation{Supported: true, CommunityMode: p.CommunityMode, PersonalNamespace: p.PersonalNamespace, Versions: p.Versions, VersionLimit: p.VersionLimit, SubmissionsToday: p.SubmissionsToday, SubmissionLimit: p.SubmissionLimit, StorageBytes: p.StorageBytes, StorageLimit: p.StorageLimit}, nil
}
func (client *Client) CommunityReport(ctx context.Context, source, id, comment, reason, key string) (string, error) {
	r, e := client.Remote(ctx, source)
	if e != nil {
		return "", e
	}
	if !slices.Contains(r.Capabilities, "community.moderation.v1") {
		return "", errors.New("hub does not support content reports")
	}
	v, e := r.Community.ReportContent(ctx, connect.NewRequest(&api.ReportContentRequest{Mutation: &common.Mutation{IdempotencyKey: key}, ReleaseId: id, CommentId: comment, Reason: reason}))
	if e != nil {
		return "", e
	}
	return v.Msg.Id, nil
}
func (client *Client) CommunityReports(ctx context.Context, source string, resolved bool, token string) (c.Page[c.Report], error) {
	result := c.Page[c.Report]{Items: []c.Report{}}
	r, e := client.Remote(ctx, source)
	if e != nil {
		return result, e
	}
	if !slices.Contains(r.Capabilities, "community.moderation.v1") {
		return result, errors.New("hub does not support content reports")
	}
	v, e := r.Community.ListReports(ctx, connect.NewRequest(&api.ListReportsRequest{Resolved: resolved, Page: &common.PageRequest{PageToken: token, PageSize: 30}}))
	if e != nil {
		return result, e
	}
	for _, p := range v.Msg.Reports {
		result.Items = append(result.Items, c.Report{ID: p.Id, Reporter: p.Reporter, ReleaseID: p.ReleaseId, CommentID: p.CommentId, Reason: p.Reason, Resolved: p.Resolved, Resolution: p.Resolution, Coordinate: p.Coordinate, Content: p.Content, CreatedAt: p.CreatedAt.AsTime().Format(time.RFC3339)})
	}
	result.NextPage = v.Msg.Page.GetNextPageToken()
	return result, nil
}
func (client *Client) CommunityResolveReport(ctx context.Context, source, id string, action taxonomy.ReportResolution, note, key string) (bool, error) {
	r, e := client.Remote(ctx, source)
	if e != nil {
		return false, e
	}
	if !slices.Contains(r.Capabilities, "community.moderation.v1") {
		return false, errors.New("hub does not support content reports")
	}
	_, e = r.Community.ResolveReport(ctx, connect.NewRequest(&api.ResolveReportRequest{Mutation: &common.Mutation{IdempotencyKey: key}, Id: id, Action: action, Note: note}))
	return e == nil, e
}
func (client *Client) CommunitySite(ctx context.Context, source string) (c.Site, error) {
	r, e := client.Remote(ctx, source)
	if e != nil {
		return c.Site{}, e
	}
	// 站点信息为公开发现数据；不向外部站点转发凭据。
	v, e := registryv1connect.NewRegistryServiceClient(client.http, r.Source.URL).GetRegistry(ctx, connect.NewRequest(&registry.GetRegistryRequest{}))
	if e != nil {
		return c.Site{}, e
	}
	p := v.Msg
	return c.Site{PrivacyURL: p.PrivacyUrl, TermsURL: p.TermsUrl, ContactURL: p.ContactUrl, FilingText: p.FilingText, FilingURL: p.FilingUrl}, nil
}
