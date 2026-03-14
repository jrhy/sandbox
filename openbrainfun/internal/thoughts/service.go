package thoughts

import "context"

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

type CreateThoughtInput struct {
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	Source        string
}

func (s *Service) CreateThought(ctx context.Context, in CreateThoughtInput) (Thought, error) {
	th, err := NewThought(NewThoughtParams{
		Content:       in.Content,
		ExposureScope: in.ExposureScope,
		UserTags:      in.UserTags,
		Source:        in.Source,
	})
	if err != nil {
		return Thought{}, err
	}
	return s.repo.CreateThought(ctx, CreateThoughtParams{Thought: th})
}
