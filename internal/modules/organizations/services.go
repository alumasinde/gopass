package organizations

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var (
	ErrOrgNotFound      = errors.New("organization not found")
	ErrOrgAlreadyExists = errors.New("organization already exists")
	ErrInvalidSlug      = errors.New("invalid slug")
	ErrInvalidOrgName   = errors.New("invalid organization name")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateOrganization(ctx context.Context, name, slug string) (*Organization, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)

	if name == "" {
		return nil, ErrInvalidOrgName
	}
	if !isValidSlug(slug) {
		return nil, ErrInvalidSlug
	}

	existing, _ := s.repo.GetBySlug(ctx, slug)
	if existing != nil {
		return nil, ErrOrgAlreadyExists
	}

	org := &Organization{
		Name:     name,
		Slug:     slug,
		IsActive: true,
	}

	if err := s.repo.Create(ctx, org); err != nil {
		return nil, err
	}

	return org, nil
}

func (s *Service) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	org, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, ErrOrgNotFound
	}
	return org, nil
}

func (s *Service) GetOrganizationBySlug(ctx context.Context, slug string) (*Organization, error) {
	org, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, ErrOrgNotFound
	}
	return org, nil
}

func (s *Service) ListOrganizations(ctx context.Context, limit int) ([]Organization, error) {
	return s.repo.List(ctx, limit)
}

func (s *Service) UpdateOrganization(ctx context.Context, org *Organization) error {
	existing, err := s.repo.GetByID(ctx, org.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrOrgNotFound
	}

	org.Name = strings.TrimSpace(org.Name)
	org.Slug = strings.TrimSpace(org.Slug)

	if org.Name == "" {
		return ErrInvalidOrgName
	}
	if !isValidSlug(org.Slug) {
		return ErrInvalidSlug
	}

	if org.Slug != existing.Slug {
		slugCheck, _ := s.repo.GetBySlug(ctx, org.Slug)
		if slugCheck != nil && slugCheck.ID != org.ID {
			return ErrOrgAlreadyExists
		}
	}

	return s.repo.Update(ctx, org)
}

func (s *Service) DeleteOrganization(ctx context.Context, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrOrgNotFound
	}
	return s.repo.Delete(ctx, id)
}

func isValidSlug(slug string) bool {
	if slug == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, slug)
	return matched
}
