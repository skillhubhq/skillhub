package client

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
)

type SkillMetadataChange struct {
	Namespace        string             `json:"namespace"`
	Name             string             `json:"name"`
	ExpectedRevision uint64             `json:"expected_revision"`
	DisplayNameEN    string             `json:"display_name_en"`
	DisplayNameZH    string             `json:"display_name_zh"`
	Description      string             `json:"description"`
	Icon             taxonomy.SkillIcon `json:"icon"`
	Category         string             `json:"category"`
	Tags             []string           `json:"tags"`
}

func (c *Client) UpdateSkillMetadata(ctx context.Context, source string, v SkillMetadataChange) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	_, err = r.Catalog.UpdateSkillMetadata(ctx, connect.NewRequest(&catalogv1.UpdateSkillMetadataRequest{Mutation: &commonv1.Mutation{IdempotencyKey: uuid.NewString()}, Namespace: v.Namespace, Name: v.Name, ExpectedRevision: v.ExpectedRevision, DisplayNameEn: v.DisplayNameEN, DisplayNameZh: v.DisplayNameZH, Description: v.Description, Icon: v.Icon, Category: v.Category, Tags: v.Tags}))
	return err
}
