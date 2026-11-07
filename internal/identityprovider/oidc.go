package identityprovider

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"net/http"
	"net/url"
)

type oidcAdapter struct {
	provider *oidc.Provider
	oauth    oauth2.Config
	client   *http.Client
}

func newOIDC(ctx context.Context, c Config, callback string, client *http.Client) (Adapter, error) {
	p, e := oidc.NewProvider(oidc.ClientContext(ctx, client), c.Issuer)
	if e != nil {
		return nil, errors.New("identity provider discovery failed")
	}
	endpoint := p.Endpoint()
	var metadata struct {
		Keys string `json:"jwks_uri"`
	}
	if e := p.Claims(&metadata); e != nil {
		return nil, errors.New("invalid identity provider metadata")
	}
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, metadata.Keys} {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return nil, errors.New("identity provider endpoints require HTTPS")
		}
	}
	return &oidcAdapter{p, oauth2.Config{ClientID: c.ClientID, ClientSecret: c.Secret, Endpoint: endpoint, RedirectURL: callback, Scopes: []string{oidc.ScopeOpenID, "profile"}}, client}, nil
}
func (o *oidcAdapter) AuthorizationURL(state, nonce, verifier string) string {
	return o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}
func (o *oidcAdapter) Exchange(ctx context.Context, code, nonce, verifier string) (Identity, error) {
	ctx = oidc.ClientContext(ctx, o.client)
	token, e := o.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if e != nil {
		return Identity{}, errors.New("identity exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, errors.New("identity token missing")
	}
	verified, e := o.provider.Verifier(&oidc.Config{ClientID: o.oauth.ClientID}).Verify(ctx, raw)
	if e != nil || verified.Subject == "" || subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(nonce)) != 1 {
		return Identity{}, errors.New("identity token invalid")
	}
	var claims struct {
		Name string `json:"name"`
	}
	if verified.Claims(&claims) != nil {
		return Identity{}, errors.New("identity claims invalid")
	}
	return Identity{verified.Issuer, verified.Subject, claims.Name}, nil
}

// OIDC picture 声明可能是任意地址；通用适配器不抓取它。
func (o *oidcAdapter) Avatar(context.Context, Identity) ([]byte, error) { return nil, nil }
