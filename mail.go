//go:build ignore

package main

import (
	"fmt"
	"log"
	"net/smtp"
)

func SendEmail00(to, subject, body string) error {
	from := "test-notification@nibss-plc.com.ng"
	password := "Moj01356"

	auth := smtp.PlainAuth("", from, password, "192.168.202.223")

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: " + subject + "\n" +
		"MIME-version: 1.0;\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\n\n" +
		body

	err := smtp.SendMail("192.168.202.223:25", auth, from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("error sending email: %w", err)
	}
	return nil
}

func main() {
	// Test the email function
	to := "osoabnde@nibss-plc.com.ng" // Replace with your test email
	subject := "Test Email"
	body := "<h1>Hello!</h1><p>This is a test email from Zidi.</p>"

	fmt.Println("Sending test email...")
	err := SendEmail00(to, subject, body)
	if err != nil {
		log.Printf("Failed to send email: %v", err)
	} else {
		fmt.Println("Email sent successfully!")
	}
}
