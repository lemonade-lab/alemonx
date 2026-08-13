package agent

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OpsProjectStore keeps the opt-in state outside a robot project and outside
// the incident directory.  A missing file is deliberately a normal disabled
// state: opening the workbench must not create operational data.
type OpsProjectStore struct {
	path string
	mu   sync.Mutex
}

type OpsProjectState struct {
	Enabled bool      `json:"enabled"`
	Updated time.Time `json:"updated"`
}

type opsProjectsFile struct {
	Projects map[string]OpsProjectState `json:"projects"`
}

func NewOpsProjectStore(path string) *OpsProjectStore { return &OpsProjectStore{path: path} }

func (s *OpsProjectStore) State(root string) (OpsProjectState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := opsProjectsFile{Projects: map[string]OpsProjectState{}}
	if err := readJSONFile(s.path, &items); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OpsProjectState{}, false, nil
		}
		return OpsProjectState{}, false, err
	}
	item, ok := items.Projects[filepath.Clean(root)]
	return item, ok, nil
}

func (s *OpsProjectStore) SetEnabled(root string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := opsProjectsFile{Projects: map[string]OpsProjectState{}}
	if err := readJSONFile(s.path, &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if items.Projects == nil {
		items.Projects = map[string]OpsProjectState{}
	}
	items.Projects[filepath.Clean(root)] = OpsProjectState{Enabled: enabled, Updated: time.Now()}
	return atomicJSONFile(s.path, items)
}

// MigratePolicies makes old installations opt in exactly once. An explicit
// disabled record always wins over an old policy file.
func (s *OpsProjectStore) MigratePolicies(policies []OpsPolicy) error {
	if len(policies) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := opsProjectsFile{Projects: map[string]OpsProjectState{}}
	if err := readJSONFile(s.path, &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if items.Projects == nil {
		items.Projects = map[string]OpsProjectState{}
	}
	changed := false
	for _, policy := range policies {
		root := filepath.Clean(policy.ProjectRoot)
		if root == "." || root == "" {
			continue
		}
		if _, known := items.Projects[root]; !known {
			items.Projects[root] = OpsProjectState{Enabled: true, Updated: time.Now()}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return atomicJSONFile(s.path, items)
}
