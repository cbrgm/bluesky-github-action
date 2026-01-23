package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSession(t *testing.T) {
	tests := []struct {
		name           string
		handle         string
		password       string
		mockResponse   string
		mockStatusCode int
		wantErr        bool
	}{
		{
			name:           "successful session creation",
			handle:         "testUser",
			password:       "testPass",
			mockResponse:   `{"accessJwt": "fake-jwt-token", "did": "user-did"}`,
			mockStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "failed session creation with incorrect credentials",
			handle:         "testUser",
			password:       "wrongPass",
			mockResponse:   `{"error": "Unauthorized"}`,
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "failed session creation with internal server error",
			handle:         "testUser",
			password:       "testPass",
			mockResponse:   `{"error": "Internal Server Error"}`,
			mockStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.mockStatusCode)
				fmt.Fprintln(w, tc.mockResponse)
			}))
			defer mockServer.Close()

			_, err := createSession(mockServer.URL, tc.handle, tc.password)

			if (err != nil) != tc.wantErr {
				t.Errorf("createSession() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestPublishPost(t *testing.T) {
	tests := []struct {
		name           string
		session        *SessionResponse
		post           *Post
		mockResponse   string
		mockStatusCode int
		wantErr        bool
	}{
		{
			name: "successful post publication",
			session: &SessionResponse{
				AccessToken: "fake-jwt-token",
				UserID:      "user-did",
			},
			post: &Post{
				Type:      "app.bsky.feed.post",
				Text:      "Test Post",
				CreatedAt: "2023-01-01T00:00:00Z",
			},
			mockResponse:   `{}`,
			mockStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name: "failed post publication with unauthorized error",
			session: &SessionResponse{
				AccessToken: "expired-jwt-token",
				UserID:      "user-did",
			},
			post: &Post{
				Type:      "app.bsky.feed.post",
				Text:      "Test Post",
				CreatedAt: "2023-01-01T00:00:00Z",
			},
			mockResponse:   `{"error": "Unauthorized"}`,
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
	}

	// Creating a logger for test purposes
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.mockStatusCode)
				fmt.Fprintln(w, tc.mockResponse)
			}))
			defer mockServer.Close()

			err := publishPost(mockServer.URL, tc.session, tc.post, logger)

			if (err != nil) != tc.wantErr {
				t.Errorf("publishPost() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestUploadBlob(t *testing.T) {
	tests := []struct {
		name           string
		imageData      []byte
		mimeType       string
		mockResponse   string
		mockStatusCode int
		wantErr        bool
	}{
		{
			name:      "successful blob upload",
			imageData: []byte("fake-image-data"),
			mimeType:  "image/png",
			mockResponse: `{
				"blob": {
					"$type": "blob",
					"ref": {
						"$link": "bafkreibabalobzn6cd366ukcsjycp4yymjymgfxcv6xczmlgpemzkz3cfa"
					},
					"mimeType": "image/png",
					"size": 15
				}
			}`,
			mockStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "failed blob upload with unauthorized error",
			imageData:      []byte("fake-image-data"),
			mimeType:       "image/jpeg",
			mockResponse:   `{"error": "Unauthorized"}`,
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "failed blob upload with server error",
			imageData:      []byte("fake-image-data"),
			mimeType:       "image/gif",
			mockResponse:   `{"error": "Internal Server Error"}`,
			mockStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				if r.Header.Get("Content-Type") != tc.mimeType {
					t.Errorf("Expected Content-Type %s, got %s", tc.mimeType, r.Header.Get("Content-Type"))
				}
				if r.Header.Get("Authorization") != "Bearer fake-token" {
					t.Errorf("Expected Authorization header with Bearer token")
				}

				w.WriteHeader(tc.mockStatusCode)
				fmt.Fprintln(w, tc.mockResponse)
			}))
			defer mockServer.Close()

			blob, err := uploadBlob(mockServer.URL, "fake-token", tc.imageData, tc.mimeType, logger)

			if (err != nil) != tc.wantErr {
				t.Errorf("uploadBlob() error = %v, wantErr %v", err, tc.wantErr)
			}

			if !tc.wantErr && blob == nil {
				t.Error("uploadBlob() returned nil blob when expecting success")
			}

			if !tc.wantErr && blob != nil {
				if blob.Type != "blob" {
					t.Errorf("Expected blob type 'blob', got '%s'", blob.Type)
				}
				if blob.MimeType != tc.mimeType {
					t.Errorf("Expected mimeType %s, got %s", tc.mimeType, blob.MimeType)
				}
			}
		})
	}
}

