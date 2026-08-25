package walker

import (
	"bufio"
	"bytes"
	"encoding/json"
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
var pythonAppPattern = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(FastAPI|Flask)\s*\(`)
var goRouterFunctionPattern = regexp.MustCompile(`(?m)func\s+(\w+)\s*\(\s*\w+\s+\*gin\.RouterGroup\b`)
var goRouterCallPattern = regexp.MustCompile("(?m)(?:\\w+\\.)?(\\w+)\\s*\\(\\s*(\\w+)(?:\\.Group\\(\\s*[\"`](/[^\"`\\s]*)[\"`]\\s*\\))?")

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
	serviceRules   []ServiceRuleDef
	rules          []Rule
	routeMethods   map[string]bool
	routeRules     []RouteRule
	prefixRules    []PrefixRule
}

type serviceEntry struct {
	path  string
	rules []ServiceRuleDef
}

func ScanDefault(root string) (Report, error) {
	cfg, err := LoadConfig("")
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
		serviceRules:   cfg.ServiceRules,
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

	exts := map[string]bool{}
	for _, ext := range def.Extensions {
		exts[normalizeExt(ext)] = true
	}

	return Rule{
		Name:       def.Name,
		Languages:  languageSet(def),
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
	goSources := map[string][]byte{}
	manifestDirs := map[string]map[string]bool{}
	var serviceEntries []serviceEntry

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
		if matchesAnyFilePattern(base, s.ignoredFiles) {
			return nil
		}
		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)
		if s.matchesServiceManifest(base) {
			dir := filepath.ToSlash(filepath.Dir(relPath))
			if manifestDirs[dir] == nil {
				manifestDirs[dir] = map[string]bool{}
			}
			manifestDirs[dir][base] = true
		}

		lang := s.languageByExt[normalizeExt(filepath.Ext(path))]
		if lang != "" {
			report.Languages[lang]++
		}
		if rules := s.matchingServiceRules(base); len(rules) > 0 {
			serviceEntries = append(serviceEntries, serviceEntry{path: relPath, rules: rules})
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
			if lang == "go" {
				goSources[relPath], _ = os.ReadFile(path)
			}
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
	if err == nil {
		report.RouteHits = s.resolveMountedGoRoutes(report.RouteHits, goSources)
		report.Services = s.discoverServices(absRoot, serviceEntries, manifestDirs)
	}

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
	sort.Slice(report.Services, func(i, j int) bool {
		if report.Services[i].Root != report.Services[j].Root {
			return report.Services[i].Root < report.Services[j].Root
		}
		if report.Services[i].Runtime != report.Services[j].Runtime {
			return report.Services[i].Runtime < report.Services[j].Runtime
		}
		return report.Services[i].Entry < report.Services[j].Entry
	})

	return report, err
}

type goRouterFunction struct {
	name, file string
	start, end int
}

// resolveMountedGoRoutes connects registration calls such as
// RegisterAuthRoutes(api.Group("/auth")) to routes declared in another file.
func (s *Scanner) resolveMountedGoRoutes(hits []RouteHit, sources map[string][]byte) []RouteHit {
	definitions, byFile := map[string][]goRouterFunction{}, map[string][]goRouterFunction{}
	for file, data := range sources {
		matches := goRouterFunctionPattern.FindAllSubmatchIndex(data, -1)
		for index, match := range matches {
			end := int(^uint(0) >> 1)
			if index+1 < len(matches) {
				end = 1 + bytes.Count(data[:matches[index+1][0]], []byte{'\n'})
			}
			fn := goRouterFunction{name: string(data[match[2]:match[3]]), file: file,
				start: 1 + bytes.Count(data[:match[0]], []byte{'\n'}), end: end}
			definitions[fn.name], byFile[file] = append(definitions[fn.name], fn), append(byFile[file], fn)
		}
	}
	mounts := map[string]map[string]bool{}
	for _, data := range sources {
		prefixes := s.resolvePrefixes(data, "go")
		for _, match := range goRouterCallPattern.FindAllStringSubmatch(string(data), -1) {
			name, prefix := match[1], prefixes[match[2]]
			if len(definitions[name]) != 1 {
				continue // Ambiguous function names stay unmodified instead of receiving a guessed prefix.
			}
			if match[3] != "" {
				prefix = normalizeRoute(prefix + match[3])
			}
			if prefix == "" || prefix == "/" {
				continue
			}
			if mounts[name] == nil {
				mounts[name] = map[string]bool{}
			}
			mounts[name][prefix] = true
		}
	}
	resolved := make([]RouteHit, 0, len(hits))
	for _, hit := range hits {
		mounted := false
		for _, fn := range byFile[hit.File] {
			if hit.Line < fn.start || hit.Line >= fn.end {
				continue
			}
			for prefix := range mounts[fn.name] {
				copy := hit
				if hit.MountRoot && hit.Path == "/" {
					copy.Path = prefix
				} else {
					copy.Path = normalizeRoute(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(hit.Path, "/"))
				}
				resolved, mounted = append(resolved, copy), true
			}
			break
		}
		if !mounted {
			resolved = append(resolved, hit)
		}
	}
	return resolved
}

func (s *Scanner) matchingServiceRules(base string) []ServiceRuleDef {
	var matches []ServiceRuleDef
	for _, rule := range s.serviceRules {
		if matchesAnyFilePattern(base, rule.Files) {
			matches = append(matches, rule)
		}
	}
	return matches
}

func (s *Scanner) matchesServiceManifest(base string) bool {
	for _, rule := range s.serviceRules {
		if matchesAnyFilePattern(base, rule.Manifests) {
			return true
		}
	}
	return false
}

func (s *Scanner) discoverServices(root string, entries []serviceEntry, manifestDirs map[string]map[string]bool) []ServiceCandidate {
	services := map[string]ServiceCandidate{}
	for _, entry := range entries {
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.path)))
		if int64(len(data)) > s.maxFileSize {
			data = nil
		}
		for _, rule := range entry.rules {
			serviceRoot, manifest := nearestServiceManifest(entry.path, rule.Manifests, manifestDirs)
			markers := serviceMarkers(data, rule.Markers, rule.Runtime)
			pkg := nodePackage{}
			if rule.Runtime == "node" && manifest == "package.json" {
				pkg = readNodePackage(root, serviceRoot)
			}

			candidate := ServiceCandidate{
				Name:         serviceName(root, serviceRoot, entry.path, pkg.Name),
				Runtime:      rule.Runtime,
				Root:         serviceRoot,
				Entry:        entry.path,
				StartCommand: serviceStartCommand(rule.Runtime, serviceRoot, entry.path, data, markers, pkg),
				Confidence:   serviceConfidence(manifest != "", len(markers) > 0),
				Evidence:     []string{"entry " + entry.path},
			}
			if manifest != "" {
				candidate.Evidence = append(candidate.Evidence, "manifest "+manifest)
			}
			for _, marker := range markers {
				candidate.Evidence = append(candidate.Evidence, "marker "+marker)
			}
			if strings.TrimSpace(pkg.Scripts["start"]) != "" {
				candidate.Evidence = append(candidate.Evidence, `package.json script "start"`)
			}

			// Python and Node projects normally have one web entrypoint per
			// manifest. Go modules may contain several cmd/* binaries, so their
			// entry directory remains part of the candidate identity.
			key := rule.Runtime + ":" + serviceRoot
			if rule.Runtime == "go" {
				key += ":" + filepath.ToSlash(filepath.Dir(entry.path))
			}
			current, exists := services[key]
			if !exists || confidenceRank(candidate.Confidence) > confidenceRank(current.Confidence) {
				services[key] = candidate
			}
		}
	}

	result := make([]ServiceCandidate, 0, len(services))
	for _, service := range services {
		result = append(result, service)
	}
	return result
}

