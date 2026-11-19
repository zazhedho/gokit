// Package middleware holds the gin middleware: request ID, recovery, CORS and
// Redis rate limiting.
package middleware

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSOptions configures CORS.
type CORSOptions struct {
	// AllowOrigins are scheme://host[:port] origins, matched exactly. Empty or
	// "*" allows any origin; other entries with a path or trailing slash panic.
	AllowOrigins []string
	// Methods defaults to GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
	Methods []string
	// Headers defaults to Content-Type, Authorization, Accept, Cache-Control, X-Request-ID.
	Headers []string
	// Expose lists readable response headers; X-Request-ID is always added.
	Expose []string
	// AllowCredentials cannot be combined with an open origin list (it panics).
	AllowCredentials bool
	// MaxAge is the preflight cache age; zero omits the header.
	MaxAge time.Duration
}

// CORS answers real preflights (OPTIONS with Origin and Access-Control-Request-Method)
// with 204, or 403 for a disallowed origin; other requests pass through with the
// allowed origin echoed. It is open (any origin, no credentials) unless restricted.
func CORS(opts CORSOptions) gin.HandlerFunc {
	wildcard := len(opts.AllowOrigins) == 0 || slices.Contains(opts.AllowOrigins, "*")
	if wildcard && opts.AllowCredentials {
		panic("gokit: CORS cannot combine the \"*\" origin (or no AllowOrigins) with AllowCredentials")
	}
	for _, o := range opts.AllowOrigins {
		if o != "*" && !validOrigin(o) {
			panic("gokit: CORS origin must be scheme://host[:port] without a path or trailing slash: " + o)
		}
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

func validOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return !strings.ContainsAny(strings.TrimPrefix(o, u.Scheme+"://"), "/?#@ ")
}
