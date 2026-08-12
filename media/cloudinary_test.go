package media

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCloudinaryImageUploaderSignsAndUploadsImage(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1_1/demo/image/upload" {
			t.Fatalf("unexpected upload path %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart upload: %v", err)
		}
		timestamp := r.FormValue("timestamp")
		expected := cloudinarySignature(map[string]string{"folder": "zidi/products", "timestamp": timestamp}, "secret")
		if r.FormValue("api_key") != "key" || r.FormValue("folder") != "zidi/products" || r.FormValue("signature") != expected {
			t.Fatalf("unexpected signed fields: %#v", r.MultipartForm.Value)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "jpeg-content" {
			t.Fatalf("unexpected file content %q", content)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"secure_url":"https://res.cloudinary.com/demo/image/upload/v1/zidi/products/tea.png","public_id":"zidi/products/tea","width":800,"height":800,"format":"png"}`)),
		}, nil
	})}

	uploader := NewCloudinaryImageUploader("demo", "key", "secret", "zidi/products", "https://api.cloudinary.test", client)
	result, err := uploader.UploadImage(context.Background(), "tea.jpg", "image/jpeg", []byte("jpeg-content"))
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	if result.PublicID != "zidi/products/tea" || result.URL != "https://res.cloudinary.com/demo/image/upload/f_jpg,q_auto/v1/zidi/products/tea.png" {
		t.Fatalf("unexpected upload result: %#v", result)
	}
}
