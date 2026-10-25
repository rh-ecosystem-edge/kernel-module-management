/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kmodparser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:generate mockgen -source=parse.go -package=kmodparser -destination=mock_parser.go

// Parser reads modules.dep and modules.softdep.
type Parser interface {
	Parse(req Request) (Success, error)
}

type parser struct{}

// NewParser returns a Parser that reads the modules tree on disk.
func NewParser() Parser {
	return &parser{}
}

var (
	// ErrMissingModulesDep is returned when modules.dep is not in the modules root.
	ErrMissingModulesDep = errors.New("missing_modules.dep")
	// ErrInvalidModulesDep is returned when modules.dep has a line without a colon.
	ErrInvalidModulesDep = errors.New("invalid_modules.dep")
	// ErrUnreadableModulesDep is returned when modules.dep cannot be read.
	ErrUnreadableModulesDep = errors.New("unreadable_modules.dep")
	// ErrInvalidModulesSoftdep is returned when a modules.softdep line is malformed.
	ErrInvalidModulesSoftdep = errors.New("invalid_modules.softdep")
	// ErrUnreadableModulesSoftdep is returned when modules.softdep exists but cannot be read.
	ErrUnreadableModulesSoftdep = errors.New("unreadable_modules.softdep")
	// ErrModuleNotInModulesDep is returned when a requested module has no modules.dep line.
	ErrModuleNotInModulesDep = errors.New("module_not_in_modules_dep")
	// ErrAmbiguousModule is returned when two modules.dep lines share a module name.
	ErrAmbiguousModule = errors.New("ambiguous_module")
	// ErrMissingModulesRoot is returned when the modules root is empty.
	ErrMissingModulesRoot = errors.New("missing_modules_root")
)

// Request is the input for a modules dependency walk.
type Request struct {
	ModuleNames []string
	Omit        []string
	ModulesRoot string
}

// Success is the termination-log body after a successful walk.
// InTreeModules is the only field. The body has no link records.
type Success struct {
	InTreeModules []string `json:"inTreeModules"`
}

// ErrorBody is the termination-log body after a failed walk.
type ErrorBody struct {
	Error string `json:"error"`
}

type depLine struct {
	path string
	deps []string
}

// Parse reads modules.dep and modules.softdep and returns in-tree module names.
func (*parser) Parse(req Request) (Success, error) {
	if req.ModulesRoot == "" {
		return Success{}, ErrMissingModulesRoot
	}

	lines, err := readDep(req.ModulesRoot)
	if err != nil {
		return Success{}, err
	}
	for _, name := range req.ModuleNames {
		if _, ok := lines[moduleName(name)]; !ok {
			return Success{}, ErrModuleNotInModulesDep
		}
	}

	outOfTree, err := outOfTreeNames(req.ModulesRoot, lines)
	if err != nil {
		return Success{}, err
	}
	names := hardDepNames(lines)
	if err := addSoftDepNames(req.ModulesRoot, outOfTree, names); err != nil {
		return Success{}, err
	}

	return Success{InTreeModules: sortedNames(names, req.ModuleNames, req.Omit)}, nil
}

func readDep(root string) (map[string]depLine, error) {
	data, err := os.ReadFile(filepath.Join(root, "modules.dep"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMissingModulesDep
		}
		return nil, ErrUnreadableModulesDep
	}

	lines := map[string]depLine{}
	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		modPath, depText, ok := strings.Cut(raw, ":")
		if !ok {
			return nil, ErrInvalidModulesDep
		}
		name := moduleName(modPath)
		if name == "" {
			return nil, ErrInvalidModulesDep
		}
		if _, exists := lines[name]; exists {
			return nil, ErrAmbiguousModule
		}
		var deps []string
		if fields := strings.Fields(depText); len(fields) > 0 {
			deps = fields
		}
		lines[name] = depLine{path: strings.TrimSpace(modPath), deps: deps}
	}
	return lines, nil
}

func outOfTreeNames(root string, lines map[string]depLine) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := moduleName(path)
		if !isModuleFile(path) {
			return nil
		}
		names[name] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, ErrUnreadableModulesDep
	}
	for _, line := range lines {
		addPathName(names, line.path)
		for _, dep := range line.deps {
			addPathName(names, dep)
		}
	}
	return names, nil
}

func addPathName(names map[string]struct{}, modulePath string) {
	if isHostPath(modulePath) {
		return
	}
	name := moduleName(modulePath)
	if name == "" {
		return
	}
	names[name] = struct{}{}
}

func hardDepNames(lines map[string]depLine) map[string]struct{} {
	names := map[string]struct{}{}
	for _, line := range lines {
		if isHostPath(line.path) {
			continue
		}
		for _, dep := range line.deps {
			if !isHostPath(dep) {
				continue
			}
			name := moduleName(dep)
			if name == "" {
				continue
			}
			names[name] = struct{}{}
		}
	}
	return names
}

func addSoftDepNames(root string, outOfTree, names map[string]struct{}) error {
	data, err := os.ReadFile(filepath.Join(root, "modules.softdep"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ErrUnreadableModulesSoftdep
	}
	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 || fields[0] != "softdep" || fields[1] == "pre:" || fields[1] == "post:" {
			return ErrInvalidModulesSoftdep
		}
		var deps []string
		for _, tok := range fields[2:] {
			if tok == "pre:" || tok == "post:" {
				continue
			}
			deps = append(deps, tok)
		}
		if _, ok := outOfTree[moduleName(fields[1])]; !ok {
			continue
		}
		for _, dep := range deps {
			name := moduleName(dep)
			if _, ok := outOfTree[name]; ok || name == "" {
				continue
			}
			names[name] = struct{}{}
		}
	}
	return nil
}

func sortedNames(names map[string]struct{}, moduleNames, omit []string) []string {
	skip := map[string]struct{}{}
	for _, name := range moduleNames {
		skip[moduleName(name)] = struct{}{}
	}
	for _, name := range omit {
		skip[moduleName(name)] = struct{}{}
	}
	list := make([]string, 0, len(names))
	for name := range names {
		if _, excluded := skip[name]; excluded || name == "" {
			continue
		}
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

func moduleName(pathOrName string) string {
	base := strings.TrimSpace(pathOrName)
	if strings.Contains(base, "/") {
		base = filepath.Base(base)
	}
	switch {
	case strings.HasSuffix(base, ".ko.xz"):
		base = strings.TrimSuffix(base, ".ko.xz")
	case strings.HasSuffix(base, ".ko.gz"):
		base = strings.TrimSuffix(base, ".ko.gz")
	case strings.HasSuffix(base, ".ko"):
		base = strings.TrimSuffix(base, ".ko")
	}
	return strings.ReplaceAll(base, "-", "_")
}

func isModuleFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".ko") || strings.HasSuffix(base, ".ko.xz") || strings.HasSuffix(base, ".ko.gz")
}

func isHostPath(modulePath string) bool {
	cleaned := strings.TrimPrefix(strings.TrimSpace(modulePath), "./")
	cleaned = strings.Trim(cleaned, "/")
	first, _, _ := strings.Cut(cleaned, "/")
	return first == "host"
}
