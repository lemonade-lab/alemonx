package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alicebob/miniredis/v2"
)

const snapshotVersion = 1

// snapshotDatabaseCount matches Redis's standard number of logical databases.
// The embedded server has no CONFIG database count, so only the default Redis
// database range is exposed and persisted.
const snapshotDatabaseCount = 16

type snapshot struct {
	Version   int                `json:"version"`
	SavedAt   time.Time          `json:"savedAt"`
	Databases []snapshotDatabase `json:"databases"`
}

type snapshotDatabase struct {
	ID   int           `json:"id"`
	Keys []snapshotKey `json:"keys"`
}

type snapshotKey struct {
	Key       []byte           `json:"key"`
	Type      string           `json:"type"`
	TTLMillis int64            `json:"ttlMillis,omitempty"`
	Value     []byte           `json:"value,omitempty"`
	Values    [][]byte         `json:"values,omitempty"`
	Fields    []snapshotField  `json:"fields,omitempty"`
	Members   []snapshotMember `json:"members,omitempty"`
	Entries   []snapshotEntry  `json:"entries,omitempty"`
}

type snapshotField struct {
	Key   []byte `json:"key"`
	Value []byte `json:"value"`
}

type snapshotMember struct {
	Value []byte  `json:"value"`
	Score float64 `json:"score"`
}

type snapshotEntry struct {
	ID     string   `json:"id"`
	Values [][]byte `json:"values"`
}

