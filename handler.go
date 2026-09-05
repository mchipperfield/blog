package blog

import (
	"bytes"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Handler struct {
	logger *slog.Logger
	svc    Service
	tpl    *template.Template
	http.Handler
}

func NewHandler(logger *slog.Logger, svc Service, tpl *template.Template) (*Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if svc == nil {
		return nil, errors.New("service cannot be nil")
	}
	if tpl == nil {
		return nil, errors.New("template cannot be nil")
	}
	if tpl.Lookup("article") == nil {
		return nil, errors.New("template 'article' not found")
	}
	if tpl.Lookup("index") == nil {
		return nil, errors.New("template 'index' not found")
	}
	h := &Handler{
		logger: logger.With("component", "blog"),
		svc:    svc,
		tpl:    tpl,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /blog", h.GetArticles())
	mux.HandleFunc("GET /blog/{slug}", h.GetArticleBySlug())
	h.Handler = h.LoggingMw(mux)
	h.Handler = otelhttp.NewHandler(h.Handler, "blog-handler")

	return h, nil
}

// GET /blog/{slug}
func (h *Handler) GetArticleBySlug() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		fm, content, err := h.svc.GetArticleBySlug(r.Context(), slug)
		if err != nil {
			switch {
			case errors.Is(err, ErrArticleNotFound):
				http.Error(w, "article not found", http.StatusNotFound)
			case errors.Is(err, ErrServiceUnavailable):
				h.logger.InfoContext(r.Context(), "blog service unavailable", "slug", slug, "err", err)
				http.Error(w, "blog service unavailable", http.StatusServiceUnavailable)
			default:
				h.logger.InfoContext(r.Context(), "failed to get article", "slug", slug, "err", err)
				http.Error(w, "failed to get article", http.StatusInternalServerError)
			}
			return
		}

		var buf bytes.Buffer
		if err := h.tpl.ExecuteTemplate(&buf, "article", ArticleView{
			FrontMatter: fm,
			Content:     template.HTML(content),
		}); err != nil {
			h.logger.InfoContext(r.Context(), "failed to execute template", "slug", slug, "err", err, "template", "article")
			http.Error(w, "failed to render article", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := buf.WriteTo(w); err != nil {
			h.logger.InfoContext(r.Context(), "failed to write response", "slug", slug, "err", err)
		}

	}
}

// GET /blog
func (h *Handler) GetArticles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		articles, err := h.svc.ListArticles(r.Context())
		if err != nil {
			switch {
			case errors.Is(err, ErrServiceUnavailable):
				h.logger.InfoContext(r.Context(), "blog service unavailable", "err", err)
				http.Error(w, "blog service unavailable", http.StatusServiceUnavailable)
			default:
				h.logger.InfoContext(r.Context(), "failed to get articles", "err", err)
				http.Error(w, "failed to get articles", http.StatusInternalServerError)
			}
			return
		}

		var buf bytes.Buffer
		if err := h.tpl.ExecuteTemplate(&buf, "index", IndexView{
			Articles: articles,
		}); err != nil {
			h.logger.InfoContext(r.Context(), "failed to execute template", "err", err, "template", "index")
			http.Error(w, "failed to render articles", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := buf.WriteTo(w); err != nil {
			h.logger.InfoContext(r.Context(), "failed to write response", "err", err)
		}

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
