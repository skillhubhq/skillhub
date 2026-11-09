package hubhttp

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	communityv1 "github.com/skillhubhq/skillhub/gen/api/community/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
)

// 自动化凭据只开放明确列出的只读和投稿操作；新增 RPC 默认不可用。
func (s *Server) authorizeAutomation(ctx context.Context, req connect.AnyRequest) error {
	raw := req.Header().Get("Authorization")
	fields := strings.Fields(raw)
	if len(fields) != 2 || !strings.HasPrefix(fields[1], "sht_") {
		return nil
	}
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return err
	}
	switch req.Spec().Procedure {
	case "/api.registry.v1.RegistryService/GetRegistry", "/api.auth.v1.AuthService/GetIdentity", "/api.catalog.v1.CatalogService/SearchSkills", "/api.catalog.v1.CatalogService/GetSkill", "/api.catalog.v1.CatalogService/ListReleases", "/api.catalog.v1.CatalogService/DownloadRelease", "/api.catalog.v1.CatalogService/CompareReleases":
		return nil
	case "/api.publishing.v1.PublishingService/SubmitRelease":
		v, ok := req.Any().(*publishingv1.SubmitReleaseRequest)
		if ok && u.AutomationScope == "publish" && v.Namespace == u.AutomationNamespace {
			return nil
		}
	}
	return hub.ErrForbidden
}
func (s *Server) CompareReleases(ctx context.Context, req *connect.Request[catalogv1.CompareReleasesRequest]) (*connect.Response[catalogv1.CompareReleasesResponse], error) {
	u, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.CompareReleases(ctx, u, req.Msg.Namespace, req.Msg.Name, req.Msg.BeforeId, req.Msg.AfterId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&catalogv1.CompareReleasesResponse{BeforeDigest: v.BeforeDigest, AfterDigest: v.AfterDigest, Files: mapItems(v.Files, func(f hub.FileDifference) *catalogv1.FileDifference {
		return &catalogv1.FileDifference{Path: f.Path, Change: f.Change, BeforeText: f.BeforeText, AfterText: f.AfterText, TextAvailable: f.TextAvailable, BeforeExecutable: f.BeforeExecutable, AfterExecutable: f.AfterExecutable}
	})}), nil
}
func (s *Server) GetSubscription(ctx context.Context, req *connect.Request[communityv1.GetSubscriptionRequest]) (*connect.Response[communityv1.GetSubscriptionResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetSubscription(ctx, u, req.Msg.Namespace, req.Msg.Name)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&communityv1.GetSubscriptionResponse{Subscribed: v}), nil
}
func (s *Server) SetSubscription(ctx context.Context, req *connect.Request[communityv1.SetSubscriptionRequest]) (*connect.Response[communityv1.SetSubscriptionResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	err = s.store.SetSubscription(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.Namespace, req.Msg.Name, req.Msg.Subscribed)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&communityv1.SetSubscriptionResponse{}), nil
}
func (s *Server) ListSubscriptions(ctx context.Context, req *connect.Request[communityv1.ListSubscriptionsRequest]) (*connect.Response[communityv1.ListSubscriptionsResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListSubscriptions(ctx, u, pageDomain(req.Msg.Page))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&communityv1.ListSubscriptionsResponse{Subscriptions: mapItems(v.Items, func(i hub.Subscription) *communityv1.Subscription {
		return &communityv1.Subscription{Namespace: i.Namespace, Name: i.Name, Available: i.Available}
	}), Page: pageDTO(v.Next)}), nil
}
func tokenDTO(v hub.AccessToken) *authv1.AccessToken {
	return &authv1.AccessToken{Id: v.ID, Name: v.Name, Scope: v.Scope, Namespace: v.Namespace, ExpiresAt: v.ExpiresAt, CreatedAt: v.CreatedAt, LastUsedAt: v.LastUsedAt, Revoked: v.Revoked}
}
func (s *Server) CreateAccessToken(ctx context.Context, req *connect.Request[authv1.CreateAccessTokenRequest]) (*connect.Response[authv1.CreateAccessTokenResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, secret, err := s.store.CreateAccessToken(ctx, u, req.Msg.Name, req.Msg.Scope, req.Msg.Namespace, req.Msg.Days)
	if err != nil {
		return nil, err
	}
	response := connect.NewResponse(&authv1.CreateAccessTokenResponse{Token: tokenDTO(v), Secret: secret})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}
func (s *Server) ListAccessTokens(ctx context.Context, req *connect.Request[authv1.ListAccessTokensRequest]) (*connect.Response[authv1.ListAccessTokensResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	v, err := s.store.ListAccessTokens(ctx, u)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.ListAccessTokensResponse{Tokens: mapItems(v, tokenDTO)}), nil
}
func (s *Server) RevokeAccessToken(ctx context.Context, req *connect.Request[authv1.RevokeAccessTokenRequest]) (*connect.Response[authv1.RevokeAccessTokenResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.RevokeAccessToken(ctx, u, req.Msg.Id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&authv1.RevokeAccessTokenResponse{}), nil
}
