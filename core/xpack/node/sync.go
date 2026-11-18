package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const fallbackSyncTmp = "/opt/1panel/tmp/sync"

type SyncTask struct {
	ID        string             `json:"id"`
	Kind      string             `json:"kind"`
	Name      string             `json:"name"`
	Src       string             `json:"src"`
	Dsts      []string           `json:"dsts"`
	Status    string             `json:"status"` // running | success | failed | cancelled
	Progress  int                `json:"progress"`
	Message   string             `json:"message"`
	Logs      []string           `json:"logs"`
	CreatedAt time.Time          `json:"createdAt"`
	EndedAt   time.Time          `json:"endedAt"`
	ctx       context.Context    `json:"-"`
	cancel    context.CancelFunc `json:"-"`
}

var (
	syncMu    sync.Mutex
	syncTasks = map[string]*SyncTask{}
)

func normNode(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "local"
	}
	return name
}

func normTargets(src string, raw []string) []string {
	seen := map[string]bool{src: true}
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		nd := normNode(d)
		if !seen[nd] {
			seen[nd] = true
			out = append(out, nd)
		}
	}
	return out
}

func newTask(kind, name, src string, dsts []string) *SyncTask {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	ctx, cancel := context.WithCancel(context.Background())
	t := &SyncTask{
		ID:        hex.EncodeToString(b),
		Kind:      kind,
		Name:      name,
		Src:       src,
		Dsts:      dsts,
		Status:    "running",
		CreatedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
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
	if t.cancel != nil {
		t.cancel()
	}
	if t.Status == "cancelled" {
		if t.EndedAt.IsZero() {
			t.EndedAt = time.Now()
		}
		return
	}
	t.EndedAt = time.Now()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			t.Status, t.Message = "cancelled", "cancelled"
			t.Logs = append(t.Logs, time.Now().Format("15:04:05 ")+"cancelled")
			return
		}
		t.Status, t.Message = "failed", err.Error()
		t.Logs = append(t.Logs, time.Now().Format("15:04:05 ")+"failed: "+err.Error())
		return
	}
	t.Status, t.Progress = "success", 100
}

