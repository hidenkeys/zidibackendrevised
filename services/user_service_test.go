package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/models"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type userRepositoryStub struct {
	created    *models.User
	assignment *models.CommerceStaffStoreAssignment
}

func (s *userRepositoryStub) Create(user *models.User) (*models.User, error) {
	s.created = user
	return user, nil
}

func (s *userRepositoryStub) CreateWithStoreAssignment(_ context.Context, user *models.User, assignment *models.CommerceStaffStoreAssignment) (*models.User, error) {
	s.created = user
	assignment.UserID = user.ID
	s.assignment = assignment
	return user, nil
}

func (*userRepositoryStub) GetAll(int, int) ([]models.User, int64, error) { return nil, 0, nil }
func (*userRepositoryStub) GetByID(uuid.UUID) (*models.User, error)       { return nil, nil }
func (*userRepositoryStub) GetByEmail(string) (*models.User, error)       { return nil, nil }
func (*userRepositoryStub) UpdateByID(uuid.UUID, *models.User) (*models.User, error) {
	return nil, nil
}
func (*userRepositoryStub) DeleteByID(uuid.UUID) error                 { return nil }
func (*userRepositoryStub) UpdatePasswordByID(uuid.UUID, string) error { return nil }
func (*userRepositoryStub) GetAllByOrganizationID(uuid.UUID, int, int) ([]models.User, int64, error) {
	return nil, 0, nil
}

func TestCreateStoreStaffRequiresAndPersistsStoreAssignment(t *testing.T) {
	organizationID, storeID := uuid.New(), uuid.New()
	repo := &userRepositoryStub{}
	service := NewUserService(repo)
	request := api.User{
		Id: uuid.New(), Firstname: "Store", Lastname: "Staff", Email: "staff@example.com",
		Role: "store_staff", OrganizationId: openapi_types.UUID(organizationID), Password: "hashed",
	}

	if _, err := service.CreateUserWithStoreAssignment(context.Background(), request, nil); err == nil {
		t.Fatal("expected a store_staff user without a store to be rejected")
	}
	if repo.created != nil {
		t.Fatal("user was created before store assignment validation")
	}

	if _, err := service.CreateUserWithStoreAssignment(context.Background(), request, &storeID); err != nil {
		t.Fatalf("create store staff: %v", err)
	}
	if repo.created == nil || repo.assignment == nil {
		t.Fatal("user and store assignment were not created together")
	}
	if repo.assignment.StoreID != storeID || repo.assignment.OrganizationID != organizationID || repo.assignment.UserID != repo.created.ID || repo.assignment.Role != "store_staff" || repo.assignment.Status != models.CommerceStatusActive {
		t.Fatalf("unexpected assignment: %+v", repo.assignment)
	}
}
