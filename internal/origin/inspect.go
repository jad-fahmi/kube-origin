package origin

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Options controls a local kubeconfig inspection. Inspect does not make network
// requests and does not execute configured credential plugins.
type Options struct {
	Context    string
	Kubeconfig string
}

// Report contains only non-secret effective settings and their source entries.
type Report struct {
	Version                string          `json:"version"`
	Context                string          `json:"context"`
	ContextSelection       string          `json:"context_selection"`
	ContextSelectionSource string          `json:"context_selection_source,omitempty"`
	ContextSource          string          `json:"context_source"`
	Cluster                string          `json:"cluster"`
	ClusterSource          string          `json:"cluster_source,omitempty"`
	Server                 string          `json:"server,omitempty"`
	ServerSource           string          `json:"server_source,omitempty"`
	Namespace              string          `json:"namespace"`
	NamespaceSource        string          `json:"namespace_source,omitempty"`
	NamespaceDefault       bool            `json:"namespace_is_default"`
	User                   string          `json:"user"`
	UserSource             string          `json:"user_source,omitempty"`
	Authentication         Authentication  `json:"authentication"`
	Inputs                 []InputFile     `json:"inputs"`
	Shadowed               []ShadowedEntry `json:"shadowed_entries"`
	Warnings               []string        `json:"warnings,omitempty"`
}

type Authentication struct {
	Methods []string `json:"methods"`
	Exec    string   `json:"exec,omitempty"`
}

type InputFile struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type ShadowedEntry struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	WinningSource string `json:"winning_source"`
	IgnoredSource string `json:"ignored_source"`
}

// Inspect uses client-go's loading rules as the authority for the selected
// configuration, then reads each input independently to explain shadowed map
// entries. No credentials are serialized into Report.
func Inspect(options Options) (Report, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if options.Kubeconfig != "" {
		rules.ExplicitPath = options.Kubeconfig
		rules.Precedence = []string{options.Kubeconfig}
	}
	config, err := rules.Load()
	if err != nil {
		return Report{}, fmt.Errorf("load kubeconfig: %w", err)
	}

	contextName := options.Context
	if contextName == "" {
		contextName = config.CurrentContext
	}
	if contextName == "" {
		return Report{}, errors.New("no context selected; set current-context or pass --context")
	}
	context, ok := config.Contexts[contextName]
	if !ok || context == nil {
		return Report{}, fmt.Errorf("context %q was not found", contextName)
	}

	report := Report{
		Version:          "1",
		Context:          contextName,
		ContextSelection: "current-context",
		Namespace:        context.Namespace,
		Cluster:          context.Cluster,
		User:             context.AuthInfo,
		Inputs:           make([]InputFile, 0, len(rules.Precedence)),
		Shadowed:         make([]ShadowedEntry, 0),
	}
	if options.Context != "" {
		report.ContextSelection = "--context flag"
	}
	if report.Namespace == "" {
		report.Namespace = "default"
		report.NamespaceDefault = true
	} else {
		report.NamespaceSource = sourcePath(context.LocationOfOrigin)
	}
	report.ContextSource = sourcePath(context.LocationOfOrigin)

	if cluster, ok := config.Clusters[context.Cluster]; ok && cluster != nil {
		report.ClusterSource = sourcePath(cluster.LocationOfOrigin)
		report.Server = safeServer(cluster.Server)
		report.ServerSource = sourcePath(cluster.LocationOfOrigin)
	} else if context.Cluster != "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("context references missing cluster %q", context.Cluster))
	}
	if user, ok := config.AuthInfos[context.AuthInfo]; ok && user != nil {
		report.UserSource = sourcePath(user.LocationOfOrigin)
		report.Authentication = authenticationFor(user)
	} else if context.AuthInfo != "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("context references missing user %q", context.AuthInfo))
	}

	shadowed, inputs, currentContextSource, err := inspectInputs(rules.Precedence)
	if err != nil {
		return Report{}, err
	}
	report.Inputs = inputs
	report.Shadowed = shadowed
	if options.Context == "" {
		report.ContextSelectionSource = currentContextSource
	}
	sort.Slice(report.Warnings, func(i, j int) bool { return report.Warnings[i] < report.Warnings[j] })
	return report, nil
}

