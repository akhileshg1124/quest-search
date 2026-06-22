package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Receipt struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	AppDir  string   `json:"app_dir"`
	Shims   []string `json:"shims"`
}

func DownloadFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	fmt.Printf("Downloading %s...\n", filepath.Base(destPath))
	_, err = io.Copy(out, resp.Body)
	return err
}

func VerifySHA256(filePath, expectedHash string) error {
	if expectedHash == "" {
		fmt.Printf("Warning: No SHA256 checksum specified in manifest for verification.\n")
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("SHA256 checksum mismatch!\nExpected: %s\nActual:   %s", expectedHash, actualHash)
	}

	fmt.Println("SHA256 checksum verified successfully.")
	return nil
}

func ExtractArchive(srcPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	lowerSrc := strings.ToLower(srcPath)
	if strings.HasSuffix(lowerSrc, ".zip") {
		if err := extractZip(srcPath, destDir); err != nil {
			return err
		}
	} else if strings.HasSuffix(lowerSrc, ".tar.gz") || strings.HasSuffix(lowerSrc, ".tgz") {
		if err := extractTarGz(srcPath, destDir); err != nil {
			return err
		}
	} else {
		baseName := filepath.Base(srcPath)
		destFile := filepath.Join(destDir, baseName)
		if err := copyFile(srcPath, destFile); err != nil {
			return err
		}
		if err := os.Chmod(destFile, 0755); err != nil {
			return err
		}
	}

	return flattenDirectory(destDir)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !isSafePath(dest, target) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		srcFile, err := f.Open()
		if err != nil {
			dstFile.Close()
			return err
		}

		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzR, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzR.Close()

	tarR := tar.NewReader(gzR)

	for {
		header, err := tarR.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)
		if !isSafePath(dest, target) {
			return fmt.Errorf("illegal file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			dstFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}

			_, err = io.Copy(dstFile, tarR)
			dstFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func isSafePath(dest, target string) bool {
	destAbs, err1 := filepath.Abs(dest)
	targetAbs, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.HasPrefix(targetAbs, destAbs)
}

func flattenDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	if len(entries) == 1 && entries[0].IsDir() {
		subDir := filepath.Join(dir, entries[0].Name())
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			return err
		}

		for _, subEntry := range subEntries {
			oldPath := filepath.Join(subDir, subEntry.Name())
			newPath := filepath.Join(dir, subEntry.Name())
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}

		if err := os.Remove(subDir); err != nil {
			return err
		}

		return flattenDirectory(dir)
	}

	return nil
}

func CreateShims(m *Manifest, arch Architecture, appDir, binDir string) ([]string, error) {
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, err
	}

	var createdShims []string

	for _, binEntry := range arch.Bin {
		parts := strings.Split(binEntry, ":")
		srcRelPath := parts[0]
		shimName := filepath.Base(srcRelPath)
		if len(parts) > 1 {
			shimName = parts[1]
		}
		shimName = strings.TrimSuffix(shimName, ".exe")

		targetBinaryPath := filepath.Join(appDir, srcRelPath)
		if _, err := os.Stat(targetBinaryPath); err != nil {
			return createdShims, fmt.Errorf("target binary not found inside app folder: %s", targetBinaryPath)
		}

		if runtime.GOOS == "windows" {
			cmdPath := filepath.Join(binDir, shimName+".cmd")
			cmdContent := fmt.Sprintf("@echo off\n\"%s\" %%*\n", targetBinaryPath)
			if err := os.WriteFile(cmdPath, []byte(cmdContent), 0755); err != nil {
				return createdShims, err
			}
			createdShims = append(createdShims, cmdPath)

			ps1Path := filepath.Join(binDir, shimName+".ps1")
			ps1Content := fmt.Sprintf("& \"%s\" $args\n", targetBinaryPath)
			if err := os.WriteFile(ps1Path, []byte(ps1Content), 0755); err != nil {
				return createdShims, err
			}
			createdShims = append(createdShims, ps1Path)

			shPath := filepath.Join(binDir, shimName)
			unixLikePath := strings.ReplaceAll(targetBinaryPath, "\\", "/")
			shContent := fmt.Sprintf("#!/bin/sh\nexec \"%s\" \"$@\"\n", unixLikePath)
			if err := os.WriteFile(shPath, []byte(shContent), 0755); err != nil {
				return createdShims, err
			}
			createdShims = append(createdShims, shPath)
		} else {
			symlinkPath := filepath.Join(binDir, shimName)
			if _, err := os.Lstat(symlinkPath); err == nil {
				os.Remove(symlinkPath)
			}

			relTarget, err := filepath.Rel(binDir, targetBinaryPath)
			if err != nil {
				relTarget = targetBinaryPath
			}

			if err := os.Symlink(relTarget, symlinkPath); err != nil {
				return createdShims, err
			}
			createdShims = append(createdShims, symlinkPath)
		}
	}

	return createdShims, nil
}

func LoadReceipt(name string, homeDir string) (*Receipt, error) {
	receiptPath := filepath.Join(homeDir, "receipts", strings.ToLower(name)+".json")
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		return nil, err
	}

	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func SaveReceipt(r *Receipt, homeDir string) error {
	receiptDir := filepath.Join(homeDir, "receipts")
	if err := os.MkdirAll(receiptDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	receiptPath := filepath.Join(receiptDir, strings.ToLower(r.Name)+".json")
	return os.WriteFile(receiptPath, data, 0644)
}

func RemoveReceipt(name string, homeDir string) error {
	receiptPath := filepath.Join(homeDir, "receipts", strings.ToLower(name)+".json")
	return os.Remove(receiptPath)
}
