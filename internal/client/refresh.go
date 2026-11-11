package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
)

const sessionPrefix = "skillhub-session-v1:"

// 整个会话只存在于系统凭据库，SQLite 仅保留不含秘密的连接引用。
type savedSession struct {
	Access         string `json:"access"`
	Refresh        string `json:"refresh"`
	Expires        int64  `json:"expires"`
	RefreshExpires int64  `json:"refresh_expires"`
	Pending        string `json:"pending,omitempty"`
}

func sessionCredential(v *authv1.LoginResponse) (string, error) {
	if v.GetRefreshToken() == "" {
		return v.GetAccessToken(), nil
	}
	if len(v.RefreshToken) != 43 || v.ExpiresAt == nil || !v.ExpiresAt.IsValid() || v.RefreshExpiresAt == nil || !v.RefreshExpiresAt.IsValid() || v.RefreshExpiresAt.AsTime().Before(v.ExpiresAt.AsTime()) {
		return "", errors.New("invalid renewable session response")
	}
	return encodeSession(savedSession{Access: v.AccessToken, Refresh: v.RefreshToken, Expires: v.ExpiresAt.AsTime().Unix(), RefreshExpires: v.RefreshExpiresAt.AsTime().Unix()}), nil
}
func encodeSession(v savedSession) string { b, _ := json.Marshal(v); return sessionPrefix + string(b) }
func decodeSession(raw string) (savedSession, error) {
	var v savedSession
	if !strings.HasPrefix(raw, sessionPrefix) {
		return savedSession{Access: raw}, nil
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(raw, sessionPrefix)), &v) != nil || len(v.Access) != 43 || len(v.Refresh) != 43 || v.Expires <= 0 || v.RefreshExpires < v.Expires {
		return v, errors.New("invalid saved session")
	}
	return v, nil
}

// 与登录、退出和连接修改使用同一个跨进程锁；CLI 与 App 不会各自轮换同一凭据。
func (c *Client) sourceAccess(ctx context.Context, source Source) (string, error) {
	var token string
	err := c.store.withTarget(ctx, "source:"+source.Alias, func() error {
		current, err := c.Source(ctx, source.Alias)
		if err != nil {
			return err
		}
		if current.CredentialRef != source.CredentialRef || current.RegistryID != source.RegistryID || current.URL != source.URL || current.AccountID != source.AccountID {
			return errors.New("source identity changed; retry the operation")
		}
		if !current.HasCredential {
			return nil
		}
		raw, err := c.credentials.Get(current.CredentialRef)
		if err != nil {
			return err
		}
		session, err := decodeSession(raw)
		if err != nil {
			return err
		}
		for attempt := 0; attempt < 2 && session.Refresh != "" && (session.Pending != "" || time.Now().Add(time.Minute).Unix() >= session.Expires); attempt++ {
			if time.Now().Unix() >= session.RefreshExpires {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("session expired; sign in again"))
			}
			if session.Pending == "" {
				session.Pending = uuid.NewString()
				if err = c.credentials.Set(current.CredentialRef, encodeSession(session)); err != nil {
					return err
				}
			}
			// 长期持有的客户端在续期前重新核对实例，刷新秘密不交给被替换的 Hub。
			info, err := c.probe(ctx, current.URL)
			if err != nil {
				return err
			}
			if info.RegistryId != current.RegistryID {
				return ErrRegistryChanged
			}
			// 持久化请求标识后才发送，响应丢失或保存失败时重试相同轮换。
			auth := authv1connect.NewAuthServiceClient(c.http, current.URL)
			result, err := auth.RefreshSession(ctx, connect.NewRequest(&authv1.RefreshSessionRequest{RefreshToken: session.Refresh, RequestId: session.Pending}))
			if err != nil {
				return err
			}
			login := result.Msg.Login
			if login.GetIdentity().GetUserId() != current.AccountID || len(login.GetAccessToken()) != 43 || len(login.GetRefreshToken()) != 43 {
				return errors.New("invalid refresh response")
			}
			raw, err = sessionCredential(login)
			if err != nil {
				return err
			}
			if err = c.credentials.Set(current.CredentialRef, raw); err != nil {
				return err
			}
			session, err = decodeSession(raw)
			if err != nil {
				return err
			}
		}
		token = session.Access
		if len(token) != 43 && !(len(token) == 47 && strings.HasPrefix(token, "sht_")) {
			return errors.New("invalid source credential")
		}
		return nil
	})
	return token, err
}
