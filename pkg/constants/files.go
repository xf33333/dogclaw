package constants

import (
	"path/filepath"
	"strings"
)

// Binary file extensions to skip for text-based operations.
var BinaryExtensions = map[string]struct{}{
	// Images
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".ico": {},
	".webp": {}, ".tiff": {}, ".tif": {},
	// Videos
	".mp4": {}, ".mov": {}, ".avi": {}, ".mkv": {}, ".webm": {}, ".wmv": {},
	".flv": {}, ".m4v": {}, ".mpeg": {}, ".mpg": {},
	// Audio
	".mp3": {}, ".wav": {}, ".ogg": {}, ".flac": {}, ".aac": {}, ".m4a": {},
	".wma": {}, ".aiff": {}, ".opus": {},
	// Archives
	".zip": {}, ".tar": {}, ".gz": {}, ".bz2": {}, ".7z": {}, ".rar": {},
	".xz": {}, ".z": {}, ".tgz": {}, ".iso": {},
	// Executables/binaries
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {}, ".bin": {}, ".o": {},
	".a": {}, ".obj": {}, ".lib": {}, ".app": {}, ".msi": {}, ".deb": {},
	".rpm": {},
	// Documents
	".pdf": {}, ".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {}, ".ppt": {},
	".pptx": {}, ".odt": {}, ".ods": {}, ".odp": {},
	// Fonts
	".ttf": {}, ".otf": {}, ".woff": {}, ".woff2": {}, ".eot": {},
	// Bytecode / VM artifacts
	".pyc": {}, ".pyo": {}, ".class": {}, ".jar": {}, ".war": {}, ".ear": {},
	".node": {}, ".wasm": {}, ".rlib": {},
	// Database files
	".sqlite": {}, ".sqlite3": {}, ".db": {}, ".mdb": {}, ".idx": {},
	// Design / 3D
	".psd": {}, ".ai": {}, ".eps": {}, ".sketch": {}, ".fig": {}, ".xd": {},
	".blend": {}, ".3ds": {}, ".max": {},
	// Flash
	".swf": {}, ".fla": {},
	// Lock/profiling data
	".lockb": {}, ".dat": {}, ".data": {},
}

// HasBinaryExtension checks if a file path has a binary extension.
func HasBinaryExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	_, ok := BinaryExtensions[ext]
	return ok
}

// BinaryCheckSize is the number of bytes to read for binary content detection.
const BinaryCheckSize = 8192

// IsBinaryContent checks if a buffer contains binary content by looking for null bytes
// or a high proportion of non-printable characters.
func IsBinaryContent(buffer []byte) bool {
	checkSize := len(buffer)
	if checkSize > BinaryCheckSize {
		checkSize = BinaryCheckSize
	}

	nonPrintable := 0
	for i := 0; i < checkSize; i++ {
		b := buffer[i]
		// Null byte is a strong indicator of binary
		if b == 0 {
			return true
		}
		// Count non-printable, non-whitespace bytes
		if b < 32 && b != 9 && b != 10 && b != 13 {
			nonPrintable++
		}
	}

	// If more than 10% non-printable, likely binary
	return float64(nonPrintable)/float64(checkSize) > 0.1
}
