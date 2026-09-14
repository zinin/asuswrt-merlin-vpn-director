package updater

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// manifestPath is where a release lists the files it ships, relative to the
// repository root. install.sh reads the same file.
const manifestPath = "router/files.manifest"

// ManifestEntry is one line of router/files.manifest: a platform tag
// ("common", "merlin", "keenetic") and a repository path under router/.
type ManifestEntry struct {
	Tag  string
	Path string
}

// manifestPathRe admits the characters repository paths use. The manifest is
// downloaded from the network and its paths are pasted into a shell script
// that runs as root, so nothing else gets through.
var manifestPathRe = regexp.MustCompile(`^router/[A-Za-z0-9._/-]+$`)

// parseManifest reads "<tag> <path>" lines. Blank lines and lines starting
// with # are skipped; any other shape is an error naming the line.
func parseManifest(r io.Reader) ([]ManifestEntry, error) {
	var entries []ManifestEntry
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("files.manifest line %d: want \"<tag> <path>\", got %q", line, text)
		}
		if !manifestPathRe.MatchString(fields[1]) || strings.Contains(fields[1], "..") {
			return nil, fmt.Errorf("files.manifest line %d: invalid path %q", line, fields[1])
		}
		entries = append(entries, ManifestEntry{Tag: fields[0], Path: fields[1]})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read files.manifest: %w", err)
	}
	return entries, nil
}

// manifestFilesFor returns the paths tagged "common" or platform, in
// manifest order. Unknown tags are ignored: a release may list files for a
// platform this binary does not know yet.
func manifestFilesFor(entries []ManifestEntry, platform string) []string {
	var files []string
	for _, e := range entries {
		if e.Tag == "common" || e.Tag == platform {
			files = append(files, e.Path)
		}
	}
	return files
}

// isExecutable reports whether an installed file gets the executable bit:
// every shipped file except templates, JSON and the manifest itself.
func isExecutable(path string) bool {
	for _, suffix := range []string{".template", ".json", ".manifest"} {
		if strings.HasSuffix(path, suffix) {
			return false
		}
	}
	return true
}

// FileEntry is one file the update script installs: Src relative to the
// payload's files/ directory, Dst absolute on the router.
type FileEntry struct {
	Src  string
	Dst  string
	Exec bool
}

// fileEntries turns the manifest lines for platform into copy instructions.
func fileEntries(entries []ManifestEntry, platform string) []FileEntry {
	var files []FileEntry
	for _, p := range manifestFilesFor(entries, platform) {
		rel := strings.TrimPrefix(p, "router/")
		files = append(files, FileEntry{Src: rel, Dst: "/" + rel, Exec: isExecutable(p)})
	}
	return files
}
