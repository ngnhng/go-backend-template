package ratelimit

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"app/modules/middleware/problem"
	rl "app/modules/ratelimit"
)

type (
	Pattern string
	method  string

	// KeyFunc extracts from a HTTP request an identifier such as remote IP, user-agent, cookies, etc.
	KeyFunc func(*http.Request) rl.Key

	// RouteInfoFunc extracts from a HTTP request the route information needed for pattern matching
	RouteInfoFunc func(*http.Request) RouteInfo

	// RouteInfo represents the framework-agnostic route information used in this middleware
	RouteInfo struct {
		ID     Pattern
		Method string
		Path   string
	}

	Policy struct {
		Limiter rl.RateLimiter
		KeyFn   KeyFunc
	}

	// compiled policy to be injected and used at runtime
	RuntimePolicy struct {
		// Policies parsed from config struct so that each route-method is accompanied with
		// a rate limiter with the pre-configured rate limit specifications.
		policyMap map[Pattern]map[method]Policy

		// Default policies applied when no route/method-specific policy exists.
		// A method-specific default takes precedence over the catch-all default.
		defaultPolicyByMethod map[method]Policy
		defaultPolicy         *Policy

		// Allow to next middleware if rate limit policy is not configured for this route
		AllowIfNoMatch bool
		// Allow to next middleware if no identifier is extracted from the http.Request using KeyFn
		AllowIfNoIdentifier bool

		RouteInfoFn RouteInfoFunc
	}
)

type policySource string

const (
	policySourceExplicit      policySource = "explicit"
	policySourceDefaultMethod policySource = "default_method"
	policySourceDefaultAll    policySource = "default"
)

func normalizeMethod(m string) method {
	return method(strings.ToUpper(m))
}

func (p *RuntimePolicy) findPolicy(routeInfo RouteInfo) (Policy, bool, policySource) {
	if pm, ok := p.policyMap[Pattern(routeInfo.ID)]; ok {
		if px, ok := pm[normalizeMethod(routeInfo.Method)]; ok {
			return px, true, policySourceExplicit
		}
	}

	if routeInfo.Method != "" && p.defaultPolicyByMethod != nil {
		if px, ok := p.defaultPolicyByMethod[normalizeMethod(routeInfo.Method)]; ok {
			return px, true, policySourceDefaultMethod
		}
	}

	if p.defaultPolicy != nil {
		return *p.defaultPolicy, true, policySourceDefaultAll
	}

	return Policy{}, false, ""
}

// here we assume the env config for route patterns must correctly reflects the registered routes by the framework
func ParsePolicy(
	factory rl.LimiterFactory,
	cfg *RestHTTPConfig,
	routeFn RouteInfoFunc,
	keyStrategies map[KeyStrategyId]KeyFunc,
) (*RuntimePolicy, error) {
	rtp := &RuntimePolicy{
		policyMap:           make(map[Pattern]map[method]Policy, 0),
		AllowIfNoIdentifier: cfg.AllowIfNoIdentifier,
		AllowIfNoMatch:      cfg.AllowIfNoMatch,
		RouteInfoFn:         routeFn,
	}

	// Default policy fallback (optional). Consider it configured only when it has
	// enough information to enforce rate limiting (window + key strategy).
	if cfg.DefaultPolicy.Window > 0 && cfg.DefaultPolicy.KeyStrategy != "" {
		ksn := KeyStrategyId(cfg.DefaultPolicy.KeyStrategy)
		ks, ok := keyStrategies[ksn]
		if !ok {
			return nil, errors.New("ratelimit parse policy: no such default key strategy")
		}

		p := Policy{
			Limiter: factory(cfg.DefaultPolicy.Limit, cfg.DefaultPolicy.Window),
			KeyFn:   ks,
		}

		if cfg.DefaultPolicy.Method != "" {
			rtp.defaultPolicyByMethod = map[method]Policy{
				normalizeMethod(cfg.DefaultPolicy.Method): p,
			}
		} else {
			rtp.defaultPolicy = &p
		}
	}

	for _, r := range cfg.Routes {

		// TODO: should we leave '/' handling to the user?
		pat := Pattern(r.Pattern)
		if _, ok := rtp.policyMap[pat]; !ok {
			rtp.policyMap[pat] = make(map[method]Policy)
		}

		endpointRules := r.EndpointRules

		for _, rule := range endpointRules {
			m := normalizeMethod(rule.Method)
			if _, ok := rtp.policyMap[pat][m]; ok {
				return nil, errors.New("ratelimit parse policy: duplicate method config on same pattern")
			}

			ksn := KeyStrategyId(rule.KeyStrategy)
			ks, ok := keyStrategies[ksn]
			if !ok {
				return nil, errors.New("ratelimit parse policy: no such key strategy")
			}

			rtp.policyMap[pat][m] = Policy{
				Limiter: factory(
					rule.Limit,
					rule.Window,
				),
				KeyFn: ks,
			}
		}
	}
	return rtp, nil
}

