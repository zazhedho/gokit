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

// requestIDHeader is always exposed and allowed so clients can read the ID.
const requestIDHeader = "X-Request-ID"

// CORS answers real preflights (OPTIONS with Origin and Access-Control-Request-Method)
// with 204, or 403 for a disallowed origin; other requests pass through with the
// allowed origin echoed. It is open (any origin, no credentials) unless restricted.
func CORS(opts CORSOptions) gin.HandlerFunc {
	return newCORSPolicy(opts).handle
}

// corsPolicy holds the validated options and the pre-joined header values.
type corsPolicy struct {
	origins        []string
	wildcard       bool
	credentials    bool
	allowedMethods string
	allowedHeaders string
	exposed        string
	maxAge         string
}

func newCORSPolicy(opts CORSOptions) *corsPolicy {
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
		headers = []string{"Content-Type", "Authorization", "Accept", "Cache-Control", requestIDHeader}
	}
	expose := slices.Clone(opts.Expose)
	if !slices.ContainsFunc(expose, func(h string) bool { return strings.EqualFold(h, requestIDHeader) }) {
		expose = append(expose, requestIDHeader)
	}

	maxAge := ""
	if opts.MaxAge > 0 {
		maxAge = strconv.FormatInt(int64(opts.MaxAge/time.Second), 10)
	}
	return &corsPolicy{
		origins:        opts.AllowOrigins,
		wildcard:       wildcard,
		credentials:    opts.AllowCredentials,
		allowedMethods: strings.Join(methods, ", "),
		allowedHeaders: strings.Join(headers, ", "),
		exposed:        strings.Join(expose, ", "),
		maxAge:         maxAge,
	}
}

func (p *corsPolicy) originAllowed(origin string) bool {
	return p.wildcard || slices.ContainsFunc(p.origins, func(o string) bool { return strings.EqualFold(o, origin) })
}

func (p *corsPolicy) handle(c *gin.Context) {
	if !p.wildcard {
		c.Writer.Header().Add("Vary", "Origin")
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		c.Next()
		return
	}
	preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""

	if !p.originAllowed(origin) {
		if preflight {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
		return
	}

	h := c.Writer.Header()
	if p.wildcard {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", origin)
	}
	if p.credentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	if preflight {
		p.answerPreflight(c)
		return
	}
	h.Set("Access-Control-Expose-Headers", p.exposed)
	c.Next()
}

func (p *corsPolicy) answerPreflight(c *gin.Context) {
	h := c.Writer.Header()
	h.Add("Vary", "Access-Control-Request-Method")
	h.Add("Vary", "Access-Control-Request-Headers")
	h.Set("Access-Control-Allow-Methods", p.allowedMethods)
	h.Set("Access-Control-Allow-Headers", p.allowedHeaders)
	if p.maxAge != "" {
		h.Set("Access-Control-Max-Age", p.maxAge)
	}
	c.AbortWithStatus(http.StatusNoContent)
}

func validOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return !strings.ContainsAny(strings.TrimPrefix(o, u.Scheme+"://"), "/?#@ ")
}
