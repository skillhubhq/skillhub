package main

import "github.com/skillhubhq/skillhub/internal/client"

func (s *DesktopService) Evaluations(source, id string) (client.EvaluationPage, error) {
	return s.core.Evaluations(s.ctx, source, id)
}
func (s *DesktopService) Evaluation(source, id string) (client.EvaluationEvidence, error) {
	return s.core.Evaluation(s.ctx, source, id)
}
func (s *DesktopService) ImportEvaluation(source, id, document, key string) (string, error) {
	return s.core.ImportEvaluation(s.ctx, source, id, document, key)
}
func (s *DesktopService) RetractEvaluation(source, id, key string) error {
	return s.core.RetractEvaluation(s.ctx, source, id, key)
}
