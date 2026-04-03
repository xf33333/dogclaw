package constants

// API Limits - server-side limits enforced by the Anthropic API.
// Last verified: 2025-12-22

// Image Limits
const (
	// APIImageMaxBase64Size is the maximum base64-encoded image size (API enforced).
	// The API rejects images where the base64 string length exceeds this value.
	// Note: This is the base64 length, NOT raw bytes. Base64 increases size by ~33%.
	APIImageMaxBase64Size = 5 * 1024 * 1024 // 5 MB

	// ImageTargetRawSize is the target raw image size to stay under base64 limit after encoding.
	// Base64 encoding increases size by 4/3, so we derive the max raw size:
	// raw_size * 4/3 = base64_size → raw_size = base64_size * 3/4
	ImageTargetRawSize = (APIImageMaxBase64Size * 3) / 4 // 3.75 MB

	// ImageMaxWidth is the client-side maximum width for image resizing.
	ImageMaxWidth = 2000

	// ImageMaxHeight is the client-side maximum height for image resizing.
	ImageMaxHeight = 2000
)

// PDF Limits
const (
	// PDFTargetRawSize is the maximum raw PDF file size that fits within the API request limit after encoding.
	// The API has a 32MB total request size limit. Base64 encoding increases size by ~33%.
	PDFTargetRawSize = 20 * 1024 * 1024 // 20 MB

	// APIPDFMaxPages is the maximum number of pages in a PDF accepted by the API.
	APIPDFMaxPages = 100

	// PDFExtractSizeThreshold is the size threshold above which PDFs are extracted into page images.
	// This applies to first-party API only; non-first-party always uses extraction.
	PDFExtractSizeThreshold = 3 * 1024 * 1024 // 3 MB

	// PDFMaxExtractSize is the maximum PDF file size for the page extraction path.
	PDFMaxExtractSize = 100 * 1024 * 1024 // 100 MB

	// PDFMaxPagesPerRead is the max pages the Read tool will extract in a single call.
	PDFMaxPagesPerRead = 20

	// PDFAtMentionInlineThreshold is the page count above which PDFs get the reference treatment
	// on @ mention instead of being inlined into context.
	PDFAtMentionInlineThreshold = 10
)

// Media Limits
const (
	// APIMaxMediaPerRequest is the maximum number of media items (images + PDFs) per API request.
	APIMaxMediaPerRequest = 100
)
