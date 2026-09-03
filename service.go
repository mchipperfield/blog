package blog

import (
	"bytes"
	"context"
	"fmt"

	"github.com/adrg/frontmatter"
	"github.com/yuin/goldmark"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type service struct {
	store     Store
	converter goldmark.Markdown
}

func NewService(store Store) *service {
	return &service{
		store:     store,
		converter: goldmark.New(),
	}
}

func (s *service) GetArticleBySlug(ctx context.Context, slug string) (*FrontMatter, []byte, error) {
	tracer := otel.Tracer("github.com/mchipperfield/blog")
	ctx, span := tracer.Start(ctx, "article.getbyslug")
	defer span.End()
	span.SetAttributes(
		attribute.String("article.slug", slug),
	)
	article, err := s.store.GetArticleBySlug(ctx, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("blog: get article by slug %s: %w", slug, err)
	}
	var fm FrontMatter
	content, err := frontmatter.Parse(bytes.NewReader(article), &fm)
	if err != nil {
		return nil, nil, fmt.Errorf("blog: parse frontmatter: %w", err)
	}

	var buf bytes.Buffer
	if err := s.converter.Convert(content, &buf); err != nil {
		return nil, nil, fmt.Errorf("blog: convert markdown: %w", err)
	}

	return &fm, buf.Bytes(), nil
}
