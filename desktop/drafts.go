package main

import "github.com/skillhubhq/skillhub/internal/client"

func (s *DesktopService) CreateDraft(directory string, target client.PreviewTarget) (client.DraftWorkspace, error) {
	return s.core.CreateDraft(s.ctx, directory, target)
}
func (s *DesktopService) ReadDraftFile(id, path string) (client.DraftFile, error) {
	return s.store.ReadDraftFile(id, path)
}
func (s *DesktopService) SaveDraftFile(id, path, digest, text string) (client.DraftFile, error) {
	return s.store.SaveDraftFile(s.ctx, id, path, digest, text)
}

func (s *DesktopService) ListDrafts() ([]client.DraftSummary, error) { return s.store.ListDrafts() }
func (s *DesktopService) OpenDraft(id string) (client.DraftWorkspace, error) {
	return s.store.OpenDraft(id)
}
