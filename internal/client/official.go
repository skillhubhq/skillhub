package client

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/skillhubhq/skillhub/internal/product"
)

func IsOfficialHub(address string) bool {
	normalized, err := canonicalURL(address)
	return err == nil && (normalized == product.Site.OfficialHub || normalized == product.Site.OfficialHub+":443")
}

// 首次空白 App 配置连接官方来源；删除后不自动加回，不改写已有连接。
// 离线失败不写完成标记，后续启动仍可重试。
func (c *Client) EnsureOfficialSource(ctx context.Context) error {
	return c.store.withTarget(ctx, "onboarding:official", func() error {
		marker := filepath.Join(c.store.root, "official-source.initialized")
		if _, err := os.Stat(marker); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		sources, err := c.ListSources(ctx)
		if err != nil {
			return err
		}
		if len(sources) == 0 {
			bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if _, err = c.AddSource(bounded, "official", product.Site.OfficialHub); err != nil {
				return err
			}
		}
		return os.WriteFile(marker, []byte("initialized\n"), 0600)
	})
}
