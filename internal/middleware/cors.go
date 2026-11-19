package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSOptions configures CORS.
type CORSOptions struct {
	// AllowOrigins lists the origins allowed to call the API, matched exactly
	// (case-insensitively). "*" allows any origin. Empty allows none.
	AllowOrigins []string
	// Methods are the allowed methods. Default: GET, POST, PUT, PATCH, DELETE,
	// HEAD, OPTIONS.
	Methods []string
	// Headers are the allowed request headers. Default: Content-Type,
	// Authorization, Accept, Cache-Control, X-Request-ID.
	Headers []string
	// Expose are response headers scripts may read; X-Request-ID is always added.
	Expose []string
	// AllowCredentials sends Access-Control-Allow-Credentials: true. It cannot be
	// combined with the "*" origin; CORS panics at construction if it is.
	AllowCredentials bool
	// MaxAge is how long browsers may cache a preflight; zero omits the header.
	MaxAge time.Duration
}

// CORS returns CORS middleware. Requests without an Origin header pass through
// untouched. A preflight (OPTIONS with Origin and Access-Control-Request-Method)
// from an allowed origin gets 204 with the Allow headers; from any other origin
// it gets 403. Other OPTIONS requests reach the handlers. Allowed origins are
// echoed back (or "*") on actual requests together with the Expose headers.
func CORS(opts CORSOptions) gin.HandlerFunc {
	wildcard := slices.Contains(opts.AllowOrigins, "*")
	if wildcard && opts.AllowCredentials {
		panic("gokit: CORS cannot combine the \"*\" origin with AllowCredentials")
	}

	methods := opts.Methods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
	headers := opts.Headers
	if len(headers) == 0 {
		headers = []string{"Content-Type", "Authorization", "Accept", "Cache-Control", "X-Request-ID"}
	}
	expose := slices.Clone(opts.Expose)
	if !slices.ContainsFunc(expose, func(h string) bool { return strings.EqualFold(h, "X-Request-ID") }) {
		expose = append(expose, "X-Request-ID")
	}

	allowedMethods := strings.Join(methods, ", ")
	allowedHeaders := strings.Join(headers, ", ")
	exposed := strings.Join(expose, ", ")
	maxAge := ""
	if opts.MaxAge > 0 {
		maxAge = strconv.FormatInt(int64(opts.MaxAge/time.Second), 10)
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""

		allowed := wildcard || slices.ContainsFunc(opts.AllowOrigins, func(o string) bool { return strings.EqualFold(o, origin) })
		h := c.Writer.Header()
		if !wildcard {
			h.Add("Vary", "Origin")
		}
		if !allowed {
			if preflight {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		if wildcard {
			h.Set("Access-Control-Allow-Origin", "*")
		} else {
			h.Set("Access-Control-Allow-Origin", origin)
		}
		if opts.AllowCredentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if preflight {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", allowedMethods)
			h.Set("Access-Control-Allow-Headers", allowedHeaders)
			if maxAge != "" {
				h.Set("Access-Control-Max-Age", maxAge)
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		h.Set("Access-Control-Expose-Headers", exposed)
		c.Next()
	}
}
