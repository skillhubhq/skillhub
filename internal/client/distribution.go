package client

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	communityv1 "github.com/skillhubhq/skillhub/gen/api/community/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
)

func (c *Client) featureRemote(ctx context.Context, source, capability string) (*Remote, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(r.Capabilities, capability) {
		return nil, errors.New("this Hub does not support this feature")
	}
	return r, nil
}
func (c *Client) CompareReleases(ctx context.Context, coordinate Coordinate, before, after string) (hub.ReleaseDifference, error) {
	result := hub.ReleaseDifference{Files: []hub.FileDifference{}}
	r, err := c.featureRemote(ctx, coordinate.Source, "catalog.compare.v1")
	if err != nil {
		return result, err
	}
	v, err := r.Catalog.CompareReleases(ctx, connect.NewRequest(&catalogv1.CompareReleasesRequest{Namespace: coordinate.Namespace, Name: coordinate.Name, BeforeId: before, AfterId: after}))
	if err != nil {
		return result, err
	}
	result.BeforeDigest = v.Msg.BeforeDigest
	result.AfterDigest = v.Msg.AfterDigest
	for _, f := range v.Msg.Files {
		result.Files = append(result.Files, hub.FileDifference{Path: f.Path, Change: f.Change, BeforeText: f.BeforeText, AfterText: f.AfterText, TextAvailable: f.TextAvailable, BeforeExecutable: f.BeforeExecutable, AfterExecutable: f.AfterExecutable})
	}
	return result, nil
}
func (c *Client) Subscription(ctx context.Context, coordinate Coordinate) (bool, error) {
	r, err := c.featureRemote(ctx, coordinate.Source, "community.subscriptions.v1")
	if err != nil {
		return false, err
	}
	v, err := r.Community.GetSubscription(ctx, connect.NewRequest(&communityv1.GetSubscriptionRequest{Namespace: coordinate.Namespace, Name: coordinate.Name}))
	if err != nil {
		return false, err
	}
	return v.Msg.Subscribed, nil
}
func (c *Client) Subscribe(ctx context.Context, coordinate Coordinate, active bool, key string) error {
	r, err := c.featureRemote(ctx, coordinate.Source, "community.subscriptions.v1")
	if err != nil {
		return err
	}
	_, err = r.Community.SetSubscription(ctx, connect.NewRequest(&communityv1.SetSubscriptionRequest{Mutation: &commonv1.Mutation{IdempotencyKey: key}, Namespace: coordinate.Namespace, Name: coordinate.Name, Subscribed: active}))
	return err
}

type SubscriptionPage struct {
	Items    []hub.Subscription `json:"items"`
	NextPage string             `json:"next_page"`
}

func (c *Client) Subscriptions(ctx context.Context, source, token string) (SubscriptionPage, error) {
	result := SubscriptionPage{Items: []hub.Subscription{}}
	r, err := c.featureRemote(ctx, source, "community.subscriptions.v1")
	if err != nil {
		return result, err
	}
	v, err := r.Community.ListSubscriptions(ctx, connect.NewRequest(&communityv1.ListSubscriptionsRequest{Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}}))
	if err != nil {
		return result, err
	}
	for _, s := range v.Msg.Subscriptions {
		result.Items = append(result.Items, hub.Subscription{Namespace: s.Namespace, Name: s.Name, Available: s.Available})
	}
	result.NextPage = v.Msg.GetPage().GetNextPageToken()
	return result, nil
}
func accessTokenInfo(v *authv1.AccessToken) hub.AccessToken {
	return hub.AccessToken{ID: v.Id, Name: v.Name, Scope: v.Scope, Namespace: v.Namespace, CreatedAt: v.CreatedAt, ExpiresAt: v.ExpiresAt, LastUsedAt: v.LastUsedAt, Revoked: v.Revoked}
}

type CreatedAccessToken struct {
	Token  hub.AccessToken `json:"token"`
	Secret string          `json:"secret"`
}

func (c *Client) CreateAccessToken(ctx context.Context, source, name, scope, namespace string, days uint32) (CreatedAccessToken, error) {
	r, err := c.featureRemote(ctx, source, "identity.tokens.v1")
	if err != nil {
		return CreatedAccessToken{}, err
	}
	v, err := r.Auth.CreateAccessToken(ctx, connect.NewRequest(&authv1.CreateAccessTokenRequest{Name: name, Scope: scope, Namespace: namespace, Days: days}))
	if err != nil {
		return CreatedAccessToken{}, err
	}
	return CreatedAccessToken{Token: accessTokenInfo(v.Msg.Token), Secret: v.Msg.Secret}, nil
}
func (c *Client) AccessTokens(ctx context.Context, source string) ([]hub.AccessToken, error) {
	r, err := c.featureRemote(ctx, source, "identity.tokens.v1")
	if err != nil {
		return nil, err
	}
	v, err := r.Auth.ListAccessTokens(ctx, connect.NewRequest(&authv1.ListAccessTokensRequest{}))
	if err != nil {
		return nil, err
	}
	result := []hub.AccessToken{}
	for _, t := range v.Msg.Tokens {
		result = append(result, accessTokenInfo(t))
	}
	return result, nil
}
func (c *Client) RevokeAccessToken(ctx context.Context, source, id string) error {
	r, err := c.featureRemote(ctx, source, "identity.tokens.v1")
	if err != nil {
		return err
	}
	_, err = r.Auth.RevokeAccessToken(ctx, connect.NewRequest(&authv1.RevokeAccessTokenRequest{Id: id}))
	return err
}
