package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha512"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed claude-catalog.json
var claudeCatalog []byte

type claudePack struct {
	URL       string `json:"url"`
	Integrity string `json:"integrity"`
	License   string `json:"license"`
	Unpacked  int64  `json:"unpacked_bytes"`
}
type ClientInstallManager struct {
	mu      sync.Mutex
	root    string
	status  DownloadStatus
	cancel  context.CancelFunc
	done    chan struct{}
	client  *http.Client
	catalog []byte
}

func newClientInstall(root string) *ClientInstallManager {
	return &ClientInstallManager{root: root, catalog: claudeCatalog, client: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 8 || r.URL.Scheme != "https" {
			return errors.New("client download redirects must use HTTPS")
		}
		return nil
	}}}
}
func (m *ClientInstallManager) snapshot() DownloadStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}
func (m *ClientInstallManager) close() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	done := m.done
	m.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (m *ClientInstallManager) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Active {
		return errors.New("Claude Code is already being downloaded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	m.status = DownloadStatus{ID: "claude", Stage: "starting", Active: true}
	go func() {
		err := m.install(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.status.Active = false
		m.cancel = nil
		if err != nil {
			m.status.Stage = "error"
			m.status.Error = err.Error()
		} else {
			m.status.Stage = "complete"
		}
		close(m.done)
	}()
	return nil
}
func (m *ClientInstallManager) install(ctx context.Context) error {
	if _, err := portableClient(m.root, "claude", true); err == nil {
		return nil
	}
	var catalog struct {
		Version string                `json:"version"`
		Packs   map[string]claudePack `json:"packs"`
	}
	if err := json.Unmarshal(m.catalog, &catalog); err != nil {
		return err
	}
	info, ok := catalog.Packs[platformID()]
	if !ok {
		return errors.New("Claude Code is not available for this platform")
	}
	u, e := url.Parse(info.URL)
	if e != nil || u.Scheme != "https" || u.Host != "registry.npmjs.org" || !strings.HasSuffix(u.Path, ".tgz") {
		return errors.New("invalid pinned Claude download")
	}
	lock, err := lockNamed(m.root, "client-install.lock")
	if err != nil {
		return errors.New("another client setup is running")
	}
	defer lock.Close()
	index, err := safePath(m.root, "harnesses/installed.json")
	if err != nil {
		return err
	}
	original, err := os.ReadFile(index)
	if err != nil {
		return errors.New("portable client index is missing")
	}
	var packs map[string]HarnessPack
	if json.Unmarshal(original, &packs) != nil {
		return errors.New("invalid client index")
	}
	var manifest map[string]string
	manifestPath := filepath.Join(m.root, "manifest.json")
	if err = readJSON(manifestPath, &manifest); err != nil {
		return errors.New("package integrity index is missing")
	}
	actual, err := hashFile(index)
	if err != nil || manifest["harnesses/installed.json"] != actual {
		return errors.New("client index failed its integrity check; restore the package")
	}
	destination, err := safePath(m.root, "harnesses/claude/"+platformID())
	if err != nil {
		return err
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("an existing Claude pack will not be overwritten; restore or inspect it first")
	}
	work, err := statePath(m.root, "downloads")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(work, 0700); err != nil {
		return err
	}
	if err = checkFreeSpace(work, uint64(max(info.Unpacked, 0))+(768<<20)); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(work, "claude-setup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, "package.tgz")
	req, err := http.NewRequestWithContext(ctx, "GET", info.URL, nil)
	if err != nil {
		return err
	}
	response, err := m.client.Do(req)
	if err != nil {
		return errors.New("Claude download could not connect; check your internet connection")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("Claude download returned HTTP %d", response.StatusCode)
	}
	const maxArchive = int64(512 << 20)
	if response.ContentLength > maxArchive {
		return errors.New("Claude archive exceeds the allowed size")
	}
	m.mu.Lock()
	m.status.Stage = "downloading"
	m.status.Total = max(response.ContentLength, 0)
	m.mu.Unlock()
	f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	h := sha512.New()
	buffer := make([]byte, 256<<10)
	var received int64
	reader := io.LimitReader(response.Body, maxArchive+1)
	for {
		n, e := reader.Read(buffer)
		if n > 0 {
			received += int64(n)
			if received > maxArchive {
				f.Close()
				return errors.New("Claude archive exceeds the allowed size")
			}
			if _, err = f.Write(buffer[:n]); err != nil {
				f.Close()
				return err
			}
			h.Write(buffer[:n])
			m.mu.Lock()
			m.status.Received = received
			m.mu.Unlock()
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			f.Close()
			return errors.New("Claude download interrupted; try Get Claude Code again")
		}
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if response.ContentLength >= 0 && received != response.ContentLength {
		return errors.New("Claude archive size does not match the source")
	}
	if info.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(h.Sum(nil)) {
		return errors.New("Claude archive failed its publisher integrity check")
	}
	m.mu.Lock()
	m.status.Stage = "checking"
	m.mu.Unlock()
	stage := filepath.Join(tmp, "pack")
	if err = os.Mkdir(stage, 0700); err != nil {
		return err
	}
	if err = extractClaudeArchive(ctx, archive, stage, strings.HasPrefix(platformID(), "windows"), max(info.Unpacked, 0)+(1<<20)); err != nil {
		return err
	}
	binary := "claude"
	if strings.HasPrefix(platformID(), "windows") {
		binary += ".exe"
	}
	sha, err := hashFile(filepath.Join(stage, binary))
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	if err = os.Rename(stage, destination); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(destination)
		}
	}()
	rel := filepath.ToSlash(filepath.Join("harnesses", "claude", platformID(), binary))
	packs["claude/"+platformID()] = HarnessPack{Version: catalog.Version, Path: rel, SHA256: sha, License: info.License}
	if err = writeJSON(index, packs); err != nil {
		return err
	}
	newIndexHash, err := hashFile(index)
	if err != nil {
		atomicWrite(index, original, 0600)
		return err
	}
	manifest["harnesses/installed.json"] = newIndexHash
	err = filepath.WalkDir(destination, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(m.root, p)
		if e != nil {
			return e
		}
		sum, e := hashFile(p)
		if e != nil {
			return e
		}
		manifest[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err == nil {
		err = writeJSON(manifestPath, manifest)
	}
	if err != nil {
		atomicWrite(index, original, 0600)
		return err
	}
	committed = true
	return nil
}
func extractClaudeArchive(ctx context.Context, archive, stage string, windows bool, limit int64) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer z.Close()
	t := tar.NewReader(z)
	var total int64
	found := false
	wanted := "claude"
	if windows {
		wanted += ".exe"
	}
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		h, e := t.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		clean := path.Clean(h.Name)
		if clean == "package" && h.Typeflag == tar.TypeDir {
			continue
		}
		if strings.Contains(h.Name, "\\") || strings.HasPrefix(clean, "/") || !strings.HasPrefix(clean, "package/") {
			return errors.New("unsafe Claude archive path")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || total+h.Size > limit {
			return errors.New("unsupported Claude archive member or excessive unpacked size")
		}
		total += h.Size
		rel := clean
		if clean == "package/"+wanted {
			rel = wanted
			found = true
		}
		p, err := safePath(stage, filepath.FromSlash(rel))
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		out, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.CopyN(out, t, h.Size)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if rel == wanted {
			if err = os.Chmod(p, 0755); err != nil {
				return err
			}
		}
	}
	if !found {
		return errors.New("Claude archive does not contain the expected native executable")
	}
	return nil
}
