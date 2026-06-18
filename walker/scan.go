package walker

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const fallbackMaxFileSizeBytes int64 = 2 * 1024 * 1024

type Rule struct {
	Name       string
	Languages  map[string]bool
	Extensions map[string]bool
	Files      []string
	Re         *regexp.Regexp
	NameIdx    int
}

type Scanner struct {
	cfg            *Config
	maxFileSize    int64
	languageByExt  map[string]string
	ignoredDirs    map[string]bool
	ignoredFiles   []string
	scanFiles      []string
	scanExtensions map[string]bool
	rules          []Rule
}

func ScanDefault(root string) (Report, error) {
	cfg, err := LoadDefaultConfig()
	if err != nil {
		return Report{}, err
	}

	scanner, err := NewScanner(cfg)
	if err != nil {
		return Report{}, err
	}

	return scanner.Scan(root)
}

func Scan(root string, configPath string) (Report, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return Report{}, err
	}

	scanner, err := NewScanner(cfg)
	if err != nil {
		return Report{}, err
	}

	return scanner.Scan(root)
}

func NewScanner(cfg *Config) (*Scanner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil repoenv config")
	}

	s := &Scanner{
		cfg:            cfg,
		maxFileSize:    cfg.MaxFileSizeBytes,
		languageByExt:  map[string]string{},
		ignoredDirs:    map[string]bool{},
		ignoredFiles:   cfg.IgnoredFiles,
		scanFiles:      cfg.ScanFiles,
		scanExtensions: map[string]bool{},
	}

	if s.maxFileSize <= 0 {
		s.maxFileSize = fallbackMaxFileSizeBytes
	}

	for _, lang := range cfg.Languages {
		if lang.Name == "" {
			continue
		}

		for _, ext := range lang.Extensions {
			s.languageByExt[normalizeExt(ext)] = lang.Name
		}
	}

	for _, dir := range cfg.IgnoredDirs {
		if dir == "" {
			continue
		}

		s.ignoredDirs[dir] = true
	}

	for _, ext := range cfg.ScanExtensions {
		s.scanExtensions[normalizeExt(ext)] = true
	}

	for _, def := range cfg.EnvRules {
		rule, err := compileRule(def)
		if err != nil {
			return nil, err
		}

		s.rules = append(s.rules, rule)
	}

	return s, nil
}

func compileRule(def RuleDef) (Rule, error) {
	if def.Name == "" {
		return Rule{}, fmt.Errorf("env rule missing name")
	}

	if def.Pattern == "" {
		return Rule{}, fmt.Errorf("env rule %q missing pattern", def.Name)
	}

	re, err := regexp.Compile(def.Pattern)
	if err != nil {
		return Rule{}, fmt.Errorf("compile env rule %q: %w", def.Name, err)
	}

	nameIdx := re.SubexpIndex("name")
	if nameIdx < 0 {
		return Rule{}, fmt.Errorf("env rule %q must expose (?P<name>...)", def.Name)
	}

	langs := map[string]bool{}
	if def.Language != "" {
		langs[def.Language] = true
	}
	for _, lang := range def.Languages {
		if lang != "" {
			langs[lang] = true
		}
	}

	exts := map[string]bool{}
	for _, ext := range def.Extensions {
		exts[normalizeExt(ext)] = true
	}

	return Rule{
		Name:       def.Name,
		Languages:  langs,
		Extensions: exts,
		Files:      def.Files,
		Re:         re,
		NameIdx:    nameIdx,
	}, nil
}

