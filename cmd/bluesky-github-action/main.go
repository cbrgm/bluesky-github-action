package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/alexflint/go-arg"
)

// Global variables for application metadata, including version, revision,
// Go runtime version, and application start time for diagnostic purposes.
var (
	Version   string              // Current application version.
	Revision  string              // Git commit hash of the application build.
	GoVersion = runtime.Version() // Go runtime version used to build the application.
	StartTime = time.Now()        // Application start time.
)

// SessionResponse holds authentication session information after a successful login.
type SessionResponse struct {
	AccessToken string `json:"accessJwt"` // JWT access token.
	UserID      string `json:"did"`       // User identifier.
}

// Post represents a message to be published to the server.
type Post struct {
	Type      string          `json:"$type"`            // Type of the post.
	Text      string          `json:"text"`             // Text content of the post.
	CreatedAt string          `json:"createdAt"`        // ISO 8601 timestamp of post creation.
	Langs     []string        `json:"langs,omitempty"`  // Optional languages the post supports.
	Facets    []RichTextFacet `json:"facets,omitempty"` // Rich text facets for links, mentions, hashtags.
	Embed     interface{}     `json:"embed,omitempty"`  // Embed can be EmbedExternal or EmbedImages.
}

// ActionInputs aggregates command line arguments and environment variables for application configuration.
type ActionInputs struct {
	PDSURL        string   `arg:"--pds-url" env:"ATP_PDS_HOST" default:"https://bsky.social"` // Base URL of the PDS service.
	Handle        string   `arg:"--handle,required" env:"ATP_AUTH_HANDLE"`                    // User handle for authentication.
	Password      string   `arg:"--password,required" env:"ATP_AUTH_PASSWORD"`                // Password for authentication.
	Text          string   `arg:"--text,required" env:"BSKY_MESSAGE"`                         // Text content for the new post.
	Lang          []string `arg:"--lang" env:"BSKY_LANG"`                                     // Languages for the new post.
	LogLevel      string   `arg:"--log-level" env:"LOG_LEVEL" default:"info"`                 // Logging level.
	EnableEmbeds  bool     `arg:"--enable-embeds" env:"BSKY_ENABLE_EMBEDS" default:"true"`    // Enable link card embeds.
	ImagePaths    string   `arg:"--image-paths" env:"BSKY_IMAGE_PATHS"`                       // Comma-separated image file paths.
	ImageAltTexts string   `arg:"--image-alt-texts" env:"BSKY_IMAGE_ALT_TEXTS"`               // Comma-separated alt texts for images.
	VideoPath     string   `arg:"--video-path" env:"BSKY_VIDEO_PATH"`                         // Video file path.
	VideoAltText  string   `arg:"--video-alt-text" env:"BSKY_VIDEO_ALT_TEXT"`                 // Alt text for video.
}

// createSession initiates a new session with the PDS service.
// nolint: errcheck
func createSession(pdsURL, handle, password string) (*SessionResponse, error) {
	loginURL := fmt.Sprintf("%s/xrpc/com.atproto.server.createSession", pdsURL)
	requestBody, err := json.Marshal(map[string]string{
		"identifier": handle,
		"password":   password,
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(loginURL, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create session, status code: %d", resp.StatusCode)
	}

	var sessionResponse SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResponse); err != nil {
		return nil, err
	}

	return &sessionResponse, nil
}

// publishPost submits a new post to the PDS service using the provided session.
// nolint: errcheck
func publishPost(pdsURL string, session *SessionResponse, post *Post, logger *slog.Logger) error {
	postURL := fmt.Sprintf("%s/xrpc/com.atproto.repo.createRecord", pdsURL)
	postData, err := json.Marshal(map[string]interface{}{
		"repo":       session.UserID,
		"collection": "app.bsky.feed.post",
		"record":     post,
	})
	if err != nil {
		logger.Error("Error marshaling post data", "err", err)
		return err
	}

	request, err := http.NewRequest("POST", postURL, bytes.NewBuffer(postData))
	if err != nil {
		logger.Error("Error creating new request", "err", err)
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		logger.Error("Error sending request", "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to publish post", "statusCode", resp.StatusCode, "body", string(body))
		return fmt.Errorf("failed to publish post, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// uploadBlob uploads a blob (image or small file) to the PDS service.
// nolint: errcheck
func uploadBlob(pdsURL, accessToken string, data []byte, mimeType string, logger *slog.Logger) (*Blob, error) {
	uploadURL := fmt.Sprintf("%s/xrpc/com.atproto.repo.uploadBlob", pdsURL)

	request, err := http.NewRequest("POST", uploadURL, bytes.NewBuffer(data))
	if err != nil {
		logger.Error("Error creating blob upload request", "err", err)
		return nil, err
	}

	request.Header.Set("Content-Type", mimeType)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		logger.Error("Error uploading blob", "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to upload blob", "statusCode", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("failed to upload blob, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var blobResp struct {
		Blob Blob `json:"blob"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&blobResp); err != nil {
		logger.Error("Error decoding blob response", "err", err)
		return nil, err
	}

	return &blobResp.Blob, nil
}

// stringToLogLevel converts a string representation of a log level to its slog.Level counterpart.
func stringToLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogger configures and returns a new logger based on the provided log level.
func setupLogger(level string) *slog.Logger {
	logLevel := stringToLogLevel(level)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	return slog.New(handler)
}

func main() {
	var args ActionInputs
	arg.MustParse(&args)

	logger := setupLogger(args.LogLevel)

	logger.Info("Starting session creation")
	session, err := createSession(args.PDSURL, args.Handle, args.Password)
	if err != nil {
		logger.Error("Error creating session", "err", err)
		os.Exit(1)
	}

	logger.Debug("Session created successfully", "userID", session.UserID)

	// Parse rich text facets from the text
	facets := parseRichTextFacets(args.Text)

	// Determine which embed to use (priority: video > images > link cards)
	var embed interface{}

	// Process video if provided (takes priority)
	if args.VideoPath != "" {
		logger.Info("Processing video for upload")
		videoEmbed, err := processVideos(args.PDSURL, session.AccessToken, session.UserID, args.VideoPath, args.VideoAltText, logger)
		if err != nil {
			logger.Error("Error processing video", "err", err)
			os.Exit(1)
		}
		embed = videoEmbed
		logger.Info("Video processed successfully")
	} else if args.ImagePaths != "" {
		// Process images if no video provided
		logger.Info("Processing images for upload")
		imageEmbed, err := processImages(args.PDSURL, session.AccessToken, args.ImagePaths, args.ImageAltTexts, logger)
		if err != nil {
			logger.Error("Error processing images", "err", err)
			os.Exit(1)
		}
		embed = imageEmbed
		logger.Info("Images processed successfully", "count", len(imageEmbed.Images))
	} else if args.EnableEmbeds && len(facets) > 0 {
		// Create embed for the first URL if embeds are enabled and no media provided
		firstURL := facets[0].Features[0].URI
		logger.Debug("Fetching embed metadata", "url", firstURL)
		embed = fetchLinkMetadata(firstURL, logger)
	}

	post := &Post{
		Type:      "app.bsky.feed.post",
		Text:      args.Text,
		CreatedAt: time.Now().Format(time.RFC3339),
		Langs:     args.Lang,
		Facets:    facets,
		Embed:     embed,
	}

	if err := publishPost(args.PDSURL, session, post, logger); err != nil {
		logger.Error("Error publishing post", "err", err)
		os.Exit(1)
	}

	logger.Info("Post published successfully")
}
