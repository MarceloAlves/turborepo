// Adapted from https://github.com/replit/upm
// Copyright (c) 2019 Neoreason d/b/a Repl.it. All rights reserved.
// SPDX-License-Identifier: MIT

package packagemanager

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vercel/turborepo/cli/internal/fs"
	"github.com/vercel/turborepo/cli/internal/globby"
	"github.com/vercel/turborepo/cli/internal/util"
)

// PackageManager is an abstraction across package managers
type PackageManager struct {
	// The descriptive name of the Package Manager.
	Name string

	// The unique identifier of the Package Manager.
	Slug string

	// The command used to invoke the Package Manager.
	Command string

	// The location of the package spec file used by the Package Manager.
	Specfile string

	// The location of the package lock file used by the Package Manager.
	Lockfile string

	// The directory in which package assets are stored by the Package Manager.
	PackageDir string

	// The separator that the Package Manger uses to identify arguments that
	// should be passed through to the underlying script.
	ArgSeparator []string

	// Return the list of workspace glob
	getWorkspaceGlobs func(rootpath fs.AbsolutePath) ([]string, error)

	// Return the list of workspace ignore globs
	getWorkspaceIgnores func(pm PackageManager, rootpath fs.AbsolutePath) ([]string, error)

	// Detect if Turbo knows how to produce a pruned workspace for the project
	canPrune func(cwd fs.AbsolutePath) (bool, error)

	// Test a manager and version tuple to see if it is the Package Manager.
	Matches func(manager string, version string) (bool, error)

	// Detect if the project is using the Package Manager by inspecting the system.
	detect func(projectDirectory fs.AbsolutePath, packageManager *PackageManager) (bool, error)
}

var packageManagers = []PackageManager{
	nodejsYarn,
	nodejsBerry,
	nodejsNpm,
	nodejsPnpm,
	nodejsPnpm6,
}

var (
	packageManagerPattern = `(npm|pnpm|yarn)@(\d+)\.\d+\.\d+(-.+)?`
	packageManagerRegex   = regexp.MustCompile(packageManagerPattern)
)

// ParsePackageManagerString takes a package manager version string parses it into consituent components
func ParsePackageManagerString(packageManager string) (manager string, version string, err error) {
	match := packageManagerRegex.FindString(packageManager)
	if len(match) == 0 {
		return "", "", fmt.Errorf("We could not parse packageManager field in package.json, expected: %s, received: %s", packageManagerPattern, packageManager)
	}

	return strings.Split(match, "@")[0], strings.Split(match, "@")[1], nil
}

// GetPackageManager attempts all methods for identifying the package manager in use.
func GetPackageManager(projectDirectory fs.AbsolutePath, pkg *fs.PackageJSON) (packageManager *PackageManager, err error) {
	result, _ := readPackageManager(pkg)
	if result != nil {
		return result, nil
	}

	return detectPackageManager(projectDirectory)
}

// readPackageManager attempts to read the package manager from the package.json.
func readPackageManager(pkg *fs.PackageJSON) (packageManager *PackageManager, err error) {
	if pkg.PackageManager != "" {
		manager, version, err := ParsePackageManagerString(pkg.PackageManager)
		if err != nil {
			return nil, err
		}

		for _, packageManager := range packageManagers {
			isResponsible, err := packageManager.Matches(manager, version)
			if isResponsible && (err == nil) {
				return &packageManager, nil
			}
		}
	}

	return nil, errors.New(util.Sprintf("We did not find a package manager specified in your root package.json. Please set the \"packageManager\" property in your root package.json (${UNDERLINE}https://nodejs.org/api/packages.html#packagemanager)${RESET} or run `npx @turbo/codemod add-package-manager` in the root of your monorepo."))
}

// detectPackageManager attempts to detect the package manager by inspecting the project directory state.
func detectPackageManager(projectDirectory fs.AbsolutePath) (packageManager *PackageManager, err error) {
	for _, packageManager := range packageManagers {
		isResponsible, err := packageManager.detect(projectDirectory, &packageManager)
		if err != nil {
			return nil, err
		}
		if isResponsible {
			return &packageManager, nil
		}
	}

	return nil, errors.New(util.Sprintf("We did not detect an in-use package manager for your project. Please set the \"packageManager\" property in your root package.json (${UNDERLINE}https://nodejs.org/api/packages.html#packagemanager)${RESET} or run `npx @turbo/codemod add-package-manager` in the root of your monorepo."))
}

// GetWorkspaces returns the list of package.json files for the current repository.
func (pm PackageManager) GetWorkspaces(rootpath fs.AbsolutePath) ([]string, error) {
	globs, err := pm.getWorkspaceGlobs(rootpath)
	if err != nil {
		return nil, err
	}

	justJsons := make([]string, len(globs))
	for i, space := range globs {
		justJsons[i] = filepath.Join(space, "package.json")
	}

	ignores, err := pm.getWorkspaceIgnores(pm, rootpath)
	if err != nil {
		return nil, err
	}

	f, err := globby.GlobFiles(rootpath.ToStringDuringMigration(), justJsons, ignores)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// GetWorkspaceIgnores returns an array of globs not to search for workspaces.
func (pm PackageManager) GetWorkspaceIgnores(rootpath fs.AbsolutePath) ([]string, error) {
	return pm.getWorkspaceIgnores(pm, rootpath)
}

// CanPrune returns if turbo can produce a pruned workspace. Can error if fs issues occur
func (pm PackageManager) CanPrune(projectDirectory fs.AbsolutePath) (bool, error) {
	if pm.canPrune != nil {
		return pm.canPrune(projectDirectory)
	}
	return false, nil
}
