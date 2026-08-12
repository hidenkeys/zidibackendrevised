package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	fiberprometheus "github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/commerceonboarding"
	"github.com/hidenkeys/zidibackend/config"
	commercefulfilment "github.com/hidenkeys/zidibackend/fulfilment"
	"github.com/hidenkeys/zidibackend/handlers"
	"github.com/hidenkeys/zidibackend/messaging"
	"github.com/hidenkeys/zidibackend/middleware"
	"github.com/hidenkeys/zidibackend/payments"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/telegrambot"
	"github.com/hidenkeys/zidibackend/utils"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	localDemoMode := os.Getenv("ZIDI_LOCAL_DEMO_MODE") == "true"
	jwtSecret, err := utils.LoadJWTSecret()
	if err != nil {
		log.Fatal(err)
	}
	config.ConnectDatabase()
	if !localDemoMode {
		config.MigrateDatabase()
		if strings.TrimSpace(os.Getenv("BING_CHUN_ORGANIZATION_ID")) != "" {
			merchantConfig, err := commerceonboarding.Load("config/merchants/bing-chun-nigeria.json")
			if err != nil {
				log.Fatal(err)
			}
			report, err := commerceonboarding.Apply(context.Background(), config.DB, merchantConfig)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Bing Chun commerce ready: stores=%d products=%d inventory_preserved=%d", report.Stores, report.Products, report.InventoryRowsPreserved)
		}
	}

	db := config.DB

	orgRepo := repository.NewOrganizationRepoPG(db)
	orgService := services.NewOrganisationService(orgRepo)

	campaignRepo := repository.NewCampaignRepoPG(db)
	campaignService := services.NewCampaignService(campaignRepo)

	balanceRepo := repository.NewBalanceRepoPG(db)
	balanceService := services.NewBalanceService(balanceRepo, campaignRepo)

	transactionRepo := repository.NewTransactionRepoPG(db)
	transactionService := services.NewTransactionService(transactionRepo)

	userRepo := repository.NewUserRepoPG(db)
	userService := services.NewUserService(userRepo)

	customerRepo := repository.NewCustomerRepoPG(db)
	customerService := services.NewCustomerService(customerRepo)

	responseRepo := repository.NewResponseRepoPG(db)
	responseService := services.NewResponseService(responseRepo)

	questionRepo := repository.NewQuestionRepoPG(db)
	questionService := services.NewQuestionService(questionRepo, responseRepo)

	paymentRepo := repository.NewPaymentRepoPG(db)
	paymentService := services.NewPaymentService(paymentRepo)

	// Commerce Dependencies
	productRepo := repository.NewProductRepoPG(db)
	orderRepo := repository.NewOrderRepoPG(db)
	orderService := services.NewOrderService(db, orderRepo, productRepo)
	commerceFoundationRepo := repository.NewCommerceFoundationRepoPG(db)
	commerceFoundationService := services.NewCommerceFoundationService(commerceFoundationRepo)
	commerceCatalogueRepo := repository.NewCommerceCatalogueRepoPG(db)
	commerceCatalogueService := services.NewCommerceCatalogueService(commerceCatalogueRepo, commerceFoundationRepo)
	commerceCustomerCartRepo := repository.NewCommerceCustomerCartRepoPG(db)
	commerceCustomerCartService := services.NewCommerceCustomerCartService(commerceCustomerCartRepo, commerceCustomerCartRepo, commerceCatalogueRepo, commerceFoundationRepo)
	commerceOrderRepo := repository.NewCommerceOrderRepoPG(db)
	commerceOrderService := services.NewCommerceOrderService(commerceOrderRepo, commerceCustomerCartRepo, commerceCustomerCartRepo, commerceFoundationRepo)
	commercePaymentRepo := repository.NewCommercePaymentRepoPG(db)
	commercePaymentProviders := payments.NewRegistry(payments.NewPaystackProvider(os.Getenv("PAYSTACK_SK"), os.Getenv("PAYSTACK_BASE_URL"), nil))
	commercePaymentProvider := os.Getenv("COMMERCE_PAYMENT_PROVIDER")
	if commercePaymentProvider == "" {
		commercePaymentProvider = payments.PaystackProviderName
	}
	commercePaymentService := services.NewCommercePaymentService(
		commercePaymentRepo, commerceOrderRepo, commerceCustomerCartRepo, commerceFoundationRepo,
		commercePaymentProviders, commercePaymentProvider, os.Getenv("COMMERCE_PAYMENT_CALLBACK_URL"),
	)
	commerceFulfilmentRepo := repository.NewCommerceFulfilmentRepoPG(db)
	commerceCodeManager, err := commercefulfilment.NewCodeManager(jwtSecret)
	if err != nil {
		log.Fatal(err)
	}
	commerceFulfilmentService := services.NewCommerceFulfilmentService(
		commerceFulfilmentRepo, commerceOrderRepo, commerceFoundationRepo,
		commercefulfilment.NewRegistry(), commerceCodeManager,
	)
	commerceStoreOrderService := services.NewCommerceStoreOrderService(commerceOrderService, commerceFulfilmentService)
	commerceChannelRepo := repository.NewCommerceChannelRepoPG(db)
	commerceChannelService := services.NewCommerceChannelService(
		commerceChannelRepo, commerceFoundationRepo, commerceCatalogueRepo,
		commerceCustomerCartService, commerceOrderService, commercePaymentService, commerceFulfilmentService,
	)
	commerceWhatsAppSender := messaging.NewMetaWhatsAppClient(
		os.Getenv("WHATSAPP_ACCESS_TOKEN"), os.Getenv("WHATSAPP_GRAPH_API_VERSION"), os.Getenv("WHATSAPP_GRAPH_API_BASE_URL"), nil,
	)
	commerceChannelDeliveryService := services.NewCommerceChannelDeliveryService(
		commerceChannelRepo, commerceWhatsAppSender, commerceOrderRepo, commerceFulfilmentRepo, commerceFulfilmentService,
	)
	server := handlers.NewServer(db, balanceService, transactionService, orgService, userService, campaignService, customerService, questionService, responseService, paymentService, orderService, commerceFoundationService, commerceCatalogueService, commerceCustomerCartService, commerceOrderService, commercePaymentService, commerceFulfilmentService, commerceStoreOrderService, commerceChannelService)

	app := fiber.New(fiber.Config{
		ProxyHeader: "X-Forwarded-For",
	})
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:3000, http://localhost:3001, http://localhost:3004, http://localhost:5173, https://bingchun.madebyletima.com, https://www.zidi-admin.vercel.app, https://zidi-admin.vercel.app, https://zidi-frontend.vercel.app, https://zidi-frontend.vercel.app/, https://216.198.79.65:3000, https://64.29.17.65:3000, https://admin.zidihq.com, https://www.admin.zidihq.com, https://www.app.zidihq.com, https://app.zidihq.com, https://zidihq.com, https://client.zidihq.com, https://www.client.zidihq.com, https://www.zidihq.com",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Prometheus metrics middleware
	prometheus := fiberprometheus.New("zidi_backend")
	prometheus.RegisterAt(app, "/metrics")
	app.Use(prometheus.Middleware)

	userAuth := middleware.AuthMiddleware(
		db,
		string(jwtSecret),
		utils.RoleUser,
		utils.RoleLegacyMerchantAdmin,
		utils.RoleMerchantAdmin,
		utils.RoleStoreManager,
		utils.RoleStoreStaff,
		utils.RoleLegacyPlatformAdmin,
		utils.RolePlatformAdmin,
	)
	//app.Post("/api/v1/auth/login", server.LoginUser)
	//app.Post("/api/v1/flutterwave/webhook", server.PostFlutterwaveWebhook)
	//app.Post("/api/v1/superuser/auth/login", server.SuperuserLogin)
	//app.Use(userAuth)

	//adminAuth := middleware.AuthMiddleware(string(jwtSecret), "admin")
	//zidiAuth := middleware.AuthMiddleware(string(jwtSecret), "zidi")
	//zidiAndAdminAuth := middleware.AuthMiddleware(string(jwtSecret), "zidi","admin")
	//adminAndUserAuth := middleware.AuthMiddleware(string(jwtSecret), )
	if !localDemoMode {
		go telegrambot.StartBot(db)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				if _, err := commercePaymentService.ExpirePendingPayments(context.Background(), 100); err != nil {
					log.Printf("expire pending commerce payments: %v", err)
				}
				<-ticker.C
			}
		}()
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				if _, err := commerceChannelDeliveryService.DispatchOnce(context.Background(), 25); err != nil {
					log.Printf("dispatch commerce channel messages: %v", err)
				}
				<-ticker.C
			}
		}()
	}

	app.Post("/api/v1/auth/login", server.LoginUser)
	app.Post("/api/v1/flutterwave/webhook", server.PostFlutterwaveWebhook)
	app.Post("/api/v1/superuser/auth/login", server.SuperuserLogin)
	app.Post("/api/v1/commerce/payments/:provider/webhook", func(c *fiber.Ctx) error {
		return server.CommercePaymentWebhook(c, c.Params("provider"))
	})

	// Protected API under /api/v1 with auth middleware
	apiGroup := app.Group("/api/v1")
	apiGroup.Get("/commerce/public/merchants/:merchant_slug/whatsapp-link", func(c *fiber.Ctx) error {
		return server.GetPublicCommerceWhatsAppLink(c, c.Params("merchant_slug"))
	})
	protectedGroup := apiGroup.Group("", userAuth)
	api.RegisterHandlers(protectedGroup, server)

	if !localDemoMode {
		server.SeedDefaultOrganization()
		server.SeedCommerceData() // Seed Lush Hair data
	}
	// Expose WhatsApp webhook publicly at root-level (avoid any auth middleware on /api/v1)
	app.Get("/whatsapp/webhook", whatsappbot.WebhookVerification)
	app.Post("/whatsapp/webhook", whatsappbot.WebhookHandlerWithCommerce(db, commerceChannelService))

	// And we serve HTTP until the world ends.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))

}
