package main

import (
	"log"
	"os"

	fiberprometheus "github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/hidenkeys/zidibackend/api"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/handlers"
	"github.com/hidenkeys/zidibackend/middleware"
	"github.com/hidenkeys/zidibackend/repository"
	"github.com/hidenkeys/zidibackend/services"
	"github.com/hidenkeys/zidibackend/telegrambot"
	"github.com/hidenkeys/zidibackend/whatsappbot"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	var jwtSecret = []byte("your-secret-key")
	config.ConnectDatabase()
	config.MigrateDatabase()

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

	server := handlers.NewServer(db, balanceService, transactionService, orgService, userService, campaignService, customerService, questionService, responseService, paymentService, orderService)

	app := fiber.New(fiber.Config{
		ProxyHeader: "X-Forwarded-For",
	})
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:3000, https://www.zidi-admin.vercel.app, https://zidi-admin.vercel.app, https://zidi-frontend.vercel.app, https://zidi-frontend.vercel.app/, https://216.198.79.65:3000, https://64.29.17.65:3000, https://admin.zidihq.com, https://www.admin.zidihq.com, https://www.app.zidihq.com, https://app.zidihq.com, https://zidihq.com, https://client.zidihq.com, https://www.client.zidihq.com, https://www.zidihq.com",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Prometheus metrics middleware
	prometheus := fiberprometheus.New("zidi_backend")
	prometheus.RegisterAt(app, "/metrics")
	app.Use(prometheus.Middleware)

	userAuth := middleware.AuthMiddleware(db, string(jwtSecret), "user", "admin", "zidi")
	//app.Post("/api/v1/auth/login", server.LoginUser)
	//app.Post("/api/v1/flutterwave/webhook", server.PostFlutterwaveWebhook)
	//app.Post("/api/v1/superuser/auth/login", server.SuperuserLogin)
	//app.Use(userAuth)

	//adminAuth := middleware.AuthMiddleware(string(jwtSecret), "admin")
	//zidiAuth := middleware.AuthMiddleware(string(jwtSecret), "zidi")
	//zidiAndAdminAuth := middleware.AuthMiddleware(string(jwtSecret), "zidi","admin")
	//adminAndUserAuth := middleware.AuthMiddleware(string(jwtSecret), )
	go telegrambot.StartBot(db)

	app.Post("/api/v1/auth/login", server.LoginUser)
	app.Post("/api/v1/flutterwave/webhook", server.PostFlutterwaveWebhook)
	app.Post("/api/v1/superuser/auth/login", server.SuperuserLogin)

	// Protected API under /api/v1 with auth middleware
	apiGroup := app.Group("/api/v1")
	protectedGroup := apiGroup.Group("", userAuth)
	api.RegisterHandlers(protectedGroup, server)

	server.SeedDefaultOrganization()
	server.SeedCommerceData() // Seed Lush Hair data
	// Expose WhatsApp webhook publicly at root-level (avoid any auth middleware on /api/v1)
	app.Get("/whatsapp/webhook", whatsappbot.WebhookVerification)
	app.Post("/whatsapp/webhook", whatsappbot.WebhookHandler(db))

	// And we serve HTTP until the world ends.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))

}
