package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// privilegeStore is deliberately owned by the unprivileged workbench. Native
// elevated helpers return a result only; they never create files in a user's
// configuration directory.
type privilegeStore struct {
	mu  sync.Mutex
	db  *sql.DB
	key []byte
}

type networkPlan struct {
	ID           string            `json:"id"`
	Operation    string            `json:"operation"`
	Params       map[string]string `json:"params"`
	Fingerprint  string            `json:"fingerprint"`
	Risk         string            `json:"risk"`
	Impact       string            `json:"impact"`
	Verification []string          `json:"verification"`
	CreatedAt    string            `json:"createdAt"`
	ExpiresAt    string            `json:"expiresAt"`
}

type privilegeAuditStatus struct {
	Valid          bool   `json:"valid"`
	PolicyVersion  string `json:"policyVersion"`
	LegacyImported bool   `json:"legacyImported"`
	Reason         string `json:"reason,omitempty"`
}

// privilegeIntent is an ephemeral, host-issued authorization ticket. It binds
// a browser's confirmed operation to the authenticated account and request
// source without ever storing a password. The host's privilege mode decides
// whether that source must be loopback.
type privilegeIntent struct {
	ID            string
	PluginID      string
	Action        string
	PlanID        string
	Account       string
	Source        string
	Authorization string
	ExpiresAt     time.Time
}

func newPrivilegeStoreAt(directory string) (*privilegeStore, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(directory, "privilege-audit.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		if err = os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("权限审计密钥无效")
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "privilege.db"))
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS privilege_plans(id TEXT PRIMARY KEY, operation TEXT NOT NULL, params BLOB NOT NULL, fingerprint TEXT NOT NULL, risk TEXT NOT NULL, impact TEXT NOT NULL, verification BLOB NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, account TEXT NOT NULL, used_at TEXT);
CREATE TABLE IF NOT EXISTS privilege_audit(sequence INTEGER PRIMARY KEY AUTOINCREMENT, operation TEXT NOT NULL, params BLOB NOT NULL, output TEXT NOT NULL, account TEXT NOT NULL, created_at TEXT NOT NULL, previous_hash TEXT NOT NULL, chain_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sudo_attempts(key TEXT PRIMARY KEY, failures INTEGER NOT NULL, locked_until TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS privilege_intents(id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL, action TEXT NOT NULL, plan_id TEXT NOT NULL, account TEXT NOT NULL, source TEXT NOT NULL, authorization TEXT NOT NULL, expires_at TEXT NOT NULL, used_at TEXT);
CREATE TABLE IF NOT EXISTS privilege_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &privilegeStore{db: db, key: key}
	store.migrateLegacyNetworkAudit()
	return store, nil
}

func (s *privilegeStore) createIntent(pluginID, action, planID, account, source, authorization string) (privilegeIntent, error) {
	if s == nil || s.db == nil {
		return privilegeIntent{}, errors.New("权限审计存储不可用")
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return privilegeIntent{}, err
	}
	intent := privilegeIntent{ID: hex.EncodeToString(bytes), PluginID: pluginID, Action: action, PlanID: planID, Account: account, Source: source, Authorization: authorization, ExpiresAt: time.Now().Add(5 * time.Minute).UTC()}
	if _, err := s.db.Exec(`DELETE FROM privilege_intents WHERE expires_at < ? OR used_at IS NOT NULL`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return privilegeIntent{}, err
	}
	_, err := s.db.Exec(`INSERT INTO privilege_intents(id,plugin_id,action,plan_id,account,source,authorization,expires_at) VALUES(?,?,?,?,?,?,?,?)`, intent.ID, intent.PluginID, intent.Action, intent.PlanID, intent.Account, intent.Source, intent.Authorization, intent.ExpiresAt.Format(time.RFC3339Nano))
	return intent, err
}

func (s *privilegeStore) validateIntent(id, pluginID, action, planID, account, source, authorization string) (privilegeIntent, error) {
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return privilegeIntent{}, errors.New("请先在工作台确认系统权限请求")
	}
	var intent privilegeIntent
	var expires string
	var used sql.NullString
	err := s.db.QueryRow(`SELECT plugin_id,action,plan_id,account,source,authorization,expires_at,used_at FROM privilege_intents WHERE id=?`, id).Scan(&intent.PluginID, &intent.Action, &intent.PlanID, &intent.Account, &intent.Source, &intent.Authorization, &expires, &used)
	if err != nil {
		return privilegeIntent{}, errors.New("权限请求已失效，请重新确认")
	}
	intent.ID = id
	intent.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil || used.Valid || time.Now().After(intent.ExpiresAt) {
		return privilegeIntent{}, errors.New("权限请求已过期或已使用，请重新确认")
	}
	if intent.PluginID != pluginID || intent.Action != action || intent.PlanID != planID || intent.Account != account || intent.Source != source || intent.Authorization != authorization {
		return privilegeIntent{}, errors.New("权限请求与当前操作不匹配，请重新确认")
	}
	return intent, nil
}