func nearestServiceManifest(entry string, patterns []string, manifests map[string]map[string]bool) (string, string) {
	dir := filepath.ToSlash(filepath.Dir(entry))
	for {
		// Rule order decides which manifest is reported when a service has
		// several (for example pyproject.toml and requirements.txt).
		for _, pattern := range patterns {
			for name := range manifests[dir] {
				if matchFilePattern(name, pattern) {
					return dir, name
				}
			}
		}
		if dir == "." {
			break
		}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.ToSlash(filepath.Dir(entry)), ""
}

func serviceMarkers(data []byte, markers []string, runtime string) []string {
	found := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || routeCommentLine(line, runtime) {
			continue
		}
		lowerLine := strings.ToLower(line)
		for _, marker := range markers {
			if !found[marker] && strings.Contains(lowerLine, strings.ToLower(marker)) {
				found[marker] = true
			}
		}
	}
	result := make([]string, 0, len(found))
	for _, marker := range markers {
		if found[marker] {
			result = append(result, marker)
		}
	}
	return result
}

type nodePackage struct {
	Name    string            `json:"name"`
	Scripts map[string]string `json:"scripts"`
}

func readNodePackage(root, serviceRoot string) nodePackage {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(serviceRoot), "package.json"))
	if err != nil {
		return nodePackage{}
	}
	var pkg nodePackage
	_ = json.Unmarshal(data, &pkg)
	return pkg
}

