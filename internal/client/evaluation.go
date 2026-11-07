package client

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	evaluationv1 "github.com/skillhubhq/skillhub/gen/api/evaluation/v1"
	"github.com/skillhubhq/skillhub/internal/evaluation"
	"slices"
)

type EvaluationEvidence struct {
	ID             string `json:"id"`
	ReleaseID      string `json:"release_id"`
	Digest         string `json:"digest"`
	SuiteName      string `json:"suite_name"`
	SuiteVersion   string `json:"suite_version"`
	SuiteDigest    string `json:"suite_digest"`
	Runner         string `json:"runner"`
	Agent          string `json:"agent"`
	Model          string `json:"model"`
	ExecutedAt     string `json:"executed_at"`
	ImportedAt     string `json:"imported_at"`
	Passed         uint32 `json:"passed"`
	Failed         uint32 `json:"failed"`
	Errors         uint32 `json:"errors"`
	Skipped        uint32 `json:"skipped"`
	BaselinePassed uint32 `json:"baseline_passed"`
	BaselineTotal  uint32 `json:"baseline_total"`
	Retracted      bool   `json:"retracted"`
	DocumentJSON   string `json:"document_json"`
}
type EvaluationPage struct {
	Supported bool                 `json:"supported"`
	Evidence  []EvaluationEvidence `json:"evidence"`
	CanManage bool                 `json:"can_manage"`
}

func evidenceInfo(e *evaluationv1.Evidence) EvaluationEvidence {
	if e == nil {
		return EvaluationEvidence{}
	}
	return EvaluationEvidence{e.Id, e.ReleaseId, e.Digest, e.SuiteName, e.SuiteVersion, e.SuiteDigest, e.Runner, e.Agent, e.Model, e.ExecutedAt, e.ImportedAt, e.Passed, e.Failed, e.Errors, e.Skipped, e.BaselinePassed, e.BaselineTotal, e.Retracted, e.DocumentJson}
}
func (c *Client) Evaluations(ctx context.Context, source, id string) (EvaluationPage, error) {
	out := EvaluationPage{Evidence: []EvaluationEvidence{}}
	r, err := c.Remote(ctx, source)
	if err != nil {
		return out, err
	}
	if !slices.Contains(r.Capabilities, "evaluation.v1") {
		return out, nil
	}
	v, err := r.Evaluation.ListEvidence(ctx, connect.NewRequest(&evaluationv1.ListEvidenceRequest{ReleaseId: id}))
	if err != nil {
		return out, err
	}
	out.Supported = true
	out.CanManage = v.Msg.CanManage
	for _, e := range v.Msg.Evidence {
		out.Evidence = append(out.Evidence, evidenceInfo(e))
	}
	return out, nil
}
func (c *Client) Evaluation(ctx context.Context, source, id string) (EvaluationEvidence, error) {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return EvaluationEvidence{}, err
	}
	if !slices.Contains(r.Capabilities, "evaluation.v1") {
		return EvaluationEvidence{}, errors.New("hub does not support evaluation evidence")
	}
	v, err := r.Evaluation.GetEvidence(ctx, connect.NewRequest(&evaluationv1.GetEvidenceRequest{Id: id}))
	if err != nil {
		return EvaluationEvidence{}, err
	}
	return evidenceInfo(v.Msg.Evidence), nil
}
func (c *Client) ImportEvaluation(ctx context.Context, source, id, document, key string) (string, error) {
	if _, err := evaluation.Parse([]byte(document)); err != nil {
		return "", err
	}
	r, err := c.Remote(ctx, source)
	if err != nil {
		return "", err
	}
	if !slices.Contains(r.Capabilities, "evaluation.v1") {
		return "", errors.New("hub does not support evaluation evidence")
	}
	v, err := r.Evaluation.ImportEvidence(ctx, connect.NewRequest(&evaluationv1.ImportEvidenceRequest{ReleaseId: id, Document: []byte(document), Mutation: &commonv1.Mutation{IdempotencyKey: key}}))
	if err != nil {
		return "", err
	}
	return v.Msg.Id, nil
}
func (c *Client) RetractEvaluation(ctx context.Context, source, id, key string) error {
	r, err := c.Remote(ctx, source)
	if err != nil {
		return err
	}
	if !slices.Contains(r.Capabilities, "evaluation.v1") {
		return errors.New("hub does not support evaluation evidence")
	}
	_, err = r.Evaluation.RetractEvidence(ctx, connect.NewRequest(&evaluationv1.RetractEvidenceRequest{Id: id, Mutation: &commonv1.Mutation{IdempotencyKey: key}}))
	return err
}