func (s *privilegeStore) consumeIntent(intent privilegeIntent) error {
	if s == nil || s.db == nil {
		return errors.New("权限审计存储不可用")
	}
	result, err := s.db.Exec(`UPDATE privilege_intents SET used_at=? WHERE id=? AND used_at IS NULL AND expires_at >= ?`, time.Now().UTC().Format(time.RFC3339Nano), intent.ID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("权限请求已过期或已使用，请重新确认")
	}
	return nil
}

func (s *privilegeStore) migrateLegacyNetworkAudit() {
	if s == nil || s.db == nil {
		return
	}
	var complete string
	_ = s.db.QueryRow(`SELECT value FROM privilege_meta WHERE key='legacy_network_audit'`).Scan(&complete)
	if complete != "" {
		return
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(config, "alx-network", "audit.json"))
	if errors.Is(err, os.ErrNotExist) {
		_, _ = s.db.Exec(`INSERT INTO privilege_meta(key,value) VALUES('legacy_network_audit','none')`)
		return
	}
	if err != nil {
		_, _ = s.db.Exec(`INSERT INTO privilege_meta(key,value) VALUES('legacy_network_audit','unreadable')`)
		return
	}
	var entries []struct {
		Operation string            `json:"operation"`
		Params    map[string]string `json:"params"`
		Output    string            `json:"output"`
		CreatedAt string            `json:"createdAt"`
	}
	if json.Unmarshal(data, &entries) != nil {
		_, _ = s.db.Exec(`INSERT INTO privilege_meta(key,value) VALUES('legacy_network_audit','invalid')`)
		return
	}
	for _, entry := range entries {
		if allowedNetworkOperation(entry.Operation) {
			_ = s.appendAudit(entry.Operation, entry.Params, "[legacy] "+entry.Output, "legacy-import")
		}
	}
	_, _ = s.db.Exec(`INSERT INTO privilege_meta(key,value) VALUES('legacy_network_audit','imported')`)
}

func (s *privilegeStore) checkSudoAttempt(key string) error {
	if s == nil || s.db == nil {
		return nil
	}
	var locked string
	err := s.db.QueryRow(`SELECT locked_until FROM sudo_attempts WHERE key=?`, key).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	until, err := time.Parse(time.RFC3339Nano, locked)
	if err == nil && time.Now().Before(until) {
		return errors.New("密码连续错误次数过多，请在 10 分钟后再试")
	}
	if err == nil {
		_, _ = s.db.Exec(`DELETE FROM sudo_attempts WHERE key=?`, key)
	}
	return nil
}

func (s *privilegeStore) recordSudoFailure(key string) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var failures int
	_ = s.db.QueryRow(`SELECT failures FROM sudo_attempts WHERE key=?`, key).Scan(&failures)
	failures++
	locked := ""
	if failures >= 3 {
		failures, locked = 0, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`INSERT INTO sudo_attempts(key,failures,locked_until) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET failures=excluded.failures,locked_until=excluded.locked_until`, key, failures, locked)
	return err
}

func (s *privilegeStore) clearSudoAttempt(key string) {
	if s != nil && s.db != nil {
		_, _ = s.db.Exec(`DELETE FROM sudo_attempts WHERE key=?`, key)
	}
}

func (s *privilegeStore) close() {
	if s != nil && s.db != nil {
		_ = s.db.Close()
	}
}

func allowedNetworkOperation(operation string) bool {
	switch operation {
	case "set-npm-registry", "reset-npm-registry", "open-port", "close-port", "iface-up", "iface-down", "ip-add", "ip-remove", "dns-set", "mtu-set", "route-add", "route-remove", "forward-add", "forward-remove", "bond-create", "bond-delete", "bridge-create", "bridge-delete", "vlan-create", "vlan-delete", "firewalld-service-add", "firewalld-service-remove", "firewalld-zone-set-default":
		return true
	default:
		return false
	}
}

