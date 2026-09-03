package blog

import "context"

type service struct {
	articles map[string][]byte
}

func NewService() *service {
	return &service{
		articles: make(map[string][]byte),
	}
}

func (s *service) GetArticleBySlug(ctx context.Context, slug string) ([]byte, error) {
	article, ok := s.articles[slug]
	if !ok {
		return nil, ErrArticleNotFound
	}
	return article, nil
}
