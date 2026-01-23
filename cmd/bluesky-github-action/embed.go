package main

// Embed types and structures for Bluesky posts

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

// BlobRef represents a reference to a blob.
type BlobRef struct {
	Link string `json:"$link"`
}

// Blob represents an uploaded blob (image or video).
type Blob struct {
	Type     string  `json:"$type"`
	Ref      BlobRef `json:"ref"`
	MimeType string  `json:"mimeType"`
	Size     int     `json:"size"`
}

// AspectRatio represents the aspect ratio of media content.
type AspectRatio struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// EmbedImage represents a single image in an embed.
type EmbedImage struct {
	Alt         string       `json:"alt"`
	Image       Blob         `json:"image"`
	AspectRatio *AspectRatio `json:"aspectRatio,omitempty"`
}

// EmbedImages represents an image embed with multiple images.
type EmbedImages struct {
	Type   string       `json:"$type"`
	Images []EmbedImage `json:"images"`
}

// EmbedVideo represents a video embed.
type EmbedVideo struct {
	Type        string       `json:"$type"`
	Video       Blob         `json:"video"`
	AspectRatio *AspectRatio `json:"aspectRatio,omitempty"`
	Alt         string       `json:"alt,omitempty"`
	Captions    []Caption    `json:"captions,omitempty"`
}

// Caption represents a video caption/subtitle.
type Caption struct {
	Lang string `json:"lang"`
	File Blob   `json:"file"`
}
