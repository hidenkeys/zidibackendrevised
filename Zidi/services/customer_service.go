package services

import (
	"context"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/asaskevich/govalidator"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
)

type CustomerService struct {
	customerRepo repository.CustomerRepository
}

func NewCustomerService(customerRepo repository.CustomerRepository) *CustomerService {
	return &CustomerService{customerRepo: customerRepo}
}

func (s *CustomerService) CreateCustomer(ctx context.Context, req api.Customer) (*api.Customer, error) {
	newCustomer := &models.Customer{
		ID:             req.Id,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Phone:          req.Phone,
		Email:          string(req.Email),
		Feedback:       req.Feedback,
		Network:        req.Network,
		Amount:         float64(req.Amount),
		Status:         req.Status,
		OrganizationID: req.OrganizationId,
		CampaignID:     req.CampaignId,
	}

	customer, err := s.customerRepo.Create(newCustomer)
	if err != nil {
		return nil, err
	}

	return mapToAPICustomer(customer), nil
}

func (s *CustomerService) GetAllCustomers(ctx context.Context, limit, offset int) ([]api.Customer, int64, error) {
	customers, count, err := s.customerRepo.GetAll(limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalCustomers []api.Customer
	for _, customer := range customers {
		finalCustomers = append(finalCustomers, *mapToAPICustomer(&customer))
	}

	return finalCustomers, count, nil
}

func (s *CustomerService) GetCustomerByID(ctx context.Context, id uuid.UUID) (*api.Customer, error) {
	customer, err := s.customerRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	return mapToAPICustomer(customer), nil
}

func (s *CustomerService) GetCustomersByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]api.Customer, int64, error) {
	customers, count, err := s.customerRepo.GetAllByOrganization(orgID, limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalCustomers []api.Customer
	for _, customer := range customers {
		finalCustomers = append(finalCustomers, *mapToAPICustomer(&customer))
	}

	return finalCustomers, count, nil
}

func (s *CustomerService) GetCustomersByCampaign(ctx context.Context, campaignID uuid.UUID, limit, offset int) ([]api.Customer, int64, error) {
	customers, count, err := s.customerRepo.GetAllByCampaign(campaignID, limit, offset)
	if err != nil {
		return nil, count, err
	}

	var finalCustomers []api.Customer
	for _, customer := range customers {
		finalCustomers = append(finalCustomers, *mapToAPICustomer(&customer))
	}

	return finalCustomers, count, nil
}

func (s *CustomerService) UpdateCustomer(ctx context.Context, id uuid.UUID, req *api.Customer) (*api.Customer, error) {
	updateData := &models.Customer{
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Phone:          req.Phone,
		Email:          string(req.Email),
		Feedback:       req.Feedback,
		Network:        req.Network,
		Amount:         float64(req.Amount),
		Status:         req.Status,
		OrganizationID: req.OrganizationId,
		CampaignID:     req.CampaignId,
	}

	updatedCustomer, err := s.customerRepo.UpdateByID(id, updateData)
	if err != nil {
		return nil, err
	}

	return mapToAPICustomer(updatedCustomer), nil
}

func (s *CustomerService) DeleteCustomer(ctx context.Context, id uuid.UUID) error {
	return s.customerRepo.DeleteByID(id)
}

// Helper function to convert models.Customer to api.Customer
func mapToAPICustomer(customer *models.Customer) *api.Customer {
	email := customer.Email
	if !govalidator.IsEmail(email) {
		email = "unknown@example.com" // replace or log as needed
	}
	return &api.Customer{
		Id:             customer.ID,
		FirstName:      customer.FirstName,
		LastName:       customer.LastName,
		Phone:          customer.Phone,
		Email:          openapi_types.Email(email), // use validated/sanitized email
		Feedback:       customer.Feedback,
		Network:        customer.Network,
		Amount:         float32(customer.Amount),
		Status:         customer.Status,
		OrganizationId: customer.OrganizationID,
		CampaignId:     customer.CampaignID,
	}
}
