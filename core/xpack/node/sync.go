package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
)

// Resource sync: the master drives two agents (local over the unix socket, children over
// mutual TLS) and streams data between them, so the nodes never need to reach each other.

const syncTmp = "/tmp"

type SyncTask struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Src       string    `json:"src"`
	Dsts      []string  `json:"dsts"`
	Status    string    `json:"status"` // running | success | failed
	Progress  int       `json:"progress"`
	Message   string    `json:"message"`
	Logs      []string  `json:"logs"`
	CreatedAt time.Time `json:"createdAt"`
	EndedAt   time.Time `json:"endedAt"`
}

var (
	syncMu    sync.Mutex
	syncTasks = map[string]*SyncTask{}
)

func newTask(kind, name, src string, dsts []string) *SyncTask {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	t := &SyncTask{ID: hex.EncodeToString(b), Kind: kind, Name: name, Src: src, Dsts: dsts, Status: "running", CreatedAt: time.Now()}
	syncMu.Lock()
	defer syncMu.Unlock()
	syncTasks[t.ID] = t
	if len(syncTasks) > 200 { // drop the oldest finished ones
		var done []*SyncTask
		for _, v := range syncTasks {
			if v.Status != "running" {
				done = append(done, v)
			}
		}
		sort.Slice(done, func(i, j int) bool { return done[i].CreatedAt.Before(done[j].CreatedAt) })
		for i := 0; i < len(done) && len(syncTasks) > 150; i++ {
			delete(syncTasks, done[i].ID)
		}
	}
	return t
}

func (t *SyncTask) logf(format string, a ...interface{}) {
	syncMu.Lock()
	defer syncMu.Unlock()
	t.Logs = append(t.Logs, time.Now().Format("15:04:05 ")+fmt.Sprintf(format, a...))
}

func (t *SyncTask) progress(p int) {
	syncMu.Lock()
	t.Progress = p
	syncMu.Unlock()
}

func (t *SyncTask) finish(err error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	t.EndedAt = time.Now()
	if err != nil {
		t.Status, t.Message = "failed", err.Error()
		t.Logs = append(t.Logs, time.Now().Format("15:04:05 ")+"failed: "+err.Error())
		return
	}
	t.Status, t.Progress = "success", 100
}

func SyncTasks() []SyncTask {
	syncMu.Lock()
	defer syncMu.Unlock()
	out := make([]SyncTask, 0, len(syncTasks))
	for _, t := range syncTasks {
		c := *t
		c.Logs = append([]string(nil), t.Logs...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func DeleteSyncTasks(ids []string) {
	syncMu.Lock()
	defer syncMu.Unlock()
	for _, id := range ids {
		if t, ok := syncTasks[id]; ok && t.Status != "running" {
			delete(syncTasks, id)
		}
	}
}

// ---- agent client

var localTransport = &http.Transport{
	DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", proxy.SockPath)
	},
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     30 * time.Second,
}

func isLocal(name string) bool { return name == "" || name == "local" }

// agentDo sends one request to the named node's agent; the body streams through.
func agentDo(node, method, p string, hdr http.Header, body io.Reader) (*http.Response, error) {
	var (
		tr     http.RoundTripper
		host   string
		scheme = "http"
		proxyID string
	)
	if isLocal(node) {
		tr, host = localTransport, "unix"
	} else {
		var n model.Node
		if err := global.DB.Where("name = ? AND kind = ?", node, kindNode).First(&n).Error; err != nil {
			return nil, fmt.Errorf("node not found: %s", node)
		}
		if n.Addr == "" || n.Status == statusPending {
			return nil, fmt.Errorf("node is not ready: %s", node)
		}
		t, err := Default.agentTransport()
		if err != nil {
			return nil, err
		}
		tr, scheme, host, proxyID = t, "https", net.JoinHostPort(n.Addr, fmt.Sprint(n.Port)), n.ProxyID
	}
	req, err := http.NewRequest(method, scheme+"://"+host+"/api/v2"+p, body)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if proxyID != "" {
		req.Header.Set("Proxy-Id", proxyID)
	}
	return (&http.Client{Transport: tr}).Do(req)
}

type agentRes struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// agentJSON calls a JSON endpoint and unwraps the {code,message,data} envelope.
func agentJSON(node, method, p string, in interface{}, out interface{}) error {
	var rd io.Reader
	hdr := http.Header{}
	if in != nil {
		b, _ := json.Marshal(in)
		rd = bytes.NewReader(b)
		hdr.Set("Content-Type", "application/json")
	}
	resp, err := agentDo(node, method, p, hdr, rd)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r agentRes
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("%s %s: http %d %s", node, p, resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 200)])))
	}
	if r.Code != http.StatusOK {
		return fmt.Errorf("%s %s: %s", node, p, r.Message)
	}
	if out != nil && len(r.Data) > 0 {
		return json.Unmarshal(r.Data, out)
	}
	return nil
}