func sourcePath(path string) string {
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(absolute)
}

func safeServer(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[configured; URL omitted because it is malformed]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func authenticationFor(user *clientcmdapi.AuthInfo) Authentication {
	methods := make([]string, 0, 5)
	var execCommand string
	if user.Exec != nil {
		methods = append(methods, "exec plugin")
		execCommand = filepath.Base(strings.ReplaceAll(user.Exec.Command, "\\", "/"))
		if execCommand == "." || execCommand == string(filepath.Separator) {
			execCommand = "configured"
		}
	}
	if user.AuthProvider != nil {
		methods = append(methods, "auth provider")
	}
	if user.Token != "" || user.TokenFile != "" {
		methods = append(methods, "token")
	}
	if user.Username != "" || user.Password != "" {
		methods = append(methods, "basic auth")
	}
	if user.ClientCertificate != "" || len(user.ClientCertificateData) != 0 {
		methods = append(methods, "client certificate")
	}
	if user.ClientKey != "" || len(user.ClientKeyData) != 0 {
		methods = append(methods, "client key")
	}
	if user.Impersonate != "" || len(user.ImpersonateGroups) != 0 || user.ImpersonateUID != "" || len(user.ImpersonateUserExtra) != 0 {
		methods = append(methods, "impersonation")
	}
	if len(methods) == 0 {
		methods = append(methods, "none configured")
	}
	sort.Strings(methods)
	return Authentication{Methods: methods, Exec: execCommand}
}

func inspectInputs(paths []string) ([]ShadowedEntry, []InputFile, string, error) {
	type origin struct {
		path string
	}
	type kindMap struct {
		name string
		get  func(*clientcmdapi.Config) map[string]string
	}
	kinds := []kindMap{
		{name: "context", get: func(c *clientcmdapi.Config) map[string]string { return names(c.Contexts) }},
		{name: "cluster", get: func(c *clientcmdapi.Config) map[string]string { return names(c.Clusters) }},
		{name: "user", get: func(c *clientcmdapi.Config) map[string]string { return names(c.AuthInfos) }},
	}
	seen := map[string]map[string]origin{
		"context": {}, "cluster": {}, "user": {},
	}
	shadowed := make([]ShadowedEntry, 0)
	inputs := make([]InputFile, 0, len(paths))
	visited := make(map[string]bool)
	currentContextSource := ""

	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, "", fmt.Errorf("resolve kubeconfig path %q: %w", path, err)
		}
		absolute = filepath.Clean(absolute)
		key := absolute
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if visited[key] {
			continue
		}
		visited[key] = true
		_, statErr := os.Stat(absolute)
		if os.IsNotExist(statErr) {
			inputs = append(inputs, InputFile{Path: absolute, Exists: false})
			continue
		}
		if statErr != nil {
			return nil, nil, "", fmt.Errorf("inspect kubeconfig %q: %w", absolute, statErr)
		}
		source, err := clientcmd.LoadFromFile(absolute)
		if err != nil {
			return nil, nil, "", fmt.Errorf("read kubeconfig %q: %w", absolute, err)
		}
		inputs = append(inputs, InputFile{Path: absolute, Exists: true})
		if currentContextSource == "" && source.CurrentContext != "" {
			currentContextSource = absolute
		}
		for _, kind := range kinds {
			for _, name := range sortedKeys(kind.get(source)) {
				if first, ok := seen[kind.name][name]; ok {
					shadowed = append(shadowed, ShadowedEntry{
						Kind: kind.name, Name: name,
						WinningSource: first.path, IgnoredSource: absolute,
					})
				} else {
					seen[kind.name][name] = origin{path: absolute}
				}
			}
		}
	}
	sort.Slice(shadowed, func(i, j int) bool {
		a, b := shadowed[i], shadowed[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.IgnoredSource < b.IgnoredSource
	})
	return shadowed, inputs, currentContextSource, nil
}

func names[T any](values map[string]*T) map[string]string {
	result := make(map[string]string, len(values))
	for name := range values {
		result[name] = name
	}
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
