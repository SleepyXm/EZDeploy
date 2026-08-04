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
	"strconv"
	"strings"
	"unicode/utf8"
)

const fallbackMaxFileSizeBytes int64 = 2 * 1024 * 1024

var methodTokenPattern = regexp.MustCompile(`[A-Za-z]+`)

type Rule struct {
	Name       string
	Languages  map[string]bool
	Extensions map[string]bool
	Files      []string
	Re         *regexp.Regexp
	NameIdx    int
}

type RouteRule struct {
	Name        string
	Languages   map[string]bool
	Re          *regexp.Regexp
	MethodIdx   int
	PathIdx     int
	ReceiverIdx int
	Multi       bool
}

type PrefixRule struct {
	Languages   map[string]bool
	Re          *regexp.Regexp
	VarIdx      int
	ReceiverIdx int
	PrefixIdx   int
}

type Scanner struct {
	cfg            *Config
	maxFileSize    int64
	languageByExt  map[string]string
	ignoredDirs    map[string]bool
	ignoredFiles   []string
	scanFiles      []string
	scanExtensions map[string]bool
	dockerfiles    []string
	rules          []Rule
	routeMethods   map[string]bool
	routeRules     []RouteRule
	prefixRules    []PrefixRule
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
		dockerfiles:    cfg.Dockerfiles,
		routeMethods:   map[string]bool{},
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

	for _, method := range cfg.RouteMethods {
		s.routeMethods[strings.ToUpper(method)] = true
	}
	methods := strings.Join(cfg.RouteMethods, "|")
	for _, def := range cfg.RouteRules {
		rule, err := compileRouteRule(def, methods)
		if err != nil {
			return nil, err
		}
		s.routeRules = append(s.routeRules, rule)
	}
	for _, def := range cfg.PrefixRules {
		rule, err := compilePrefixRule(def)
		if err != nil {
			return nil, err
		}
		s.prefixRules = append(s.prefixRules, rule)
	}

	return s, nil
}

func compileRouteRule(def RuleDef, methods string) (RouteRule, error) {
	// METHOD keeps the YAML readable while the configured method list remains authoritative.
	re, err := regexp.Compile(strings.ReplaceAll(def.Pattern, "METHOD", "(?i:"+methods+")"))
	if err != nil {
		return RouteRule{}, fmt.Errorf("compile route rule %q: %w", def.Name, err)
	}
	methodIdx, pathIdx := re.SubexpIndex("method"), re.SubexpIndex("path")
	if def.Name == "" || methodIdx < 0 || pathIdx < 0 {
		return RouteRule{}, fmt.Errorf("route rule %q requires method and path captures", def.Name)
	}
	return RouteRule{
		Name: def.Name, Languages: languageSet(def), Re: re,
		MethodIdx: methodIdx, PathIdx: pathIdx,
		ReceiverIdx: re.SubexpIndex("receiver"), Multi: def.Multi,
	}, nil
}

func compilePrefixRule(def RuleDef) (PrefixRule, error) {
	re, err := regexp.Compile(def.Pattern)
	if err != nil {
		return PrefixRule{}, fmt.Errorf("compile prefix rule %q: %w", def.Name, err)
	}
	varIdx, prefixIdx := re.SubexpIndex("var"), re.SubexpIndex("prefix")
	if varIdx < 0 || prefixIdx < 0 {
		return PrefixRule{}, fmt.Errorf("prefix rule %q requires var and prefix captures", def.Name)
	}
	return PrefixRule{languageSet(def), re, varIdx, re.SubexpIndex("receiver"), prefixIdx}, nil
}

