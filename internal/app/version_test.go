package app

import (
	"os"
	"regexp"
	"testing"
)

func TestVersionMatchesBuildFiles(t *testing.T) {
	files := map[string]string{
		"../../build/config.yml":                    `(?m)^\s*version:\s*"([^"]+)"`,
		"../../build/darwin/Info.plist":             `<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`,
		"../../build/darwin/Info.dev.plist":         `<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`,
		"../../build/windows/info.json":             `"ProductVersion":\s*"([^"]+)"`,
		"../../build/windows/wails.exe.manifest":    `name="io\.github\.myrheimb\.pageroom" version="([^"]+)"`,
		"../../build/windows/nsis/wails_tools.nsh":  `(?m)^\s*!define INFO_PRODUCTVERSION "([^"]+)"`,
		"../../build/windows/msix/app_manifest.xml": `(?s)<Identity.*?Version="([^"]+)\.0"`,
		"../../build/windows/msix/template.xml":     `(?s)<PackageInformation.*?Version="([^"]+)\.0"`,
		"../../build/linux/nfpm/nfpm.yaml":          `(?m)^version:\s*"([^"]+)"`,
		"../../frontend/package.json":               `"version":\s*"([^"]+)"`,
	}
	for path, pattern := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}

		match := regexp.MustCompile(pattern).FindSubmatch(content)
		if match == nil {
			t.Errorf("%s: no version found", path)
			continue
		}

		if string(match[1]) != version {
			t.Errorf("%s has version %s, want %s", path, match[1], version)
		}
	}
}
