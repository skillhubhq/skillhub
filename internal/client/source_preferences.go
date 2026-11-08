package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// UpdateSourcePreferences 只修改本机展示信息；稳定别名、地址、身份和凭据引用保持不变。
func (c *Client) UpdateSourcePreferences(ctx context.Context, alias, label, note string) (Source, error) {
	label, note = strings.TrimSpace(label), strings.TrimSpace(note)
	for _, field := range []struct {
		value string
		limit int
	}{{label, 80}, {note, 300}} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.limit || strings.ContainsFunc(field.value, func(r rune) bool { return unicode.IsControl(r) || unicode.In(r, unicode.Cf) }) {
			return Source{}, errors.New("connection name or note contains unsupported characters or is too long")
		}
	}
	var result Source
	err := c.store.withTarget(ctx, "source:"+alias, func() error {
		current, err := c.Source(ctx, alias)
		if err != nil {
			return err
		}
		current.Label, current.Note = label, note
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		if _, err = c.store.db.ExecContext(ctx, "UPDATE sources SET record=? WHERE alias=?", string(raw), alias); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}

// RefreshSource 仅匿名探测原地址；实例身份不匹配时拒绝更新，绝不转发登录凭据。
func (c *Client) RefreshSource(ctx context.Context, alias string) (Source, error) {
	var result Source
	err := c.store.withTarget(ctx, "source:"+alias, func() error {
		current, err := c.Source(ctx, alias)
		if err != nil {
			return err
		}
		info, err := c.probe(ctx, current.URL)
		if err != nil {
			return err
		}
		if info.RegistryId != current.RegistryID {
			return ErrRegistryChanged
		}
		current.DisplayName, current.Private = info.DisplayName, info.PrivateMode
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		if _, err = c.store.db.ExecContext(ctx, "UPDATE sources SET record=? WHERE alias=?", string(raw), alias); err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}
