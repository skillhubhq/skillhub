package main

import (
	"errors"
	"slices"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
)

type SkillView struct {
	Provenance    *provenance.Record `json:"provenance,omitempty"`
	Featured      bool               `json:"featured"`
	DisplayNameEN string             `json:"display_name_en"`
	DisplayNameZH string             `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon `json:"icon"`
	Namespace     string             `json:"namespace"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Category      string             `json:"category"`
	Tags          []string           `json:"tags"`
	Version       string             `json:"version"`
}
type SearchPage struct {
	Source   string      `json:"source"`
	Skills   []SkillView `json:"skills"`
	NextPage string      `json:"next_page"`
	Error    string      `json:"error"`
}
type ReleasePage struct {
	Releases []client.ReleaseInfo `json:"releases"`
	NextPage string               `json:"next_page"`
}

func (s *DesktopService) Search(source, query, category string, tags []string, token string, order taxonomy.CatalogOrder, featured bool) ([]SearchPage, error) {
	aliases := []string{}
	if source != "" {
		aliases = append(aliases, source)
	}
	results, err := s.core.Search(s.ctx, aliases, &catalogv1.SearchSkillsRequest{Query: query, Category: category, Tags: tags, Order: order, FeaturedOnly: featured, Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}})
	if err != nil {
		return nil, err
	}
	pages := []SearchPage{}
	for _, result := range results {
		page := SearchPage{Source: result.Source, Skills: []SkillView{}, NextPage: result.NextPage, Error: result.Error}
		for _, skill := range result.Skills {
			tags := skill.Tags
			if tags == nil {
				tags = []string{}
			}
			page.Skills = append(page.Skills, SkillView{Namespace: skill.Namespace, Name: skill.Name, DisplayNameEN: skill.DisplayNameEn, DisplayNameZH: skill.DisplayNameZh, Icon: skill.Icon, Provenance: provenancewire.Decode(skill.Provenance), Description: skill.Description, Category: skill.Category, Tags: tags, Version: skill.LatestVersion, Featured: skill.Featured})
		}
		pages = append(pages, page)
	}
	return pages, nil
}
func (s *DesktopService) Releases(coordinate, token string) (ReleasePage, error) {
	c, err := client.ParseCoordinate(coordinate, false)
	if err != nil {
		return ReleasePage{}, err
	}
	response, err := s.core.Releases(s.ctx, c, token)
	if err != nil {
		return ReleasePage{}, err
	}
	result := ReleasePage{Releases: []client.ReleaseInfo{}, NextPage: response.GetPage().GetNextPageToken()}
	for _, release := range response.Releases {
		result.Releases = append(result.Releases, client.ReleaseInformation(release))
	}
	return result, nil
}
func (s *DesktopService) Preview(coordinate string) (client.ReleasePreview, error) {
	c, err := client.ParseCoordinate(coordinate, true)
	if err != nil {
		return client.ReleasePreview{}, err
	}
	return s.core.PreviewRelease(s.ctx, c)
}
func (s *DesktopService) Submissions(source, namespace, token string) (ReleasePage, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return ReleasePage{}, err
	}
	response, err := remote.Publishing.ListSubmissions(s.ctx, connect.NewRequest(&publishingv1.ListSubmissionsRequest{Namespace: namespace, Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}}))
	if err != nil {
		return ReleasePage{}, err
	}
	result := ReleasePage{Releases: []client.ReleaseInfo{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, release := range response.Msg.Releases {
		result.Releases = append(result.Releases, client.ReleaseInformation(release))
	}
	return result, nil
}
func (s *DesktopService) PreviewSubmission(source, id string) (client.ReleasePreview, error) {
	return s.core.PreviewSubmission(s.ctx, source, id)
}
func (s *DesktopService) ReadPreviewFile(target client.PreviewTarget, path string) (client.PreviewContent, error) {
	return s.core.ReadPreviewFile(s.ctx, target, path)
}

type PublishForm struct {
	Provenance    *provenance.Record  `json:"provenance,omitempty"`
	DisplayNameEN string              `json:"display_name_en"`
	DisplayNameZH string              `json:"display_name_zh"`
	Icon          taxonomy.SkillIcon  `json:"icon"`
	Source        string              `json:"source"`
	Directory     string              `json:"directory"`
	Namespace     string              `json:"namespace"`
	Version       string              `json:"version"`
	Visibility    taxonomy.Visibility `json:"visibility"`
	Groups        []string            `json:"groups"`
	Category      string              `json:"category"`
	Tags          []string            `json:"tags"`
	RequestID     string              `json:"request_id"`
}

type DraftInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Markdown    string   `json:"markdown"`
	Digest      string   `json:"digest"`
	Files       []string `json:"files"`
}

func (s *DesktopService) InspectDirectory(directory string) (DraftInfo, error) {
	archive, _, err := packageformat.Pack(directory)
	if err != nil {
		return DraftInfo{}, err
	}
	bundle, err := packageformat.Inspect(archive)
	if err != nil {
		return DraftInfo{}, err
	}
	result := DraftInfo{Name: bundle.Metadata.Name, Description: bundle.Metadata.Description, Markdown: bundle.Markdown, Digest: bundle.Digest, Files: []string{}}
	for _, file := range bundle.Files {
		result.Files = append(result.Files, file.Path)
	}
	return result, nil
}

