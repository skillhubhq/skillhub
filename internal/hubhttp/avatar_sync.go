package hubhttp

import (
	"context"
	"time"

	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
)

// 头像是可选资料；有界后台同步避免外部 CDN 拖延登录，不持久化第三方令牌。
func (s *Server) startAvatarSync(adapter identityprovider.Adapter, identity identityprovider.Identity, actor hub.Identity) {
	s.avatarMu.Lock()
	if s.avatarStopped || len(s.avatarTasks) >= 4 || s.avatarTasks[actor.ID] != nil {
		s.avatarMu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	s.avatarTasks[actor.ID] = cancel
	s.avatarWG.Add(1)
	s.avatarMu.Unlock()
	go func() {
		defer s.avatarWG.Done()
		defer func() { cancel(); s.avatarMu.Lock(); delete(s.avatarTasks, actor.ID); s.avatarMu.Unlock() }()
		current, err := s.store.GetProfile(ctx, actor, actor.ID)
		if err != nil || current.Revision != 0 || current.Avatar != "" {
			return
		}
		raw, err := adapter.Avatar(ctx, identity)
		if err == nil && len(raw) > 0 {
			_ = s.store.SyncExternalAvatar(ctx, actor, raw)
		}
	}()
}

// 先停止 HTTP 接入再取消后台资料同步，关闭数据库前等待它们退出。
func (s *Server) CloseBackground() {
	s.avatarMu.Lock()
	s.avatarStopped = true
	for _, cancel := range s.avatarTasks {
		cancel()
	}
	s.avatarMu.Unlock()
	s.avatarWG.Wait()
}
