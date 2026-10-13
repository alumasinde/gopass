package identificationtypes

import (
	"context"
	"errors"
)

var (
	ErrIdentificationTypeNotFound = errors.New("identification type not found")
	ErrCannotDeleteDefault    = errors.New("cannot delete default identification type")
)

type Service struct {
	repo *Repository
}

func NewService(db any) *Service {
	return &Service{repo: NewRepository(db)}
}

func (s *Service) List(ctx context.Context, orgID int64) ([]IdentificationType, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id int64) (*IdentificationType, error) {
	item, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrIdentificationTypeNotFound
	}
	return item, nil
}

func (s *Service) GetByCode(ctx context.Context, orgID int64, code string) (*IdentificationType, error) {
	item, err := s.repo.GetByCode(ctx, orgID, code)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrIdentificationTypeNotFound
	}
	return item, nil
}

func (s *Service) Create(ctx context.Context, item *IdentificationType) error {
	return s.repo.Create(ctx, item)
}

func (s *Service) Update(ctx context.Context, item *IdentificationType) error {
	existing, err := s.repo.Get(ctx, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrIdentificationTypeNotFound
	}
	return s.repo.Update(ctx, item)
}

func (s *Service) Delete(ctx context.Context, orgID, id int64) error {
	existing, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrIdentificationTypeNotFound
	}
	if existing.IsDefault {
		return ErrCannotDeleteDefault
	}
	return s.repo.Delete(ctx, orgID, id)
}