func (m *Manager) startSnapshotterLocked() {
	if m.snapshotStop != nil {
		return
	}
	stop := make(chan struct{})
	m.snapshotStop = stop
	go func() {
		ticker := time.NewTicker(snapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.mu.Lock()
				if m.server != nil {
					if err := m.saveSnapshotLocked(); err != nil {
						m.message = fmt.Sprintf("内置 Redis 正在运行，但数据保存失败：%v", err)
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

// stopSnapshotterLocked requests a background flush to stop. It intentionally
// does not wait for completion: callers hold m.mu and the worker needs that
// same mutex before its next cycle.
func (m *Manager) stopSnapshotterLocked() {
	if m.snapshotStop == nil {
		return
	}
	close(m.snapshotStop)
	m.snapshotStop = nil
}

func (m *Manager) saveSnapshotLocked() error {
	if m.server == nil {
		return nil
	}
	data, err := makeSnapshot(m.server)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化 Redis 数据快照失败：%w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.snapshotPath), 0o755); err != nil {
		return fmt.Errorf("创建 Redis 数据目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.snapshotPath), ".alx-redis-data-*")
	if err != nil {
		return fmt.Errorf("创建 Redis 数据临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入 Redis 数据快照失败：%w", err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置 Redis 数据快照权限失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 Redis 数据快照失败：%w", err)
	}
	if err := os.Rename(temporaryPath, m.snapshotPath); err != nil {
		return fmt.Errorf("替换 Redis 数据快照失败：%w", err)
	}
	m.lastSaved = data.SavedAt
	return nil
}

func (m *Manager) restoreSnapshotLocked() error {
	raw, err := os.ReadFile(m.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 Redis 数据快照失败：%w", err)
	}
	var data snapshot
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("Redis 数据快照无效：%w", err)
	}
	if data.Version != snapshotVersion {
		return fmt.Errorf("不支持的 Redis 数据快照版本 %d", data.Version)
	}
	if err := restoreSnapshot(m.server, data); err != nil {
		return err
	}
	m.lastSaved = data.SavedAt
	return nil
}

func makeSnapshot(server *miniredis.Miniredis) (snapshot, error) {
	data := snapshot{Version: snapshotVersion, SavedAt: time.Now().UTC()}
	for id := 0; id < snapshotDatabaseCount; id++ {
		database := server.DB(id)
		keys := database.Keys()
		if len(keys) == 0 {
			continue
		}
		record := snapshotDatabase{ID: id, Keys: make([]snapshotKey, 0, len(keys))}
		for _, key := range keys {
			entry, err := snapshotRedisKey(database, key)
			if err != nil {
				return snapshot{}, fmt.Errorf("读取数据库 %d 的键 %q 失败：%w", id, key, err)
			}
			record.Keys = append(record.Keys, entry)
		}
		data.Databases = append(data.Databases, record)
	}
	return data, nil
}

func snapshotRedisKey(database *miniredis.RedisDB, key string) (snapshotKey, error) {
	entry := snapshotKey{Key: []byte(key), Type: database.Type(key)}
	if ttl := database.TTL(key); ttl > 0 {
		entry.TTLMillis = ttl.Milliseconds()
	}
	switch entry.Type {
	case "string":
		value, err := database.Get(key)
		entry.Value = []byte(value)
		return entry, err
	case "list":
		values, err := database.List(key)
		entry.Values = byteSlices(values)
		return entry, err
	case "set":
		values, err := database.Members(key)
		entry.Values = byteSlices(values)
		return entry, err
	case "hash":
		fields, err := database.HKeys(key)
		if err != nil {
			return entry, err
		}
		entry.Fields = make([]snapshotField, 0, len(fields))
		for _, field := range fields {
			entry.Fields = append(entry.Fields, snapshotField{Key: []byte(field), Value: []byte(database.HGet(key, field))})
		}
		return entry, nil
	case "zset":
		members, err := database.SortedSet(key)
		if err != nil {
			return entry, err
		}
		entry.Members = make([]snapshotMember, 0, len(members))
		for member, score := range members {
			entry.Members = append(entry.Members, snapshotMember{Value: []byte(member), Score: score})
		}
		sort.Slice(entry.Members, func(i, j int) bool { return string(entry.Members[i].Value) < string(entry.Members[j].Value) })
		return entry, nil
	case "stream":
		entries, err := database.Stream(key)
		if err != nil {
			return entry, err
		}
		entry.Entries = make([]snapshotEntry, 0, len(entries))
		for _, value := range entries {
			entry.Entries = append(entry.Entries, snapshotEntry{ID: value.ID, Values: byteSlices(value.Values)})
		}
		return entry, nil
	default:
		return entry, fmt.Errorf("暂不支持持久化 Redis %s 类型", entry.Type)
	}
}

func restoreSnapshot(server *miniredis.Miniredis, data snapshot) error {
	for _, stored := range data.Databases {
		if stored.ID < 0 || stored.ID >= snapshotDatabaseCount {
			return fmt.Errorf("Redis 数据快照包含无效数据库 %d", stored.ID)
		}
		database := server.DB(stored.ID)
		for _, entry := range stored.Keys {
			if err := restoreRedisKey(database, entry); err != nil {
				return fmt.Errorf("恢复数据库 %d 的键 %q 失败：%w", stored.ID, entry.Key, err)
			}
		}
	}
	return nil
}

func restoreRedisKey(database *miniredis.RedisDB, entry snapshotKey) error {
	key := string(entry.Key)
	switch entry.Type {
	case "string":
		if err := database.Set(key, string(entry.Value)); err != nil {
			return err
		}
	case "list":
		if _, err := database.Push(key, stringsFromBytes(entry.Values)...); err != nil {
			return err
		}
	case "set":
		if _, err := database.SetAdd(key, stringsFromBytes(entry.Values)...); err != nil {
			return err
		}
	case "hash":
		values := make([]string, 0, len(entry.Fields)*2)
		for _, field := range entry.Fields {
			values = append(values, string(field.Key), string(field.Value))
		}
		database.HSet(key, values...)
	case "zset":
		for _, member := range entry.Members {
			if _, err := database.ZAdd(key, member.Score, string(member.Value)); err != nil {
				return err
			}
		}
	case "stream":
		for _, value := range entry.Entries {
			if _, err := database.XAdd(key, value.ID, stringsFromBytes(value.Values)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("不支持的 Redis 数据类型 %q", entry.Type)
	}
	if entry.TTLMillis > 0 {
		database.SetTTL(key, time.Duration(entry.TTLMillis)*time.Millisecond)
	}
	return nil
}

func byteSlices(values []string) [][]byte {
	result := make([][]byte, len(values))
	for index, value := range values {
		result[index] = []byte(value)
	}
	return result
}

func stringsFromBytes(values [][]byte) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
