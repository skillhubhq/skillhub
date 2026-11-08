package client

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	"slices"
	"strconv"
)

// NamespaceLifecycle 保持字符串修订号，避免 JavaScript 大整数精度损失。
type NamespaceLifecycle struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Archived bool   `json:"archived"`
	Releases string `json:"releases"`
	Grants   string `json:"grants"`
	Comments string `json:"comments"`
	Evidence string `json:"evidence"`
}

func lifecycleInfo(v *adminv1.NamespaceLifecycle) NamespaceLifecycle {
	return NamespaceLifecycle{v.Name, strconv.FormatUint(v.Revision, 10), v.Archived, strconv.FormatUint(v.Releases, 10), strconv.FormatUint(v.Grants, 10), strconv.FormatUint(v.Comments, 10), strconv.FormatUint(v.Evidence, 10)}
}
func (c *Client) NamespaceLifecycleSupported(ctx context.Context, source string) (bool, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return false, err
	}
	return slices.Contains(r.Capabilities, "namespace.lifecycle.v1"), nil
}
func (c *Client) namespaceLifecycleRemote(ctx context.Context, source string) (*Remote, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(r.Capabilities, "namespace.lifecycle.v1") {
		return nil, errors.New("Hub does not support team lifecycle management; upgrade the Hub")
	}
	return r, nil
}
func (c *Client) NamespaceLifecycle(ctx context.Context, source, name string) (NamespaceLifecycle, error) {
	r, err := c.namespaceLifecycleRemote(ctx, source)
	if err != nil {
		return NamespaceLifecycle{}, err
	}
	v, err := r.Admin.GetNamespaceLifecycle(ctx, connect.NewRequest(&adminv1.GetNamespaceLifecycleRequest{Name: name}))
	if err != nil {
		return NamespaceLifecycle{}, err
	}
	return lifecycleInfo(v.Msg.Lifecycle), nil
}
func (c *Client) ChangeNamespaceLifecycle(ctx context.Context, source, name, action, revision, confirm, key string) (NamespaceLifecycle, error) {
	r, err := c.namespaceLifecycleRemote(ctx, source)
	if err != nil {
		return NamespaceLifecycle{}, err
	}
	rev, err := strconv.ParseUint(revision, 10, 64)
	if err != nil || rev == 0 {
		return NamespaceLifecycle{}, errors.New("a current namespace revision is required")
	}
	m := &commonv1.Mutation{IdempotencyKey: key}
	if action == "purge" {
		v, e := r.Admin.PurgeNamespace(ctx, connect.NewRequest(&adminv1.PurgeNamespaceRequest{Mutation: m, Name: name, ExpectedRevision: rev, ConfirmName: confirm}))
		if e != nil {
			return NamespaceLifecycle{}, e
		}
		return lifecycleInfo(v.Msg.Lifecycle), nil
	}
	if action != "archive" && action != "restore" {
		return NamespaceLifecycle{}, errors.New("unknown team lifecycle action")
	}
	v, e := r.Admin.SetNamespaceArchived(ctx, connect.NewRequest(&adminv1.SetNamespaceArchivedRequest{Mutation: m, Name: name, ExpectedRevision: rev, Archived: action == "archive"}))
	if e != nil {
		return NamespaceLifecycle{}, e
	}
	return lifecycleInfo(v.Msg.Lifecycle), nil
}
