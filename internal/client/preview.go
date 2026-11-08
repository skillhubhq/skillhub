package client

import (
	"bytes"
	"context"
	"errors"
	"unicode/utf8"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
)

type ReleaseInfo struct {
	Provenance    *provenance.Record     `json:"provenance,omitempty"`
	DisplayNameEN string                 `json:"display_name_en"`
	DisplayNameZH string                 `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon     `json:"icon"`
	ID            string                 `json:"id"`
	Namespace     string                 `json:"namespace"`
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Digest        string                 `json:"digest"`
	Status        taxonomy.ReleaseStatus `json:"status"`
	Visibility    taxonomy.Visibility    `json:"visibility"`
	Groups        []string               `json:"groups"`
	SubmittedBy   string                 `json:"submitted_by"`
	CreatedAt     string                 `json:"created_at"`
	Reason        string                 `json:"reason"`
	Markdown      string                 `json:"markdown"`
}

func ReleaseInformation(r *catalogv1.Release) ReleaseInfo {
	if r == nil {
		return ReleaseInfo{}
	}
	result := ReleaseInfo{ID: r.ReleaseId, Namespace: r.Namespace, Name: r.Name, DisplayNameEN: r.DisplayNameEn, DisplayNameZH: r.DisplayNameZh, Icon: r.Icon, Provenance: provenancewire.Decode(r.Provenance), Version: r.Version, Digest: r.Digest, Status: r.Status, Visibility: r.Visibility, Groups: r.GroupIds, SubmittedBy: r.SubmittedBy, Reason: r.ReviewReason, Markdown: r.SkillMarkdown}
	if result.Groups == nil {
		result.Groups = []string{}
	}
	if r.CreatedAt != nil {
		result.CreatedAt = r.CreatedAt.AsTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	return result
}

type PreviewFile struct {
	Path       string `json:"path"`
	Size       int    `json:"size"`
	Executable bool   `json:"executable"`
}
type ReleasePreview struct {
	Release ReleaseInfo   `json:"release"`
	Files   []PreviewFile `json:"files"`
	Target  PreviewTarget `json:"target"`
}

// PreviewTarget 绑定待查看的固定版本；文件读取仍需重新验证源身份与下载权限。
type PreviewTarget struct {
	Source       string `json:"source"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	SubmissionID string `json:"submission_id"`
	Digest       string `json:"digest"`
}
type PreviewContent struct {
	Path    string `json:"path"`
	Text    bool   `json:"text"`
	Content string `json:"content"`
}

func inspectPreviewBundle(release *catalogv1.Release, archive []byte) (*packageformat.Bundle, error) {
	if release == nil || packageformat.Digest(archive) != release.Digest {
		return nil, errors.New("preview archive does not match release digest")
	}
	bundle, err := packageformat.Inspect(archive)
	if err != nil {
		return nil, err
	}
	if bundle.Metadata.Name != release.Name {
		return nil, errors.New("preview skill name does not match release")
	}
	return bundle, nil
}
func inspectPreview(release *catalogv1.Release, archive []byte) (ReleasePreview, error) {
	bundle, err := inspectPreviewBundle(release, archive)
	if err != nil {
		return ReleasePreview{}, err
	}
	result := ReleasePreview{Release: ReleaseInformation(release), Files: []PreviewFile{}}
	// 正文来自已校验的包，不能让独立的服务端预览文本替代审核内容。
	result.Release.Markdown = bundle.Markdown
	for _, file := range bundle.Files {
		result.Files = append(result.Files, PreviewFile{Path: file.Path, Size: len(file.Data), Executable: file.Executable})
	}
	return result, nil
}
func (c *Client) PreviewRelease(ctx context.Context, coordinate Coordinate) (ReleasePreview, error) {
	return c.preview(ctx, PreviewTarget{Source: coordinate.Source, Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version})
}
func (c *Client) PreviewSubmission(ctx context.Context, alias, id string) (ReleasePreview, error) {
	if id == "" {
		return ReleasePreview{}, errors.New("submission ID is required")
	}
	return c.preview(ctx, PreviewTarget{Source: alias, SubmissionID: id})
}
func (c *Client) preview(ctx context.Context, target PreviewTarget) (ReleasePreview, error) {
	release, archive, err := c.downloadPreview(ctx, target)
	if err != nil {
		return ReleasePreview{}, err
	}
	result, err := inspectPreview(release, archive)
	if err != nil {
		return ReleasePreview{}, err
	}
	target.Digest = release.Digest
	result.Target = target
	return result, nil
}
func (c *Client) downloadPreview(ctx context.Context, target PreviewTarget) (*catalogv1.Release, []byte, error) {
	remote, err := c.Remote(ctx, target.Source)
	if err != nil {
		return nil, nil, err
	}
	if target.SubmissionID != "" {
		response, err := remote.Publishing.GetSubmission(ctx, connect.NewRequest(&publishingv1.GetSubmissionRequest{ReleaseId: target.SubmissionID}))
		if err != nil {
			return nil, nil, err
		}
		if response.Msg.Release.GetReleaseId() != target.SubmissionID {
			return nil, nil, errors.New("preview does not match requested submission")
		}
		return response.Msg.Release, response.Msg.Archive, nil
	}
	return c.downloadRelease(ctx, remote, Coordinate{Source: target.Source, Namespace: target.Namespace, Name: target.Name, Version: target.Version})
}
func (c *Client) ReadPreviewFile(ctx context.Context, target PreviewTarget, path string) (PreviewContent, error) {
	release, archive, err := c.downloadPreview(ctx, target)
	if err != nil {
		return PreviewContent{}, err
	}
	if target.Digest == "" || release.GetDigest() != target.Digest {
		return PreviewContent{}, errors.New("file content does not match preview digest")
	}
	bundle, err := inspectPreviewBundle(release, archive)
	if err != nil {
		return PreviewContent{}, err
	}
	return previewFile(bundle, path)
}
func previewFile(bundle *packageformat.Bundle, path string) (PreviewContent, error) {
	// 仅查找包内的精确路径，不访问本地文件系统；单文件大小已由 Inspect 限制。
	for _, file := range bundle.Files {
		if file.Path != path {
			continue
		}
		result := PreviewContent{Path: file.Path, Text: utf8.Valid(file.Data) && !bytes.ContainsRune(file.Data, '\x00')}
		if result.Text {
			result.Content = string(file.Data)
		}
		return result, nil
	}
	return PreviewContent{}, errors.New("file is not present in the preview archive")
}