func (s *Scanner) Scan(root string) (Report, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	report := Report{
		Root:      absRoot,
		Languages: map[string]int{},
	}

	seenHits := map[string]struct{}{}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if path != absRoot && s.ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		base := filepath.Base(path)
		if s.matchesIgnoredFile(base) {
			return nil
		}

		lang := s.languageForPath(path)
		if lang != "" {
			report.Languages[lang]++
		}

		if !s.shouldScan(path, lang) {
			return nil
		}

		hits, scanned, err := s.scanFile(absRoot, path, lang)
		if err != nil {
			return nil
		}

		if scanned {
			report.FilesScanned++
		}

		for _, hit := range hits {
			key := fmt.Sprintf("%s:%d:%s:%s", hit.Path, hit.Line, hit.Name, hit.Rule)
			if _, exists := seenHits[key]; exists {
				continue
			}

			seenHits[key] = struct{}{}
			report.EnvHits = append(report.EnvHits, hit)
		}

		return nil
	})

	sort.Slice(report.EnvHits, func(i, j int) bool {
		a := report.EnvHits[i]
		b := report.EnvHits[j]

		if a.Path != b.Path {
			return a.Path < b.Path
		}

		if a.Line != b.Line {
			return a.Line < b.Line
		}

		return a.Name < b.Name
	})

	return report, err
}

func (s *Scanner) scanFile(root, path, lang string) ([]EnvHit, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, nil
	}

	if info.Size() > s.maxFileSize {
		return nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, nil
	}

	if !looksText(data) {
		return nil, false, nil
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}

	relPath = filepath.ToSlash(relPath)

	var hits []EnvHit

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		for _, rule := range s.rules {
			if !rule.applies(path, lang) {
				continue
			}

			matches := rule.Re.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) <= rule.NameIdx {
					continue
				}

				name := strings.TrimSpace(match[rule.NameIdx])
				if !validEnvName(name) {
					continue
				}

				hits = append(hits, EnvHit{
					Name:     name,
					Language: lang,
					Path:     relPath,
					Line:     lineNo,
					Rule:     rule.Name,
				})
			}
		}
	}

	return hits, true, nil
}

func (s *Scanner) shouldScan(path string, lang string) bool {
	if lang != "" {
		return true
	}

	ext := normalizeExt(filepath.Ext(path))
	if s.scanExtensions[ext] {
		return true
	}

	base := filepath.Base(path)
	if matchesAnyFilePattern(base, s.scanFiles) {
		return true
	}

	// Rules can also make files scannable.
	// This means YAML selectors drive the scanner instead of hardcoded Go switches.
	for _, rule := range s.rules {
		if rule.Extensions[ext] {
			return true
		}

		if matchesAnyFilePattern(base, rule.Files) {
			return true
		}
	}

	return false
}

func (s *Scanner) languageForPath(path string) string {
	ext := normalizeExt(filepath.Ext(path))
	return s.languageByExt[ext]
}

func (s *Scanner) matchesIgnoredFile(base string) bool {
	return matchesAnyFilePattern(base, s.ignoredFiles)
}

func (r Rule) applies(path string, lang string) bool {
	ext := normalizeExt(filepath.Ext(path))
	base := filepath.Base(path)

	hasSelector := false

	if len(r.Languages) > 0 {
		hasSelector = true
		if r.Languages[lang] {
			return true
		}
	}

	if len(r.Extensions) > 0 {
		hasSelector = true
		if r.Extensions[ext] {
			return true
		}
	}

	if len(r.Files) > 0 {
		hasSelector = true
		if matchesAnyFilePattern(base, r.Files) {
			return true
		}
	}

	return !hasSelector
}

func matchesAnyFilePattern(base string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchFilePattern(base, pattern) {
			return true
		}
	}

	return false
}

func matchFilePattern(base, pattern string) bool {
	if pattern == "" {
		return false
	}

	matched, err := filepath.Match(pattern, base)
	if err == nil && matched {
		return true
	}

	return base == pattern
}

func looksText(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}

	return utf8.Valid(data)
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		if i == 0 {
			return r == '_' || isAlpha(r)
		}

		if r != '_' && !isAlpha(r) && !isDigit(r) {
			return false
		}
	}

	return true
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(strings.ToLower(ext))
	if ext == "" {
		return ""
	}

	if !strings.HasPrefix(ext, ".") {
		return "." + ext
	}

	return ext
}