func NewRateLimitMiddleware(p *RuntimePolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeInfo := p.RouteInfoFn(r)
			if routeInfo.Method == "" {
				slog.Error("no method found",
					slog.String("middleware", "rate_limiter"),
					slog.String("url", r.URL.Path),
					slog.Any("route_info", routeInfo),
				)
				problem.Write(w, problem.MethodNotAllowed("method not allowed"))
				return
			}

			px, ok, src := p.findPolicy(routeInfo)
			if !ok {
				if routeInfo.ID == "" {
					if p.AllowIfNoMatch {
						next.ServeHTTP(w, r)
						return
					}
					problem.Write(w, problem.MethodNotAllowed("not allowed"))
					return
				}

				slog.Warn("no rate limit policy found",
					slog.String("middleware", "rate_limiter"),
					slog.String("url", r.URL.Path),
					slog.Any("route_info", routeInfo),
				)
				if p.AllowIfNoMatch {
					next.ServeHTTP(w, r)
					return
				}
				problem.Write(w, problem.TooManyRequests(http.StatusText(http.StatusTooManyRequests)))
				return
			}

			if src != policySourceExplicit {
				slog.Debug("using default rate limit policy",
					slog.String("middleware", "rate_limiter"),
					slog.String("url", r.URL.Path),
					slog.String("policy_source", string(src)),
					slog.Any("route_info", routeInfo),
				)
			}

			if px.KeyFn == nil {
				if !p.AllowIfNoIdentifier {
					slog.Warn("no rate limit key func found",
						slog.String("middleware", "rate_limiter"),
						slog.String("url", r.URL.Path),
						slog.Any("route_info", routeInfo),
					)
					problem.Write(w, problem.TooManyRequests(http.StatusText(http.StatusTooManyRequests)))
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			key := px.KeyFn(r)
			if key == "" && !p.AllowIfNoIdentifier {
				slog.Warn("bad key",
					slog.String("middleware", "rate_limiter"),
					slog.String("url", r.URL.Path),
					slog.Any("route_info", routeInfo),
					slog.String("key", string(key)),
				)
				problem.Write(w, problem.TooManyRequests(http.StatusText(http.StatusTooManyRequests)))
				return
			}

			result, err := px.Limiter.Allow(r.Context(), key)
			if err != nil {
				slog.Error("rate limit error",
					slog.Any("error", err),
					slog.String("url", r.URL.Path),
				)
				// Counter store may be down
				problem.Write(w, problem.Internal(http.StatusText(http.StatusInternalServerError)))
				return
			}

			// generated code's response visitor unconditionally does w.Header().Set("X-RateLimit-Limit", fmt.Sprint(response.Headers.XRateLimitLimit)), etc.
			// so we have to re-apply before response is committed
			w = &rateLimitHeaderWriter{ResponseWriter: w, result: result}

			if !result.Allowed {
				slog.Debug("rate limited",
					slog.String("middleware", "rate_limiter"),
					slog.String("url", r.URL.Path),
				)
				problem.Write(w, problem.TooManyRequests(http.StatusText(http.StatusTooManyRequests)))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeRateLimitHeaders(w http.ResponseWriter, result rl.Result) {
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
	h.Set("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
	h.Set("X-RateLimit-Window-Seconds",
		strconv.FormatInt(int64(result.Window.Seconds()), 10))
	h.Set("X-RateLimit-Reset-Seconds",
		strconv.FormatInt(int64(result.WindowResetIn.Seconds()), 10))
}

type rateLimitHeaderWriter struct {
	http.ResponseWriter
	result  rl.Result
	ensured bool
}

func (w *rateLimitHeaderWriter) ensure() {
	if w.ensured {
		return
	}
	writeRateLimitHeaders(w.ResponseWriter, w.result)
	w.ensured = true
}

func (w *rateLimitHeaderWriter) WriteHeader(statusCode int) {
	w.ensure()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *rateLimitHeaderWriter) Write(p []byte) (int, error) {
	w.ensure()
	return w.ResponseWriter.Write(p)
}

func (w *rateLimitHeaderWriter) Flush() {
	w.ensure()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// TODO: temporary mechanism, realistically need something more robust (api key, jwt, etc)
func RemoteIpKeyFunc(r *http.Request) rl.Key {
	h := r.Header
	ips := strings.Split(h.Get("X-Forwarded-For"), ",")
	if len(ips) == 0 {
		return rl.Key(r.RemoteAddr)
	}

	return rl.Key(ips[len(ips)-1])
}
