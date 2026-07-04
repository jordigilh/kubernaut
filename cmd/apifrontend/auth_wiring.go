package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
)

// replayCacheTTL matches (and slightly exceeds) the maximum expected token
// lifetime so replayed tokens cannot outlive their own cache entry.
const replayCacheTTL = 10 * time.Minute

// buildReplayCache constructs the jti replay-detection store selected by
// cfg (GAP-08, #1505). When cfg specifies a distributed "redis"/"valkey"
// backend, replay state is shared across all APIFrontend replicas via Valkey;
// if that backend cannot be constructed (bad address, unreadable credentials),
// it falls back to the legacy single-process in-memory cache rather than
// disabling replay protection outright — logged loudly so the HA degradation
// is observable. legacyEnable preserves the pre-GAP-08 boolean toggle for
// configs that predate the structured auth.replayCache block.
func buildReplayCache(cfg *config.ReplayCacheConfig, legacyEnable bool, logger logr.Logger) auth.ReplayCacheStore {
	if cfg == nil {
		if legacyEnable {
			return auth.NewReplayCache(replayCacheTTL)
		}
		return nil
	}
	if !cfg.IsDistributed() {
		return auth.NewReplayCache(replayCacheTTL)
	}
	rc, err := newValkeyReplayCache(cfg, logger)
	if err != nil {
		logger.Error(err, "failed to initialize valkey replay cache; falling back to in-memory cache (HA replay detection degraded)",
			"redisAddr", cfg.RedisAddr)
		return auth.NewReplayCache(replayCacheTTL)
	}
	logger.Info("auth mode: distributed replay cache (valkey)", "redisAddr", cfg.RedisAddr, "redisDB", cfg.RedisDB)
	return rc
}

// newValkeyReplayCache builds a Redis client from cfg and wraps it in a
// ValkeyReplayCache. Credentials are optional: an empty CredentialsPath
// connects without authentication (dev/test Valkey instances).
func newValkeyReplayCache(cfg *config.ReplayCacheConfig, logger logr.Logger) (*auth.ValkeyReplayCache, error) {
	password, err := loadReplayCachePassword(cfg.CredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("load replay cache credentials: %w", err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: password,
		DB:       cfg.RedisDB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping valkey at %s: %w", cfg.RedisAddr, err)
	}
	return auth.NewValkeyReplayCache(client, replayCacheTTL, logger), nil
}

// loadReplayCachePassword reads the "password" key from a YAML credentials
// file mounted from a Kubernetes Secret (same "password" key convention as
// DataStorage's valkey-secrets.yaml projection). Returns "" without error
// when path is empty (unauthenticated Valkey).
func loadReplayCachePassword(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path from trusted, operator-controlled config
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var secrets map[string]string
	if err := yaml.Unmarshal(data, &secrets); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	password, ok := secrets["password"]
	if !ok {
		return "", fmt.Errorf(`%s: missing required "password" key`, path)
	}
	return password, nil
}

// denyAllMiddleware returns a middleware that rejects every request with a
// 503, used whenever the auth system cannot be safely initialized.
func denyAllMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "authentication system unavailable", http.StatusServiceUnavailable)
		})
	}
}

// notReadyChecker returns a ReadyChecker that always reports not-ready,
// paired with denyAllMiddleware when auth infra is unavailable.
func notReadyChecker() handler.ReadyChecker {
	return handler.ReadyChecker(func() bool { return false })
}

// getK8sConfigFunc resolves the in-cluster or local kubeconfig used by the
// TokenReview auth fallback. It is a package-level indirection (rather than
// calling ctrl.GetConfig directly) so unit tests can deterministically
// exercise both the "kubeconfig available" and "kubeconfig unavailable"
// branches of buildTokenReviewFallback (AF-CRIT-1) without requiring a real
// cluster. Production code always uses the real ctrl.GetConfig.
var getK8sConfigFunc = ctrl.GetConfig

// buildTokenReviewFallback assembles the TokenReview-based JWT validator
// option used when no OIDC/JWT issuer is configured (AF-CRIT-1). Returns
// ok=false with a deny-all middleware and not-ready checker if kubeconfig or
// the Kubernetes client cannot be constructed — fail closed rather than
// silently allowing unauthenticated requests.
func buildTokenReviewFallback(logger logr.Logger) (opt auth.JWTValidatorOption, denyAll func(http.Handler) http.Handler, notReady handler.ReadyChecker, ok bool) {
	restCfg, k8sErr := getK8sConfigFunc()
	if k8sErr != nil {
		logger.Error(k8sErr, "CRITICAL: no auth issuer configured and kubeconfig unavailable — denying all authenticated requests (AF-CRIT-1)")
		return nil, denyAllMiddleware(), notReadyChecker(), false
	}
	k8sClient, k8sErr := kubernetes.NewForConfig(restCfg)
	if k8sErr != nil {
		logger.Error(k8sErr, "CRITICAL: failed to create kubernetes client for TokenReview — denying all authenticated requests (AF-CRIT-1)")
		return nil, denyAllMiddleware(), notReadyChecker(), false
	}
	logger.Info("auth mode: TokenReview (no OIDC issuer configured)")
	return auth.WithTokenReviewer(auth.NewTokenReviewer(k8sClient)), nil, nil, true
}

