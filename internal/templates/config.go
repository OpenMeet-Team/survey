package templates

// NoIndex controls whether search engines should index pages.
// Default is true (block indexing). Set to false in production to allow indexing.
var NoIndex = true

// SetNoIndex sets the noindex configuration.
// Call this at startup based on environment configuration.
func SetNoIndex(val bool) {
	NoIndex = val
}
