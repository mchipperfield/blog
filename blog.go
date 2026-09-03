package blog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var ErrArticleNotFound = errors.New("article not found")

type Service interface {
	GetArticleBySlug(ctx context.Context, slug string) (*FrontMatter, []byte, error)
}
type Handler struct {
	logger *slog.Logger
	svc    Service
	http.Handler
}

type FrontMatter struct {
	Title       string
	Slug        string
	Description string
	Tags        []string
	Author      string
	PublishedAt time.Time
	UpdatedAt   time.Time
}

func NewHandler(logger *slog.Logger, svc Service) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if svc == nil {
		logger.Warn("service is nil, some functionality may not work")
		//return nil, errors.New("service cannot be nil")
	}
	h := &Handler{
		logger: logger.With("component", "blog"),
		svc:    svc,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /blog/{slug}", h.GetArticleBySlug())
	h.Handler = h.LoggingMw(mux)
	h.Handler = otelhttp.NewHandler(h.Handler, "blog-handler")

	return h, nil
}

func (h *Handler) GetArticleBySlug() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		fm, content, err := h.svc.GetArticleBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, ErrArticleNotFound) {
				http.Error(w, "article not found", http.StatusNotFound)
				return
			}
			h.logger.InfoContext(r.Context(), "failed to get article", "slug", slug, "err", err)
			http.Error(w, "failed to get article", http.StatusInternalServerError)
			return
		}

		// Implement the logic to get the article by slug here
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%+v", fm)))
		w.Write(content)
	}
}

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
