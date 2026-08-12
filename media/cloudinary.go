package media

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ImageUpload struct {
	URL      string
	PublicID string
	Width    int
	Height   int
	Format   string
}

type ImageUploader interface {
	UploadImage(ctx context.Context, fileName, contentType string, content []byte) (*ImageUpload, error)
}

type CloudinaryImageUploader struct {
	cloudName string
	apiKey    string
	apiSecret string
	folder    string
	apiURL    string
	client    *http.Client
}

func NewCloudinaryImageUploader(cloudName, apiKey, apiSecret, folder, apiURL string, client *http.Client) *CloudinaryImageUploader {
	if strings.TrimSpace(folder) == "" {
		folder = "zidi-commerce/products"
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.cloudinary.com"
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &CloudinaryImageUploader{
		cloudName: strings.TrimSpace(cloudName), apiKey: strings.TrimSpace(apiKey), apiSecret: strings.TrimSpace(apiSecret),
		folder: strings.Trim(strings.TrimSpace(folder), "/"), apiURL: strings.TrimRight(strings.TrimSpace(apiURL), "/"), client: client,
	}
}

func (u *CloudinaryImageUploader) UploadImage(ctx context.Context, fileName, contentType string, content []byte) (*ImageUpload, error) {
	if u == nil || u.cloudName == "" || u.apiKey == "" || u.apiSecret == "" {
		return nil, errors.New("Cloudinary image uploads are not configured")
	}
	if len(content) == 0 {
		return nil, errors.New("image content is empty")
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := cloudinarySignature(map[string]string{"folder": u.folder, "timestamp": timestamp}, u.apiSecret)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFileName(fileName)))
	fileHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	for key, value := range map[string]string{
		"api_key": u.apiKey, "folder": u.folder, "timestamp": timestamp, "signature": signature,
	} {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v1_1/%s/image/upload", u.apiURL, url.PathEscape(u.cloudName))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := u.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("upload image to Cloudinary: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Cloudinary upload returned %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		SecureURL string `json:"secure_url"`
		PublicID  string `json:"public_id"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Format    string `json:"format"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("decode Cloudinary upload: %w", err)
	}
	if !strings.HasPrefix(result.SecureURL, "https://") || result.PublicID == "" {
		return nil, errors.New("Cloudinary returned an incomplete image upload")
	}
	return &ImageUpload{
		URL: cloudinaryWhatsAppImageURL(result.SecureURL), PublicID: result.PublicID,
		Width: result.Width, Height: result.Height, Format: result.Format,
	}, nil
}

func cloudinarySignature(params map[string]string, secret string) string {
	keys := []string{"folder", "timestamp"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	digest := sha1.Sum([]byte(strings.Join(parts, "&") + secret))
	return hex.EncodeToString(digest[:])
}

func cloudinaryWhatsAppImageURL(value string) string {
	return strings.Replace(value, "/image/upload/", "/image/upload/f_jpg,q_auto/", 1)
}

func escapeMultipartFileName(value string) string {
	value = strings.ReplaceAll(value, "\\", "_")
	return strings.ReplaceAll(value, `"`, "_")
}