func (s *privilegeStore) saveNetworkPlan(data json.RawMessage, account string) (json.RawMessage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("权限审计存储不可用")
	}
	var plan networkPlan
	if err := json.Unmarshal(data, &plan); err != nil || !allowedNetworkOperation(plan.Operation) || plan.Fingerprint == "" {
		return nil, errors.New("网络插件返回的变更计划无效")
	}
	if _, err := time.Parse(time.RFC3339, plan.ExpiresAt); err != nil {
		return nil, errors.New("网络变更计划缺少有效过期时间")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	plan.ID = hex.EncodeToString(id)
	params, _ := json.Marshal(plan.Params)
	verification, _ := json.Marshal(plan.Verification)
	if _, err := s.db.Exec(`INSERT INTO privilege_plans(id,operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account) VALUES(?,?,?,?,?,?,?,?,?,?)`, plan.ID, plan.Operation, params, plan.Fingerprint, plan.Risk, plan.Impact, verification, plan.CreatedAt, plan.ExpiresAt, account); err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func (s *privilegeStore) consumeNetworkPlan(id, account string) (networkPlan, error) {
	var plan networkPlan
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return plan, errors.New("未找到宿主签发的网络变更计划")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var params, verification []byte
	var expiresAt string
	var usedAt sql.NullString
	var owner string
	err := s.db.QueryRow(`SELECT operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account,used_at FROM privilege_plans WHERE id=?`, id).Scan(&plan.Operation, &params, &plan.Fingerprint, &plan.Risk, &plan.Impact, &verification, &plan.CreatedAt, &expiresAt, &owner, &usedAt)
	if err != nil {
		return plan, errors.New("未找到宿主签发的网络变更计划")
	}
	if usedAt.Valid && usedAt.String != "" {
		return plan, errors.New("网络变更计划已使用，请重新预演")
	}
	if owner != account {
		return plan, errors.New("网络变更计划只能由创建它的账户执行")
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().After(expires) {
		return plan, errors.New("网络变更计划已过期，请重新预演")
	}
	if json.Unmarshal(params, &plan.Params) != nil || json.Unmarshal(verification, &plan.Verification) != nil || !allowedNetworkOperation(plan.Operation) {
		return plan, errors.New("网络变更计划损坏")
	}
	plan.ID, plan.ExpiresAt = id, expiresAt
	if _, err := s.db.Exec(`UPDATE privilege_plans SET used_at=? WHERE id=? AND used_at IS NULL`, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return plan, err
	}
	return plan, nil
}

// peekNetworkPlan validates a plan for a preflight without consuming it. The
// actual network mutation still consumes the plan only after authorization.
func (s *privilegeStore) peekNetworkPlan(id, account string) (networkPlan, error) {
	var plan networkPlan
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return plan, errors.New("未找到宿主签发的网络变更计划")
	}
	var params, verification []byte
	var expiresAt, owner string
	var usedAt sql.NullString
	err := s.db.QueryRow(`SELECT operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account,used_at FROM privilege_plans WHERE id=?`, id).Scan(&plan.Operation, &params, &plan.Fingerprint, &plan.Risk, &plan.Impact, &verification, &plan.CreatedAt, &expiresAt, &owner, &usedAt)
	if err != nil || owner != account || (usedAt.Valid && usedAt.String != "") {
		return plan, errors.New("未找到可用的宿主签发网络变更计划")
	}
	expires, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil || time.Now().After(expires) || json.Unmarshal(params, &plan.Params) != nil || json.Unmarshal(verification, &plan.Verification) != nil || !allowedNetworkOperation(plan.Operation) {
		return plan, errors.New("网络变更计划已过期或损坏，请重新预演")
	}
	plan.ID, plan.ExpiresAt = id, expiresAt
	return plan, nil
}

// releaseNetworkPlan is used only when sudo rejects a password before the
// reviewed runner starts. Other execution failures may have changed the
// system and therefore intentionally keep the plan consumed.
func (s *privilegeStore) releaseNetworkPlan(id, account string) {
	if s == nil || s.db == nil || id == "" {
		return
	}
	_, _ = s.db.Exec(`UPDATE privilege_plans SET used_at=NULL WHERE id=? AND account=?`, id, account)
}

func inverseNetworkOperation(operation string) string {
	return map[string]string{"open-port": "close-port", "close-port": "open-port", "iface-up": "iface-down", "iface-down": "iface-up", "ip-add": "ip-remove", "ip-remove": "ip-add", "route-add": "route-remove", "route-remove": "route-add", "forward-add": "forward-remove", "forward-remove": "forward-add", "bond-create": "bond-delete", "bridge-create": "bridge-delete", "vlan-create": "vlan-delete", "firewalld-service-add": "firewalld-service-remove", "firewalld-service-remove": "firewalld-service-add"}[operation]
}

func (s *privilegeStore) latestUndoPlan() (networkPlan, error) {
	var operation string
	var params []byte
	if s == nil || s.db == nil {
		return networkPlan{}, errors.New("权限审计存储不可用")
	}
	if err := s.db.QueryRow(`SELECT operation,params FROM privilege_audit ORDER BY sequence DESC LIMIT 1`).Scan(&operation, &params); err != nil {
		return networkPlan{}, errors.New("没有可撤销的网络变更")
	}
	inverse := inverseNetworkOperation(operation)
	if inverse == "" {
		return networkPlan{}, errors.New("最近网络变更不支持自动撤销")
	}
	plan := networkPlan{Operation: inverse, Params: map[string]string{}}
	if err := json.Unmarshal(params, &plan.Params); err != nil {
		return networkPlan{}, errors.New("网络审计记录损坏")
	}
	return plan, nil
}

func (s *privilegeStore) appendAudit(operation string, params map[string]string, output, account string) error {
	if s == nil || s.db == nil {
		return errors.New("权限审计存储不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	previous := ""
	_ = s.db.QueryRow(`SELECT chain_hash FROM privilege_audit ORDER BY sequence DESC LIMIT 1`).Scan(&previous)
	created := time.Now().UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(strings.Join([]string{previous, operation, string(encoded), output, account, created}, "\x00")))
	chain := hex.EncodeToString(mac.Sum(nil))
	_, err = s.db.Exec(`INSERT INTO privilege_audit(operation,params,output,account,created_at,previous_hash,chain_hash) VALUES(?,?,?,?,?,?,?)`, operation, encoded, output, account, created, previous, chain)
	return err
}

func (s *privilegeStore) auditStatus(policyVersion string) privilegeAuditStatus {
	result := privilegeAuditStatus{Valid: true, PolicyVersion: policyVersion}
	if s == nil || s.db == nil {
		result.Valid, result.Reason = false, "权限审计存储不可用"
		return result
	}
	var legacy string
	_ = s.db.QueryRow(`SELECT value FROM privilege_meta WHERE key='legacy_network_audit'`).Scan(&legacy)
	result.LegacyImported = legacy == "imported"
	if legacy == "unreadable" {
		result.Reason = "旧网络审计文件无法读取，未自动修改其权限"
	}
	rows, err := s.db.Query(`SELECT operation,params,output,account,created_at,previous_hash,chain_hash FROM privilege_audit ORDER BY sequence ASC`)
	if err != nil {
		result.Valid, result.Reason = false, err.Error()
		return result
	}
	defer rows.Close()
	previous := ""
	for rows.Next() {
		var operation, output, account, created, previousHash, chain string
		var params []byte
		if err := rows.Scan(&operation, &params, &output, &account, &created, &previousHash, &chain); err != nil || previousHash != previous {
			result.Valid, result.Reason = false, "权限审计链不连续"
			return result
		}
		mac := hmac.New(sha256.New, s.key)
		_, _ = mac.Write([]byte(strings.Join([]string{previous, operation, string(params), output, account, created}, "\x00")))
		if !hmac.Equal([]byte(chain), []byte(hex.EncodeToString(mac.Sum(nil)))) {
			result.Valid, result.Reason = false, "权限审计链校验失败"
			return result
		}
		previous = chain
	}
	return result
}

func (s *privilegeStore) auditItems() ([]map[string]any, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("权限审计存储不可用")
	}
	rows, err := s.db.Query(`SELECT sequence,operation,params,output,created_at FROM privilege_audit ORDER BY sequence DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var sequence int
		var operation, output, created string
		var params json.RawMessage
		if err := rows.Scan(&sequence, &operation, &params, &output, &created); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": fmt.Sprintf("%d", sequence), "operation": operation, "params": params, "output": output, "createdAt": created, "undoOperation": inverseNetworkOperation(operation)})
	}
	return items, rows.Err()
}
