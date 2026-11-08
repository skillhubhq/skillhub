// Package hubhttp 将有界 Connect API 与最小浏览器授权页适配到 Hub 领域。
package hubhttp

import (
	"context"
	"errors"
	"github.com/skillhubhq/skillhub/internal/community"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/gen/api/admin/v1/adminv1connect"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	"github.com/skillhubhq/skillhub/gen/api/catalog/v1/catalogv1connect"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	"github.com/skillhubhq/skillhub/gen/api/community/v1/communityv1connect"
	"github.com/skillhubhq/skillhub/gen/api/evaluation/v1/evaluationv1connect"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
	"github.com/skillhubhq/skillhub/gen/api/publishing/v1/publishingv1connect"
	registryv1 "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubpolicy"
	"github.com/skillhubhq/skillhub/internal/identityprovider"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/webui"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	Site               community.Site
	Policy             hubpolicy.Policy
	Providers          []identityprovider.Config
	providerHTTPClient *http.Client
	TrustedProxies     []string
	DisplayName        string
	PublicURL          string
	Private            bool
	Logger             *slog.Logger
}

type Server struct {
	avatarMu      sync.Mutex
	avatarTasks   map[string]context.CancelFunc
	avatarWG      sync.WaitGroup
	avatarStopped bool
	proxies       []netip.Prefix
	providers     map[string]*externalLogin
	providerOrder []string
	store         *hub.Store
	config        Config
	handler       http.Handler
	limiter       *rateLimiter
	uploads       chan struct{}
}

func New(store *hub.Store, config Config) (*Server, error) {
	u, err := url.Parse(config.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("public URL must be an absolute HTTP(S) origin")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return nil, errors.New("public URL requires HTTPS except on loopback")
		}
	}
	config.PublicURL = strings.TrimSuffix(config.PublicURL, "/")
	if config.DisplayName == "" {
		config.DisplayName = "SkillHub"
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	validator, err := protovalidate.New()
	if err != nil {
		return nil, err
	}
	s := &Server{avatarTasks: make(map[string]context.CancelFunc), store: store, config: config, limiter: &rateLimiter{entries: make(map[string]rateEntry)}, uploads: make(chan struct{}, 2)}
	for _, raw := range config.TrustedProxies {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		prefix, e := netip.ParsePrefix(strings.TrimSpace(raw))
		if e != nil {
			return nil, errors.New("trusted proxies must be explicit IP CIDRs")
		}
		s.proxies = append(s.proxies, prefix)
	}
	if err := store.ConfigurePolicy(config.Policy); err != nil {
		return nil, err
	}
	if err := s.configureProviders(); err != nil {
		return nil, err
	}
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			message, ok := req.Any().(proto.Message)
			if !ok {
				return nil, publicError(hub.ErrInvalid)
			}
			if err := validator.Validate(message); err != nil {
				return nil, publicError(hub.ErrInvalid)
			}
			response, err := next(ctx, req)
			if err != nil {
				return nil, publicError(err)
			}
			return response, nil
		}
	})
	opts := []connect.HandlerOption{connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(8192)}
	mux := http.NewServeMux()
	mux.Handle(authv1connect.NewAuthServiceHandler(s, opts...))
	mux.Handle(governancev1connect.NewGovernanceServiceHandler(s, opts...))
	mux.Handle(evaluationv1connect.NewEvaluationServiceHandler(s, connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(400<<10)))
	mux.Handle(communityv1connect.NewCommunityServiceHandler(s, connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(1<<20)))
	mux.Handle(registryv1connect.NewRegistryServiceHandler(s, opts...))
	mux.Handle(adminv1connect.NewAdminServiceHandler(s, connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(64<<10)))
	mux.Handle(catalogv1connect.NewCatalogServiceHandler(s, connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(64<<10)))
	mux.Handle(publishingv1connect.NewPublishingServiceHandler(s, connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(32<<20)))
	s.registerProviderRoutes(mux)
	mux.HandleFunc("/authorize", s.authorize)
	mux.HandleFunc("/login", s.session)
	mux.HandleFunc("/logout", s.session)
	mux.Handle("/", webui.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	s.handler = s.boundary(mux)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-s.store.DatabaseLeaseLost():
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	default:
	}
	s.handler.ServeHTTP(w, r)
}

