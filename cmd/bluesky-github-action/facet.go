package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RichTextFacet represents a rich text formatting facet.
type RichTextFacet struct {
	Index    RichTextIndex     `json:"index"`
	Features []RichTextFeature `json:"features"`
}

// RichTextIndex represents the byte range for a rich text facet.
type RichTextIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

// RichTextFeature represents a rich text feature (link, mention, hashtag).
type RichTextFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri,omitempty"`
}

// EmbedExternal represents an external link embed.
type EmbedExternal struct {
	Type     string               `json:"$type"`
	External EmbedExternalContent `json:"external"`
}

// EmbedExternalContent represents the content of an external embed.
type EmbedExternalContent struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// parseRichTextFacets extracts URLs from text and creates rich text facets.
func parseRichTextFacets(text string) []RichTextFacet {
	var facets []RichTextFacet

	// URL regex pattern based on standard URL matching
	urlRegex := regexp.MustCompile(`https?://[^\s<>"'{}|\\\^` + "`" + `\[\]]+`)

	// Find all URL matches
	matches := urlRegex.FindAllString(text, -1)
	matchIndices := urlRegex.FindAllStringIndex(text, -1)

	for i, match := range matches {
		// Clean trailing punctuation that's not part of the URL
		cleanMatch := strings.TrimRight(match, ".,!?;:")

		// Validate URL
		if _, err := url.Parse(cleanMatch); err != nil {
			continue
		}

		// Get byte positions for the original match
		start := matchIndices[i][0]
		originalEnd := matchIndices[i][1]

		// Adjust end position if we trimmed punctuation
		trimmedLength := len(match) - len(cleanMatch)
		end := originalEnd - trimmedLength

		// Convert string positions to byte positions
		byteStart := len([]byte(text[:start]))
		byteEnd := len([]byte(text[:end]))

		facet := RichTextFacet{
			Index: RichTextIndex{
				ByteStart: byteStart,
				ByteEnd:   byteEnd,
			},
			Features: []RichTextFeature{
				{
					Type: "app.bsky.richtext.facet#link",
					URI:  cleanMatch,
				},
			},
		}

		facets = append(facets, facet)
	}

	return facets
}

// fetchLinkMetadata fetches metadata for a URL to create link embeds.
func fetchLinkMetadata(url string, logger *slog.Logger) *EmbedExternal {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		logger.Debug("Failed to fetch URL for embed", "url", url, "err", err)
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("Failed to close response body", "err", err)
		}
	}()

	// Only process HTML content
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		logger.Debug("Skipping non-HTML URL for embed", "url", url, "contentType", contentType)
		return nil
	}

	// Read and parse basic metadata
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Debug("Failed to read response body for embed", "url", url, "err", err)
		return nil
	}

	html := string(body)

	// Extract title
	title := extractMetaContent(html, `<title[^>]*>([^<]*)</title>`)
	if title == "" {
		title = extractMetaContent(html, `<meta[^>]*property="og:title"[^>]*content="([^"]*)"`)
	}

	// Extract description
	description := extractMetaContent(html, `<meta[^>]*property="og:description"[^>]*content="([^"]*)"`)
	if description == "" {
		description = extractMetaContent(html, `<meta[^>]*name="description"[^>]*content="([^"]*)"`)
	}

	// Only create embed if we have at least a title
	if title == "" {
		logger.Debug("No title found for embed", "url", url)
		return nil
	}

	// Truncate title and description to reasonable lengths
	if len(title) > 100 {
		title = title[:97] + "..."
	}
	if len(description) > 200 {
		description = description[:197] + "..."
	}

	return &EmbedExternal{
		Type: "app.bsky.embed.external",
		External: EmbedExternalContent{
			URI:         url,
			Title:       title,
			Description: description,
		},
	}
}

// extractMetaContent extracts content from HTML using regex.
func extractMetaContent(html, pattern string) string {
	re := regexp.MustCompile(`(?i)` + pattern)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}