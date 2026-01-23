package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Constants for image upload constraints.
const (
	maxImagesPerPost = 4
	maxImageSize     = 1000000 // 1MB in bytes
)

// detectImageMimeType detects the MIME type based on file extension.
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

// validateImageData validates image file size and format.
func validateImageData(path string, imageData []byte) error {
	if len(imageData) > maxImageSize {
		return fmt.Errorf("image %s exceeds maximum size of %d bytes (got %d bytes)", path, maxImageSize, len(imageData))
	}

	mimeType := detectImageMimeType(path)
	if mimeType == "" {
		return fmt.Errorf("unsupported image format for file %s (supported: JPEG, PNG, GIF, WebP)", path)
	}

	return nil
}

// getImageDimensions extracts image dimensions for aspect ratio calculation.
func getImageDimensions(imageData []byte, logger *slog.Logger) *AspectRatio {
	config, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		logger.Debug("Could not determine image dimensions", "err", err)
		return nil
	}

	logger.Debug("Image dimensions", "width", config.Width, "height", config.Height)
	return &AspectRatio{
		Width:  config.Width,
		Height: config.Height,
	}
}

// resolveAltText determines the appropriate alt text for an image.
func resolveAltText(index int, altTexts []string) string {
	// If we have a specific alt text for this index, use it
	if index < len(altTexts) && strings.TrimSpace(altTexts[index]) != "" {
		return strings.TrimSpace(altTexts[index])
	}

	// If only one alt text provided, use it for all images
	if len(altTexts) == 1 && strings.TrimSpace(altTexts[0]) != "" {
		return strings.TrimSpace(altTexts[0])
	}

	// Default alt text
	return fmt.Sprintf("Image %d", index+1)
}

// parseImagePaths splits and trims image paths from comma-separated input.
func parseImagePaths(imagePaths string) []string {
	if imagePaths == "" {
		return nil
	}

	paths := strings.Split(imagePaths, ",")
	var validPaths []string
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed != "" {
			validPaths = append(validPaths, trimmed)
		}
	}
	return validPaths
}

// processImage processes a single image file: reads, validates, uploads, and creates an embed.
func processImage(pdsURL, accessToken, path, altText string, logger *slog.Logger) (*EmbedImage, error) {
	logger.Debug("Processing image", "path", path, "alt", altText)

	// Read image file
	imageData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file %s: %w", path, err)
	}

	// Validate image
	if err := validateImageData(path, imageData); err != nil {
		return nil, err
	}

	mimeType := detectImageMimeType(path)
	logger.Debug("Uploading image blob", "path", path, "size", len(imageData), "mimeType", mimeType)

	// Upload blob
	blob, err := uploadBlob(pdsURL, accessToken, imageData, mimeType, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to upload image %s: %w", path, err)
	}

	// Get image dimensions
	aspectRatio := getImageDimensions(imageData, logger)

	return &EmbedImage{
		Alt:         altText,
		Image:       *blob,
		AspectRatio: aspectRatio,
	}, nil
}

// processImages reads image files, uploads them as blobs, and creates an EmbedImages structure.
func processImages(pdsURL, accessToken, imagePaths, altTexts string, logger *slog.Logger) (*EmbedImages, error) {
	paths := parseImagePaths(imagePaths)
	if len(paths) == 0 {
		return nil, nil
	}

	// Validate image count
	if len(paths) > maxImagesPerPost {
		return nil, fmt.Errorf("maximum %d images allowed per post, got %d", maxImagesPerPost, len(paths))
	}

	alts := strings.Split(altTexts, ",")
	var embedImages []EmbedImage

	for i, path := range paths {
		altText := resolveAltText(i, alts)

		embedImage, err := processImage(pdsURL, accessToken, path, altText, logger)
		if err != nil {
			return nil, err
		}

		embedImages = append(embedImages, *embedImage)
	}

	return &EmbedImages{
		Type:   "app.bsky.embed.images",
		Images: embedImages,
	}, nil
}
