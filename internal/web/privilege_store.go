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

// privilegedPlan is plugin-owned preview data persisted by the host only to
// bind an approval to one account and one short-lived operation. The host does
// not interpret its operation, risk, or parameters.
type privilegedPlan struct {
	ID           string            `json:"id"`
	PluginID     string            `json:"pluginId,omitempty"`
	Action       string            `json:"action,omitempty"`
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
	Valid         bool   `json:"valid"`
	PolicyVersion string `json:"policyVersion"`
	Reason        string `json:"reason,omitempty"`
}

const (
	privilegeAuditSignatureV1 = 1
	privilegeAuditSignatureV2 = 2
)

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
CREATE TABLE IF NOT EXISTS privilege_plans(id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', action TEXT NOT NULL DEFAULT '', operation TEXT NOT NULL, params BLOB NOT NULL, fingerprint TEXT NOT NULL, risk TEXT NOT NULL, impact TEXT NOT NULL, verification BLOB NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, account TEXT NOT NULL, used_at TEXT);
CREATE TABLE IF NOT EXISTS privilege_audit(sequence INTEGER PRIMARY KEY AUTOINCREMENT, plugin_id TEXT NOT NULL DEFAULT '', action TEXT NOT NULL DEFAULT '', signature_version INTEGER NOT NULL DEFAULT 1, operation TEXT NOT NULL, params BLOB NOT NULL, output TEXT NOT NULL, account TEXT NOT NULL, created_at TEXT NOT NULL, previous_hash TEXT NOT NULL, chain_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sudo_attempts(key TEXT PRIMARY KEY, failures INTEGER NOT NULL, locked_until TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS privilege_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS privilege_intents(id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL, action TEXT NOT NULL, plan_id TEXT NOT NULL, account TEXT NOT NULL, source TEXT NOT NULL, authorization TEXT NOT NULL, expires_at TEXT NOT NULL, used_at TEXT);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &privilegeStore{db: db, key: key}
	_, _ = db.Exec(`ALTER TABLE privilege_plans ADD COLUMN plugin_id TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE privilege_plans ADD COLUMN action TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE privilege_audit ADD COLUMN plugin_id TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE privilege_audit ADD COLUMN action TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE privilege_audit ADD COLUMN signature_version INTEGER NOT NULL DEFAULT 1`)
	// Earlier audit rows predate generic plugin ownership. Keep their original
	// v1 signatures intact, add display-only legacy metadata, and never offer
	// them as an automatic restore candidate.
	_, _ = db.Exec(`UPDATE privilege_audit SET signature_version=? WHERE COALESCE(plugin_id,'')<>'' AND signature_version=?`, privilegeAuditSignatureV2, privilegeAuditSignatureV1)
	_, _ = db.Exec(`UPDATE privilege_audit SET plugin_id='alemonx-network', action='legacy', signature_version=? WHERE COALESCE(plugin_id,'')=''`, privilegeAuditSignatureV1)
	store.migrateLegacyNetworkAudit()
	return store, nil
}

// migrateLegacyNetworkAudit is a one-time compatibility importer. It treats
// historic JSON as v1 display-only records, never rewrites an existing audit
// signature and never makes those records eligible for restore.
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
		if strings.TrimSpace(entry.Operation) == "" {
			continue
		}
		_ = s.appendLegacyAudit("alemonx-network", entry.Operation, entry.Params, entry.Output, entry.CreatedAt)
	}
	_, _ = s.db.Exec(`INSERT INTO privilege_meta(key,value) VALUES('legacy_network_audit','imported')`)
}

func (s *privilegeStore) appendLegacyAudit(pluginID, operation string, params map[string]string, output, created string) error {
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339Nano)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := ""
	_ = s.db.QueryRow(`SELECT chain_hash FROM privilege_audit ORDER BY sequence DESC LIMIT 1`).Scan(&previous)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(strings.Join([]string{previous, operation, string(encoded), output, "legacy-import", created}, "\x00")))
	_, err = s.db.Exec(`INSERT INTO privilege_audit(plugin_id,action,signature_version,operation,params,output,account,created_at,previous_hash,chain_hash) VALUES(?,?,?,?,?,?,?,?,?,?)`, pluginID, "legacy", privilegeAuditSignatureV1, operation, encoded, output, "legacy-import", created, previous, hex.EncodeToString(mac.Sum(nil)))
	return err
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

