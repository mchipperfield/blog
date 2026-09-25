package api

import (
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/mchipperfield/blog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Handler struct {
	logger *slog.Logger
	svc    blog.Service
	http.Handler
}

// NewHandler creates a handler for GET /blog and GET /blog/{slug}
func NewHandler(logger *slog.Logger, svc blog.Service) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("service cannot be nil")
	}

	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "api")
	h := Handler{
		logger: logger,
		svc:    svc,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", h.GetArticleBySlug())
	mux.HandleFunc("GET /{$}", h.GetArticles())
	h.Handler = h.LoggingMw(mux)
	h.Handler = otelhttp.NewHandler(h.Handler, "blog-handler")
	return &h, nil
}

type Article struct {
	Metadata    *Metadata `json:"metadata,omitempty"`
	ContentHTML string    `json:"content_html"`
}

type Metadata struct {
	Title       string    `json:"title,omitempty"`
	Slug        string    `json:"slug,omitempty"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Author      string    `json:"author,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type ListArticlesResponse struct {
	Data []*Metadata `json:"data,omitempty"`
}

func toMetadata(md *blog.Metadata) *Metadata {
	if md == nil {
		return nil
	}
	return &Metadata{
		Title:       md.Title,
		Slug:        md.Slug,
		Description: md.Description,
		Tags:        md.Tags,
		Author:      md.Author,
		PublishedAt: md.PublishedAt,
		UpdatedAt:   md.UpdatedAt,
	}
}

func (h *Handler) GetArticleBySlug() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		md, content, err := h.svc.GetArticleBySlug(r.Context(), slug)
		if err != nil {
			h.respond(w, r, err)
			return
		}
		article := Article{
			Metadata:    toMetadata(md),
			ContentHTML: string(content),
		}

		h.respond(w, r, article)
	}
}

func (h *Handler) GetArticles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		articles, err := h.svc.ListArticles(r.Context())
		if err != nil {
			h.respond(w, r, err)
			return
		}
		var response ListArticlesResponse
		for _, md := range articles {
			response.Data = append(response.Data, toMetadata(md))
		}
		h.respond(w, r, response)
	}
}
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, data any) {
	if err, ok := data.(error); ok {
		var code int
		var detail string
		switch {
		case errors.Is(err, blog.ErrArticleNotFound):
			code = http.StatusNotFound
			detail = blog.ErrArticleNotFound.Error()
		case errors.Is(err, blog.ErrInvalidSlug):
			code = http.StatusBadRequest
			detail = blog.ErrInvalidSlug.Error()
		case errors.Is(err, blog.ErrServiceUnavailable):
			code = http.StatusServiceUnavailable
			detail = blog.ErrServiceUnavailable.Error()
		case errors.Is(err, blog.ErrAccessDenied):
			code = http.StatusInternalServerError
			detail = blog.ErrAccessDenied.Error()
		default:
			h.logger.Error("unexpected error", "error", err, slog.Group("http.request", "method", r.Method, "url", r.URL, "proto", r.Proto, "remote_addr", r.RemoteAddr))
			code = http.StatusInternalServerError
			detail = "internal server error"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if err := json.MarshalWrite(w, ErrorResponse{
			Errors: []Error{NewError(code, detail)},
		}); err != nil {
			h.logger.Error("failed to write response", "error", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.MarshalWrite(w, data); err != nil {
		h.logger.Error("failed to write response", "error", err)
	}
}

// LoggingMw records method, path, peer, protocol, duration, and response status
// after each request completes.
func (h *Handler) LoggingMw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		m := httpsnoop.CaptureMetrics(next, w, r)

		h.logger.InfoContext(r.Context(), "http request completed", slog.Group("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("proto", r.Proto),
			slog.Duration("duration", m.Duration),
			slog.Int("status", m.Code),
		))
	})
}
