package main

import "github.com/skillhubhq/skillhub/internal/client"

func (s *DesktopService) NamespaceLifecycleSupported(source string) (bool, error) {
	return s.core.NamespaceLifecycleSupported(s.ctx, source)
}
func (s *DesktopService) NamespaceLifecycle(source, name string) (client.NamespaceLifecycle, error) {
	return s.core.NamespaceLifecycle(s.ctx, source, name)
}
func (s *DesktopService) ChangeNamespaceLifecycle(source, name, action, revision, confirm, key string) (client.NamespaceLifecycle, error) {
	return s.core.ChangeNamespaceLifecycle(s.ctx, source, name, action, revision, confirm, key)
}
