package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Constants for video upload constraints.
const (
	videoServiceURL      = "https://video.bsky.app"
	maxVideoSize         = 50 * 1024 * 1024 // 50MB in bytes (reasonable default)
	videoStatusPollDelay = 2 * time.Second
	videoStatusMaxWait   = 5 * time.Minute
)

// ServiceAuthResponse represents the response from getServiceAuth.
type ServiceAuthResponse struct {
	Token string `json:"token"`
}

// VideoUploadResponse represents the response from video upload.
type VideoUploadResponse struct {
	JobID  string          `json:"jobId"`
	Status *VideoJobStatus `json:"jobStatus,omitempty"`
}

// VideoJobStatus represents the status of a video processing job.
type VideoJobStatus struct {
	JobID    string `json:"jobId"`
	State    string `json:"state"`
	Progress int    `json:"progress,omitempty"`
	Blob     *Blob  `json:"blob,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// detectVideoMimeType detects the MIME type based on file extension.
func detectVideoMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		return ""
	}
}

// validateVideoData validates video file size and format.
func validateVideoData(path string, videoData []byte) error {
	if len(videoData) > maxVideoSize {
		return fmt.Errorf("video %s exceeds maximum size of %d bytes (got %d bytes)", path, maxVideoSize, len(videoData))
	}

	mimeType := detectVideoMimeType(path)
	if mimeType == "" {
		return fmt.Errorf("unsupported video format for file %s (supported: MP4, MOV, WebM)", path)
	}

	return nil
}

// getServiceAuthToken creates a service authentication token for video upload.
// nolint: errcheck
func getServiceAuthToken(pdsURL, accessToken, userDID string, logger *slog.Logger) (string, error) {
	// Extract host from video service URL for audience
	videoURL, err := url.Parse(videoServiceURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse video service URL: %w", err)
	}

	audience := fmt.Sprintf("did:web:%s", videoURL.Host)
	expiryTime := time.Now().Unix() + 1800 // 30 minutes

	authURL := fmt.Sprintf("%s/xrpc/com.atproto.server.getServiceAuth", pdsURL)

	reqBody := map[string]interface{}{
		"aud": audience,
		"lxm": "com.atproto.repo.uploadBlob",
		"exp": expiryTime,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequest("POST", authURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		logger.Error("Error creating service auth request", "err", err)
		return "", err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		logger.Error("Error getting service auth token", "err", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to get service auth token", "statusCode", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("failed to get service auth token, status code: %d", resp.StatusCode)
	}

	var authResp ServiceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		logger.Error("Error decoding service auth response", "err", err)
		return "", err
	}

	return authResp.Token, nil
}

// uploadVideoToService uploads a video to the Bluesky video service.
// nolint: errcheck
func uploadVideoToService(userDID, serviceToken string, videoData []byte, filename, mimeType string, logger *slog.Logger) (*VideoUploadResponse, error) {
	uploadURL := fmt.Sprintf("%s/xrpc/app.bsky.video.uploadVideo?did=%s&name=%s",
		videoServiceURL,
		url.QueryEscape(userDID),
		url.QueryEscape(filename),
	)

	request, err := http.NewRequest("POST", uploadURL, bytes.NewBuffer(videoData))
	if err != nil {
		logger.Error("Error creating video upload request", "err", err)
		return nil, err
	}

	request.Header.Set("Content-Type", mimeType)
	request.Header.Set("Content-Length", fmt.Sprintf("%d", len(videoData)))
	request.Header.Set("Authorization", "Bearer "+serviceToken)

	client := &http.Client{
		Timeout: 5 * time.Minute, // Long timeout for large video uploads
	}
	resp, err := client.Do(request)
	if err != nil {
		logger.Error("Error uploading video", "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("Failed to upload video", "statusCode", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("failed to upload video, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var uploadResp VideoUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		logger.Error("Error decoding video upload response", "err", err)
		return nil, err
	}

	return &uploadResp, nil
}

// getVideoJobStatus polls for the video processing job status.
// nolint: errcheck
func getVideoJobStatus(serviceToken, jobID string, logger *slog.Logger) (*VideoJobStatus, error) {
	statusURL := fmt.Sprintf("%s/xrpc/app.bsky.video.getJobStatus?jobId=%s",
		videoServiceURL,
		url.QueryEscape(jobID),
	)

	request, err := http.NewRequest("GET", statusURL, nil)
	if err != nil {
		logger.Error("Error creating job status request", "err", err)
		return nil, err
	}

	request.Header.Set("Authorization", "Bearer "+serviceToken)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		logger.Error("Error getting job status", "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Check for "already_exists" error which means video was already processed
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error   string          `json:"error"`
			Message string          `json:"message"`
			Status  *VideoJobStatus `json:"jobStatus,omitempty"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errorResp.Error == "already_exists" && errorResp.Status != nil && errorResp.Status.Blob != nil {
				logger.Debug("Video already processed", "jobId", jobID)
				return errorResp.Status, nil
			}
		}
		logger.Error("Failed to get job status", "statusCode", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("failed to get job status, status code: %d", resp.StatusCode)
	}

	var statusResp struct {
		JobStatus VideoJobStatus `json:"jobStatus"`
	}
	if err := json.Unmarshal(body, &statusResp); err != nil {
		logger.Error("Error decoding job status response", "err", err)
		return nil, err
	}

	return &statusResp.JobStatus, nil
}

