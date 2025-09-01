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
