package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/global"
)

// Alerting: the master polls every node's agent, applies a small fixed rule set and
// pushes notifications to Telegram.

type AlertRule struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold"`
}

type AlertTelegram struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
	ChatID  string `json:"chatId"`
	Proxy   string `json:"proxy"` // optional http(s)/socks5 proxy for reaching api.telegram.org
}

type AlertConfig struct {
	Telegram    AlertTelegram        `json:"telegram"`
	Interval    int                  `json:"interval"`    // seconds between checks
	CooldownMin int                  `json:"cooldownMin"` // minutes before the same alert repeats
	Rules       map[string]AlertRule `json:"rules"`
}

type AlertRecord struct {
	Time    time.Time `json:"time"`
	Rule    string    `json:"rule"`
	Node    string    `json:"node"`
	Message string    `json:"message"`
	Sent    bool      `json:"sent"`
	Error   string    `json:"error"`
}

var alertDefaults = map[string]AlertRule{
	"cert_expire":  {true, 7},
	"site_5xx":     {true, 50},
	"waf_attack":   {true, 200},
	"cron_failed":  {true, 0},
	"app_error":    {true, 0},
	"node_offline": {true, 0},
	"sync_failed":  {true, 0},
	"site_tamper":  {true, 0},
}

var (
	alertMu    sync.Mutex
	alertCfg   *AlertConfig
	alertRecs  []AlertRecord
	alertFired = map[string]time.Time{}
	alertPrev  = map[string]int64{}
	alertSince = time.Now()
)

func alertDir() string { return path.Join(global.CONF.Base.InstallDir, "1panel", "alert") }

func loadAlertCfgLocked() *AlertConfig {
	if alertCfg != nil {
		return alertCfg
	}
	c := &AlertConfig{Interval: 60, CooldownMin: 30, Rules: map[string]AlertRule{}}
	if b, err := os.ReadFile(path.Join(alertDir(), "config.json")); err == nil {
		_ = json.Unmarshal(b, c)
	}
	if c.Rules == nil {
		c.Rules = map[string]AlertRule{}
	}
	for id, d := range alertDefaults {
		if _, ok := c.Rules[id]; !ok {
			c.Rules[id] = d
		}
	}
	if c.Interval < 30 {
		c.Interval = 60
	}
	if c.CooldownMin < 1 {
		c.CooldownMin = 30
	}
	alertCfg = c
	if b, err := os.ReadFile(path.Join(alertDir(), "records.json")); err == nil {
		_ = json.Unmarshal(b, &alertRecs)
	}
	return c
}

func GetAlertConfig() AlertConfig {
	alertMu.Lock()
	defer alertMu.Unlock()
	c := *loadAlertCfgLocked()
	rules := map[string]AlertRule{}
	for k, v := range c.Rules {
		rules[k] = v
	}
	c.Rules = rules
	if c.Telegram.Token != "" { // never hand the token back to the browser
		c.Telegram.Token = "********"
	}
	return c
}

func SaveAlertConfig(in AlertConfig) error {
	alertMu.Lock()
	defer alertMu.Unlock()
	c := loadAlertCfgLocked()
	if in.Telegram.Token == "" || in.Telegram.Token == "********" {
		in.Telegram.Token = c.Telegram.Token
	}
	in.Telegram.Token = strings.TrimSpace(in.Telegram.Token)
	in.Telegram.ChatID = strings.TrimSpace(in.Telegram.ChatID)
	in.Telegram.Proxy = strings.TrimSpace(in.Telegram.Proxy)
	if in.Interval < 30 {
		in.Interval = 30
	}
	if in.CooldownMin < 1 {
		in.CooldownMin = 1
	}
	if in.Rules == nil {
		in.Rules = map[string]AlertRule{}
	}
	for id, d := range alertDefaults {
		if _, ok := in.Rules[id]; !ok {
			in.Rules[id] = d
		}
	}
	if err := os.MkdirAll(alertDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(in, "", "  ")
	if err := os.WriteFile(path.Join(alertDir(), "config.json"), b, 0o600); err != nil {
		return err
	}
	*c = in
	return nil
}

func AlertRecords() []AlertRecord {
	alertMu.Lock()
	defer alertMu.Unlock()
	loadAlertCfgLocked()
	out := make([]AlertRecord, len(alertRecs))
	for i, r := range alertRecs {
		out[len(alertRecs)-1-i] = r // newest first
	}
	return out
}

func ClearAlertRecords() {
	alertMu.Lock()
	defer alertMu.Unlock()
	loadAlertCfgLocked()
	alertRecs = nil
	_ = os.Remove(path.Join(alertDir(), "records.json"))
}

func tgClient(proxy string) *http.Client {
	tr := &http.Transport{}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Second}
}

