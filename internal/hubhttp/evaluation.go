package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	evaluationv1 "github.com/skillhubhq/skillhub/gen/api/evaluation/v1"
	"github.com/skillhubhq/skillhub/internal/hub"
)

func evidenceDTO(e hub.EvaluationEvidence) *evaluationv1.Evidence {
	return &evaluationv1.Evidence{Id: e.ID, ReleaseId: e.ReleaseID, Digest: e.Digest, SuiteName: e.SuiteName, SuiteVersion: e.SuiteVersion, SuiteDigest: e.SuiteDigest, Runner: e.Runner, Agent: e.Agent, Model: e.Model, ExecutedAt: e.ExecutedAt, ImportedAt: e.ImportedAt, Passed: e.Passed, Failed: e.Failed, Errors: e.Errors, Skipped: e.Skipped, BaselinePassed: e.BaselinePassed, BaselineTotal: e.BaselineTotal, Retracted: e.Retracted, DocumentJson: e.DocumentJSON}
}
func (s *Server) ImportEvidence(ctx context.Context, req *connect.Request[evaluationv1.ImportEvidenceRequest]) (*connect.Response[evaluationv1.ImportEvidenceResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	id, err := s.store.ImportEvaluation(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.ReleaseId, req.Msg.Document)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&evaluationv1.ImportEvidenceResponse{Id: id}), nil
}
func (s *Server) ListEvidence(ctx context.Context, req *connect.Request[evaluationv1.ListEvidenceRequest]) (*connect.Response[evaluationv1.ListEvidenceResponse], error) {
	u, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	page, err := s.store.ListEvaluations(ctx, u, req.Msg.ReleaseId)
	if err != nil {
		return nil, err
	}
	out := &evaluationv1.ListEvidenceResponse{CanManage: page.CanManage}
	for _, e := range page.Evidence {
		out.Evidence = append(out.Evidence, evidenceDTO(e))
	}
	return connect.NewResponse(out), nil
}
func (s *Server) GetEvidence(ctx context.Context, req *connect.Request[evaluationv1.GetEvidenceRequest]) (*connect.Response[evaluationv1.GetEvidenceResponse], error) {
	u, err := s.catalogIdentity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	e, err := s.store.GetEvaluation(ctx, u, req.Msg.Id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&evaluationv1.GetEvidenceResponse{Evidence: evidenceDTO(e)}), nil
}
func (s *Server) RetractEvidence(ctx context.Context, req *connect.Request[evaluationv1.RetractEvidenceRequest]) (*connect.Response[evaluationv1.RetractEvidenceResponse], error) {
	u, err := s.identity(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	if err = s.store.RetractEvaluation(ctx, u, req.Msg.GetMutation().GetIdempotencyKey(), req.Msg.Id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&evaluationv1.RetractEvidenceResponse{}), nil
}
