package hub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenance"
)

func TestProvenanceReviewAndIdentitySeparation(t *testing.T) {
	testProvenance(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresProvenanceReviewAndIdentitySeparation(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testProvenance(t, cfg)
}
func testProvenance(t *testing.T, cfg hubdb.Config) {
	s, e := OpenDatabase(t.TempDir(), cfg)
	mustGovernance(t, e)
	defer s.Close()
	ctx := context.Background()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	curator := governanceUser(t, s, admin, "curator")
	_, e = s.PutNamespace(ctx, admin, "provenance-space", PutNamespace{Name: "curated", ReviewRequired: false})
	mustGovernance(t, e)
	e = s.SetNamespaceGrant(ctx, admin, "provenance-grant", SetNamespaceGrant{NamespaceGrant: NamespaceGrant{Namespace: "curated", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: curator.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}, Granted: true})
	mustGovernance(t, e)
	dir := t.TempDir()
	mustGovernance(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: upstream-example\ndescription: A curated example\n---\nOriginal skill content."), 0600))
	mustGovernance(t, os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("Example license notice"), 0600))
	archive, _, e := packageformat.Pack(dir)
	mustGovernance(t, e)
	p := &provenance.Record{AuthorName: "Original Authors", AuthorURL: "https://example.org/authors", SourceURL: "https://example.org/source/" + strings.Repeat("a", 40), Revision: strings.Repeat("a", 40), License: "Apache-2.0", LicenseFile: "LICENSE.txt"}
	cmd := SubmitRelease{Namespace: "curated", Version: "0.0.0-upstream.aaaaaaaaaaaa", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, Provenance: p}
	r, e := s.SubmitRelease(ctx, curator, "provenance-submit", cmd)
	mustGovernance(t, e)
	if r.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING || r.SubmittedBy != curator.ID || r.Namespace != "curated" || *r.Provenance != *p {
		t.Fatal("identity attribution or forced review lost")
	}
	if _, e = s.ReviewRelease(ctx, curator, "provenance-self", ReviewRelease{ReleaseID: r.ID, Approve: true, Reason: "Self"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("self review accepted", e)
	}
	if _, e = s.GetSkill(ctx, Identity{}, "curated", "upstream-example"); !errors.Is(e, ErrNotFound) {
		t.Fatal("pending attribution exposed", e)
	}
	_, e = s.ReviewRelease(ctx, admin, "provenance-approve", ReviewRelease{ReleaseID: r.ID, Approve: true, Reason: "Reviewed source, license and content"})
	mustGovernance(t, e)
	skill, e := s.GetSkill(ctx, Identity{}, "curated", "upstream-example")
	mustGovernance(t, e)
	if skill.Provenance == nil || *skill.Provenance != *p {
		t.Fatal("catalog lost provenance")
	}
	contributions, e := s.ListContributions(ctx, Identity{}, "", "curated", Page{}, false)
	mustGovernance(t, e)
	if len(contributions.Items) != 1 || contributions.Items[0].Author.UserID != curator.ID || contributions.Items[0].Provenance.AuthorName != p.AuthorName {
		t.Fatal("operator was replaced with upstream identity")
	}
	got, data, e := s.DownloadRelease(ctx, Identity{}, "curated", "upstream-example", cmd.Version)
	mustGovernance(t, e)
	if *got.Provenance != *p || packageformat.Digest(data) != packageformat.Digest(archive) {
		t.Fatal("archive or attribution changed")
	}
	changed := *p
	changed.AuthorName = "Different Author"
	cmd.Provenance = &changed
	if _, e = s.SubmitRelease(ctx, curator, "provenance-submit", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("idempotency ignored attribution", e)
	}
	if _, e = s.SubmitRelease(ctx, curator, "provenance-new-key", cmd); !errors.Is(e, ErrConflict) {
		t.Fatal("existing version attribution overwritten", e)
	}
	var actor string
	mustGovernance(t, s.db.QueryRow("SELECT actor_id FROM audit WHERE action='release.review' AND resource=?", r.ID).Scan(&actor))
	if actor != admin.ID {
		t.Fatal("review audit identity lost")
	}
}
