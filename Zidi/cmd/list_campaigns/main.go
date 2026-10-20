package main

import (
	"fmt"
	"log"

	"github.com/hidenkeys/zidibackend/config"
	"github.com/hidenkeys/zidibackend/models"
)

func main() {
	config.ConnectDatabase()

	var campaigns []models.Campaign
	if err := config.DB.Order("created_at DESC").Find(&campaigns).Error; err != nil {
		log.Fatal("query failed:", err)
	}

	if len(campaigns) == 0 {
		fmt.Println("no campaigns found")
		return
	}

	for _, c := range campaigns {
		fmt.Printf("%s\t%s\t%s\n", c.ID, c.Status, c.CampaignName)
	}
}