func (s *Server) GetRegistry(ctx context.Context, req *connect.Request[registryv1.GetRegistryRequest]) (*connect.Response[registryv1.GetRegistryResponse], error) {
	capabilities := []string{"identity.local", "identity.device", "catalog.v1", "publishing.review.v1", "publishing.cleanup.v1", "admin.v1", "namespace.lifecycle.v1", "community.v1", "community.moderation.v1", "catalog.presentation.v1", "catalog.provenance.v1", "governance.v1", "catalog.discovery.v1", "governance.tasks.v1", "evaluation.v1"}

	if s.config.Policy.IsCommunity() {
		capabilities = append(capabilities, "community.public.v1")
	}
	if len(s.providers) > 0 {
		capabilities = append(capabilities, "identity.external")
		// 保留协议能力声明供旧客户端探测；登录界面仍只消费通用提供方列表。
		seen := map[string]bool{}
		for _, provider := range s.config.Providers {
			if !seen[provider.Type] {
				capabilities = append(capabilities, "identity."+provider.Type)
				seen[provider.Type] = true
			}
		}
	}
	return connect.NewResponse(&registryv1.GetRegistryResponse{RegistryId: s.store.RegistryID(), DisplayName: s.config.DisplayName, ProtocolMajor: 1, PrivateMode: s.config.Private, Capabilities: capabilities, MaxPackageBytes: packageformat.MaxArchiveBytes, CommunityMode: s.config.Policy.IsCommunity(), PrivacyUrl: s.config.Site.PrivacyURL, TermsUrl: s.config.Site.TermsURL, ContactUrl: s.config.Site.ContactURL, FilingText: s.config.Site.FilingText, FilingUrl: s.config.Site.FilingURL}), nil
}

func publicError(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}
	code := connect.CodeInternal
	key := "INTERNAL"
	message := "request could not be completed"
	switch {
	case errors.Is(err, hub.ErrQuota):
		code = connect.CodeResourceExhausted
		key = "COMMUNITY_QUOTA"
		message = "community quota reached; contact the hub operator"
	case errors.Is(err, hub.ErrUnauthenticated):
		code = connect.CodeUnauthenticated
		key = "AUTH_REQUIRED"
		message = "authentication required or credentials invalid"
	case errors.Is(err, hub.ErrInvalid):
		code = connect.CodeInvalidArgument
		key = "INVALID_REQUEST"
		message = "invalid request"
	case errors.Is(err, hub.ErrForbidden):
		code = connect.CodePermissionDenied
		key = "FORBIDDEN"
		message = "permission denied"
	case errors.Is(err, hub.ErrNotFound):
		code = connect.CodeNotFound
		key = "NOT_FOUND"
		message = "resource not found"
	case errors.Is(err, hub.ErrScanRequired):
		code = connect.CodeFailedPrecondition
		key = "SCAN_REQUIRED"
		message = "inspect the current scan report and record a review note for findings before approval"
	case errors.Is(err, hub.ErrConflict):
		code = connect.CodeAlreadyExists
		key = "CONFLICT"
		message = "resource already exists or changed"
	case errors.Is(err, hub.ErrRateLimited):
		code = connect.CodeResourceExhausted
		key = "RATE_LIMITED"
		message = "too many attempts; try again later"
	case errors.Is(err, context.Canceled):
		code = connect.CodeCanceled
		key = "CANCELED"
		message = "request canceled"
	case errors.Is(err, context.DeadlineExceeded):
		code = connect.CodeDeadlineExceeded
		key = "TIMEOUT"
		message = "request timed out"
	}
	result := connect.NewError(code, errors.New(message))
	if detail, e := connect.NewErrorDetail(&commonv1.ErrorDetail{ErrorCode: key, SafeMessage: message}); e == nil {
		result.AddDetail(detail)
	}
	return result
}