// pollVideoJobUntilComplete polls the video job status until it's complete or times out.
func pollVideoJobUntilComplete(serviceToken, jobID string, logger *slog.Logger) (*Blob, error) {
	startTime := time.Now()

	for {
		// Check timeout
		if time.Since(startTime) > videoStatusMaxWait {
			return nil, fmt.Errorf("video processing timed out after %v", videoStatusMaxWait)
		}

		status, err := getVideoJobStatus(serviceToken, jobID, logger)
		if err != nil {
			return nil, err
		}

		logger.Debug("Video processing status",
			"jobId", jobID,
			"state", status.State,
			"progress", status.Progress,
		)

		// Check if blob is ready
		if status.Blob != nil {
			logger.Info("Video processing complete", "jobId", jobID)
			return status.Blob, nil
		}

		// Check for error state
		if status.State == "failed" || status.Error != "" {
			return nil, fmt.Errorf("video processing failed: %s", status.Error)
		}

		// Wait before next poll
		time.Sleep(videoStatusPollDelay)
	}
}

// processVideo processes a single video file: reads, validates, uploads, and creates an embed.
func processVideo(pdsURL, accessToken, userDID, path, altText string, logger *slog.Logger) (*EmbedVideo, error) {
	logger.Info("Processing video", "path", path)

	// Read video file
	videoData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read video file %s: %w", path, err)
	}

	// Validate video
	if err := validateVideoData(path, videoData); err != nil {
		return nil, err
	}

	mimeType := detectVideoMimeType(path)
	filename := filepath.Base(path)

	logger.Info("Getting service auth token for video upload")

	// Get service auth token
	serviceToken, err := getServiceAuthToken(pdsURL, accessToken, userDID, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get service auth token: %w", err)
	}

	logger.Info("Uploading video to service", "size", len(videoData), "mimeType", mimeType)

	// Upload video
	uploadResp, err := uploadVideoToService(userDID, serviceToken, videoData, filename, mimeType, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}

	// Check if blob is immediately available (already processed)
	if uploadResp.Status != nil && uploadResp.Status.Blob != nil {
		logger.Info("Video already processed, using existing blob")
		return &EmbedVideo{
			Type:  "app.bsky.embed.video",
			Video: *uploadResp.Status.Blob,
			Alt:   altText,
		}, nil
	}

	logger.Info("Video uploaded, waiting for processing", "jobId", uploadResp.JobID)

	// Poll for processing completion
	blob, err := pollVideoJobUntilComplete(serviceToken, uploadResp.JobID, logger)
	if err != nil {
		return nil, fmt.Errorf("video processing failed: %w", err)
	}

	return &EmbedVideo{
		Type:  "app.bsky.embed.video",
		Video: *blob,
		Alt:   altText,
	}, nil
}

// processVideos processes video file and creates an EmbedVideo structure.
func processVideos(pdsURL, accessToken, userDID, videoPath, altText string, logger *slog.Logger) (*EmbedVideo, error) {
	if videoPath == "" {
		return nil, nil
	}

	path := strings.TrimSpace(videoPath)
	if path == "" {
		return nil, nil
	}

	// Default alt text if not provided
	if altText == "" {
		altText = "Video"
	}

	return processVideo(pdsURL, accessToken, userDID, path, strings.TrimSpace(altText), logger)
}
