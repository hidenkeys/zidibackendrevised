//go:build ignore

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func main() {
	// Generate a new RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Println("Error generating key pair:", err)
		return
	}

	// Extract the public key from the private key
	publicKey := &privateKey.PublicKey

	// Save the private key to a file
	privateKeyFile, err := os.Create("private.key")
	if err != nil {
		fmt.Println("Error creating private key file:", err)
		return
	}
	defer func(privateKeyFile *os.File) {
		err := privateKeyFile.Close()
		if err != nil {
			fmt.Println("Error closing private key file:", err)
		}
	}(privateKeyFile)

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	err = pem.Encode(privateKeyFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	if err != nil {
		fmt.Println("Error encoding private key:", err)
		return
	}

	// Save the public key to a file
	publicKeyFile, err := os.Create("public.key")
	if err != nil {
		fmt.Println("Error creating public key file:", err)
		return
	}
	defer func(publicKeyFile *os.File) {
		err := publicKeyFile.Close()
		if err != nil {
			fmt.Println("Error closing public key file:", err)
		}
	}(publicKeyFile)

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		fmt.Println("Error marshalling public key:", err)
		return
	}

	err = pem.Encode(publicKeyFile, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	if err != nil {
		fmt.Println("Error encoding public key:", err)
		return
	}

	fmt.Println("Private and public keys generated and saved.")
}
