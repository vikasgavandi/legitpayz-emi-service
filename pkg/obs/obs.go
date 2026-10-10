// Package observability: one-line RED metrics + structured JSON logs for every service.
//
// Usage:
//
//	r := obs.Engine("customer") // gin engine with Recovery, request logging, /metrics
//
// Metrics (Prometheus):
//
//	cc_http_requests_total{service,method,route,code}
//	cc_http_request_duration_seconds{service,route}
//
// Logs (slog JSON, stdout): service, request_id, method, route, path, status,
// latency_ms, client_ip. Failures (4xx/5xx, panics) additionally log error
// (handler error or response body), query, user_agent; panics add stack.
// /health and /metrics are excluded from both to keep signals clean.
package obs

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "cc_http_requests_total", Help: "HTTP requests by service, method, route and status code."},
		[]string{"service", "method", "route", "code"},
	)
	latency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cc_http_request_duration_seconds",
			Help:    "HTTP request latency by service and route.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"service", "route"},
	)
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
)

func init() {
	prometheus.MustRegister(requests, latency)
}

func skip(path string) bool { return path == "/health" || path == "/metrics" }

// captureWriter records the response body so failure logs can include the
// error payload returned to the client (handlers don't need any changes).
type captureWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// Fail records err on the gin context and aborts with a JSON error body.
// Use it in handlers for failures so the request log carries the cause:
//
//	obs.Fail(c, http.StatusBadRequest, err)
//	obs.Fail(c, http.StatusNotFound, errors.New("card not found"))
func Fail(c *gin.Context, status int, err error) {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}
	_ = c.Error(err)
	c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
}

// Log exposes the shared JSON logger (e.g. for gateway upstream errors).
func Log() *slog.Logger { return logger }

func logRequest(service, rid string, c *gin.Context, route string, code int, dur time.Duration, body string) {
	requests.WithLabelValues(service, c.Request.Method, route, strconv.Itoa(code)).Inc()
	latency.WithLabelValues(service, route).Observe(dur.Seconds())
	attrs := []any{
		"service", service, "request_id", rid,
		"method", c.Request.Method, "route", route,
		"path", c.Request.URL.Path,
		"status", code, "latency_ms", dur.Milliseconds(),
		"client_ip", c.ClientIP(),
	}
	if q := c.Request.URL.RawQuery; q != "" {
		attrs = append(attrs, "query", truncate(q, 500))
	}
	if ua := c.Request.UserAgent(); ua != "" {
		attrs = append(attrs, "user_agent", truncate(ua, 200))
	}
	// Failure context: handler errors (via obs.Fail / c.Error) + response body.
	if len(c.Errors) > 0 {
		attrs = append(attrs, "error", truncate(c.Errors.String(), 1000))
	} else if code >= 400 && body != "" {
		attrs = append(attrs, "error", truncate(body, 1000))
	}
	switch {
	case code >= 500:
		logger.Error("http_request failed", attrs...)
	case code >= 400:
		logger.Warn("http_request failed", attrs...)
	default:
		logger.Info("http_request", attrs...)
	}
}

func middleware(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if skip(c.FullPath()) && c.FullPath() != "" {
			c.Next()
			return
		}
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()[:8]
		}
		c.Header("X-Request-ID", rid)
		c.Request.Header.Set("X-Request-ID", rid)
		start := time.Now()

		// Capture response body for failure logs (no handler changes needed).
		cw := &captureWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = cw

		// Panic recovery with JSON failure log (replaces gin.Recovery so
		// panics are logged in the same structured format with a stack).
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				route := c.FullPath()
				if route == "" {
					route = "unknown"
				}
				dur := time.Since(start)
				code := c.Writer.Status()
				if code < 500 {
					code = http.StatusInternalServerError
				}
				requests.WithLabelValues(service, c.Request.Method, route, strconv.Itoa(code)).Inc()
				latency.WithLabelValues(service, route).Observe(dur.Seconds())
				logger.Error("http_request failed",
					"service", service, "request_id", rid,
					"method", c.Request.Method, "route", route,
					"path", c.Request.URL.Path,
					"status", code, "latency_ms", dur.Milliseconds(),
					"client_ip", c.ClientIP(),
					"panic", fmt.Sprint(rec),
					"stack", truncate(string(stack), 2000),
				)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error", "request_id": rid})
				} else {
					c.Abort()
				}
			}
		}()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		logRequest(service, rid, c, route, c.Writer.Status(), time.Since(start), cw.body.String())
	}
}

// Engine returns a gin engine with recovery + observability middleware and /metrics.
func Engine(service string) *gin.Engine {
	r := gin.New()
	r.Use(middleware(service))
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.NoRoute(func(c *gin.Context) {
		_ = c.Error(errors.New("not found: " + c.Request.URL.Path))
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "service": service})
	})
	return r
}
