package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectImageMimeType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "JPEG file",
			filename: "image.jpg",
			want:     "image/jpeg",
		},
		{
			name:     "JPEG file with uppercase extension",
			filename: "IMAGE.JPEG",
			want:     "image/jpeg",
		},
		{
			name:     "PNG file",
			filename: "screenshot.png",
			want:     "image/png",
		},
		{
			name:     "GIF file",
			filename: "animation.gif",
			want:     "image/gif",
		},
		{
			name:     "WebP file",
			filename: "photo.webp",
			want:     "image/webp",
		},
		{
			name:     "unsupported file type",
			filename: "document.pdf",
			want:     "",
		},
		{
			name:     "file with no extension",
			filename: "image",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectImageMimeType(tc.filename)
			if got != tc.want {
				t.Errorf("detectImageMimeType(%s) = %s, want %s", tc.filename, got, tc.want)
			}
		})
	}
}

func TestResolveAltText(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		altTexts []string
		want     string
	}{
		{
			name:     "specific alt text for index",
			index:    0,
			altTexts: []string{"First image", "Second image"},
			want:     "First image",
		},
		{
			name:     "second image with specific alt text",
			index:    1,
			altTexts: []string{"First image", "Second image"},
			want:     "Second image",
		},
		{
			name:     "single alt text for all images",
			index:    2,
			altTexts: []string{"Same alt for all"},
			want:     "Same alt for all",
		},
		{
			name:     "default alt text when no alts provided",
			index:    0,
			altTexts: []string{},
			want:     "Image 1",
		},
		{
			name:     "default alt text when empty alt provided",
			index:    1,
			altTexts: []string{"   "},
			want:     "Image 2",
		},
		{
			name:     "default alt when index exceeds alt texts length",
			index:    3,
			altTexts: []string{"First", "Second"},
			want:     "Image 4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAltText(tc.index, tc.altTexts)
			if got != tc.want {
				t.Errorf("resolveAltText(%d, %v) = %s, want %s", tc.index, tc.altTexts, got, tc.want)
			}
		})
	}
}

func TestParseImagePaths(t *testing.T) {
	tests := []struct {
		name       string
		imagePaths string
		want       []string
	}{
		{
			name:       "single path",
			imagePaths: "/path/to/image.png",
			want:       []string{"/path/to/image.png"},
		},
		{
			name:       "multiple paths",
			imagePaths: "/path/one.png,/path/two.jpg,/path/three.gif",
			want:       []string{"/path/one.png", "/path/two.jpg", "/path/three.gif"},
		},
		{
			name:       "paths with spaces",
			imagePaths: " /path/one.png , /path/two.jpg , /path/three.gif ",
			want:       []string{"/path/one.png", "/path/two.jpg", "/path/three.gif"},
		},
		{
			name:       "empty string",
			imagePaths: "",
			want:       nil,
		},
		{
			name:       "paths with empty entries",
			imagePaths: "/path/one.png,,/path/two.jpg",
			want:       []string{"/path/one.png", "/path/two.jpg"},
		},
		{
			name:       "only commas and spaces",
			imagePaths: " , , ",
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseImagePaths(tc.imagePaths)
			if len(got) != len(tc.want) {
				t.Errorf("parseImagePaths() length = %d, want %d", len(got), len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseImagePaths()[%d] = %s, want %s", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestValidateImageData(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		imageData []byte
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid PNG image under size limit",
			path:      "test.png",
			imageData: make([]byte, 500000),
			wantErr:   false,
		},
		{
			name:      "valid JPEG image at size limit",
			path:      "test.jpg",
			imageData: make([]byte, maxImageSize),
			wantErr:   false,
		},
		{
			name:      "image exceeds size limit",
			path:      "large.png",
			imageData: make([]byte, maxImageSize+1),
			wantErr:   true,
			errMsg:    "exceeds maximum size",
		},
		{
			name:      "unsupported file format",
			path:      "document.pdf",
			imageData: make([]byte, 1000),
			wantErr:   true,
			errMsg:    "unsupported image format",
		},
		{
			name:      "valid GIF image",
			path:      "animation.gif",
			imageData: make([]byte, 100000),
			wantErr:   false,
		},
		{
			name:      "valid WebP image",
			path:      "photo.webp",
			imageData: make([]byte, 100000),
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateImageData(tc.path, tc.imageData)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateImageData() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil && tc.errMsg != "" {
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("validateImageData() error = %v, want error containing %q", err, tc.errMsg)
				}
			}
		})
	}
}

