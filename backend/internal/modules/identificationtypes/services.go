package identificationtypes

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var (
	ErrIdentificationTypeNotFound = errors.New("identification type not found")
	ErrCannotDeleteDefault        = errors.New("cannot delete default identification type")
	ErrInvalidIdentificationType  = errors.New("invalid identification type")
	ErrDuplicateCode              = errors.New("identification type code already exists")
	ErrIdentificationTypeInUse    = errors.New("identification type is used by visitor records")
)

var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,49}$`)

type Service struct{ repo *Repository }

func NewService(db any) *Service { return &Service{repo: NewRepository(db)} }
func (s *Service) List(ctx context.Context, orgID int64) ([]IdentificationType, error) {
	if err := s.repo.EnsureDefaults(ctx, orgID); err != nil {
		return nil, err
	}
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
	item, err := s.repo.GetByCode(ctx, orgID, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrIdentificationTypeNotFound
	}
	return item, nil
}
func validate(item *IdentificationType) error {
	item.Code = strings.ToUpper(strings.TrimSpace(item.Code))
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.ValidationRegex = strings.TrimSpace(item.ValidationRegex)
	item.ValidationExample = strings.TrimSpace(item.ValidationExample)
	if !codePattern.MatchString(item.Code) || item.Name == "" {
		return ErrInvalidIdentificationType
	}
	if item.ValidationRegex != "" {
		if _, err := regexp.Compile(item.ValidationRegex); err != nil {
			return ErrInvalidIdentificationType
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, item *IdentificationType) error {
	if err := validate(item); err != nil {
		return err
	}
	existing, err := s.repo.GetByCode(ctx, item.OrganizationID, item.Code)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrDuplicateCode
	}
	return s.repo.Create(ctx, item)
}
func (s *Service) Update(ctx context.Context, item *IdentificationType) error {
	if item.Name == "" {
		return ErrInvalidIdentificationType
	}
	if err := validateUpdate(item); err != nil {
		return err
	}
	existing, err := s.repo.Get(ctx, item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrIdentificationTypeNotFound
	}
	return s.repo.Update(ctx, item)
}
func validateUpdate(item *IdentificationType) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.ValidationRegex = strings.TrimSpace(item.ValidationRegex)
	item.ValidationExample = strings.TrimSpace(item.ValidationExample)
	if item.Name == "" {
		return ErrInvalidIdentificationType
	}
	if item.ValidationRegex != "" {
		if _, err := regexp.Compile(item.ValidationRegex); err != nil {
			return ErrInvalidIdentificationType
		}
	}
	return nil
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
	inUse, err := s.repo.InUse(ctx, orgID, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrIdentificationTypeInUse
	}
	return s.repo.Delete(ctx, orgID, id)
}
