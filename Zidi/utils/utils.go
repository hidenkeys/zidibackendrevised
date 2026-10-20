package utils

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	brevo "github.com/sendinblue/APIv3-go-library/v2/lib"
	"io"
	"net/smtp"

	//"log"
	"math/rand"
	"net/http"
	//"net/smtp"
	"os"
	"time"
)

func GenerateTokens(charType string, length int, count int) []string {
	charsets := map[string]string{
		"alphanumerical": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		"numeric":        "0123456789",
	}

	charset, ok := charsets[charType]
	if !ok {
		charset = charsets["alphanumerical"] // Default to alphanumerical
	}

	tokens := make(map[string]struct{})
	var tokenList []string

	for len(tokenList) < count {
		token := make([]byte, length)
		for i := range token {
			token[i] = charset[rand.Intn(len(charset))]
		}

		tokenStr := string(token)
		if _, exists := tokens[tokenStr]; !exists {
			tokens[tokenStr] = struct{}{}
			tokenList = append(tokenList, tokenStr)
		}
	}

	return tokenList
}

// send email zoho
func SendEmail00(to, subject, body string) error {
	from := os.Getenv("ZOHO_SMTP_FROM")
	password := os.Getenv("ZOHO_SMTP_PASSWORD")
	if from == "" || password == "" {
		return errors.New("ZOHO_SMTP_FROM and ZOHO_SMTP_PASSWORD must be configured")
	}

	// Use Zoho SMTP host in PlainAuth
	auth := smtp.PlainAuth("", from, password, "smtp.zoho.com")

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: " + subject + "\n" +
		"MIME-version: 1.0;\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\n\n" +
		body

	err := smtp.SendMail("smtp.zoho.com:587", auth, from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("error sending email: %w", err)
	}
	return nil
}

func SendEmail0(toEmail, subject, htmlBody string) error {
	cfg := brevo.NewConfiguration()
	cfg.AddDefaultHeader("api-key", os.Getenv("BREVO_API_KEY")) // Replace with your actual Brevo API key

	client := brevo.NewAPIClient(cfg)

	sender := brevo.SendSmtpEmailSender{
		Name:  "zidi",
		Email: "letimapro23@gmail.com", // Must be a verified sender in Brevo
	}

	to := []brevo.SendSmtpEmailTo{
		{
			Email: toEmail,
		},
	}

	email := brevo.SendSmtpEmail{
		Sender:      &sender,
		To:          to,
		Subject:     subject,
		HtmlContent: htmlBody,
	}

	_, _, err := client.TransactionalEmailsApi.SendTransacEmail(context.Background(), email)
	return err

}

// List of Paystack webhook IPs (you can update these if Paystack changes them)
var paystackIPWhitelist = []string{
	"52.31.139.75",
	"52.49.173.169",
	"52.214.14.220",
}
var paystackIPs = []string{
	"52.31.139.75",
	"52.49.173.169",
	"52.214.14.220",
}

// IsPaystackIPWhitelisted checks if the request IP is in the known Paystack IPs
func IsPaystackIPWhitelisted(clientIP string) bool {
	for _, ip := range paystackIPs {
		if clientIP == ip {
			return true
		}
	}
	return false
}

// PaystackResponse holds the response from Paystack API
type PaystackResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

// CreatePaystackPaymentLink initializes a payment with metadata
func CreatePaystackPaymentLink(email string, amount int, campaignID, organizationID string) (string, error) {
	paystackURL := "https://api.paystack.co/transaction/initialize"
	apiKey := os.Getenv("PAYSTACK_SK") // Store in .env

	// Convert amount to kobo (Paystack requires amount in kobo)
	amountKobo := amount * 100

	// Create the request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"email":  email,
		"amount": amountKobo,
		"metadata": map[string]interface{}{
			"campaign_id":     campaignID,
			"organization_id": organizationID,
		},
	})
	if err != nil {
		return "", err
	}

	// Make the HTTP request
	req, err := http.NewRequest("POST", paystackURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	// Parse the response
	var paystackResp PaystackResponse
	if err := json.NewDecoder(resp.Body).Decode(&paystackResp); err != nil {
		return "", err
	}

	if !paystackResp.Status {
		return "", fmt.Errorf("failed to initialize payment: %s", paystackResp.Message)
	}

	return paystackResp.Data.AuthorizationURL, nil
}

type PaystackVerifyResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Amount   int    `json:"amount"`
		Status   string `json:"status"`
		Ref      string `json:"reference"`
		Currency string `json:"currency"`
	} `json:"data"`
}

func VerifyPaystackTransaction(reference string) (bool, error) {
	url := fmt.Sprintf("https://api.paystack.co/transaction/verify/%s", reference)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Add("Authorization", "Bearer "+os.Getenv("PAYSTACK_SK"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	var result PaystackVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	if result.Status && result.Data.Status == "success" {
		return true, nil
	}
	return false, nil
}

// FlutterwaveResponse defines the response structure
type FlutterwaveResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Link string `json:"link"`
	} `json:"data"`
}

func CreateFlutterwavePaymentLink(email string, amount int, campaignID, organizationID string) (string, error) {
	flutterwaveURL := "https://api.flutterwave.com/v3/payments"
	apiKey := os.Getenv("FLW_SECRET_KEY")

	// Generate a unique transaction reference
	txRef := fmt.Sprintf("%s-%s", campaignID, uuid.New().String())

	requestBody, err := json.Marshal(map[string]interface{}{
		"tx_ref":       txRef,
		"amount":       amount,
		"currency":     "NGN",
		"redirect_url": "https://yourwebsite.com/payment-success",
		"customer": map[string]string{
			"email": email,
		},
		"meta": map[string]string{
			"campaign_id":     campaignID,
			"organization_id": organizationID,
		},
		"customizations": map[string]string{
			"title":       "Campaign Payment",
			"description": "Payment for campaign",
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", flutterwaveURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	var flutterwaveResp FlutterwaveResponse
	if err := json.NewDecoder(resp.Body).Decode(&flutterwaveResp); err != nil {
		return "", err
	}

	if flutterwaveResp.Status != "success" {
		return "", fmt.Errorf("failed to initialize payment: %s", flutterwaveResp.Message)
	}

	return flutterwaveResp.Data.Link, nil
}

func VerifyFlutterwaveSignature(body []byte, signature, secret string) bool {
	fmt.Println("00")
	if secret == "" || signature == "" {
		return false
	}

	fmt.Println("01")

	// Compute HMAC SHA-256 hash of the request body
	hash := hmac.New(sha256.New, []byte(secret))
	fmt.Println("03")
	hash.Write(body)
	fmt.Println("03")
	expectedSignature := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	fmt.Println("signature", signature)
	fmt.Println("expected sigaturew", expectedSignature)
	fmt.Println("04")

	return signature == expectedSignature
}

// verifyFlutterwaveTransaction checks transaction status from Flutterwave API
func VerifyFlutterwaveTransaction(transactionID int) (bool, error) {
	apiURL := fmt.Sprintf("https://api.flutterwave.com/v3/transactions/%d/verify", transactionID)
	apiKey := os.Getenv("FLW_SECRET_KEY") // Get secret key from .env

	// Make request to Flutterwave API
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	// Parse JSON response
	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return false, err
	}

	// Check if the transaction was successful
	if status, ok := response["status"].(string); ok && status == "success" {
		data, exists := response["data"].(map[string]interface{})
		if exists {
			if txStatus, ok := data["status"].(string); ok && txStatus == "successful" {
				return true, nil
			}
		}
	}
	return false, fmt.Errorf("transaction not successful")
}

// AirtimeRequest represents the structure for the Flutterwave airtime API call
//type AirtimeRequest struct {
//	Country     string  `json:"country"`
//	CustomerID  string  `json:"customer_id"`
//	Amount      float64 `json:"amount"`
//	Reference   string  `json:"reference"`
//	CallbackURL string  `json:"callback_url"`
//}
//
//type AirtimeResponse struct {
//	Status  string `json:"status"`
//	Message string `json:"message"`
//	Data    struct {
//		PhoneNumber   string  `json:"phone_number"`
//		Amount        float64 `json:"amount"`
//		Network       string  `json:"network"`
//		Code          string  `json:"code"`
//		TxRef         string  `json:"tx_ref"`
//		Reference     string  `json:"reference"`
//		BatchRef      string  `json:"batch_reference"`
//		RechargeToken string  `json:"recharge_token"`
//		Fee           float64 `json:"fee"`
//	} `json:"data"`
//}
//
//// sendAirtime triggers the Flutterwave bill payment API to send airtime
//func SendAirtime(phone string, amount float64) (*AirtimeResponse, error) {
//	url := "https://api.flutterwave.com/v3/billers/BIL099/items/AT099/payment"
//	token := os.Getenv("FLW_SECRET_KEY")
//
//	requestBody := AirtimeRequest{
//		Country:     "NG",
//		CustomerID:  phone,
//		Amount:      amount,
//		Reference:   fmt.Sprintf("%d", time.Now().Unix()), // Generate a unique reference
//		CallbackURL: "https://your-callback-url.com",
//	}
//
//	jsonData, err := json.Marshal(requestBody)
//	if err != nil {
//		return nil, err
//	}
//
//	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
//	if err != nil {
//		return nil, err
//	}
//	req.Header.Set("Authorization", "Bearer "+token)
//	req.Header.Set("Content-Type", "application/json")
//	req.Header.Set("Accept", "application/json")
//
//	client := &http.Client{}
//	resp, err := client.Do(req)
//	if err != nil {
//		return nil, err
//	}
//	defer func(Body io.ReadCloser) {
//		err := Body.Close()
//		if err != nil {
//			// Handle error
//		}
//	}(resp.Body)
//
//	if resp.StatusCode != http.StatusOK {
//		return nil, fmt.Errorf("failed to send airtime: status %d", resp.StatusCode, resp.Body)
//	}
//
//	// Parse the response body into AirtimeResponse struct
//	var airtimeResponse AirtimeResponse
//	err = json.NewDecoder(resp.Body).Decode(&airtimeResponse)
//	if err != nil {
//		return nil, fmt.Errorf("failed to parse response: %v", err)
//	}
//
//	// Check if the response status is not success
//	if airtimeResponse.Status != "success" {
//		return nil, fmt.Errorf("airtime transaction failed: %s", airtimeResponse.Message)
//	}
//
//	// Return the parsed response data
//	return &airtimeResponse, nil
//}

type AirtimeRequest struct {
	RequestID     string `json:"request_id"`
	ServiceID     string `json:"serviceID"`
	VariationCode string `json:"variation_code"`
	Amount        string `json:"amount"`
	Phone         string `json:"phone"`
}

type Transaction struct {
	Status     string `json:"status"`
	Amount     string `json:"amount"`
	Network    string `json:"product_name"`
	RequestID  string `json:"request_id"`
	Phone      string `json:"phone"`
	Commission string `json:"commission"`
}

type vtpassResponse struct {
	Code                string      `json:"code"`
	ResponseDescription string      `json:"response_description"`
	RequestID           string      `json:"requestId"`
	Amount              json.Number `json:"amount"`
	TransactionDate     string      `json:"transaction_date"`
	PurchasedCode       string      `json:"purchased_code"`
	Content             struct {
		Transactions struct {
			Status            string      `json:"status"`
			Product           string      `json:"product_name"`
			Phone             string      `json:"phone"`
			Amount            json.Number `json:"amount"`
			Commission        float64     `json:"commission"`
			CommissionDetails struct {
				Amount          float64 `json:"amount"`
				Rate            string  `json:"rate"`
				RateType        string  `json:"rate_type"`
				ComputationType string  `json:"computation_type"`
			} `json:"commission_details"`
		} `json:"transactions"`
	} `json:"content"`
}

func SendAirtime(amount, network, phone string) (*Transaction, error) {
	url := "https://vtpass.com/api/pay"
	client := &http.Client{Timeout: 10 * time.Second}

	reqBody := AirtimeRequest{
		RequestID:     fmt.Sprintf("%d", time.Now().UnixNano()),
		ServiceID:     network,
		VariationCode: "",
		Amount:        amount,
		Phone:         phone,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", os.Getenv("VTPASS_API_KEY"))
	req.Header.Set("secret-key", os.Getenv("VTPASS_API_SECRET_KEY"))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	// Read body once
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: print raw JSON response
	fmt.Println("Raw response:", string(respBody))

	// Unmarshal into struct
	var vtResp vtpassResponse
	if err := json.Unmarshal(respBody, &vtResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Return formatted transaction
	return &Transaction{
		Status:     vtResp.Content.Transactions.Status,
		Amount:     vtResp.Amount.String(),
		Network:    vtResp.Content.Transactions.Product,
		RequestID:  vtResp.RequestID,
		Phone:      vtResp.Content.Transactions.Phone,
		Commission: fmt.Sprintf("%.2f", vtResp.Content.Transactions.Commission),
	}, nil
}