// remoteSize returns the size of a file on a node via a one-byte range read, or -1.
func remoteSize(node, p string) int64 {
	hdr := http.Header{"Range": []string{"bytes=0-0"}}
	resp, err := agentDo(node, http.MethodGet, "/files/download?path="+url.QueryEscape(p), hdr, nil)
	if err != nil {
		return -1
	}
	defer resp.Body.Close() // not drained: a server that ignores Range would stream the whole file
	if resp.StatusCode == http.StatusPartialContent {
		if i := strings.LastIndex(resp.Header.Get("Content-Range"), "/"); i >= 0 {
			if n, err := strconv.ParseInt(resp.Header.Get("Content-Range")[i+1:], 10, 64); err == nil {
				return n
			}
		}
	}
	if resp.StatusCode == http.StatusOK {
		return resp.ContentLength
	}
	return -1
}

// waitFile blocks until a file exists on the node and stops growing.
func waitFile(node, p string, timeout time.Duration) (int64, error) {
	deadline := time.Now().Add(timeout)
	last := int64(-2)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		n := remoteSize(node, p)
		if n > 0 && n == last {
			return n, nil
		}
		last = n
	}
	return 0, fmt.Errorf("timed out waiting for %s on %s", p, node)
}

// waitTask polls an agent task (compress, extract, image save/load) until it ends.
func waitTask(node, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		var res struct {
			Items []struct {
				Status   string `json:"status"`
				ErrorMsg string `json:"errorMsg"`
			} `json:"items"`
		}
		if err := agentJSON(node, http.MethodPost, "/logs/tasks/search", map[string]interface{}{"taskID": id, "page": 1, "pageSize": 1}, &res); err != nil || len(res.Items) == 0 {
			continue
		}
		switch res.Items[0].Status {
		case "Success":
			return nil
		case "Failed", "Canceled":
			return fmt.Errorf("task failed on %s: %s", node, res.Items[0].ErrorMsg)
		}
	}
	return fmt.Errorf("timed out waiting for task on %s", node)
}

func removeRemote(node, p string) {
	_ = agentJSON(node, http.MethodPost, "/files/del", map[string]interface{}{"path": p, "isDir": false, "forceDelete": true}, nil)
}

type countReader struct {
	r    io.Reader
	n    int64
	size int64
	cb   func(int)
	last int
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.size > 0 && c.cb != nil {
		if pc := int(c.n * 100 / c.size); pc != c.last {
			c.last = pc
			c.cb(pc)
		}
	}
	return n, err
}

// copyFile streams srcPath on src into dstDir on dst (kept under its own name unless rename is set).
func copyFile(src, srcPath, dst, dstDir, rename string, cb func(int)) error {
	resp, err := agentDo(src, http.MethodGet, "/files/download?path="+url.QueryEscape(srcPath), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("read %s on %s: %s", srcPath, src, strings.TrimSpace(string(b)))
	}
	name := rename
	if name == "" {
		name = path.Base(srcPath)
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		defer func() { _ = pw.CloseWithError(werr); _ = mw.Close() }()
		if werr = mw.WriteField("path", dstDir); werr != nil {
			return
		}
		if werr = mw.WriteField("overwrite", "true"); werr != nil {
			return
		}
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			werr = err
			return
		}
		_, werr = io.Copy(fw, &countReader{r: resp.Body, size: resp.ContentLength, cb: cb})
		if werr == nil {
			werr = mw.Close()
		}
	}()
	hdr := http.Header{"Content-Type": []string{mw.FormDataContentType()}}
	up, err := agentDo(dst, http.MethodPost, "/files/upload", hdr, pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return err
	}
	defer up.Body.Close()
	raw, _ := io.ReadAll(up.Body)
	var r agentRes
	if json.Unmarshal(raw, &r) != nil || r.Code != http.StatusOK {
		return fmt.Errorf("upload to %s failed: %s", dst, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
	}
	return nil
}

func tmpName(prefix, ext string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b) + ext
}

// ---- file transfer

type SyncFileItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