// PublishChecked 拒绝预览之后被修改的目录，避免确认内容与实际提交不一致。
func (s *DesktopService) PublishChecked(form PublishForm, digest string) (client.ReleaseInfo, error) {
	if digest == "" {
		return client.ReleaseInfo{}, errors.New("preview required")
	}
	return s.publish(form, digest)
}
func (s *DesktopService) Publish(form PublishForm) (client.ReleaseInfo, error) {
	return s.publish(form, "")
}
func (s *DesktopService) publish(form PublishForm, expected string) (client.ReleaseInfo, error) {
	remote, err := s.core.Remote(s.ctx, form.Source)
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	if form.Provenance != nil {
		if err = remote.RequireProvenance(); err != nil {
			return client.ReleaseInfo{}, err
		}
	}
	if form.DisplayNameEN != "" || form.DisplayNameZH != "" || form.Icon != 0 {
		if err = remote.RequireSkillPresentation(); err != nil {
			return client.ReleaseInfo{}, err
		}
	}
	archive, _, err := packageformat.Pack(form.Directory)
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	if expected != "" && packageformat.Digest(archive) != expected {
		return client.ReleaseInfo{}, errors.New("content changed after preview; inspect the directory again")
	}
	response, err := remote.Publishing.SubmitRelease(s.ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: &commonv1.Mutation{IdempotencyKey: form.RequestID}, Namespace: form.Namespace, Version: form.Version, Archive: archive, Visibility: form.Visibility, GroupIds: form.Groups, Category: form.Category, Tags: form.Tags, DisplayNameEn: form.DisplayNameEN, DisplayNameZh: form.DisplayNameZH, Icon: form.Icon, Provenance: provenancewire.Encode(form.Provenance)}))
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	return client.ReleaseInformation(response.Msg.Release), nil
}
func (s *DesktopService) Review(source, id, reason, requestID string, approve bool) (client.ReleaseInfo, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	response, err := remote.Publishing.ReviewRelease(s.ctx, connect.NewRequest(&publishingv1.ReviewReleaseRequest{Mutation: &commonv1.Mutation{IdempotencyKey: requestID}, ReleaseId: id, Approve: approve, Reason: reason}))
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	return client.ReleaseInformation(response.Msg.Release), nil
}
func (s *DesktopService) Yank(source, id, reason, requestID string) (client.ReleaseInfo, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	response, err := remote.Publishing.YankRelease(s.ctx, connect.NewRequest(&publishingv1.YankReleaseRequest{Mutation: &commonv1.Mutation{IdempotencyKey: requestID}, ReleaseId: id, Reason: reason}))
	if err != nil {
		return client.ReleaseInfo{}, err
	}
	return client.ReleaseInformation(response.Msg.Release), nil
}

func (s *DesktopService) GetSkill(source, namespace, name string) (SkillView, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return SkillView{}, err
	}
	response, err := remote.Catalog.GetSkill(s.ctx, connect.NewRequest(&catalogv1.GetSkillRequest{Namespace: namespace, Name: name}))
	if err != nil {
		return SkillView{}, err
	}
	v := response.Msg.Skill
	if v == nil {
		return SkillView{}, errors.New("skill unavailable")
	}
	return SkillView{Namespace: v.Namespace, Name: v.Name, DisplayNameEN: v.DisplayNameEn, DisplayNameZH: v.DisplayNameZh, Icon: v.Icon, Provenance: provenancewire.Decode(v.Provenance), Description: v.Description, Category: v.Category, Tags: v.Tags, Version: v.LatestVersion, Featured: v.Featured}, nil
}
func (s *DesktopService) FilterSubmissions(source, query string, status taxonomy.ReleaseStatus, mine bool, token string, activeOnly bool) (ReleasePage, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return ReleasePage{}, err
	}
	response, err := remote.Publishing.ListSubmissions(s.ctx, connect.NewRequest(&publishingv1.ListSubmissionsRequest{ActiveOnly: activeOnly, Query: query, Status: status, Mine: mine, Page: &commonv1.PageRequest{PageSize: 30, PageToken: token}}))
	if err != nil {
		return ReleasePage{}, err
	}
	result := ReleasePage{Releases: []client.ReleaseInfo{}, NextPage: response.Msg.GetPage().GetNextPageToken()}
	for _, r := range response.Msg.Releases {
		result.Releases = append(result.Releases, client.ReleaseInformation(r))
	}
	return result, nil
}

func (s *DesktopService) DeleteSubmission(source, id, coordinate, requestID string) error {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return err
	}
	_, err = remote.Publishing.DeleteRelease(s.ctx, connect.NewRequest(&publishingv1.DeleteReleaseRequest{Mutation: &commonv1.Mutation{IdempotencyKey: requestID}, ReleaseId: id, ConfirmCoordinate: coordinate}))
	return err
}
func (s *DesktopService) SubmissionCleanupSupported(source string) (bool, error) {
	remote, err := s.core.Remote(s.ctx, source)
	if err != nil {
		return false, err
	}
	return slices.Contains(remote.Capabilities, "publishing.cleanup.v1"), nil
}
