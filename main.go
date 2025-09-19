package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"archive/zip"
	"strings"
)
func isAdmin() bool {
	f, err := os.OpenFile("C:\\Windows\\System32\\test_admin.txt", os.O_WRONLY|os.O_CREATE, 0644)
	if err == nil {
		f.Close()
		os.Remove("C:\\Windows\\System32\\test_admin.txt")
		return true
	}
	return false
}

// relaunchAsAdmin relaunches the current executable with UAC prompt
func relaunchAsAdmin() {
	exe, _ := os.Executable()
	args := ""
	if len(os.Args) > 1 {
		args = strings.Join(os.Args[1:], " ")
	}
	var psCmd string
	if args != "" {
		psCmd = fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs", exe, args)
	} else {
		psCmd = fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs", exe)
	}
	cmd := exec.Command("powershell", "-Command", psCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

const (
	releasesAPI = "https://api.github.com/repos/pancakeOS/pancakeOS/releases/latest"
	installDir = "C:\\Program Files\\PancakeOS"
	versionFile = "version.txt"
	pancakeExe = "PancakeOS.exe"
)

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func main() {
   fmt.Println("Checking for updates...")
   latest, url, err := getLatestRelease()
   if err != nil {
	   fmt.Println("Failed to check releases:", err)
	   launchPancake()
	   return
   }
   current := getCurrentVersion()
   exePath, _ := os.Executable()
   destPath := filepath.Join(installDir, "PancakeOSLauncher.exe")
   if current != latest {
	   fmt.Println("Updating to version", latest)
	   // Request UAC only if update is needed
	   if !isAdmin() {
		   fmt.Println("Requesting administrator privileges for update...")
		   relaunchAsAdmin()
		   return
	   }
	   // If not in folder, copy self and create shortcut
	   if exePath != destPath {
		   os.MkdirAll(installDir, 0755)
		   err := copySelf(exePath, destPath)
		   if err != nil {
			   fmt.Println("Failed to copy updater:", err)
		   }
		   shortcutName := "PancakeOS.lnk"
		   err = createShortcut(destPath, shortcutName)
		   if err != nil {
			   fmt.Println("Failed to create shortcut:", err)
		   }
	   }
	   zipPath := filepath.Join(os.TempDir(), "PancakeOS-windows.zip")
	   if err := downloadFile(url, zipPath); err != nil {
		   fmt.Println("Download failed:", err)
		   launchPancake()
		   return
	   }
	   if err := unzip(zipPath, installDir); err != nil {
		   fmt.Println("Extraction failed:", err)
		   launchPancake()
		   return
	   }
	   os.WriteFile(filepath.Join(installDir, versionFile), []byte(latest), 0644)
   }
   launchPancake()
}

func getLatestRelease() (string, string, error) {
	resp, err := http.Get(releasesAPI)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	for _, asset := range rel.Assets {
		if asset.Name == "PancakeOS-windows.zip" {
			return rel.TagName, asset.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("Asset not found")
}

func getCurrentVersion() string {
	data, err := os.ReadFile(filepath.Join(installDir, versionFile))
	if err != nil {
		return ""
	}
	return string(data)
}


func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, f.Mode())
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}


func copySelf(src, dst string) error {
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

func createShortcut(target, shortcutName string) error {
   startMenu := os.Getenv("APPDATA") + "\\Microsoft\\Windows\\Start Menu\\Programs"
   shortcutPath := filepath.Join(startMenu, shortcutName)
   psCmd := fmt.Sprintf("$s=(New-Object -COM WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.Save()", shortcutPath, target)
   cmd := exec.Command("powershell", "-Command", psCmd)
   return cmd.Run()
}

func launchPancake() {
	cmd := exec.Command(filepath.Join(installDir, pancakeExe))
	cmd.Start()
}