type SyncFileReq struct {
	Src    string         `json:"src"`
	Dst    string         `json:"dst"`
	Items  []SyncFileItem `json:"items"`
	DstDir string         `json:"dstDir"`
}

func SyncFiles(req SyncFileReq) (*SyncTask, error) {
	if len(req.Items) == 0 || req.DstDir == "" || !strings.HasPrefix(req.DstDir, "/") {
		return nil, fmt.Errorf("source items and an absolute target directory are required")
	}
	if req.Src == req.Dst && isLocal(req.Src) == isLocal(req.Dst) {
		return nil, fmt.Errorf("source and target must be different nodes")
	}
	t := newTask("file", path.Base(req.Items[0].Path), req.Src, []string{req.Dst})
	go func() {
		var err error
		for i, it := range req.Items {
			base := int(i * 100 / len(req.Items))
			span := 100 / len(req.Items)
			cb := func(p int) { t.progress(base + p*span/100) }
			if err = syncOneFile(t, req, it, cb); err != nil {
				break
			}
		}
		t.finish(err)
	}()
	return t, nil
}

func syncOneFile(t *SyncTask, req SyncFileReq, it SyncFileItem, cb func(int)) error {
	if !it.IsDir {
		t.logf("copy %s (%s) -> %s:%s", it.Path, req.Src, req.Dst, req.DstDir)
		return copyFile(req.Src, it.Path, req.Dst, req.DstDir, "", cb)
	}
	name := tmpName(".sync-", ".tar.gz")
	t.logf("compress %s on %s", it.Path, req.Src)
	tid := tmpName("sync-", "")
	err := agentJSON(req.Src, http.MethodPost, "/files/compress", map[string]interface{}{
		"files": []string{it.Path}, "dst": syncTmp, "type": "tar.gz", "name": name, "replace": true, "secret": "", "taskID": tid,
	}, nil)
	if err != nil {
		return err
	}
	srcTmp := path.Join(syncTmp, name)
	defer removeRemote(req.Src, srcTmp)
	if err = waitTask(req.Src, tid, 30*time.Minute); err != nil {
		return err
	}
	t.logf("copy archive -> %s:%s", req.Dst, req.DstDir)
	if err = copyFile(req.Src, srcTmp, req.Dst, syncTmp, name, cb); err != nil {
		return err
	}
	dstTmp := path.Join(syncTmp, name)
	defer removeRemote(req.Dst, dstTmp)
	t.logf("extract on %s", req.Dst)
	tid = tmpName("sync-", "")
	if err = agentJSON(req.Dst, http.MethodPost, "/files/decompress", map[string]interface{}{
		"dst": req.DstDir, "type": "tar.gz", "path": dstTmp, "secret": "", "taskID": tid,
	}, nil); err != nil {
		return err
	}
	return waitTask(req.Dst, tid, 30*time.Minute)
}

// ---- image transfer

type SyncImageReq struct {
	Src    string   `json:"src"`
	Dsts   []string `json:"dsts"`
	Images []string `json:"images"` // tag names such as redis:7.2.4
}

