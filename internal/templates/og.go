package templates

// OGMeta holds Open Graph metadata for social sharing
type OGMeta struct {
	Title       string // og:title - defaults to page title if empty
	Description string // og:description
	URL         string // og:url - canonical URL
	Image       string // og:image - defaults to /static/og-image.png if empty
	Type        string // og:type - defaults to "website" if empty
}

// DefaultOGImage is the default Open Graph image path
const DefaultOGImage = "/static/og-image.png"

// DefaultOGType is the default Open Graph type
const DefaultOGType = "website"