func serviceName(repoRoot, serviceRoot, entry, packageName string) string {
	if strings.TrimSpace(packageName) != "" {
		return packageName
	}
	entryDir := filepath.ToSlash(filepath.Dir(entry))
	if entryDir != "." && entryDir != serviceRoot {
		return filepath.Base(entryDir)
	}
	if serviceRoot != "." {
		return filepath.Base(serviceRoot)
	}
	return filepath.Base(repoRoot)
}

func serviceStartCommand(runtime, root, entry string, data []byte, markers []string, pkg nodePackage) string {
	relEntry, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(entry))
	if err != nil {
		relEntry = entry
	}
	relEntry = filepath.ToSlash(relEntry)

	switch runtime {
	case "go":
		dir := filepath.ToSlash(filepath.Dir(relEntry))
		if dir == "." {
			return "go run ."
		}
		return "go run ./" + strings.TrimPrefix(dir, "./")
	case "node":
		if strings.TrimSpace(pkg.Scripts["start"]) != "" {
			return "npm start"
		}
		if ext := strings.ToLower(filepath.Ext(relEntry)); ext != ".ts" {
			return "node " + relEntry
		}
	case "python":
		module := strings.TrimSuffix(strings.ReplaceAll(relEntry, "/", "."), filepath.Ext(relEntry))
		appName := "app"
		if match := pythonAppPattern.FindSubmatch(data); len(match) > 1 {
			appName = string(match[1])
		}
		if markerPresent(markers, "FastAPI(") {
			return fmt.Sprintf(`.venv/bin/python -m uvicorn %s:%s --host 127.0.0.1 --port "$PORT"`, module, appName)
		}
		if markerPresent(markers, "Flask(") {
			return fmt.Sprintf(`.venv/bin/python -m flask --app %s:%s run --host 127.0.0.1 --port "$PORT"`, module, appName)
		}
		if strings.Contains(string(data), `if __name__`) {
			return filepath.ToSlash(filepath.Join(".venv", "bin", "python")) + " " + relEntry
		}
	}
	return ""
}

func markerPresent(markers []string, wanted string) bool {
	for _, marker := range markers {
		if strings.EqualFold(marker, wanted) {
			return true
		}
	}
	return false
}

func serviceConfidence(hasManifest, hasMarker bool) string {
	if hasManifest && hasMarker {
		return "high"
	}
	if hasManifest || hasMarker {
		return "medium"
	}
	return "low"
}

func confidenceRank(value string) int {
	switch value {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
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
				mountRoot := routePath == ""
				if prefix := prefixes[capture(match, rule.ReceiverIdx)]; prefix != "" {
					if mountRoot {
						routePath = prefix
					} else {
						routePath = strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(routePath, "/")
					}
				}
				for _, resolved := range s.expandMethods(method, rule.Multi) {
					routeHits = append(routeHits, RouteHit{
						Method: resolved, Path: normalizeRoute(routePath),
						File: relPath, Line: lineNo, Rule: rule.Name, MountRoot: mountRoot,
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
