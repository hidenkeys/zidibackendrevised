package services

import (
	"context"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type UserService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (us *UserService) CreateUser(ctx context.Context, req api.User) (*api.User, error) {
	orgId, err := uuid.Parse(req.OrganizationId.String())
	if err != nil {
		return nil, err
	}

	newUser := &models.User{
		ID:             req.Id,
		FirstName:      string(req.Firstname),
		LastName:       string(req.Lastname),
		Email:          string(req.Email),
		Password:       string(req.Password),
		Address:        string(req.Address),
		OrganizationID: orgId,
		Role:           string(req.Role),
	}

	// Save user to repository
	user, err := us.userRepo.Create(newUser)
	if err != nil {
		return nil, err
	}

	finalUser := &api.User{
		Id:             user.ID,
		Firstname:      user.FirstName,
		Lastname:       user.LastName,
		Email:          openapi_types.Email(user.Email),
		Address:        user.Address,
		OrganizationId: openapi_types.UUID(user.OrganizationID),
		Role:           user.Role,
	}

	return finalUser, nil
}

func (us *UserService) UpdateUser(ctx context.Context, id uuid.UUID, req api.User) (*api.User, error) {
	updateUser := &models.User{
		ID:        req.Id,
		FirstName: string(req.Firstname),
		LastName:  string(req.Lastname),
		Email:     string(req.Email),
		Address:   string(req.Address),
		Role:      string(req.Role),
	}

	user, err := us.userRepo.UpdateByID(id, updateUser)
	if err != nil {
		return nil, err
	}
	finalUser := &api.User{
		Id:             user.ID,
		Firstname:      user.FirstName,
		Lastname:       user.LastName,
		Email:          openapi_types.Email(user.Email),
		Address:        user.Address,
		OrganizationId: openapi_types.UUID(user.OrganizationID),
		Role:           user.Role,
	}
	return finalUser, nil
}

func (us *UserService) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return us.userRepo.DeleteByID(id)
}

func (us *UserService) GetUserByID(ctx context.Context, id uuid.UUID) (*api.User, error) {
	user, err := us.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	finalUser := &api.User{
		Id:             user.ID,
		Firstname:      string(user.FirstName),
		Lastname:       string(user.LastName),
		Email:          openapi_types.Email(string(user.Email)),
		Address:        string(user.Address),
		Role:           string(user.Role),
		OrganizationId: openapi_types.UUID(user.OrganizationID),
		Password:       string(user.Password),
	}
	return finalUser, nil
}

func (us *UserService) GetAllUsers(ctx context.Context, limit, offset int) ([]api.User, error) {
	users, _, err := us.userRepo.GetAll(limit, offset)
	if err != nil {
		return nil, err
	}
	var finalUsers []api.User
	for _, user := range users {
		finalUsers = append(finalUsers, api.User{
			Id:             user.ID,
			Firstname:      string(user.FirstName),
			Lastname:       string(user.LastName),
			Email:          openapi_types.Email(string(user.Email)),
			Address:        string(user.Address),
			Role:           string(user.Role),
			Password:       string(user.Password),
			OrganizationId: openapi_types.UUID(user.OrganizationID),
		})
	}
	return finalUsers, nil
}

func (us *UserService) GetUserByEmail(ctx context.Context, email string) (*api.User, error) {
	user, err := us.userRepo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	finalUser := &api.User{
		Id:             user.ID,
		Firstname:      string(user.FirstName),
		Lastname:       string(user.LastName),
		Email:          openapi_types.Email(string(user.Email)),
		Address:        string(user.Address),
		Role:           string(user.Role),
		OrganizationId: openapi_types.UUID(user.OrganizationID),
		Password:       string(user.Password),
	}
	return finalUser, nil
}

func (us *UserService) GetUserByOrganizationID(ctx context.Context, orgId uuid.UUID, limit, offset int) ([]api.User, error) {
	users, _, err := us.userRepo.GetAllByOrganizationID(orgId, limit, offset)
	if err != nil {
		return nil, err
	}
	var finalUsers []api.User
	for _, user := range users {
		finalUsers = append(finalUsers, api.User{
			Id:             user.ID,
			Firstname:      string(user.FirstName),
			Lastname:       string(user.LastName),
			Email:          openapi_types.Email(string(user.Email)),
			Address:        string(user.Address),
			Role:           string(user.Role),
			Password:       string(user.Password),
			OrganizationId: openapi_types.UUID(user.OrganizationID),
		})
	}
	return finalUsers, nil
}

func (us *UserService) UpdatePassword(ctx context.Context, id uuid.UUID, newPassword string) error {
	// Call repository function to update the password
	err := us.userRepo.UpdatePasswordByID(id, newPassword)
	if err != nil {
		return err
	}
	return nil
}
