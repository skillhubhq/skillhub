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

type limitedRegistry struct {
	registryv1connect.UnimplementedRegistryServiceHandler
}

func (limitedRegistry) GetRegistry(context.Context, *connect.Request[registryv1.GetRegistryRequest]) (*connect.Response[registryv1.GetRegistryResponse], error) {
	return connect.NewResponse(&registryv1.GetRegistryResponse{RegistryId: "limited-fixture", DisplayName: "Limited", ProtocolMajor: 2, Capabilities: []string{"catalog.v1"}}), nil
}
func TestGovernanceMissingCapabilityIsNotCleanScan(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle(registryv1connect.NewRegistryServiceHandler(limitedRegistry{}))
	server := httptest.NewServer(mux)
	defer server.Close()
	store, e := client.OpenStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	c := client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{unavailable: true}})
	ctx := context.Background()
	if _, e = c.AddSource(ctx, "limited", server.URL); e != nil {
		t.Fatal(e)
	}
	tasks, e := c.ReviewTasks(ctx, "limited", "")
	if e != nil || tasks.Supported || len(tasks.Tasks) > 0 {
		t.Fatal("limited tasks", tasks, e)
	}
	if e = c.SetFeaturedSkill(ctx, "limited", "team", "skill", true, "test-key"); e == nil {
		t.Fatal("limited curation accepted")
	}
	results, e := c.Search(ctx, []string{"limited"}, &catalogv1.SearchSkillsRequest{Order: taxonomy.CatalogOrder_CATALOG_ORDER_RECENT})
	if e != nil || len(results) != 1 || results[0].Error == "" {
		t.Fatal("limited order silently ignored", results, e)
	}
	evidence, e := c.Evaluations(ctx, "limited", "release")
	if e != nil || evidence.Supported || len(evidence.Evidence) != 0 {
		t.Fatal("unsupported evidence appeared available", e)
	}
	if _, e = c.Evaluation(ctx, "limited", "report"); e == nil {
		t.Fatal("unsupported evidence detail accepted")
	}
	if e = c.RetractEvaluation(ctx, "limited", "report", "key"); e == nil {
		t.Fatal("unsupported evidence mutation accepted")
	}
	r, e := c.ScanReport(ctx, "limited", "release")
	if e != nil || r.Supported || r.Complete || r.Engine != "" {
		t.Fatal("unsupported scan appeared valid", r, e)
	}
	n, e := c.Notifications(ctx, "limited", "")
	if e != nil || n.Supported {
		t.Fatal(e)
	}
	a, e := c.PlatformAccess(ctx, "limited", "")
	if e != nil || a.Supported || a.CanManageUsers || a.CanReadAudit {
		t.Fatal(e)
	}
	if _, e = c.ScanRelease(ctx, "limited", "release", "scan-key"); e == nil {
		t.Fatal("unsupported scan accepted")
	}
}