func (s *privilegeStore) savePlan(pluginID, action string, data json.RawMessage, account string) (json.RawMessage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("权限审计存储不可用")
	}
	var plan privilegedPlan
	if err := json.Unmarshal(data, &plan); err != nil || strings.TrimSpace(plan.Operation) == "" || plan.Fingerprint == "" {
		return nil, errors.New("插件返回的变更计划无效")
	}
	if _, err := time.Parse(time.RFC3339, plan.ExpiresAt); err != nil {
		return nil, errors.New("网络变更计划缺少有效过期时间")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	plan.ID, plan.PluginID, plan.Action = hex.EncodeToString(id), pluginID, action
	params, _ := json.Marshal(plan.Params)
	verification, _ := json.Marshal(plan.Verification)
	if _, err := s.db.Exec(`INSERT INTO privilege_plans(id,plugin_id,action,operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, plan.ID, pluginID, action, plan.Operation, params, plan.Fingerprint, plan.Risk, plan.Impact, verification, plan.CreatedAt, plan.ExpiresAt, account); err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func (s *privilegeStore) consumePlan(id, pluginID, action, account string) (privilegedPlan, error) {
	var plan privilegedPlan
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return plan, errors.New("未找到宿主签发的插件变更计划")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var params, verification []byte
	var expiresAt string
	var usedAt sql.NullString
	var owner string
	err := s.db.QueryRow(`SELECT plugin_id,action,operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account,used_at FROM privilege_plans WHERE id=?`, id).Scan(&plan.PluginID, &plan.Action, &plan.Operation, &params, &plan.Fingerprint, &plan.Risk, &plan.Impact, &verification, &plan.CreatedAt, &expiresAt, &owner, &usedAt)
	if err != nil {
		return plan, errors.New("未找到宿主签发的插件变更计划")
	}
	if usedAt.Valid && usedAt.String != "" {
		return plan, errors.New("网络变更计划已使用，请重新预演")
	}
	if owner != account || plan.PluginID != pluginID || plan.Action != action {
		return plan, errors.New("插件变更计划与当前操作不匹配")
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().After(expires) {
		return plan, errors.New("插件变更计划已过期，请重新预演")
	}
	if json.Unmarshal(params, &plan.Params) != nil || json.Unmarshal(verification, &plan.Verification) != nil {
		return plan, errors.New("插件变更计划损坏")
	}
	plan.ID, plan.ExpiresAt = id, expiresAt
	if _, err := s.db.Exec(`UPDATE privilege_plans SET used_at=? WHERE id=? AND used_at IS NULL`, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return plan, err
	}
	return plan, nil
}

// peekPlan validates a plugin plan for preflight without consuming it.
func (s *privilegeStore) peekPlan(id, pluginID, action, account string) (privilegedPlan, error) {
	var plan privilegedPlan
	if s == nil || s.db == nil || strings.TrimSpace(id) == "" {
		return plan, errors.New("未找到宿主签发的插件变更计划")
	}
	var params, verification []byte
	var expiresAt, owner string
	var usedAt sql.NullString
	err := s.db.QueryRow(`SELECT plugin_id,action,operation,params,fingerprint,risk,impact,verification,created_at,expires_at,account,used_at FROM privilege_plans WHERE id=?`, id).Scan(&plan.PluginID, &plan.Action, &plan.Operation, &params, &plan.Fingerprint, &plan.Risk, &plan.Impact, &verification, &plan.CreatedAt, &expiresAt, &owner, &usedAt)
	if err != nil || owner != account || plan.PluginID != pluginID || plan.Action != action || (usedAt.Valid && usedAt.String != "") {
		return plan, errors.New("未找到可用的宿主签发插件变更计划")
	}
	expires, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil || time.Now().After(expires) || json.Unmarshal(params, &plan.Params) != nil || json.Unmarshal(verification, &plan.Verification) != nil {
		return plan, errors.New("插件变更计划已过期或损坏，请重新预演")
	}
	plan.ID, plan.ExpiresAt = id, expiresAt
	return plan, nil
}

// releasePlan is available for password-based operations that fail before
// their plugin runner starts. Native authorization attempts stay consumed:
// they may already have changed the system before reporting failure.
func (s *privilegeStore) releasePlan(id, pluginID, action, account string) {
	if s == nil || s.db == nil || id == "" {
		return
	}
	_, _ = s.db.Exec(`UPDATE privilege_plans SET used_at=NULL WHERE id=? AND plugin_id=? AND action=? AND account=?`, id, pluginID, action, account)
}

func (s *privilegeStore) latestAudit(pluginID string) (privilegedPlan, error) {
	var plan privilegedPlan
	var params []byte
	if s == nil || s.db == nil {
		return plan, errors.New("权限审计存储不可用")
	}
	if err := s.db.QueryRow(`SELECT action,operation,params FROM privilege_audit WHERE plugin_id=? AND signature_version=? ORDER BY sequence DESC LIMIT 1`, pluginID, privilegeAuditSignatureV2).Scan(&plan.Action, &plan.Operation, &params); err != nil {
		return plan, errors.New("该插件没有可用于恢复的最近操作")
	}
	if err := json.Unmarshal(params, &plan.Params); err != nil {
		return plan, errors.New("插件审计记录损坏")
	}
	return plan, nil
}

func (s *privilegeStore) appendAudit(pluginID, action, operation string, params map[string]string, output, account string) error {
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
	_, _ = mac.Write([]byte(strings.Join([]string{previous, pluginID, action, operation, string(encoded), output, account, created}, "\x00")))
	chain := hex.EncodeToString(mac.Sum(nil))
	_, err = s.db.Exec(`INSERT INTO privilege_audit(plugin_id,action,signature_version,operation,params,output,account,created_at,previous_hash,chain_hash) VALUES(?,?,?,?,?,?,?,?,?,?)`, pluginID, action, privilegeAuditSignatureV2, operation, encoded, output, account, created, previous, chain)
	return err
}

func (s *privilegeStore) auditStatus(policyVersion string) privilegeAuditStatus {
	result := privilegeAuditStatus{Valid: true, PolicyVersion: policyVersion}
	if s == nil || s.db == nil {
		result.Valid, result.Reason = false, "权限审计存储不可用"
		return result
	}
	rows, err := s.db.Query(`SELECT plugin_id,action,signature_version,operation,params,output,account,created_at,previous_hash,chain_hash FROM privilege_audit ORDER BY sequence ASC`)
	if err != nil {
		result.Valid, result.Reason = false, err.Error()
		return result
	}
	defer rows.Close()
	previous := ""
	for rows.Next() {
		var pluginID, action, operation, output, account, created, previousHash, chain string
		var signatureVersion int
		var params []byte
		if err := rows.Scan(&pluginID, &action, &signatureVersion, &operation, &params, &output, &account, &created, &previousHash, &chain); err != nil || previousHash != previous {
			result.Valid, result.Reason = false, "权限审计链不连续"
			return result
		}
		mac := hmac.New(sha256.New, s.key)
		values := []string{previous, operation, string(params), output, account, created}
		if signatureVersion == privilegeAuditSignatureV2 {
			values = []string{previous, pluginID, action, operation, string(params), output, account, created}
		} else if signatureVersion != privilegeAuditSignatureV1 {
			result.Valid, result.Reason = false, "权限审计签名版本无效"
			return result
		}
		_, _ = mac.Write([]byte(strings.Join(values, "\x00")))
		if !hmac.Equal([]byte(chain), []byte(hex.EncodeToString(mac.Sum(nil)))) {
			result.Valid, result.Reason = false, "权限审计链校验失败"
			return result
		}
		previous = chain
	}
	return result
}

func (s *privilegeStore) auditItems(pluginID string) ([]map[string]any, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("权限审计存储不可用")
	}
	rows, err := s.db.Query(`SELECT sequence,action,signature_version,operation,params,output,created_at FROM privilege_audit WHERE plugin_id=? ORDER BY sequence DESC LIMIT 100`, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var sequence int
		var action, operation, output, created string
		var signatureVersion int
		var params json.RawMessage
		if err := rows.Scan(&sequence, &action, &signatureVersion, &operation, &params, &output, &created); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": fmt.Sprintf("%d", sequence), "action": action, "operation": operation, "params": params, "output": output, "createdAt": created, "legacy": signatureVersion == privilegeAuditSignatureV1})
	}
	return items, rows.Err()
}
