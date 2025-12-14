package api

import (
	"github.com/labstack/echo/v4"
)

// SecurityHeadersMiddleware adds security headers to all responses
// to protect against common web vulnerabilities
func SecurityHeadersMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			res := c.Response()

			// Set security headers before calling next handler
			// This ensures they're set even if handler errors

			// X-Frame-Options: Prevent clickjacking attacks
			// DENY = page cannot be displayed in a frame/iframe
			if res.Header().Get("X-Frame-Options") == "" {
				res.Header().Set("X-Frame-Options", "DENY")
			}

			// X-Content-Type-Options: Prevent MIME type sniffing
			// nosniff = browser must respect declared Content-Type
			if res.Header().Get("X-Content-Type-Options") == "" {
				res.Header().Set("X-Content-Type-Options", "nosniff")
			}

			// X-XSS-Protection: Legacy XSS protection for older browsers
			// 1; mode=block = enable filter and block page if attack detected
			if res.Header().Get("X-XSS-Protection") == "" {
				res.Header().Set("X-XSS-Protection", "1; mode=block")
			}

			// Referrer-Policy: Control referrer information sent
			// strict-origin-when-cross-origin = send origin for cross-origin, full URL for same-origin
			if res.Header().Get("Referrer-Policy") == "" {
				res.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			}

			// Strict-Transport-Security (HSTS): Enforce HTTPS
			// Only set for HTTPS requests (setting on HTTP can cause issues)
			if c.Request().URL.Scheme == "https" && res.Header().Get("Strict-Transport-Security") == "" {
				res.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			// Content-Security-Policy: Protect against XSS and injection attacks
			// This is a balanced policy that allows common use cases while maintaining security
			if res.Header().Get("Content-Security-Policy") == "" {
				csp := "default-src 'self'; " +
					"script-src 'self' 'unsafe-inline' https://unpkg.com https://cdnjs.cloudflare.com https://*.posthog.com https://*.i.posthog.com; " + // Allow HTMX, Monaco, and PostHog
					"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; " + // unsafe-inline needed for inline styles, Monaco CSS from CDN
					"img-src 'self' data: https:; " + // Allow images from same origin, data URIs, and HTTPS
					"font-src 'self' data: https://cdnjs.cloudflare.com; " + // Allow fonts from same origin, data URIs, and Monaco fonts
					"connect-src 'self' https://*.posthog.com https://*.i.posthog.com; " + // Allow PostHog analytics
					"worker-src 'self' blob: https://cdnjs.cloudflare.com;" // Allow PostHog web workers and Monaco workers

				res.Header().Set("Content-Security-Policy", csp)
			}

			// Call next handler
			return next(c)
		}
	}
}
