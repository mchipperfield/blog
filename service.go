package blog

import (
	"bytes"
	"context"
	"fmt"
	"strings"

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

func (s *service) ListArticles(ctx context.Context) ([]*FrontMatter, error) {
	articles, err := s.store.ListArticles(ctx)
	if err != nil {
		return nil, err
	}
	frontMatters := make([]*FrontMatter, len(articles))
	for i, name := range articles {
		article, err := s.store.GetArticleBySlug(ctx, strings.TrimSuffix(name, ".md"))
		if err != nil {
			continue
		}
		var fm FrontMatter
		frontmatter.Parse(bytes.NewReader(article), &fm)
		fm.Slug = strings.TrimSuffix(name, ".md")
		frontMatters[i] = &fm
	}
	return frontMatters, nil
}