func TestProcessImages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a temporary directory for test images
	tempDir, err := os.MkdirTemp("", "bluesky-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a small test PNG image (1x1 pixel, valid PNG format)
	testPNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}

	testImagePath := filepath.Join(tempDir, "test.png")
	if err := os.WriteFile(testImagePath, testPNG, 0644); err != nil {
		t.Fatalf("Failed to write test image: %v", err)
	}

	tests := []struct {
		name         string
		imagePaths   string
		altTexts     string
		setupMock    func() *httptest.Server
		wantErr      bool
		wantCount    int
		wantAltTexts []string
	}{
		{
			name:       "successful single image upload",
			imagePaths: testImagePath,
			altTexts:   "Test image",
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := map[string]interface{}{
						"blob": map[string]interface{}{
							"$type": "blob",
							"ref": map[string]interface{}{
								"$link": "bafkreibabalobzn6cd366ukcsjycp4yymjymgfxcv6xczmlgpemzkz3cfa",
							},
							"mimeType": "image/png",
							"size":     len(testPNG),
						},
					}
					json.NewEncoder(w).Encode(response)
				}))
			},
			wantErr:      false,
			wantCount:    1,
			wantAltTexts: []string{"Test image"},
		},
		{
			name:       "multiple images with individual alt texts",
			imagePaths: testImagePath + "," + testImagePath,
			altTexts:   "First image,Second image",
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := map[string]interface{}{
						"blob": map[string]interface{}{
							"$type": "blob",
							"ref": map[string]interface{}{
								"$link": "bafkreibabalobzn6cd366ukcsjycp4yymjymgfxcv6xczmlgpemzkz3cfa",
							},
							"mimeType": "image/png",
							"size":     len(testPNG),
						},
					}
					json.NewEncoder(w).Encode(response)
				}))
			},
			wantErr:      false,
			wantCount:    2,
			wantAltTexts: []string{"First image", "Second image"},
		},
		{
			name:       "empty image paths",
			imagePaths: "",
			altTexts:   "",
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:       "non-existent file",
			imagePaths: "/nonexistent/image.png",
			altTexts:   "Test",
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			},
			wantErr:   true,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := tc.setupMock()
			defer mockServer.Close()

			result, err := processImages(mockServer.URL, "fake-token", tc.imagePaths, tc.altTexts, logger)

			if (err != nil) != tc.wantErr {
				t.Errorf("processImages() error = %v, wantErr %v", err, tc.wantErr)
			}

			if tc.wantCount == 0 && result != nil {
				t.Error("processImages() expected nil result for no images")
			}

			if tc.wantCount > 0 {
				if result == nil {
					t.Fatal("processImages() returned nil result when expecting images")
				}

				if len(result.Images) != tc.wantCount {
					t.Errorf("processImages() returned %d images, want %d", len(result.Images), tc.wantCount)
				}

				if result.Type != "app.bsky.embed.images" {
					t.Errorf("processImages() embed type = %s, want 'app.bsky.embed.images'", result.Type)
				}

				for i, altText := range tc.wantAltTexts {
					if i < len(result.Images) && result.Images[i].Alt != altText {
						t.Errorf("processImages() image[%d] alt = %s, want %s", i, result.Images[i].Alt, altText)
					}
				}
			}
		})
	}
}

func TestProcessImagesMaxCount(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a temporary directory for test images
	tempDir, err := os.MkdirTemp("", "bluesky-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test image
	testPNG := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}

	testImagePath := filepath.Join(tempDir, "test.png")
	if err := os.WriteFile(testImagePath, testPNG, 0644); err != nil {
		t.Fatalf("Failed to write test image: %v", err)
	}

	// Test with more than 4 images (should fail)
	fiveImages := testImagePath + "," + testImagePath + "," + testImagePath + "," + testImagePath + "," + testImagePath

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mockServer.Close()

	_, err = processImages(mockServer.URL, "fake-token", fiveImages, "Test", logger)
	if err == nil {
		t.Error("processImages() expected error for more than 4 images, got nil")
	}
}

func TestProcessImagesMaxSize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a temporary directory for test images
	tempDir, err := os.MkdirTemp("", "bluesky-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an image larger than 1MB
	largeImage := make([]byte, 1000001)
	largeImagePath := filepath.Join(tempDir, "large.png")
	if err := os.WriteFile(largeImagePath, largeImage, 0644); err != nil {
		t.Fatalf("Failed to write large image: %v", err)
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mockServer.Close()

	_, err = processImages(mockServer.URL, "fake-token", largeImagePath, "Test", logger)
	if err == nil {
		t.Error("processImages() expected error for image larger than 1MB, got nil")
	}
}
