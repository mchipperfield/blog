package blog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type service struct {
	store     Store
	converter goldmark.Markdown
}

// NewService creates a blog service backed by store using the default Markdown
// renderer.
func NewService(store Store) (*service, error) {
	if store == nil {
		return nil, errors.New("blog: store cannot be nil")
	}
	return &service{
		store:     store,
		converter: goldmark.New(goldmark.WithExtensions(extension.GFM)),
	}, nil
}

// GetArticleBySlug loads a raw document, extracts its frontmatter, and renders
// the remaining Markdown as HTML.
func (s *service) GetArticleBySlug(ctx context.Context, slug string) (*Metadata, []byte, error) {
	tracer := otel.Tracer("github.com/mchipperfield/blog")
	ctx, span := tracer.Start(ctx, "article.getbyslug")
	defer span.End()
	span.SetAttributes(
		attribute.String("article.slug", slug),
	)

	if !ValidSlug(slug) {
		return nil, nil, ErrInvalidSlug
	}
	article, err := s.store.GetArticleBySlug(ctx, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("blog: get article by slug %s: %w", slug, err)
	}

	var buf bytes.Buffer
	if err := s.converter.Convert(article.Markdown, &buf); err != nil {
		return nil, nil, fmt.Errorf("blog: convert markdown: %w", err)
	}

	return article.Metadata, buf.Bytes(), nil
}

// ListArticles returns metadata for every document that can be fetched and
// parsed. An individual invalid or unavailable document is omitted so that a
// partial archive can still be rendered.
func (s *service) ListArticles(ctx context.Context) ([]*Metadata, error) {
	articles, err := s.store.ListArticles(ctx)
	if err != nil {
		return nil, err
	}
	return articles, nil
}

// ValidSlug returns true if the provided slug is valid for an article path.
func ValidSlug(slug string) bool {
	return slug != "" &&
		slug != "." &&
		slug != ".." &&
		!strings.ContainsAny(slug, `/\`)
}