func tgCall(tg AlertTelegram, method string, body map[string]interface{}, out interface{}) error {
	if tg.Token == "" {
		return fmt.Errorf("bot token is empty")
	}
	b, _ := json.Marshal(body)
	resp, err := tgClient(tg.Proxy).Post("https://api.telegram.org/bot"+tg.Token+"/"+method, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("telegram unreachable: %s", strings.ReplaceAll(err.Error(), tg.Token, "***"))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.OK {
		return fmt.Errorf("telegram: %s", r.Description)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// effectiveTelegram lets a masked/empty token fall back to the saved one.
func effectiveTelegram(in AlertTelegram) AlertTelegram {
	alertMu.Lock()
	defer alertMu.Unlock()
	if in.Token == "" || in.Token == "********" {
		in.Token = loadAlertCfgLocked().Telegram.Token
	}
	in.Token = strings.TrimSpace(in.Token)
	return in
}

func TestTelegram(in AlertTelegram) error {
	tg := effectiveTelegram(in)
	if strings.TrimSpace(tg.ChatID) == "" {
		return fmt.Errorf("chat id is empty")
	}
	return tgCall(tg, "sendMessage", map[string]interface{}{"chat_id": strings.TrimSpace(tg.ChatID), "text": "✅ 1Panel 告警通知测试：连接正常"}, nil)
}

// DiscoverChatIDs lists chats that recently messaged the bot, so the id need not be looked up by hand.
func DiscoverChatIDs(in AlertTelegram) ([]map[string]string, error) {
	tg := effectiveTelegram(in)
	var ups []struct {
		Message struct {
			Chat struct {
				ID       int64  `json:"id"`
				Title    string `json:"title"`
				Username string `json:"username"`
				First    string `json:"first_name"`
			} `json:"chat"`
		} `json:"message"`
	}
	if err := tgCall(tg, "getUpdates", map[string]interface{}{"limit": 50}, &ups); err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	out := []map[string]string{}
	for _, u := range ups {
		c := u.Message.Chat
		if c.ID == 0 || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		name := c.Title
		if name == "" {
			name = strings.TrimSpace(c.First + " @" + c.Username)
		}
		out = append(out, map[string]string{"id": fmt.Sprint(c.ID), "name": name})
	}
	return out, nil
}

// fire records and sends one alert unless the same key fired within the cooldown (or ever, when once).
func fire(rule, node, key, msg string, once bool) {
	alertMu.Lock()
	c := loadAlertCfgLocked()
	last, seen := alertFired[key]
	if seen && (once || time.Since(last) < time.Duration(c.CooldownMin)*time.Minute) {
		alertMu.Unlock()
		return
	}
	alertFired[key] = time.Now()
	tg := c.Telegram
	alertMu.Unlock()

	rec := AlertRecord{Time: time.Now(), Rule: rule, Node: node, Message: msg}
	if tg.Enabled && tg.Token != "" && tg.ChatID != "" {
		text := fmt.Sprintf("🔔 [%s] %s", node, msg)
		if err := tgCall(tg, "sendMessage", map[string]interface{}{"chat_id": tg.ChatID, "text": text}, nil); err != nil {
			rec.Error = err.Error()
		} else {
			rec.Sent = true
		}
	}
	alertMu.Lock()
	alertRecs = append(alertRecs, rec)
	if len(alertRecs) > 500 {
		alertRecs = alertRecs[len(alertRecs)-500:]
	}
	b, _ := json.Marshal(alertRecs)
	_ = os.MkdirAll(alertDir(), 0o700)
	_ = os.WriteFile(path.Join(alertDir(), "records.json"), b, 0o600)
	alertMu.Unlock()
}

func init() { go alertLoop() }

func alertLoop() {
	for global.DB == nil || global.CONF.Base.InstallDir == "" { // the database and config open after package init
		time.Sleep(2 * time.Second)
	}
	time.Sleep(20 * time.Second) // let nodes settle after a restart
	for {
		func() {
			defer func() { _ = recover() }()
			alertRound()
		}()
		alertMu.Lock()
		iv := loadAlertCfgLocked().Interval
		alertMu.Unlock()
		time.Sleep(time.Duration(iv) * time.Second)
	}
}

func ruleOf(id string) AlertRule {
	alertMu.Lock()
	defer alertMu.Unlock()
	return loadAlertCfgLocked().Rules[id]
}

// delta returns how much a cumulative counter grew since the previous round (0 on first sight or reset).
func delta(key string, cur int64) int64 {
	alertMu.Lock()
	defer alertMu.Unlock()
	prev, ok := alertPrev[key]
	alertPrev[key] = cur
	if !ok || cur < prev {
		return 0
	}
	return cur - prev
}

func alertRound() {
	since := alertSince
	alertSince = time.Now()
	nodes, err := List()
	if err != nil {
		return
	}
	if r := ruleOf("node_offline"); r.Enabled {
		for _, n := range nodes {
			if n.Status == statusUnhealthy || n.Status == statusFailed {
				fire("node_offline", n.Name, "offline|"+n.Name, "节点离线："+n.Name, false)
			}
		}
	}
	if r := ruleOf("sync_failed"); r.Enabled {
		for _, t := range SyncTasks() {
			if t.Status == "failed" && t.EndedAt.After(since.Add(-time.Minute)) {
				fire("sync_failed", t.Src, "sync|"+t.ID, fmt.Sprintf("资源同步失败：%s（%s）%s", t.Name, t.Kind, truncate(t.Message, 120)), true)
			}
		}
	}
	for _, n := range nodes {
		if n.Status != statusHealthy {
			continue
		}
		alertNode(n.Name, since)
	}
}

func alertNode(node string, since time.Time) {
	if r := ruleOf("cert_expire"); r.Enabled {
		var res struct {
			Items []struct {
				PrimaryDomain string    `json:"primaryDomain"`
				Description   string    `json:"description"`
				ExpireDate    time.Time `json:"expireDate"`
			} `json:"items"`
		}
		if agentJSON(node, http.MethodPost, "/websites/ssl/search", map[string]interface{}{"page": 1, "pageSize": 1000}, &res) == nil {
			for _, c := range res.Items {
				if c.ExpireDate.Year() < 2000 {
					continue
				}
				days := int(time.Until(c.ExpireDate).Hours() / 24)
				if days <= r.Threshold {
					name := c.PrimaryDomain
					if name == "" {
						name = c.Description
					}
					msg := fmt.Sprintf("证书即将到期：%s 剩余 %d 天", name, days)
					if days < 0 {
						msg = fmt.Sprintf("证书已过期：%s（%d 天前）", name, -days)
					}
					fire("cert_expire", node, "cert|"+node+"|"+name+"|"+c.ExpireDate.Format("20060102"), msg, false)
				}
			}
		}
	}
	if r := ruleOf("site_5xx"); r.Enabled {
		now := time.Now()
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
		var rows []struct {
			Alias string `json:"alias"`
			Err5  int64  `json:"err5"`
		}
		if agentJSON(node, http.MethodPost, "/xpack/monitor/sites", map[string]interface{}{"site": "", "start": day, "end": now.Unix() + 60}, &rows) == nil {
			for _, s := range rows {
				if d := delta("5xx|"+node+"|"+s.Alias, s.Err5); r.Threshold > 0 && d >= int64(r.Threshold) {
					fire("site_5xx", node, "5xx|"+node+"|"+s.Alias, fmt.Sprintf("网站 %s 最近一个周期出现 %d 个 5xx 错误", s.Alias, d), false)
				}
			}
		}
	}
	if r := ruleOf("site_tamper"); r.Enabled {
		key := "tamper|" + node
		alertMu.Lock()
		last, known := alertPrev[key]
		alertMu.Unlock()
		var res struct {
			Items []struct {
				Site, Kind, Path, Action string
				ID                       int64
			} `json:"items"`
			Last int64 `json:"last"`
		}
		if agentJSON(node, http.MethodGet, fmt.Sprintf("/xpack/tamper/events/since?id=%d", last), nil, &res) == nil {
			if known { // the first poll only sets the watermark, so old events are not replayed
				kinds := map[string]string{"modified": "被修改", "added": "新增文件", "deleted": "被删除"}
				acts := map[string]string{"restored": "，已自动恢复", "quarantined": "，已隔离", "restore_failed": "，恢复失败"}
				for _, e := range res.Items {
					fire("site_tamper", node, fmt.Sprintf("tamper|%s|%d", node, e.ID),
						fmt.Sprintf("网站 %s 检测到篡改：%s %s%s", e.Site, e.Path, kinds[e.Kind], acts[e.Action]), true)
				}
			}
			alertMu.Lock()
			alertPrev[key] = res.Last
			alertMu.Unlock()
		}
	}
	if r := ruleOf("waf_attack"); r.Enabled {
		var ov struct {
			Today struct {
				AttackCount int64 `json:"attackCount"`
			} `json:"today"`
		}
		if agentJSON(node, http.MethodGet, "/xpack/waf/overview", nil, &ov) == nil {
			if d := delta("waf|"+node, ov.Today.AttackCount); r.Threshold > 0 && d >= int64(r.Threshold) {
				fire("waf_attack", node, "waf|"+node, fmt.Sprintf("WAF 最近一个周期拦截 %d 次攻击", d), false)
			}
		}
	}
	if r := ruleOf("app_error"); r.Enabled {
		var res struct {
			Items []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"items"`
		}
		if agentJSON(node, http.MethodPost, "/apps/installed/search", map[string]interface{}{"page": 1, "pageSize": 500, "name": "", "type": "", "update": false, "sync": false}, &res) == nil {
			for _, a := range res.Items {
				if a.Status == "Error" || a.Status == "UnHealthy" {
					fire("app_error", node, "app|"+node+"|"+a.Name, fmt.Sprintf("应用异常：%s（%s）", a.Name, a.Status), false)
				}
			}
		}
	}
	if r := ruleOf("cron_failed"); r.Enabled {
		var jobs struct {
			Items []struct {
				ID   uint   `json:"id"`
				Name string `json:"name"`
			} `json:"items"`
		}
		if agentJSON(node, http.MethodPost, "/cronjobs/search", map[string]interface{}{"page": 1, "pageSize": 500, "info": "", "orderBy": "createdAt", "order": "null", "groupIDs": []int{}}, &jobs) == nil {
			for _, j := range jobs.Items {
				var recs struct {
					Items []struct {
						ID      uint   `json:"id"`
						Message string `json:"message"`
					} `json:"items"`
				}
				err := agentJSON(node, http.MethodPost, "/cronjobs/search/records", map[string]interface{}{
					"page": 1, "pageSize": 5, "cronjobID": j.ID, "status": "Failed",
					"startTime": since.Add(-time.Minute).Format("2006-01-02 15:04:05"), "endTime": "2099-01-01 00:00:00",
				}, &recs)
				if err != nil {
					continue
				}
				for _, rc := range recs.Items {
					msg := fmt.Sprintf("计划任务失败：%s", j.Name)
					if rc.Message != "" {
						msg += "（" + truncate(rc.Message, 120) + "）"
					}
					fire("cron_failed", node, fmt.Sprintf("cron|%s|%d|%d", node, j.ID, rc.ID), msg, true)
				}
			}
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
