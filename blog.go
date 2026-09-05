package blog

import (
	"context"
	"errors"
	"time"
)

var ErrArticleNotFound = errors.New("article not found")

type Service interface {
	GetArticleBySlug(ctx context.Context, slug string) (*FrontMatter, []byte, error)
	ListArticles(ctx context.Context) ([]*FrontMatter, error)
}

type Store interface {
	GetArticleBySlug(ctx context.Context, slug string) ([]byte, error)
	ListArticles(ctx context.Context) ([]string, error)
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
