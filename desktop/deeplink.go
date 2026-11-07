package main

import (
	"errors"
	"sync"

	"github.com/skillhubhq/skillhub/internal/client"
)

var openLinks struct {
	sync.Mutex
	queue []client.OpenRequest
}

func queueOpenLink(raw string) {
	request, err := client.ParseOpenLink(raw)
	if err != nil {
		return
	}
	openLinks.Lock()
	defer openLinks.Unlock()
	if len(openLinks.queue) >= 8 {
		return
	}
	for _, q := range openLinks.queue {
		if q == request {
			return
		}
	}
	openLinks.queue = append(openLinks.queue, request)
}
func (s *DesktopService) NextOpenRequest() *client.OpenRequest {
	openLinks.Lock()
	defer openLinks.Unlock()
	if len(openLinks.queue) == 0 {
		return nil
	}
	value := openLinks.queue[0]
	openLinks.queue = openLinks.queue[1:]
	return &value
}

// 用户确认来源后才联网或连接；只返回路由，文件安装仍需正常变更预览。
func (s *DesktopService) ConnectOpenRequest(hub, alias string) (client.Source, error) {
	sources, err := s.core.ListSources(s.ctx)
	if err != nil {
		return client.Source{}, err
	}
	for _, source := range sources {
		if source.URL == hub {
			return source, nil
		}
	}
	if alias == "" {
		return client.Source{}, errors.New("choose a connection name")
	}
	return s.core.AddSource(s.ctx, alias, hub)
}
