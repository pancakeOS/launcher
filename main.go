package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func isAdmin() bool {
	if runtime.GOOS == "windows" {
		f, err := os.OpenFile("C:\\Windows\\System32\\test_admin.txt", os.O_WRONLY|os.O_CREATE, 0644)
		if err == nil {
			f.Close()
			os.Remove("C:\\Windows\\System32\\test_admin.txt")
			return true
		}
		return false
	}
	// On Unix-like systems, check uid == 0
	cmd := exec.Command("id", "-u")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "0"
}

// relaunchAsAdmin relaunches the current executable with UAC prompt
func relaunchAsAdmin() {
	exe, _ := os.Executable()
	if runtime.GOOS == "windows" {
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
		return
	}
	// Unix-like: use sudo to re-run the binary
	args := append([]string{exe}, os.Args[1:]...)
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

const (
	releasesAPI = "https://api.github.com/repos/pancakeOS/pancakeOS/releases/latest"
	versionFile = "version.txt"
)

var (
	installDir string
	pancakeExe string
)

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func main() {
	if runtime.GOOS == "windows" {
		if isWine() {
			showWineError()
			os.Exit(1)
		}
	}
	// set platform-specific paths
	switch runtime.GOOS {
	case "windows":
		installDir = "C:\\Program Files\\PancakeOS"
		pancakeExe = "PancakeOS.exe"
	case "darwin":
		installDir = "/Applications"
		pancakeExe = "PancakeOS.app"
	default:
		// assume linux/other unix
		installDir = "/opt/pancakeos"
		pancakeExe = "PancakeOS.AppImage"
	}
	fmt.Println("Checking for updates...")
	assetName := "PancakeOS-windows.zip"
	if runtime.GOOS == "linux" {
		assetName = "PancakeOS-x86_64.AppImage"
	} else if runtime.GOOS == "darwin" {
		assetName = "PancakeOS-macOS.zip"
	}
	latest, url, err := getLatestRelease(assetName)
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
		// platform-specific install flow
		if runtime.GOOS == "windows" {
			zipPath := filepath.Join(os.TempDir(), filepath.Base(url))
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
		} else if runtime.GOOS == "darwin" {
			// macOS: expect a zip that contains PancakeOS.app; extract and move to /Applications
			zipPath := filepath.Join(os.TempDir(), filepath.Base(url))
			if err := downloadFile(url, zipPath); err != nil {
				fmt.Println("Download failed:", err)
				launchPancake()
				return
			}
			tmpDir := filepath.Join(os.TempDir(), "pancakeos_mac_extract")
			os.RemoveAll(tmpDir)
			os.MkdirAll(tmpDir, 0755)
			if err := unzip(zipPath, tmpDir); err != nil {
				fmt.Println("Extraction failed:", err)
				launchPancake()
				return
			}
			// find PancakeOS.app inside tmpDir
			var appPath string
			filepath.Walk(tmpDir, func(p string, info os.FileInfo, err error) error {
				if err != nil || info == nil {
					return nil
				}
				if info.IsDir() && strings.EqualFold(filepath.Base(p), "PancakeOS.app") {
					appPath = p
					return filepath.SkipDir
				}
				return nil
			})
			if appPath == "" {
				fmt.Println("PancakeOS.app not found in archive")
				launchPancake()
				return
			}
			dest := filepath.Join("/Applications", "PancakeOS.app")
			// remove any existing app bundle
			os.RemoveAll(dest)
			// try to rename (fast), otherwise copy recursively
			if err := os.Rename(appPath, dest); err != nil {
				if err := copyDir(appPath, dest); err != nil {
					fmt.Println("Failed to install app bundle:", err)
					launchPancake()
					return
				}
			}
			// write version file inside the app bundle if possible
			verDir := filepath.Join(dest, "Contents", "Resources")
			if _, err := os.Stat(verDir); os.IsNotExist(err) {
				verDir = dest
			}
			os.MkdirAll(verDir, 0755)
			os.WriteFile(filepath.Join(verDir, versionFile), []byte(latest), 0644)
		} else {
			// linux: download AppImage and place in installDir
			imgName := filepath.Base(url)
			imgPath := filepath.Join(os.TempDir(), imgName)
			if err := downloadFile(url, imgPath); err != nil {
				fmt.Println("Download failed:", err)
				launchPancake()
				return
			}
			os.MkdirAll(installDir, 0755)
			dest := filepath.Join(installDir, pancakeExe)
			os.Rename(imgPath, dest)
			// make executable
			os.Chmod(dest, 0755)
		}
		os.WriteFile(filepath.Join(installDir, versionFile), []byte(latest), 0644)
		// Log update event
		logUpdate("Updated to version " + latest)
	} else {
		// Log install event (if not updating)
		logUpdate("Installed version " + current)
	}
	launchPancake()
}

// logUpdate writes a log file to C:/Program Files/PancakeOS/logs/ with timestamp and date as filename
func logUpdate(message string) {
	logDir := filepath.Join(installDir, "logs")
	os.MkdirAll(logDir, 0755)
	now := time.Now()
	filename := now.Format("2006-01-02_15-04-05") + ".txt"
	logPath := filepath.Join(logDir, filename)
	logMsg := now.Format("2006-01-02 15:04:05") + " - " + message + "\n"
	os.WriteFile(logPath, []byte(logMsg), 0644)
}

// isWine checks if the program is running under Wine
func isWine() bool {
	// Wine sets the WINELOADERNOEXEC environment variable
	if os.Getenv("WINELOADERNOEXEC") != "" {
		return true
	}
	// Wine also sets the "wine" in the process name sometimes
	if strings.Contains(strings.ToLower(os.Getenv("PATH")), "wine") {
		return true
	}
	// Try to run a Wine-specific command (reg query for Wine registry key)
	cmd := exec.Command("reg", "query", "HKCU\\Software\\Wine")
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

// showWineError displays an error message and exits
func showWineError() {
	// Use a message box for visibility
	exec.Command("powershell", "-Command", "[System.Windows.MessageBox]::Show('Wine is not supported.')").Run()
	fmt.Println("Wine is not supported.")
}

func getLatestRelease(assetName string) (string, string, error) {
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
		if asset.Name == assetName {
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
	size := resp.ContentLength
	if size <= 0 {
		// fallback if size unknown
		_, err = io.Copy(out, resp.Body)
		return err
	}
	fmt.Print("Downloading: [")
	var downloaded int64 = 0
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			downloaded += int64(n)
			percent := int(float64(downloaded) / float64(size) * 50)
			fmt.Print("\rDownloading: [")
			for i := 0; i < 50; i++ {
				if i < percent {
					fmt.Print("=")
				} else {
					fmt.Print(" ")
				}
			}
			fmt.Printf("] %d%%", int(float64(downloaded)/float64(size)*100))
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	fmt.Println()
	return nil
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

// copyDir recursively copies a directory tree, preserving permissions and symlinks.
func copyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			// preserve symlink
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// remove existing and create symlink
			os.RemoveAll(target)
			return os.Symlink(link, target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		// file
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	})
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