func SyncImages(req SyncImageReq) (*SyncTask, error) {
	if len(req.Images) == 0 || len(req.Dsts) == 0 {
		return nil, fmt.Errorf("select images and at least one target node")
	}
	dsts := make([]string, 0, len(req.Dsts))
	for _, d := range req.Dsts {
		if d != req.Src {
			dsts = append(dsts, d)
		}
	}
	if len(dsts) == 0 {
		return nil, fmt.Errorf("source node cannot be a target")
	}
	t := newTask("image", strings.Join(req.Images, ", "), req.Src, dsts)
	go func() {
		total := len(req.Images) * len(dsts)
		done := 0
		var firstErr error
		for _, img := range req.Images {
			if err := syncOneImage(t, req.Src, dsts, img, func() {
				done++
				t.progress(done * 100 / total)
			}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		t.finish(firstErr)
	}()
	return t, nil
}

func hasImage(node, tag string) bool {
	var list []struct {
		Tags []string `json:"tags"`
	}
	if err := agentJSON(node, http.MethodGet, "/containers/image/all", nil, &list); err != nil {
		return false
	}
	for _, i := range list {
		for _, tg := range i.Tags {
			if tg == tag {
				return true
			}
		}
	}
	return false
}

func syncOneImage(t *SyncTask, src string, dsts []string, tag string, step func()) error {
	name := strings.TrimSuffix(tmpName("sync-", ""), "")
	t.logf("export %s on %s", tag, src)
	tid := tmpName("sync-", "")
	if err := agentJSON(src, http.MethodPost, "/containers/image/save", map[string]interface{}{
		"tagName": tag, "path": syncTmp, "name": name, "taskID": tid,
	}, nil); err != nil {
		return err
	}
	tarPath := path.Join(syncTmp, name+".tar")
	defer removeRemote(src, tarPath)
	if err := waitTask(src, tid, 30*time.Minute); err != nil {
		return err
	}
	var firstErr error
	for _, d := range dsts {
		err := func() error {
			t.logf("send %s -> %s", tag, d)
			if err := copyFile(src, tarPath, d, syncTmp, name+".tar", nil); err != nil {
				return err
			}
			dstTar := path.Join(syncTmp, name+".tar")
			defer removeRemote(d, dstTar)
			t.logf("load on %s", d)
			lid := tmpName("sync-", "")
			if err := agentJSON(d, http.MethodPost, "/containers/image/load", map[string]interface{}{"paths": []string{dstTar}, "taskID": lid}, nil); err != nil {
				return err
			}
			if err := waitTask(d, lid, 30*time.Minute); err != nil {
				return err
			}
			for i := 0; i < 150; i++ { // up to ~5 min
				if hasImage(d, tag) {
					return nil
				}
				time.Sleep(2 * time.Second)
			}
			return fmt.Errorf("image %s did not show up on %s", tag, d)
		}()
		if err != nil {
			t.logf("%s -> %s failed: %v", tag, d, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			t.logf("%s -> %s ok", tag, d)
		}
		step()
	}
	return firstErr
}

// ---- certificate sync

type SyncCertReq struct {
	Src  string   `json:"src"`
	Dsts []string `json:"dsts"`
	IDs  []uint   `json:"ids"`
}

type sslInfo struct {
	ID          uint   `json:"id"`
	PrimaryDom  string `json:"primaryDomain"`
	Pem         string `json:"pem"`
	PrivateKey  string `json:"privateKey"`
	Description string `json:"description"`
}

func SyncCerts(req SyncCertReq) (*SyncTask, error) {
	if len(req.IDs) == 0 || len(req.Dsts) == 0 {
		return nil, fmt.Errorf("select certificates and at least one target node")
	}
	dsts := make([]string, 0, len(req.Dsts))
	for _, d := range req.Dsts {
		if d != req.Src {
			dsts = append(dsts, d)
		}
	}
	if len(dsts) == 0 {
		return nil, fmt.Errorf("source node cannot be a target")
	}
	t := newTask("cert", fmt.Sprintf("%d certificate(s)", len(req.IDs)), req.Src, dsts)
	go func() {
		var firstErr error
		total := len(req.IDs) * len(dsts)
		done := 0
		existing := map[string]map[string]bool{}
		for _, d := range dsts {
			set := map[string]bool{}
			var res struct {
				Items []sslInfo `json:"items"`
			}
			if err := agentJSON(d, http.MethodPost, "/websites/ssl/search", map[string]interface{}{"page": 1, "pageSize": 2000}, &res); err == nil {
				for _, i := range res.Items {
					set[strings.TrimSpace(i.Pem)] = true
				}
			}
			existing[d] = set
		}
		for _, id := range req.IDs {
			var c sslInfo
			if err := agentJSON(req.Src, http.MethodGet, "/websites/ssl/"+fmt.Sprint(id), nil, &c); err != nil || c.Pem == "" {
				if err == nil {
					err = fmt.Errorf("certificate %d has no content", id)
				}
				t.logf("read certificate %d failed: %v", id, err)
				if firstErr == nil {
					firstErr = err
				}
				done += len(dsts)
				continue
			}
			for _, d := range dsts {
				switch {
				case existing[d][strings.TrimSpace(c.Pem)]:
					t.logf("%s already on %s, skipped", c.PrimaryDom, d)
				default:
					desc := c.Description
					if desc == "" {
						desc = c.PrimaryDom
					}
					err := agentJSON(d, http.MethodPost, "/websites/ssl/upload", map[string]interface{}{
						"type": "paste", "certificate": c.Pem, "privateKey": c.PrivateKey, "description": desc,
					}, nil)
					if err != nil {
						t.logf("%s -> %s failed: %v", c.PrimaryDom, d, err)
						if firstErr == nil {
							firstErr = err
						}
					} else {
						t.logf("%s -> %s ok", c.PrimaryDom, d)
					}
				}
				done++
				t.progress(done * 100 / total)
			}
		}
		t.finish(firstErr)
	}()
	return t, nil
}