// CancelSyncTask cancels a running sync task by ID.
func CancelSyncTask(id string) error {
	syncMu.Lock()
	t, ok := syncTasks[id]
	if !ok {
		syncMu.Unlock()
		return fmt.Errorf("sync task not found: %s", id)
	}
	if t.Status != "running" {
		syncMu.Unlock()
		return nil
	}
	t.Status = "cancelled"
	t.Message = "cancelled by user"
	t.EndedAt = time.Now()
	t.Logs = append(t.Logs, time.Now().Format("15:04:05 ")+"cancelled by user")
	cancel := t.cancel
	syncMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func SyncTasks() []SyncTask {
	syncMu.Lock()
	defer syncMu.Unlock()
	out := make([]SyncTask, 0, len(syncTasks))
	for _, t := range syncTasks {
		c := *t
		c.Logs = append([]string(nil), t.Logs...)
		c.ctx, c.cancel = nil, nil
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

// agentDoCtx sends one context-bound request to the named node's agent; the body streams through.
func agentDoCtx(ctx context.Context, node, method, p string, hdr http.Header, body io.Reader) (*http.Response, error) {
	node = normNode(node)
	var (
		tr      http.RoundTripper
		host    string
		scheme  = "http"
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
	req, err := http.NewRequestWithContext(ctx, method, scheme+"://"+host+"/api/v2"+p, body)
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

func agentDo(node, method, p string, hdr http.Header, body io.Reader) (*http.Response, error) {
	return agentDoCtx(context.Background(), node, method, p, hdr, body)
}

type agentRes struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// agentJSONCtx calls a JSON endpoint with context and unwraps the {code,message,data} envelope.
func agentJSONCtx(ctx context.Context, node, method, p string, in interface{}, out interface{}) error {
	var rd io.Reader
	hdr := http.Header{}
	if in != nil {
		b, _ := json.Marshal(in)
		rd = bytes.NewReader(b)
		hdr.Set("Content-Type", "application/json")
	}
	resp, err := agentDoCtx(ctx, node, method, p, hdr, rd)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
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

func agentJSON(node, method, p string, in interface{}, out interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return agentJSONCtx(ctx, node, method, p, in, out)
}

func nodeSyncTmpDir(parent context.Context, node string) string {
	dir := fallbackSyncTmp
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	var dataDir string
	if err := agentJSONCtx(ctx, node, http.MethodGet, "/settings/basedir", nil, &dataDir); err == nil {
		dataDir = strings.TrimRight(strings.TrimSpace(dataDir), "/")
		if dataDir != "" {
			if strings.HasSuffix(dataDir, "/1panel") {
				dir = path.Join(dataDir, "tmp/sync")
			} else {
				dir = path.Join(dataDir, "1panel/tmp/sync")
			}
		}
	}
	cancel()

	mctx, mcancel := context.WithTimeout(parent, 10*time.Second)
	_ = agentJSONCtx(mctx, node, http.MethodPost, "/files", map[string]interface{}{
		"path": dir, "isDir": true, "mode": 493,
	}, nil)
	mcancel()
	return dir
}

func isJSONErrorResponse(resp *http.Response) bool {
	return strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") &&
		resp.Header.Get("Content-Disposition") == ""
}

// remoteSize returns the size of a file on a node via a one-byte range read, or -1.
func remoteSize(ctx context.Context, node, p string) int64 {
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	hdr := http.Header{"Range": []string{"bytes=0-0"}}
	resp, err := agentDoCtx(rctx, node, http.MethodGet, "/files/download?path="+url.QueryEscape(p), hdr, nil)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPartialContent {
		if i := strings.LastIndex(resp.Header.Get("Content-Range"), "/"); i >= 0 {
			if n, err := strconv.ParseInt(resp.Header.Get("Content-Range")[i+1:], 10, 64); err == nil {
				return n
			}
		}
	}
	if resp.StatusCode == http.StatusOK && !isJSONErrorResponse(resp) {
		return resp.ContentLength
	}
	return -1
}

// waitFile blocks until a file exists on the node and stops growing.
func waitFile(ctx context.Context, node, p string, timeout time.Duration) (int64, error) {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	last := int64(-2)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-wctx.Done():
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, fmt.Errorf("timed out waiting for %s on %s", p, node)
		case <-ticker.C:
			n := remoteSize(wctx, node, p)
			if n > 0 && n == last {
				return n, nil
			}
			last = n
		}
	}
}

// waitTask polls an agent task (compress, extract, image save/load) until it ends.
func waitTask(ctx context.Context, node, id string, timeout time.Duration) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-wctx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("timed out waiting for task on %s", node)
		case <-ticker.C:
			var res struct {
				Items []struct {
					Status   string `json:"status"`
					ErrorMsg string `json:"errorMsg"`
				} `json:"items"`
			}
			pctx, pcancel := context.WithTimeout(wctx, 15*time.Second)
			err := agentJSONCtx(pctx, node, http.MethodPost, "/logs/tasks/search", map[string]interface{}{"taskID": id, "page": 1, "pageSize": 1}, &res)
			pcancel()
			if err != nil || len(res.Items) == 0 {
				continue
			}
			switch res.Items[0].Status {
			case "Success":
				return nil
			case "Failed", "Canceled":
				return fmt.Errorf("task failed on %s: %s", node, res.Items[0].ErrorMsg)
			}
		}
	}
}

func removeRemote(node, p string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = agentJSONCtx(ctx, node, http.MethodPost, "/files/del", map[string]interface{}{"path": p, "isDir": false, "forceDelete": true}, nil)
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

// copyFile streams srcPath on src directly into dstDir/name on dst via /xpack/node/sync/stream.
func copyFile(ctx context.Context, src, srcPath, dst, dstDir, rename string, cb func(int)) error {
	tctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	resp, err := agentDoCtx(tctx, src, http.MethodGet, "/files/download?path="+url.QueryEscape(srcPath), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || isJSONErrorResponse(resp) {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("read %s on %s: %s", srcPath, src, strings.TrimSpace(string(b)))
	}
	name := rename
	if name == "" {
		name = path.Base(srcPath)
	}
	dstPath := path.Join(dstDir, name)
	streamPath := "/xpack/node/sync/stream?dst=" + url.QueryEscape(dstPath) + "&mode=0644"
	hdr := http.Header{"Content-Type": []string{"application/octet-stream"}}
	reader := &countReader{r: resp.Body, size: resp.ContentLength, cb: cb}
	up, err := agentDoCtx(tctx, dst, http.MethodPost, streamPath, hdr, reader)
	if err != nil {
		return err
	}
	defer up.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(up.Body, 64<<10))
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
	req.Src = normNode(req.Src)
	req.Dst = normNode(req.Dst)
	if len(req.Items) == 0 || req.DstDir == "" || !strings.HasPrefix(req.DstDir, "/") {
		return nil, fmt.Errorf("source items and an absolute target directory are required")
	}
	if req.Src == req.Dst {
		return nil, fmt.Errorf("source and target must be different nodes")
	}
	t := newTask("file", path.Base(req.Items[0].Path), req.Src, []string{req.Dst})
	go func() {
		var err error
		for i, it := range req.Items {
			if err = t.ctx.Err(); err != nil {
				break
			}
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
		return copyFile(t.ctx, req.Src, it.Path, req.Dst, req.DstDir, "", cb)
	}
	srcTmpDir := nodeSyncTmpDir(t.ctx, req.Src)
	dstTmpDir := nodeSyncTmpDir(t.ctx, req.Dst)
	name := tmpName(".sync-", ".tar.gz")
	t.logf("compress %s on %s", it.Path, req.Src)
	tid := tmpName("sync-", "")
	cctx, ccancel := context.WithTimeout(t.ctx, 30*time.Second)
	err := agentJSONCtx(cctx, req.Src, http.MethodPost, "/files/compress", map[string]interface{}{
		"files": []string{it.Path}, "dst": srcTmpDir, "type": "tar.gz", "name": name, "replace": true, "secret": "", "taskID": tid,
	}, nil)
	ccancel()
	if err != nil {
		return err
	}
	srcTmp := path.Join(srcTmpDir, name)
	defer removeRemote(req.Src, srcTmp)
	if err = waitTask(t.ctx, req.Src, tid, 30*time.Minute); err != nil {
		return err
	}
	t.logf("copy archive -> %s:%s", req.Dst, req.DstDir)
	if err = copyFile(t.ctx, req.Src, srcTmp, req.Dst, dstTmpDir, name, cb); err != nil {
		return err
	}
	dstTmp := path.Join(dstTmpDir, name)
	defer removeRemote(req.Dst, dstTmp)
	t.logf("extract on %s", req.Dst)
	tid = tmpName("sync-", "")
	dctx, dcancel := context.WithTimeout(t.ctx, 30*time.Second)
	err = agentJSONCtx(dctx, req.Dst, http.MethodPost, "/files/decompress", map[string]interface{}{
		"dst": req.DstDir, "type": "tar.gz", "path": dstTmp, "secret": "", "taskID": tid,
	}, nil)
	dcancel()
	if err != nil {
		return err
	}
	return waitTask(t.ctx, req.Dst, tid, 30*time.Minute)
}

// ---- image transfer

type SyncImageReq struct {
	Src    string   `json:"src"`
	Dsts   []string `json:"dsts"`
	Images []string `json:"images"` // tag names such as redis:7.2.4
}

func SyncImages(req SyncImageReq) (*SyncTask, error) {
	req.Src = normNode(req.Src)
	if len(req.Images) == 0 || len(req.Dsts) == 0 {
		return nil, fmt.Errorf("select images and at least one target node")
	}
	dsts := normTargets(req.Src, req.Dsts)
	if len(dsts) == 0 {
		return nil, fmt.Errorf("source node cannot be a target")
	}
	t := newTask("image", strings.Join(req.Images, ", "), req.Src, dsts)
	go func() {
		total := len(req.Images) * len(dsts)
		done := 0
		var firstErr error
		for _, img := range req.Images {
			if err := t.ctx.Err(); err != nil {
				firstErr = err
				break
			}
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

func normImageTag(tag string) string {
	s := strings.TrimSpace(tag)
	s = strings.TrimPrefix(s, "docker.io/")
	s = strings.TrimPrefix(s, "index.docker.io/")
	s = strings.TrimPrefix(s, "library/")
	if s != "" && !strings.Contains(s, ":") && !strings.Contains(s, "@") {
		s += ":latest"
	}
	return s
}

func hasImage(ctx context.Context, node, tag string) bool {
	qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var list []struct {
		Tags []string `json:"tags"`
	}
	if err := agentJSONCtx(qctx, node, http.MethodGet, "/containers/image/all", nil, &list); err != nil {
		return false
	}
	want := normImageTag(tag)
	for _, i := range list {
		for _, tg := range i.Tags {
			if tg == tag || normImageTag(tg) == want {
				return true
			}
		}
	}
	return false
}

func syncOneImage(t *SyncTask, src string, dsts []string, tag string, step func()) error {
	srcTmpDir := nodeSyncTmpDir(t.ctx, src)
	name := tmpName("sync-", "")
	t.logf("export %s on %s", tag, src)
	tid := tmpName("sync-", "")
	sctx, scancel := context.WithTimeout(t.ctx, 30*time.Second)
	err := agentJSONCtx(sctx, src, http.MethodPost, "/containers/image/save", map[string]interface{}{
		"tagName": tag, "path": srcTmpDir, "name": name, "taskID": tid,
	}, nil)
	scancel()
	if err != nil {
		return err
	}
	tarPath := path.Join(srcTmpDir, name+".tar")
	defer removeRemote(src, tarPath)
	if err := waitTask(t.ctx, src, tid, 30*time.Minute); err != nil {
		return err
	}
	var firstErr error
	for _, d := range dsts {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		err := func() error {
			dstTmpDir := nodeSyncTmpDir(t.ctx, d)
			t.logf("send %s -> %s", tag, d)
			if err := copyFile(t.ctx, src, tarPath, d, dstTmpDir, name+".tar", nil); err != nil {
				return err
			}
			dstTar := path.Join(dstTmpDir, name+".tar")
			defer removeRemote(d, dstTar)
			t.logf("load on %s", d)
			lid := tmpName("sync-", "")
			lctx, lcancel := context.WithTimeout(t.ctx, 30*time.Second)
			err := agentJSONCtx(lctx, d, http.MethodPost, "/containers/image/load", map[string]interface{}{"paths": []string{dstTar}, "taskID": lid}, nil)
			lcancel()
			if err != nil {
				return err
			}
			if err := waitTask(t.ctx, d, lid, 30*time.Minute); err != nil {
				return err
			}
			for i := 0; i < 150; i++ { // up to ~5 min
				if hasImage(t.ctx, d, tag) {
					return nil
				}
				select {
				case <-t.ctx.Done():
					return t.ctx.Err()
				case <-time.After(2 * time.Second):
				}
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
	ID            uint      `json:"id"`
	PrimaryDomain string    `json:"primaryDomain"`
	Domains       string    `json:"domains"`
	Pem           string    `json:"pem"`
	PrivateKey    string    `json:"privateKey"`
	Organization  string    `json:"organization"`
	Type          string    `json:"type"`
	KeyType       string    `json:"keyType"`
	Description   string    `json:"description"`
	ExpireDate    time.Time `json:"expireDate"`
	StartDate     time.Time `json:"startDate"`
}

func SyncCerts(req SyncCertReq) (*SyncTask, error) {
	req.Src = normNode(req.Src)
	if len(req.IDs) == 0 || len(req.Dsts) == 0 {
		return nil, fmt.Errorf("select certificates and at least one target node")
	}
	dsts := normTargets(req.Src, req.Dsts)
	if len(dsts) == 0 {
		return nil, fmt.Errorf("source node cannot be a target")
	}
	t := newTask("cert", fmt.Sprintf("%d certificate(s)", len(req.IDs)), req.Src, dsts)
	go func() {
		var firstErr error
		total := len(req.IDs) * len(dsts)
		done := 0
		for _, id := range req.IDs {
			if err := t.ctx.Err(); err != nil {
				firstErr = err
				break
			}
			var c sslInfo
			rctx, rcancel := context.WithTimeout(t.ctx, 20*time.Second)
			err := agentJSONCtx(rctx, req.Src, http.MethodGet, "/websites/ssl/"+fmt.Sprint(id), nil, &c)
			rcancel()
			if err != nil || c.Pem == "" {
				if err == nil {
					err = fmt.Errorf("certificate %d has no content", id)
				}
				t.logf("read certificate %d failed: %v", id, err)
				if firstErr == nil {
					firstErr = err
				}
				done += len(dsts)
				t.progress(done * 100 / total)
				continue
			}
			masterID := uint(0)
			if isLocal(req.Src) {
				masterID = c.ID
			}
			payload := map[string]interface{}{
				"masterSslId":   masterID,
				"primaryDomain": c.PrimaryDomain,
				"domains":       c.Domains,
				"pem":           c.Pem,
				"privateKey":    c.PrivateKey,
				"organization":  c.Organization,
				"type":          c.Type,
				"keyType":       c.KeyType,
				"description":   c.Description,
				"expireDate":    c.ExpireDate,
				"startDate":     c.StartDate,
			}
			for _, d := range dsts {
				if err := t.ctx.Err(); err != nil {
					firstErr = err
					break
				}
				pctx, pcancel := context.WithTimeout(t.ctx, 30*time.Second)
				err := agentJSONCtx(pctx, d, http.MethodPost, "/xpack/node/ssl-sync", payload, nil)
				pcancel()
				if err != nil {
					t.logf("%s -> %s failed: %v", c.PrimaryDomain, d, err)
					if firstErr == nil {
						firstErr = err
					}
				} else {
					t.logf("%s -> %s ok", c.PrimaryDomain, d)
				}
				done++
				t.progress(done * 100 / total)
			}
		}
		t.finish(firstErr)
	}()
	return t, nil
}

// ---- app transfer

type SyncAppReq struct {
	Src  string   `json:"src"`
	Dsts []string `json:"dsts"`
	Name string   `json:"name"`
	Stop bool     `json:"stop"` // stop the source app while its files are copied
}

type appMeta struct {
	Install json.RawMessage `json:"install"`
	AppKey  string          `json:"appKey"`
	Dir     string          `json:"dir"`
	Images  []string        `json:"images"`
}

func SyncApp(req SyncAppReq) (*SyncTask, error) {
	req.Src = normNode(req.Src)
	if req.Name == "" || len(req.Dsts) == 0 {
		return nil, fmt.Errorf("select an app and at least one target node")
	}
	dsts := normTargets(req.Src, req.Dsts)
	if len(dsts) == 0 {
		return nil, fmt.Errorf("source node cannot be a target")
	}
	t := newTask("app", req.Name, req.Src, dsts)
	go func() {
		t.finish(runSyncApp(t, req, dsts))
	}()
	return t, nil
}

func runSyncApp(t *SyncTask, req SyncAppReq, dsts []string) error {
	var m appMeta
	mctx, mcancel := context.WithTimeout(t.ctx, 30*time.Second)
	err := agentJSONCtx(mctx, req.Src, http.MethodGet, "/xpack/node/app/meta?name="+url.QueryEscape(req.Name), nil, &m)
	mcancel()
	if err != nil {
		return err
	}

	var inst struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(m.Install, &inst)
	wasRunning := inst.Status == "Running"

	validDsts := make([]string, 0, len(dsts))
	dstParents := make(map[string]string, len(dsts))
	var firstErr error
	for _, dst := range dsts {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		cctx, ccancel := context.WithTimeout(t.ctx, 20*time.Second)
		existsErr := agentJSONCtx(cctx, dst, http.MethodGet, "/xpack/node/app/meta?name="+url.QueryEscape(req.Name), nil, nil)
		ccancel()
		if existsErr == nil {
			err := fmt.Errorf("an app named %s already exists on %s", req.Name, dst)
			t.logf("%s -> %s failed: %v", req.Name, dst, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		var parent string
		pctx, pcancel := context.WithTimeout(t.ctx, 20*time.Second)
		err := agentJSONCtx(pctx, dst, http.MethodGet, "/xpack/node/app/dir?key="+url.QueryEscape(m.AppKey), nil, &parent)
		pcancel()
		if err != nil {
			t.logf("%s -> %s failed: %v", req.Name, dst, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		validDsts = append(validDsts, dst)
		dstParents[dst] = parent
	}
	if len(validDsts) == 0 {
		return firstErr
	}

	for _, img := range m.Images {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		needImg := make([]string, 0, len(validDsts))
		for _, dst := range validDsts {
			if !hasImage(t.ctx, dst, img) {
				needImg = append(needImg, dst)
			}
		}
		if len(needImg) == 0 {
			continue
		}
		t.logf("sync image %s -> %s", img, strings.Join(needImg, ", "))
		if err := syncOneImage(t, req.Src, needImg, img, func() {}); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if req.Stop && wasRunning {
		t.logf("stop %s on %s", req.Name, req.Src)
		sctx, scancel := context.WithTimeout(t.ctx, 60*time.Second)
		stopErr := agentJSONCtx(sctx, req.Src, http.MethodPost, "/xpack/node/app/compose", map[string]string{"name": req.Name, "op": "stop"}, nil)
		scancel()
		if stopErr != nil {
			return stopErr
		}
		defer func() {
			t.logf("start %s on %s", req.Name, req.Src)
			stctx, stcancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer stcancel()
			_ = agentJSONCtx(stctx, req.Src, http.MethodPost, "/xpack/node/app/compose", map[string]string{"name": req.Name, "op": "start"}, nil)
		}()
	}

	for i, dst := range validDsts {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		parent := dstParents[dst]
		t.logf("copy app files %s -> %s:%s", m.Dir, dst, parent)
		fr := SyncFileReq{Src: req.Src, Dst: dst, DstDir: parent}
		if err := syncOneFile(t, fr, SyncFileItem{Path: m.Dir, IsDir: true}, func(int) {}); err != nil {
			t.logf("%s -> %s failed: %v", req.Name, dst, err)
			if firstErr == nil {
				firstErr = err
			}
			t.progress((i + 1) * 100 / len(validDsts))
			continue
		}
		t.logf("register on %s", dst)
		ictx, icancel := context.WithTimeout(t.ctx, 2*time.Minute)
		err := agentJSONCtx(ictx, dst, http.MethodPost, "/xpack/node/app/import", map[string]interface{}{
			"install": m.Install, "appKey": m.AppKey, "start": wasRunning,
		}, nil)
		icancel()
		if err != nil {
			t.logf("%s -> %s failed: %v", req.Name, dst, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			t.logf("%s -> %s ok", req.Name, dst)
		}
		t.progress((i + 1) * 100 / len(validDsts))
	}
	return firstErr
}
