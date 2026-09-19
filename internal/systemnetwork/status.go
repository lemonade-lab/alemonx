package systemnetwork

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Status struct {
	Revision    uint64            `json:"revision"`
	Definitions []GroupDefinition `json:"definitions"`
	Groups      []GroupState      `json:"groups"`
}
type previewJob struct {
	owner   *Manager
	config  Config
	expires time.Time
}

var previewJobs = struct {
	sync.Mutex
	values map[string]previewJob
}{values: map[string]previewJob{}}

func (m *Manager) Status() Status {
	c := m.ConfigSnapshot()
	if c == nil {
		return Status{Definitions: GroupDefinitions(), Groups: []GroupState{}}
	}
	if c.Mode == "auto" {
		for _, g := range c.Automatic.Groups {
			m.launchDetection(*c, g, false)
		}
	}
	return Status{Revision: c.Revision, Definitions: GroupDefinitions(), Groups: m.States(*c)}
}
func (m *Manager) StartPreview(input Config, id string) (string, error) {
	if _, ok := groupDefinition(id); !ok {
		return "", errors.New("请选择分组")
	}
	prepared, err := m.saveConfig(input, true)
	if err != nil {
		return "", err
	}
	if prepared.Mode != "auto" {
		return "", errors.New("候选检测仅适用于自动模式")
	}
	c := cloneConfig(*prepared.Config)
	c.Proxy = ProxyConfig{}
	draft := &Manager{config: &c}
	previewJobs.Lock()
	defer previewJobs.Unlock()
	for k, j := range previewJobs.values {
		if time.Now().After(j.expires) {
			delete(previewJobs.values, k)
		}
	}
	if len(previewJobs.values) >= 32 {
		return "", errors.New("检测任务较多，请稍后重试")
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", errors.New("无法创建检测任务")
	}
	key := hex.EncodeToString(token)
	draft.StartDetection(c, id)
	previewJobs.values[key] = previewJob{owner: draft, config: c, expires: time.Now().Add(5 * time.Minute)}
	return key, nil
}
func PreviewStatus(key string) (Status, bool) {
	previewJobs.Lock()
	j, ok := previewJobs.values[key]
	previewJobs.Unlock()
	if !ok || time.Now().After(j.expires) {
		return Status{}, false
	}
	return Status{Revision: j.config.Revision, Definitions: GroupDefinitions(), Groups: j.owner.States(j.config)}, true
}
