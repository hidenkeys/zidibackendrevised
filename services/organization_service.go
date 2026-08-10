package services

import (
	"context"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type OrganizationService struct {
	orgRepo repository.OrganizationRepository
}

func NewOrganisationService(orgRepo repository.OrganizationRepository) *OrganizationService {
	return &OrganizationService{orgRepo: orgRepo}
}

func (org *OrganizationService) CreateOrganization(ctx context.Context, req api.Organization) (*api.Organization, error) {
	newOrg := &models.Organization{
		ID:                 req.Id,
		Email:              string(req.Email),
		ContactPersonName:  string(req.ContactPersonName),
		ContactPersonPhone: string(req.ContactPersonPhone),
		Address:            string(req.Address),
		Industry:           string(req.Industry),
		CompanySize:        int(req.CompanySize),
		CompanyName:        string(req.CompanyName),
	}
	organization, err := org.orgRepo.Create(newOrg)
	if err != nil {
		return nil, err
	}
	finalOrg := &api.Organization{
		Id:                 organization.ID,
		Address:            string(organization.Address),
		Industry:           string(organization.Industry),
		CompanyName:        string(organization.CompanyName),
		ContactPersonName:  string(organization.ContactPersonName),
		ContactPersonPhone: string(organization.ContactPersonPhone),
		Email:              openapi_types.Email(string(organization.Email)),
		CompanySize:        int(organization.CompanySize),
	}
	return finalOrg, nil
}

func (org *OrganizationService) UpdateOrganization(ctx context.Context, id uuid.UUID, req api.Organization) (*api.Organization, error) {
	newOrg := &models.Organization{
		ID:                 req.Id,
		Address:            string(req.Address),
		Industry:           string(req.Industry),
		CompanySize:        int(req.CompanySize),
		CompanyName:        string(req.CompanyName),
		ContactPersonName:  string(req.ContactPersonName),
		ContactPersonPhone: string(req.ContactPersonPhone),
		Email:              string(req.Email),
	}

	organization, err := org.orgRepo.UpdateByID(id, newOrg)
	if err != nil {
		return nil, err
	}
	finalOrg := &api.Organization{
		Id:                 organization.ID,
		Address:            string(organization.Address),
		Industry:           string(organization.Industry),
		CompanySize:        int(organization.CompanySize),
		CompanyName:        string(organization.CompanyName),
		ContactPersonName:  string(organization.ContactPersonName),
		ContactPersonPhone: string(organization.ContactPersonPhone),
		Email:              openapi_types.Email(string(organization.Email)),
	}
	return finalOrg, nil
}

func (org *OrganizationService) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	err := org.orgRepo.DeleteByID(id)
	if err != nil {
		return err
	}
	return nil
}

func (org *OrganizationService) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*api.Organization, error) {
	organization, err := org.orgRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	finalOrg := &api.Organization{
		Id:                 organization.ID,
		Address:            string(organization.Address),
		Industry:           string(organization.Industry),
		CompanySize:        int(organization.CompanySize),
		CompanyName:        string(organization.CompanyName),
		ContactPersonName:  string(organization.ContactPersonName),
		ContactPersonPhone: string(organization.ContactPersonPhone),
		Email:              openapi_types.Email(string(organization.Email)),
	}
	return finalOrg, nil
}

func (org *OrganizationService) GetAllOrganizations(ctx context.Context, limit, offset int) ([]api.Organization, error) {
	organizations, _, err := org.orgRepo.GetAll(limit, offset)
	if err != nil {
		return nil, err
	}
	finalOrgs := []api.Organization{}
	for _, organization := range organizations {
		finalOrgs = append(finalOrgs, api.Organization{
			Id:                 organization.ID,
			Address:            string(organization.Address),
			Industry:           string(organization.Industry),
			CompanySize:        int(organization.CompanySize),
			CompanyName:        string(organization.CompanyName),
			ContactPersonName:  string(organization.ContactPersonName),
			ContactPersonPhone: string(organization.ContactPersonPhone),
			Email:              openapi_types.Email(string(organization.Email)),
		})
	}
	return finalOrgs, nil
}

func (org *OrganizationService) GetOrganizationByName(ctx context.Context, name string, limit, offset int) ([]api.Organization, error) {
	organizations, _, err := org.orgRepo.GetByName(name, limit, offset)
	if err != nil {
		return nil, err
	}
	finalOrgs := []api.Organization{}
	for _, organization := range organizations {
		finalOrgs = append(finalOrgs, api.Organization{
			Id:                 organization.ID,
			Address:            string(organization.Address),
			Industry:           string(organization.Industry),
			CompanySize:        int(organization.CompanySize),
			CompanyName:        string(organization.CompanyName),
			ContactPersonName:  string(organization.ContactPersonName),
			ContactPersonPhone: string(organization.ContactPersonPhone),
			Email:              openapi_types.Email(string(organization.Email)),
		})
	}
	return finalOrgs, nil
}
