package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidibackend/models"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/utils"
	"gorm.io/gorm"
)

type OrderService struct {
	db          *gorm.DB
	orderRepo   repository.OrderRepository
	productRepo repository.ProductRepository
}

func NewOrderService(db *gorm.DB, orderRepo repository.OrderRepository, productRepo repository.ProductRepository) *OrderService {
	return &OrderService{
		db:          db,
		orderRepo:   orderRepo,
		productRepo: productRepo,
	}
}

func (s *OrderService) CreateOrder(institutionID, clientID uuid.UUID, items []models.OrderItem) (*models.Order, error) {
	var totalAmount float64

	// Validate stock and calculate total
	for i, item := range items {
		product, err := s.productRepo.GetByID(item.ProductID)
		if err != nil {
			return nil, fmt.Errorf("product not found: %v", err)
		}
		if product.StockQuantity < item.Quantity {
			return nil, fmt.Errorf("insufficient stock for product: %s", product.Name)
		}

		// Snapshot product details
		snapshot, _ := json.Marshal(map[string]interface{}{
			"name":  product.Name,
			"price": product.Price,
			"sku":   product.SKU,
		})
		items[i].ProductSnapshot = snapshot
		items[i].UnitPrice = product.Price
		items[i].SubTotal = product.Price * float64(item.Quantity)
		totalAmount += items[i].SubTotal
	}

	order := &models.Order{
		InstitutionID:  institutionID,
		ClientID:       clientID,
		Items:          items,
		TotalAmount:    totalAmount,
		Status:         "pending_payment",
		PaymentStatus:  "pending",
		TrackingNumber: fmt.Sprintf("ORD-%d", time.Now().Unix()), // Simple tracking number generation
	}

	createdOrder, err := s.orderRepo.Create(order)
	if err != nil {
		return nil, err
	}

	// Initialize tracking
	initialTracking := &models.OrderTracking{
		OrderID:     createdOrder.ID,
		Status:      "pending_payment",
		Description: "Order created, waiting for payment",
		Location:    "System",
	}
	_ = s.orderRepo.AddTrackingUpdate(initialTracking)

	return createdOrder, nil
}

func (s *OrderService) GenerateCheckoutLink(orderID uuid.UUID) string {
	// In a real app, this would generate a payment gateway link
	// For this pilot/mock, we return a format the bot can use or a dummy link
	return fmt.Sprintf("https://zidi.app/checkout/%s", orderID.String())
}

func (s *OrderService) GetOrderByTracking(trackingNumber string) (*models.Order, error) {
	return s.orderRepo.GetByTrackingNumber(trackingNumber)
}

func (s *OrderService) GetOrder(id uuid.UUID) (*models.Order, error) {
	return s.orderRepo.GetByID(id)
}

func (s *OrderService) UpdateOrderStatus(id uuid.UUID, status string) error {
	return s.orderRepo.UpdateStatus(id, status)
}

func (s *OrderService) ProcessOrderPayment(orderID uuid.UUID, amount float64, ref, email, gateway string) error {
	// 1. Get Order with Items
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return err
	}

	// 2. Wrap in transaction logic (Ideally Repo should handle this, but for speed injecting here via repo methods if possible)
	// We'll trust optimistic locking or just simple decrements for this pilot.

	// Decrement Stock
	for _, item := range order.Items {
		product, err := s.productRepo.GetByID(item.ProductID)
		if err == nil {
			newStock := product.StockQuantity - item.Quantity
			if newStock < 0 {
				newStock = 0 // Prevent negative
			}
			product.StockQuantity = newStock
			_, _ = s.productRepo.Update(product)
		}
	}

	// 3. Update Order Status
	if err := s.orderRepo.UpdateStatus(orderID, models.OrderStatusPaid); err != nil {
		return err
	}

	// 4. Record Payment
	payment := models.CommercePayment{
		OrderID:          orderID,
		Amount:           amount,
		PaymentReference: ref,
		Status:           "success",
		PayerEmail:       email,
		Gateway:          gateway,
	}
	if err := s.db.Create(&payment).Error; err != nil {
		fmt.Printf("Error creating commerce payment record: %v\n", err)
	}

	// 5. Build Notifications
	var itemsList string
	for _, item := range order.Items {
		// Try to parse snapshot for name or fetch product
		var name string = "Item"
		// If wePreloaded product, use it
		if item.Product.Name != "" {
			name = item.Product.Name
		} else {
			// Try snapshot
			var snap map[string]interface{}
			if len(item.ProductSnapshot) > 0 {
				if err := json.Unmarshal(item.ProductSnapshot, &snap); err == nil {
					if n, ok := snap["name"].(string); ok {
						name = n
					}
				}
			}
		}
		itemsList += fmt.Sprintf("<li>%s (x%d) - ₦%.2f</li>", name, item.Quantity, item.SubTotal)
	}

	// Fetch Client for Phone Number
	var client models.Client
	if err := s.db.Where("id = ?", order.ClientID).First(&client).Error; err != nil {
		fmt.Printf("Error fetching client for notification: %v\n", err)
	}

	// Send Receipt Email
	subject := fmt.Sprintf("Payment Receipt - Order #%s", order.TrackingNumber)
	body := fmt.Sprintf(`Hello %s,<br><br>
We received your payment of <b>₦%.2f</b> for Order <b>%s</b>.<br>
<br><b>Items Bought:</b><ul>%s</ul><br>
Your order is now being processed.<br><br>Reference: %s`, client.Name, amount, order.TrackingNumber, itemsList, ref)

	go utils.SendEmail00(email, subject, body)

	// Send WhatsApp Notification
	if client.Phone != "" {
		waMsg := fmt.Sprintf("✅ *Payment Successful!*\n\nOrder #%s has been paid.\nAmount: ₦%.2f\n\nWe are processing your order now. You will be notified when it ships.\n\nThank you for shopping with LUSH Hair!", order.TrackingNumber, amount)
		go func() {
			if err := utils.SendWhatsAppMessage(client.Phone, waMsg); err != nil {
				fmt.Printf("Failed to send WhatsApp confirmation: %v\n", err)
			}
		}()
	}

	return nil
}
