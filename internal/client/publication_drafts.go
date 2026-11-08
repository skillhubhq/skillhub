package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
)

// 发布表单与工作副本一起保存在设备，按已验证的 Hub 身份和账号隔离。
func (c *Client) publicationDraft(ctx context.Context, source, account string, fn func(*os.Root, string) error) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	identity, err := r.Auth.GetIdentity(ctx, connect.NewRequest(&authv1.GetIdentityRequest{}))
	if err != nil {
		return err
	}
	if account == "" || identity.Msg.GetIdentity().GetUserId() != account {
		return errors.New("draft account changed")
	}
	key := configHash([]string{r.Source.RegistryID, account}) + ".json"
	return c.store.withTarget(ctx, "publication-drafts", func() error {
		root, err := os.OpenRoot(c.store.root)
		if err != nil {
			return err
		}
		defer root.Close()
		if err = root.MkdirAll("publication-drafts", 0700); err != nil {
			return err
		}
		info, err := root.Lstat("publication-drafts")
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid drafts directory")
		}
		dir, err := root.OpenRoot("publication-drafts")
		if err != nil {
			return err
		}
		defer dir.Close()
		return fn(dir, key)
	})
}
func (c *Client) LoadPublicationDraft(ctx context.Context, source, account string) (string, error) {
	var payload string
	err := c.publicationDraft(ctx, source, account, func(dir *os.Root, key string) error {
		info, err := dir.Lstat(key)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return errors.New("invalid publication draft file")
		}
		f, err := dir.Open(key)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
		if err != nil {
			return err
		}
		if len(data) > 32<<20 || !json.Valid(data) {
			return errors.New("invalid publication draft")
		}
		payload = string(data)
		return nil
	})
	return payload, err
}
func (c *Client) SavePublicationDraft(ctx context.Context, source, account, payload string) error {
	if len(payload) > 32<<20 || !utf8.ValidString(payload) || (payload != "" && !json.Valid([]byte(payload))) {
		return errors.New("invalid publication draft")
	}
	return c.publicationDraft(ctx, source, account, func(dir *os.Root, key string) error {
		if payload == "" {
			err := dir.Remove(key)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			return syncDirectory(dir.Name())
		}
		tmp := ".pending-" + uuid.NewString()
		f, err := dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer dir.Remove(tmp)
		_, err = f.WriteString(payload)
		err = errors.Join(err, f.Sync(), f.Close())
		if err != nil {
			return err
		}
		if err = dir.Rename(tmp, key); err != nil {
			return err
		}
		return syncDirectory(dir.Name())
	})
}
