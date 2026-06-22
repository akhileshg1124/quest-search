package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Unable to find user home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".quest")
}

func printUsage() {
	fmt.Println(`Quest - A lightweight user-space package manager for Windows (and macOS)

Usage:
  quest <command> [arguments]

Commands:
  install <package>    Install a package
  uninstall <package>  Uninstall a package
  upgrade <package>    Upgrade a package to the latest version
  list                 List installed packages
  search <query>       Search for packages in the registry
  info <package>       Show details about a package
  update               Update package manifests

Options:
  -os string           Override operating system (darwin, windows)
  -arch string         Override architecture (amd64, arm64)

Examples:
  quest search jq
  quest install jq
  quest list
  quest info ripgrep
  quest uninstall jq`)
}

func main() {
	osFlag := flag.String("os", runtime.GOOS, "Target operating system")
	archFlag := flag.String("arch", runtime.GOARCH, "Target architecture")

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	flagArgs := os.Args[2:]

	fs := flag.NewFlagSet(command, flag.ExitOnError)
	fs.StringVar(osFlag, "os", runtime.GOOS, "Target operating system")
	fs.StringVar(archFlag, "arch", runtime.GOARCH, "Target architecture")
	
	var cmdArgs []string
	for i := 0; i < len(flagArgs); i++ {
		arg := flagArgs[i]
		if strings.HasPrefix(arg, "-") {
			err := fs.Parse(flagArgs[i:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
				os.Exit(1)
			}
			break
		} else {
			cmdArgs = append(cmdArgs, arg)
		}
	}

	homeDir := getHomeDir()

	for _, dir := range []string{"bin", "apps", "cache", "manifests", "receipts"} {
		if err := os.MkdirAll(filepath.Join(homeDir, dir), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing directory structure: %v\n", err)
			os.Exit(1)
		}
	}

	switch command {
	case "install":
		if len(cmdArgs) < 1 {
			fmt.Println("Error: Please specify a package name to install.")
			fmt.Println("Usage: quest install <package>")
			os.Exit(1)
		}
		pkgName := cmdArgs[0]
		err := handleInstall(pkgName, *osFlag, *archFlag, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error installing package: %v\n", err)
			os.Exit(1)
		}

	case "uninstall":
		if len(cmdArgs) < 1 {
			fmt.Println("Error: Please specify a package name to uninstall.")
			fmt.Println("Usage: quest uninstall <package>")
			os.Exit(1)
		}
		pkgName := cmdArgs[0]
		err := handleUninstall(pkgName, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error uninstalling package: %v\n", err)
			os.Exit(1)
		}

	case "upgrade":
		if len(cmdArgs) < 1 {
			fmt.Println("Error: Please specify a package name to upgrade.")
			fmt.Println("Usage: quest upgrade <package>")
			os.Exit(1)
		}
		pkgName := cmdArgs[0]
		err := handleUpgrade(pkgName, *osFlag, *archFlag, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading package: %v\n", err)
			os.Exit(1)
		}

	case "list":
		err := handleList(homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing packages: %v\n", err)
			os.Exit(1)
		}

	case "search":
		query := ""
		if len(cmdArgs) > 0 {
			query = cmdArgs[0]
		}
		err := handleSearch(query, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error searching packages: %v\n", err)
			os.Exit(1)
		}

	case "info":
		if len(cmdArgs) < 1 {
			fmt.Println("Error: Please specify a package name.")
			fmt.Println("Usage: quest info <package>")
			os.Exit(1)
		}
		pkgName := cmdArgs[0]
		err := handleInfo(pkgName, homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting package info: %v\n", err)
			os.Exit(1)
		}

	case "update":
		err := handleUpdate(homeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error updating registry: %v\n", err)
			os.Exit(1)
		}

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleInstall(pkgName, targetOS, targetArch, homeDir string) error {
	fmt.Printf("Searching for %s...\n", pkgName)
	m, err := LoadManifest(pkgName, homeDir)
	if err != nil {
		return err
	}

	fmt.Printf("Found package %s (version %s)\n", m.Name, m.Version)

	receipt, err := LoadReceipt(m.Name, homeDir)
	if err == nil {
		fmt.Printf("Package %s is already installed (version %s).\n", receipt.Name, receipt.Version)
		fmt.Print("Would you like to reinstall? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") {
			fmt.Println("Installation cancelled.")
			return nil
		}
		fmt.Println("Uninstalling existing version first...")
		if err := handleUninstall(pkgName, homeDir); err != nil {
			return fmt.Errorf("failed to uninstall old version: %w", err)
		}
	}

	archKey := fmt.Sprintf("%s-%s", targetOS, targetArch)
	arch, ok := m.GetArchitecture(targetOS, targetArch)
	if !ok {
		return fmt.Errorf("architecture %s is not supported by %s", archKey, m.Name)
	}

	url := arch.URL
	fileName := filepath.Base(url)
	if idx := strings.Index(fileName, "?"); idx != -1 {
		fileName = fileName[:idx]
	}
	cachePath := filepath.Join(homeDir, "cache", fileName)

	if _, err := os.Stat(cachePath); err != nil {
		err = DownloadFile(url, cachePath)
		if err != nil {
			return fmt.Errorf("failed to download: %w", err)
		}
	} else {
		fmt.Printf("Using cached installer: %s\n", fileName)
	}

	if err := VerifySHA256(cachePath, arch.SHA256); err != nil {
		os.Remove(cachePath)
		return err
	}

	appDir := filepath.Join(homeDir, "apps", strings.ToLower(m.Name), m.Version)
	os.RemoveAll(appDir)
	fmt.Printf("Installing to %s...\n", appDir)
	if err := ExtractArchive(cachePath, appDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	binDir := filepath.Join(homeDir, "bin")
	fmt.Println("Creating command shims/links...")
	shims, err := CreateShims(m, arch, appDir, binDir)
	if err != nil {
		os.RemoveAll(appDir)
		return fmt.Errorf("failed to create shims: %w", err)
	}

	r := &Receipt{
		Name:    m.Name,
		Version: m.Version,
		AppDir:  appDir,
		Shims:   shims,
	}
	if err := SaveReceipt(r, homeDir); err != nil {
		return fmt.Errorf("failed to save receipt: %w", err)
	}

	fmt.Printf("\nSuccessfully installed %s (version %s)!\n", m.Name, m.Version)
	if runtime.GOOS == "windows" {
		fmt.Printf("Make sure '%s' is in your PATH environment variable.\n", binDir)
	} else {
		fmt.Printf("Make sure '%s' is in your PATH.\n", binDir)
		fmt.Printf("Example: export PATH=\"$HOME/.quest/bin:$PATH\"\n")
	}

	return nil
}

func handleUninstall(pkgName string, homeDir string) error {
	fmt.Printf("Uninstalling %s...\n", pkgName)
	r, err := LoadReceipt(pkgName, homeDir)
	if err != nil {
		return fmt.Errorf("package %s is not installed (no receipt found)", pkgName)
	}

	for _, shim := range r.Shims {
		if err := os.Remove(shim); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to remove shim %s: %v\n", shim, err)
		}
	}

	if err := os.RemoveAll(r.AppDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove application folder %s: %w", r.AppDir, err)
	}

	parentDir := filepath.Dir(r.AppDir)
	if entries, err := os.ReadDir(parentDir); err == nil && len(entries) == 0 {
		os.Remove(parentDir)
	}

	if err := RemoveReceipt(r.Name, homeDir); err != nil {
		return fmt.Errorf("failed to remove receipt: %w", err)
	}

	fmt.Printf("Successfully uninstalled %s.\n", r.Name)
	return nil
}

func handleUpgrade(pkgName, targetOS, targetArch, homeDir string) error {
	fmt.Printf("Checking for upgrades for %s...\n", pkgName)

	r, err := LoadReceipt(pkgName, homeDir)
	if err != nil {
		return fmt.Errorf("package %s is not installed; run 'quest install %s' instead", pkgName, pkgName)
	}

	m, err := LoadManifest(pkgName, homeDir)
	if err != nil {
		return err
	}

	if r.Version == m.Version {
		fmt.Printf("Package %s is already at the latest version (%s).\n", m.Name, m.Version)
		return nil
	}

	fmt.Printf("Upgrading %s from version %s to %s...\n", m.Name, r.Version, m.Version)

	if err := handleUninstall(pkgName, homeDir); err != nil {
		return fmt.Errorf("failed to uninstall old version: %w", err)
	}

	return handleInstall(pkgName, targetOS, targetArch, homeDir)
}

func handleList(homeDir string) error {
	receiptDir := filepath.Join(homeDir, "receipts")
	entries, err := os.ReadDir(receiptDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No packages installed.")
			return nil
		}
		return err
	}

	installedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		pkgName := strings.TrimSuffix(entry.Name(), ".json")
		r, err := LoadReceipt(pkgName, homeDir)
		if err == nil {
			fmt.Printf("- %s (version %s)\n", r.Name, r.Version)
			installedCount++
		}
	}

	if installedCount == 0 {
		fmt.Println("No packages installed.")
	}
	return nil
}

func handleSearch(query string, homeDir string) error {
	manifests, err := ListManifests(homeDir)
	if err != nil {
		return err
	}

	query = strings.ToLower(query)
	fmt.Printf("Search results for '%s':\n\n", query)
	fmt.Printf("%-15s %-10s %s\n", "Package", "Version", "Description")
	fmt.Println(strings.Repeat("-", 60))

	matchCount := 0
	for _, m := range manifests {
		if query == "" || strings.Contains(strings.ToLower(m.Name), query) || strings.Contains(strings.ToLower(m.Description), query) {
			fmt.Printf("%-15s %-10s %s\n", m.Name, m.Version, m.Description)
			matchCount++
		}
	}

	if matchCount == 0 {
		fmt.Println("No matching packages found.")
	}
	return nil
}

func handleInfo(pkgName string, homeDir string) error {
	m, err := LoadManifest(pkgName, homeDir)
	if err != nil {
		return err
	}

	fmt.Printf("Package:      %s\n", m.Name)
	fmt.Printf("Version:      %s\n", m.Version)
	fmt.Printf("Description:  %s\n", m.Description)
	fmt.Printf("Homepage:     %s\n", m.Homepage)
	
	fmt.Print("Architectures:\n")
	for arch := range m.Architectures {
		fmt.Printf("  - %s\n", arch)
	}

	r, err := LoadReceipt(pkgName, homeDir)
	if err == nil {
		fmt.Printf("Status:       Installed (version %s)\n", r.Version)
	} else {
		fmt.Println("Status:       Not installed")
	}

	return nil
}

func handleUpdate(homeDir string) error {
	fmt.Println("Updating package manifests from registry...")

	zipURL := "https://github.com/akhileshg1124/quest-search/archive/refs/heads/main.zip"
	tempZip := filepath.Join(homeDir, "cache", "registry-update.zip")
	tempExtractDir := filepath.Join(homeDir, "cache", "registry-extracted")

	os.RemoveAll(tempExtractDir)
	os.Remove(tempZip)

	if err := DownloadFile(zipURL, tempZip); err != nil {
		return fmt.Errorf("failed to fetch updates from registry: %w", err)
	}

	if err := ExtractArchive(tempZip, tempExtractDir); err != nil {
		return fmt.Errorf("failed to extract updates: %w", err)
	}

	extractedManifestsDir := filepath.Join(tempExtractDir, "manifests")
	if _, err := os.Stat(extractedManifestsDir); os.IsNotExist(err) {
		entries, err := os.ReadDir(tempExtractDir)
		if err == nil && len(entries) > 0 {
			extractedManifestsDir = filepath.Join(tempExtractDir, entries[0].Name(), "manifests")
		}
	}

	destManifestsDir := filepath.Join(homeDir, "manifests")
	entries, err := os.ReadDir(extractedManifestsDir)
	if err != nil {
		return fmt.Errorf("no manifests found in registry: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		srcFile := filepath.Join(extractedManifestsDir, entry.Name())
		destFile := filepath.Join(destManifestsDir, entry.Name())

		if err := copyFile(srcFile, destFile); err != nil {
			fmt.Printf("Warning: Failed to update manifest %s: %v\n", entry.Name(), err)
		}
	}

	os.RemoveAll(tempExtractDir)
	os.Remove(tempZip)

	fmt.Println("Registry manifests updated successfully!")
	return nil
}
