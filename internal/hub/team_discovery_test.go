package hub

import (
	"context"
	"errors"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	c "github.com/skillhubhq/skillhub/internal/community"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
)

func TestEnterpriseTeamDiscovery(t *testing.T) { testEnterpriseTeamDiscovery(t, hubdb.Config{}) }
func TestPostgresEnterpriseTeamDiscovery(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testEnterpriseTeamDiscovery(t, cfg)
}
func testEnterpriseTeamDiscovery(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	defer s.Close()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	member := governanceUser(t, s, admin, "member")
	_, e = s.PutNamespace(ctx, admin, "create-team", PutNamespace{Name: "platform"})
	mustGovernance(t, e)
	_, e = s.UpdateTeam(ctx, admin, "team-profile", c.TeamChange{Namespace: "platform", DisplayName: "Platform", Bio: "Shared practices"})
	mustGovernance(t, e)
	teams, e := s.ListTeams(ctx, member, Page{})
	mustGovernance(t, e)
	if len(teams.Items) != 1 || teams.Items[0].DisplayName != "Platform" || teams.Items[0].CanEdit || teams.Items[0].CanViewMembers {
		t.Fatalf("directory changed privileges: %+v", teams.Items)
	}
	team, e := s.GetTeam(ctx, member, "platform")
	mustGovernance(t, e)
	if team.Bio != "Shared practices" {
		t.Fatal("missing team profile")
	}
	if _, e = s.ListMembers(ctx, member, "platform", Page{}); !errors.Is(e, ErrForbidden) {
		t.Fatal("directory exposes members", e)
	}
	if _, e = s.UpdateTeam(ctx, member, "edit-denied", c.TeamChange{Namespace: "platform", DisplayName: "Changed", ExpectedRevision: 1}); !errors.Is(e, ErrForbidden) {
		t.Fatal("directory permits edits", e)
	}
	group, e := s.CreateGroup(ctx, admin, "private-group", "private")
	mustGovernance(t, e)
	r, e := s.SubmitRelease(ctx, admin, "private-skill", SubmitRelease{Namespace: "platform", Version: "1.0.0", Archive: governanceArchive(t, "Private content"), Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIDs: []string{group.ID}})
	mustGovernance(t, e)
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED {
		t.Fatal("fixture must be a published restricted skill")
	}
	if _, _, e = s.DownloadRelease(ctx, member, r.Namespace, r.Name, r.Version); !errors.Is(e, ErrNotFound) {
		t.Fatal("directory exposes restricted skill", e)
	}
	if _, e = s.GetTeam(ctx, Identity{}, "platform"); !errors.Is(e, ErrNotFound) {
		t.Fatal("anonymous directory exposure", e)
	}
	s.policy.Mode = hubpolicy.Community
	if _, e = s.GetTeam(ctx, member, "platform"); !errors.Is(e, ErrNotFound) {
		t.Fatal("community exposes unrelated private team", e)
	}
	s.policy.Mode = hubpolicy.Enterprise
	_, e = s.SetNamespaceArchived(ctx, admin, "archive-team", ChangeNamespaceLifecycle{Name: "platform", ExpectedRevision: 1, Archived: true})
	mustGovernance(t, e)
	teams, e = s.ListTeams(ctx, member, Page{})
	mustGovernance(t, e)
	if len(teams.Items) != 0 {
		t.Fatal("archived team is discoverable")
	}
}