func languageSet(def RuleDef) map[string]bool {
	set := map[string]bool{}
	if def.Language != "" {
		set[def.Language] = true
	}
	for _, language := range def.Languages {
		set[language] = true
	}
	return set
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
	seenRoutes := map[string]struct{}{}

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

		hits, routes, scanned, err := s.scanFile(absRoot, path, lang)
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
		for _, hit := range routes {
			key := fmt.Sprintf("%s:%d:%s:%s", hit.File, hit.Line, hit.Method, hit.Path)
			if _, exists := seenRoutes[key]; !exists {
				seenRoutes[key] = struct{}{}
				report.RouteHits = append(report.RouteHits, hit)
			}
		}
		if s.matchesDockerfile(base) {
			dockerfile, err := inspectDockerfile(absRoot, path)
			if err != nil {
				return err
			}
			report.Dockerfiles = append(report.Dockerfiles, dockerfile)
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
	sort.Slice(report.RouteHits, func(i, j int) bool {
		if report.RouteHits[i].File != report.RouteHits[j].File {
			return report.RouteHits[i].File < report.RouteHits[j].File
		}
		return report.RouteHits[i].Line < report.RouteHits[j].Line
	})
	sort.Slice(report.Dockerfiles, func(i, j int) bool {
		return report.Dockerfiles[i].Path < report.Dockerfiles[j].Path
	})

	return report, err
}

func inspectDockerfile(root, path string) (DockerfileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return DockerfileInfo{}, fmt.Errorf("open Dockerfile %s: %w", path, err)
	}
	defer file.Close()

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}
	info := DockerfileInfo{Path: filepath.ToSlash(relPath)}
	seenPorts := map[int]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			// Only metadata from the final runnable stage should drive deployment.
			info.ExposedPorts = nil
			seenPorts = map[int]bool{}
			index := 1
			for index < len(fields) && strings.HasPrefix(fields[index], "--") {
				index++
			}
			if index < len(fields) {
				// The last FROM is the image that will actually run.
				info.BaseImage = fields[index]
			}
		case "EXPOSE":
			for _, value := range fields[1:] {
				port, err := strconv.Atoi(strings.SplitN(value, "/", 2)[0])
				if err == nil && port > 0 && port <= 65535 && !seenPorts[port] {
					seenPorts[port] = true
					info.ExposedPorts = append(info.ExposedPorts, port)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return DockerfileInfo{}, fmt.Errorf("scan Dockerfile %s: %w", path, err)
	}
	sort.Ints(info.ExposedPorts)
	return info, nil
}

func (s *Scanner) scanFile(root, path, lang string) ([]EnvHit, []RouteHit, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, false, nil
	}

	if info.Size() > s.maxFileSize {
		return nil, nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false, nil
	}

	if !looksText(data) {
		return nil, nil, false, nil
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		relPath = path
	}

	relPath = filepath.ToSlash(relPath)

	var hits []EnvHit
	var routeHits []RouteHit
	prefixes := s.resolvePrefixes(data, lang)

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
		if routeCommentLine(line, lang) {
			continue
		}

		for _, rule := range s.routeRules {
			if len(rule.Languages) > 0 && !rule.Languages[lang] {
				continue
			}
			for _, match := range rule.Re.FindAllStringSubmatch(line, -1) {
				method, routePath := capture(match, rule.MethodIdx), capture(match, rule.PathIdx)
				if prefix := prefixes[capture(match, rule.ReceiverIdx)]; prefix != "" {
					routePath = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(routePath, "/")
				}
				for _, resolved := range s.expandMethods(method, rule.Multi) {
					routeHits = append(routeHits, RouteHit{
						Method: resolved, Path: normalizeRoute(routePath),
						File: relPath, Line: lineNo, Rule: rule.Name,
					})
				}
			}
		}
	}

	return hits, routeHits, true, nil
}

func (s *Scanner) resolvePrefixes(data []byte, lang string) map[string]string {
	type foundPrefix struct{ variable, receiver, prefix string }
	var found []foundPrefix
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if routeCommentLine(scanner.Text(), lang) {
			continue
		}
		for _, rule := range s.prefixRules {
			if len(rule.Languages) > 0 && !rule.Languages[lang] {
				continue
			}
			for _, match := range rule.Re.FindAllStringSubmatch(scanner.Text(), -1) {
				found = append(found, foundPrefix{capture(match, rule.VarIdx), capture(match, rule.ReceiverIdx), capture(match, rule.PrefixIdx)})
			}
		}
	}
	prefixes := map[string]string{}
	// Router groups may be nested, so resolve parent prefixes in bounded passes.
	for pass := 0; pass <= len(found); pass++ {
		for _, item := range found {
			if item.variable == item.receiver {
				continue
			}
			prefixes[item.variable] = normalizeRoute(prefixes[item.receiver] + item.prefix)
		}
	}
	return prefixes
}

// Ignore declarations disabled by the language's ordinary line-comment syntax.
func routeCommentLine(line, lang string) bool {
	line = strings.TrimSpace(line)
	if lang == "python" {
		return strings.HasPrefix(line, "#")
	}
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
}

func (s *Scanner) expandMethods(raw string, multi bool) []string {
	if !multi {
		method := strings.ToUpper(raw)
		if s.routeMethods[method] {
			return []string{method}
		}
		return nil
	}
	var methods []string
	for _, method := range methodTokenPattern.FindAllString(raw, -1) {
		method = strings.ToUpper(method)
		if s.routeMethods[method] {
			methods = append(methods, method)
		}
	}
	return methods
}

func capture(match []string, index int) string {
	if index < 0 || index >= len(match) {
		return ""
	}
	return strings.TrimSpace(match[index])
}

func normalizeRoute(route string) string {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	return route
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
	if s.matchesDockerfile(base) {
		return true
	}
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

func (s *Scanner) matchesDockerfile(base string) bool {
	for _, pattern := range s.dockerfiles {
		if matchFilePattern(strings.ToLower(base), strings.ToLower(pattern)) {
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
