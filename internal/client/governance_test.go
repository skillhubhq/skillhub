package client_test

import (
	"connectrpc.com/connect"
	"context"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	registryv1 "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"net/http"
	"net/http/httptest"
	"testing"
)

type legacyRegistry struct {
	registryv1connect.UnimplementedRegistryServiceHandler
}

func (legacyRegistry) GetRegistry(context.Context, *connect.Request[registryv1.GetRegistryRequest]) (*connect.Response[registryv1.GetRegistryResponse], error) {
	return connect.NewResponse(&registryv1.GetRegistryResponse{RegistryId: "legacy-fixture", DisplayName: "Legacy", ProtocolMajor: 1, Capabilities: []string{"catalog.v1"}}), nil
}
func TestGovernanceMissingCapabilityIsNotCleanScan(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle(registryv1connect.NewRegistryServiceHandler(legacyRegistry{}))
	server := httptest.NewServer(mux)
	defer server.Close()
	store, e := client.OpenStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	c := client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{unavailable: true}})
	ctx := context.Background()
	if _, e = c.AddSource(ctx, "legacy", server.URL); e != nil {
		t.Fatal(e)
	}
	tasks, e := c.ReviewTasks(ctx, "legacy", "")
	if e != nil || tasks.Supported || len(tasks.Tasks) > 0 {
		t.Fatal("legacy tasks", tasks, e)
	}
	if e = c.SetFeaturedSkill(ctx, "legacy", "team", "skill", true, "test-key"); e == nil {
		t.Fatal("legacy curation accepted")
	}
	results, e := c.Search(ctx, []string{"legacy"}, &catalogv1.SearchSkillsRequest{Order: taxonomy.CatalogOrder_CATALOG_ORDER_RECENT})
	if e != nil || len(results) != 1 || results[0].Error == "" {
		t.Fatal("legacy order silently ignored", results, e)
	}
	evidence, e := c.Evaluations(ctx, "legacy", "release")
	if e != nil || evidence.Supported || len(evidence.Evidence) != 0 {
		t.Fatal("unsupported evidence appeared available", e)
	}
	if _, e = c.Evaluation(ctx, "legacy", "report"); e == nil {
		t.Fatal("unsupported evidence detail accepted")
	}
	if e = c.RetractEvaluation(ctx, "legacy", "report", "key"); e == nil {
		t.Fatal("unsupported evidence mutation accepted")
	}
	r, e := c.ScanReport(ctx, "legacy", "release")
	if e != nil || r.Supported || r.Complete || r.Engine != "" {
		t.Fatal("unsupported scan appeared valid", r, e)
	}
	n, e := c.Notifications(ctx, "legacy", "")
	if e != nil || n.Supported {
		t.Fatal(e)
	}
	a, e := c.PlatformAccess(ctx, "legacy", "")
	if e != nil || a.Supported || a.CanManageUsers || a.CanReadAudit {
		t.Fatal(e)
	}
	if _, e = c.ScanRelease(ctx, "legacy", "release", "scan-key"); e == nil {
		t.Fatal("unsupported scan accepted")
	}
}