func bearer(header http.Header) (string, error) {
	values := header.Values("Authorization")
	if len(values) != 1 {
		return "", hub.ErrUnauthenticated
	}
	parts := strings.Split(values[0], " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != 43 {
		return "", hub.ErrUnauthenticated
	}
	return parts[1], nil
}

func (s *Server) identity(ctx context.Context, header http.Header) (hub.Identity, error) {
	token, err := bearer(header)
	if err != nil {
		return hub.Identity{}, err
	}
	return s.store.Authenticate(ctx, token)
}

func (s *Server) boundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if !s.browserSession(w, r) {
			return
		}
		if s.config.Policy.IsCommunity() && strings.HasPrefix(r.URL.Path, "/api.") && !s.limiter.allow("community|"+s.clientAddress(r), s.config.Policy.Limits.RequestsPerMinute) {
			w.Header().Set("Retry-After", "60")
			_ = connect.NewErrorWriter().Write(w, r, publicError(hub.ErrRateLimited))
			return
		}
		if r.URL.Path == publishingv1connect.PublishingServiceSubmitReleaseProcedure || r.URL.Path == communityv1connect.CommunityServiceUpdateAvatarProcedure || (r.URL.Path == governancev1connect.GovernanceServiceScanReleaseProcedure || r.URL.Path == evaluationv1connect.EvaluationServiceImportEvidenceProcedure) {
			if _, err := s.identity(r.Context(), r.Header); err != nil {
				_ = connect.NewErrorWriter().Write(w, r, publicError(err))
				return
			}
			select {
			case s.uploads <- struct{}{}:
				defer func() { <-s.uploads }()
			default:
				_ = connect.NewErrorWriter().Write(w, r, publicError(hub.ErrRateLimited))
				return
			}
		}
		defer func() {
			if recover() != nil {
				s.config.Logger.Error("request panic", "request_id", id)
				http.Error(w, "request could not be completed", http.StatusInternalServerError)
			}
		}()
		if (r.URL.Path == governancev1connect.GovernanceServiceScanReleaseProcedure || r.URL.Path == evaluationv1connect.EvaluationServiceImportEvidenceProcedure) || strings.HasPrefix(r.URL.Path, "/auth/") || strings.HasPrefix(r.URL.Path, "/api.auth.v1.AuthService/") || r.URL.Path == "/authorize" || r.URL.Path == "/login" || r.URL.Path == "/logout" {
			ip := s.clientAddress(r)
			limit := 120
			if (r.URL.Path == governancev1connect.GovernanceServiceScanReleaseProcedure || r.URL.Path == evaluationv1connect.EvaluationServiceImportEvidenceProcedure) || r.URL.Path == authv1connect.AuthServiceLoginProcedure || r.URL.Path == authv1connect.AuthServiceStartDeviceLoginProcedure || ((r.URL.Path == "/authorize" || r.URL.Path == "/login") && r.Method == http.MethodPost) {
				limit = 20
			}
			if !s.limiter.allow(ip+"|"+r.URL.Path+"|"+r.Method, limit) {
				w.Header().Set("Retry-After", "60")
				if strings.HasPrefix(r.URL.Path, "/api.") {
					_ = connect.NewErrorWriter().Write(w, r, publicError(hub.ErrRateLimited))
				} else {
					http.Error(w, "too many attempts; try again later", http.StatusTooManyRequests)
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type rateEntry struct {
	until time.Time
	count int
}
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

func (l *rateLimiter) allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[key]
	if !ok || !entry.until.After(now) {
		if len(l.entries) >= 10000 {
			for k, v := range l.entries {
				if !v.until.After(now) {
					delete(l.entries, k)
				}
			}
			if len(l.entries) >= 10000 {
				return false
			}
		}
		entry = rateEntry{until: now.Add(time.Minute)}
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}
