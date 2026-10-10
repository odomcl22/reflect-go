package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed model-catalog.json
var catalogData []byte

type CatalogModel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Filename     string `json:"filename"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	License      string `json:"license"`
	Source       string `json:"source"`
	Quantization string `json:"quantization"`
	Purpose      string `json:"purpose"`
	Note         string `json:"note"`
}

func modelCatalog() []CatalogModel {
	var entries []CatalogModel
	_ = json.Unmarshal(catalogData, &entries)
	return entries
}

type DownloadStatus struct {
	ID       string `json:"id"`
	Stage    string `json:"stage"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Error    string `json:"error,omitempty"`
	Active   bool   `json:"active"`
}
type DownloadManager struct {
	mu     sync.Mutex
	root   string
	status DownloadStatus
	cancel context.CancelFunc
	done   chan struct{}
	client *http.Client
}

func newDownloads(root string) *DownloadManager {
	return &DownloadManager{root: root, client: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 8 || r.URL.Scheme != "https" {
			return errors.New("download redirect must use HTTPS")
		}
		return nil
	}}}
}
func (d *DownloadManager) snapshot() DownloadStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}
func (d *DownloadManager) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
}
func (d *DownloadManager) close() {
	d.stop()
	d.mu.Lock()
	done := d.done
	d.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (d *DownloadManager) start(id string) error {
	var model CatalogModel
	for _, m := range modelCatalog() {
		if m.ID == id {
			model = m
		}
	}
	if model.ID == "" {
		return errors.New("choose a model from the verified catalog")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status.Active {
		return errors.New("finish or cancel the current download first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.done = make(chan struct{})
	done := d.done
	d.status = DownloadStatus{ID: id, Stage: "starting", Total: model.Size, Active: true}
	go func() {
		err := d.download(ctx, model)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.status.Active = false
		d.cancel = nil
		if err != nil {
			d.status.Stage = "error"
			d.status.Error = err.Error()
			if ctx.Err() != nil {
				d.status.Stage = "paused"
				d.status.Error = "Download paused. Choose Download / resume to continue."
			}
		} else {
			d.status.Stage = "complete"
		}
		close(done)
	}()
	return nil
}
func (d *DownloadManager) download(ctx context.Context, m CatalogModel) error {
	s, err := loadSettings(d.root)
	if err != nil {
		return err
	}
	dir, err := modelsDirectory(d.root, s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Across phases and app instances, only one writer may modify the library.
	lock, err := lockNamed(dir, "library-download.lock")
	if err != nil {
		return errors.New("another Reflect Go instance is modifying the shared library")
	}
	defer lock.Close()
	target, err := safePath(dir, m.Filename)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(target); err == nil {
		return errors.New("this model is already installed; existing files are never overwritten")
	}
	partial, err := safePath(dir, m.Filename+".partial")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset > m.Size {
		return errors.New("partial download exceeds expected size; remove the .partial file and retry")
	}
	if err = checkFreeSpace(dir, uint64(m.Size-offset)+(256<<20)); err != nil {
		return err
	}
	if offset < m.Size {
		req, err := http.NewRequestWithContext(ctx, "GET", m.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept-Encoding", "identity")
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		response, err := d.client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode == 200 {
			if err = checkFreeSpace(dir, uint64(m.Size)+(256<<20)); err != nil {
				return err
			}
			offset = 0
			if err = f.Truncate(0); err != nil {
				return err
			}
		} else if response.StatusCode == 206 {
			expected := fmt.Sprintf("bytes %d-%d/%d", offset, m.Size-1, m.Size)
			if response.Header.Get("Content-Range") != expected {
				return errors.New("server returned an unexpected resume range")
			}
		} else {
			return fmt.Errorf("download source returned HTTP %d; retry later", response.StatusCode)
		}
		if response.ContentLength >= 0 && response.ContentLength != m.Size-offset {
			return errors.New("download size differs from the pinned catalog")
		}
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		d.mu.Lock()
		d.status.Stage = "downloading"
		d.status.Received = offset
		d.mu.Unlock()
		buffer := make([]byte, 256<<10)
		reader := io.LimitReader(response.Body, m.Size-offset+1)
		for {
			n, e := reader.Read(buffer)
			if n > 0 {
				if offset+int64(n) > m.Size {
					return errors.New("download exceeded expected size")
				}
				written, we := f.Write(buffer[:n])
				offset += int64(written)
				if we != nil {
					return we
				}
				d.mu.Lock()
				d.status.Received = offset
				d.mu.Unlock()
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
		}
		if offset != m.Size {
			return errors.New("download interrupted; resume to finish")
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	d.mu.Lock()
	d.status.Stage = "verifying"
	d.status.Received = m.Size
	d.mu.Unlock()
	actual, err := hashFileContext(ctx, partial)
	if err != nil {
		return err
	}
	if actual != m.SHA256 {
		os.Remove(partial)
		return errors.New("integrity check failed; damaged partial removed. Download again")
	}
	inspected := inspectModel(partial, dir, 2048, ^uint64(0))
	if inspected.Issue != "" {
		return errors.New(inspected.Issue)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return errors.New("destination appeared during download; keeping the verified partial")
	}
	return os.Rename(partial, target)
}
func importModel(root, name string, body io.Reader, size int64) error {
	if name != filepath.Base(name) || strings.ContainsAny(name, "/\\") || !strings.EqualFold(filepath.Ext(name), ".gguf") || splitModel.MatchString(name) || strings.HasPrefix(name, ".") {
		return errors.New("import a single-file text GGUF")
	}
	s, err := loadSettings(root)
	if err != nil {
		return err
	}
	dir, err := modelsDirectory(root, s)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	lock, err := lockNamed(dir, "library-download.lock")
	if err != nil {
		return errors.New("finish the current library operation first")
	}
	defer lock.Close()
	target, err := safePath(dir, name)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return errors.New("a model with this filename already exists")
	}
	if size <= 0 || size > 64<<30 {
		return errors.New("invalid import size")
	}
	if err = checkFreeSpace(dir, uint64(size)+(256<<20)); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".import-*.partial")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(body, size+1))
	if err != nil {
		return err
	}
	if n != size {
		return errors.New("import size mismatch")
	}
	if err = f.Sync(); err != nil {
		return err
	}
	f.Close()
	m := inspectModel(f.Name(), dir, 2048, ^uint64(0))
	if m.Issue != "" {
		return errors.New(m.Issue)
	}
	return os.Rename(f.Name(), target)
}
