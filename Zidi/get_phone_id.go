//go:build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	
	token := os.Getenv("WHATSAPP_ACCESS_TOKEN")
	
	// Get WhatsApp Business Account ID first
	url := "https://graph.facebook.com/v18.0/me?fields=whatsapp_business_accounts"
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Account info: %s\n", string(body))
}