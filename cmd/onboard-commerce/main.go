package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/hidenkeys/zidibackend/commerceonboarding"
	"github.com/hidenkeys/zidibackend/config"
	"github.com/joho/godotenv"
)

func main() {
	configPath := flag.String("config", "config/merchants/bing-chun-nigeria.json", "path to a commerce merchant onboarding JSON file")
	dryRun := flag.Bool("dry-run", false, "validate and summarize without connecting to the database")
	flag.Parse()

	_ = godotenv.Load()
	merchantConfig, err := commerceonboarding.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	summary := merchantConfig.Summary()
	if *dryRun {
		fmt.Printf("valid commerce config: %d stores, %d categories, %d products, %d variants, %d store catalogue rows\n", summary.Stores, summary.Categories, summary.Products, summary.Variants, summary.StoreItems)
		return
	}

	config.ConnectDatabase()
	config.MigrateDatabase()
	report, err := commerceonboarding.Apply(context.Background(), config.DB, merchantConfig)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"onboarded %s: %d stores, %d categories, %d products, %d variants; inventory created=%d preserved=%d\n",
		merchantConfig.Merchant.DisplayName, report.Stores, report.Categories, report.Products, report.Variants,
		report.InventoryRowsCreated, report.InventoryRowsPreserved,
	)
}
