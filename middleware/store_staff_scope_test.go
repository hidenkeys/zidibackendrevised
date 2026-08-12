package middleware

import "testing"

func TestStoreStaffRouteAllowed(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		allowed bool
	}{
		{"GET", "/api/v1/commerce/store-orders", true},
		{"POST", "/api/v1/commerce/store-orders/order-id/prepared", true},
		{"POST", "/api/v1/commerce/fulfilments/fulfilment-id/handover", true},
		{"POST", "/api/v1/commerce/fulfilments/fulfilment-id/delivery-confirmation-requests", true},
		{"POST", "/api/v1/auth/logout", true},
		{"GET", "/api/v1/commerce/stores", false},
		{"GET", "/api/v1/commerce/orders", false},
		{"POST", "/api/v1/users", false},
		{"PUT", "/api/v1/commerce/stores/store-id", false},
	}
	for _, test := range tests {
		if got := storeStaffRouteAllowed(test.method, test.path); got != test.allowed {
			t.Errorf("%s %s: expected %t, got %t", test.method, test.path, test.allowed, got)
		}
	}
}