// buildOIDCValidatorOpts assembles JWT validator options for the OIDC/JWKS
// auth mode: an optional custom-CA HTTP client for the JWKS fetcher, plus the
// replay cache. Returns ok=false with a deny-all middleware if the custom CA
// HTTP client cannot be built.
func buildOIDCValidatorOpts(cfg *config.Config, authCfg auth.Config, logger logr.Logger) (opts []auth.JWTValidatorOption, denyAll func(http.Handler) http.Handler, ok bool) {
	if cfg.Auth.OIDCCaFile != "" {
		httpClient, err := buildOIDCHTTPClient(cfg.Auth.OIDCCaFile)
		if err != nil {
			logger.Error(err, "failed to build OIDC HTTP client with custom CA")
			return nil, denyAllMiddleware(), false
		}
		opts = append(opts, auth.WithHTTPClient(httpClient))
		logger.Info("OIDC JWKS fetcher configured with custom CA", "caFile", cfg.Auth.OIDCCaFile)
	}
	if rc := buildReplayCache(cfg.Auth.ReplayCache, cfg.Auth.EnableReplayProtection, logger); rc != nil {
		opts = append(opts, auth.WithReplayCache(rc))
	}
	logger.Info("auth mode: OIDC/JWKS", "providers", len(authCfg.JWT))
	return opts, nil, true
}

func buildAuthMiddleware(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (func(http.Handler) http.Handler, handler.ReadyChecker) {
	alwaysReady := handler.ReadyChecker(func() bool { return true })

	authCfg := buildAuthConfig(cfg)

	var validatorOpts []auth.JWTValidatorOption

	if len(authCfg.JWT) == 0 {
		tokenReviewOpt, denyAll, notReady, ok := buildTokenReviewFallback(logger)
		if !ok {
			return denyAll, notReady
		}
		validatorOpts = append(validatorOpts, tokenReviewOpt)
	} else {
		oidcOpts, denyAll, ok := buildOIDCValidatorOpts(cfg, authCfg, logger)
		if !ok {
			return denyAll, alwaysReady
		}
		validatorOpts = append(validatorOpts, oidcOpts...)
	}
	providerLimiter := ratelimit.NewProviderLimiter(ratelimit.PerProviderConfig{
		FetchIntervalSeconds: 300,
	})
	validatorOpts = append(validatorOpts, auth.WithProviderLimiter(providerLimiter))
	validatorOpts = append(validatorOpts, auth.WithCBMetrics(reg.CircuitBreakerState))
	validator, err := auth.NewJWTValidator(authCfg, validatorOpts...)
	if err != nil {
		logger.Error(err, "failed to create JWT validator — falling back to deny-all")
		return denyAllMiddleware(), alwaysReady
	}

	mw := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
		Validator:    validator,
		Logger:       logger,
		Auditor:      auditor,
		AuthDuration: reg.AuthDuration,
	})
	return mw, validator.Ready
}

// buildOIDCHTTPClient creates an HTTP client that trusts the system CAs plus
// the additional CA bundle at caFile. Used to reach OIDC providers whose TLS
// certificate is signed by a non-public CA (e.g., OpenShift ingress operator).
func buildOIDCHTTPClient(caFile string) (*http.Client, error) {
	caPEM, err := os.ReadFile(caFile) //nolint:gosec // path from trusted config
	if err != nil {
		return nil, fmt.Errorf("reading OIDC CA file %s: %w", caFile, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid certificates found in %s", caFile)
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

func parseLogLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level: %q", s)
	}
}

// buildAuthConfig maps config.AuthConfig to auth.Config.
// Priority: jwtProviders[] > legacy issuerURL > empty (TokenReview auto-detect).
func buildAuthConfig(cfg *config.Config) auth.Config {
	if len(cfg.Auth.JWTProviders) > 0 {
		providers := make([]auth.ProviderConfig, 0, len(cfg.Auth.JWTProviders))
		for _, p := range cfg.Auth.JWTProviders {
			providers = append(providers, auth.ProviderConfig{
				Issuer: auth.IssuerConfig{
					URL:       p.IssuerURL,
					JWKSURL:   p.JWKSURL,
					Audiences: p.Audiences,
				},
				ClaimMappings: auth.ClaimMappings{
					Username: p.ClaimMappings.Username,
					Groups:   p.ClaimMappings.Groups,
				},
			})
		}
		return auth.Config{
			JWT:                  providers,
			AllowInsecureIssuers: cfg.Auth.AllowInsecureIssuers,
		}
	}
	if cfg.Auth.IssuerURL != "" {
		return auth.Config{
			JWT: []auth.ProviderConfig{
				{
					Issuer: auth.IssuerConfig{
						URL:       cfg.Auth.IssuerURL,
						JWKSURL:   cfg.Auth.JWKSURL,
						Audiences: []string{cfg.Auth.Audience},
					},
				},
			},
			AllowInsecureIssuers: cfg.Auth.AllowInsecureIssuers,
		}
	}
	return auth.Config{}
}
