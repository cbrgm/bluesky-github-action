package main

import (
	"testing"
)

func TestParseRichTextFacets(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int // expected number of facets
	}{
		{
			name:     "single URL",
			text:     "Check out https://github.com/cbrgm/bluesky-github-action",
			expected: 1,
		},
		{
			name:     "multiple URLs",
			text:     "Visit https://github.com and https://google.com for more info",
			expected: 2,
		},
		{
			name:     "no URLs",
			text:     "This is just plain text without any links",
			expected: 0,
		},
		{
			name:     "URL at the beginning",
			text:     "https://example.com is a great website",
			expected: 1,
		},
		{
			name:     "URL at the end",
			text:     "Visit this site: https://example.com",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facets := parseRichTextFacets(tt.text)
			if len(facets) != tt.expected {
				t.Errorf("parseRichTextFacets() = %v facets, want %v", len(facets), tt.expected)
			}

			// Verify facet structure for URLs found
			for i, facet := range facets {
				if facet.Features[0].Type != "app.bsky.richtext.facet#link" {
					t.Errorf("Facet %d type = %v, want app.bsky.richtext.facet#link", i, facet.Features[0].Type)
				}
				if facet.Features[0].URI == "" {
					t.Errorf("Facet %d URI is empty", i)
				}
				if facet.Index.ByteStart < 0 || facet.Index.ByteEnd <= facet.Index.ByteStart {
					t.Errorf("Facet %d has invalid byte range: start=%d, end=%d", i, facet.Index.ByteStart, facet.Index.ByteEnd)
				}
			}
		})
	}
}

func TestExtractMetaContent(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		pattern  string
		expected string
	}{
		{
			name:     "extract title",
			html:     "<title>Test Page Title</title>",
			pattern:  `<title[^>]*>([^<]*)</title>`,
			expected: "Test Page Title",
		},
		{
			name:     "extract og:title",
			html:     `<meta property="og:title" content="Open Graph Title">`,
			pattern:  `<meta[^>]*property="og:title"[^>]*content="([^"]*)"`,
			expected: "Open Graph Title",
		},
		{
			name:     "no match",
			html:     "<div>No title here</div>",
			pattern:  `<title[^>]*>([^<]*)</title>`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMetaContent(tt.html, tt.pattern)
			if result != tt.expected {
				t.Errorf("extractMetaContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}