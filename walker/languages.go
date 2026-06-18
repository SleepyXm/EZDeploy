package walker

import (
	"path/filepath"
	"strings"
)

var extLanguage = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".jsx":  "javascript",
	".mjs":  "javascript",
	".cjs":  "javascript",
	".ts":   "typescript",
	".tsx":  "typescript",
	".rs":   "rust",
	".rb":   "ruby",
	".java": "java",
	".kt":   "kotlin",
	".kts":  "kotlin",
	".php":  "php",
	".cs":   "csharp",
	".sh":   "shell",
	".bash": "shell",
	".zsh":  "shell",
}

func LanguageForPath(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	lang, ok := extLanguage[ext]
	return lang, ok
}
