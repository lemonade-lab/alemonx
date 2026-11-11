package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type RobotEntry struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Pinned bool   `json:"pinned,omitempty"`
}

type RobotList struct {
	Scope           string       `json:"scope"`
	Local           bool         `json:"local"`
	Initialized     bool         `json:"initialized"`
	Projects        []RobotEntry `json:"projects"`
	ActiveProjectID string       `json:"activeProjectID"`
}

type RobotListChange struct {
	Scope    string       `json:"scope"`
	Action   string       `json:"action"`
	Projects []RobotEntry `json:"projects,omitempty"`
	ID       string       `json:"id,omitempty"`
	TargetID string       `json:"targetId,omitempty"`
}

var ErrRobotListScope = errors.New("机器人列表所属账户已变更，请刷新工作台。")

func robotListPath(root, account string) (string, RobotList, error) {
	resolved, err := ResolveRoot(root)
	if err != nil {
		return "", RobotList{}, err
	}
	sum := sha256.Sum256([]byte("alemonx-robots:" + account))
	scope := hex.EncodeToString(sum[:])
	return filepath.Join(resolved, "robot-lists", scope+".json"), RobotList{
		Scope: scope, Local: account == "", Projects: []RobotEntry{},
	}, nil
}

func LoadRobots(root, account string) (RobotList, error) {
	_, list, err := readRobotList(root, account)
	return list, err
}

func readRobotList(root, account string) (string, RobotList, error) {
	path, list, err := robotListPath(root, account)
	if err != nil {
		return "", list, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, list, nil
	}
	if err != nil {
		return path, list, err
	}
	var saved struct {
		Projects        []RobotEntry
		ActiveProjectID string
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		return path, list, err
	}
	list.Initialized = true
	list.Projects = append(list.Projects, saved.Projects...)
	list.ActiveProjectID = saved.ActiveProjectID
	return path, list, nil
}

func SameRobotPath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func UpdateRobots(root, account string, change RobotListChange) (RobotList, error) {
	path, list, err := robotListPath(root, account)
	if err != nil {
		return list, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return list, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return list, err
	}
	defer lock.Close()
	if err := lockRobotList(lock); err != nil {
		return list, err
	}
	_, list, err = readRobotList(root, account)
	if err != nil {
		return list, err
	}
	if change.Scope != list.Scope {
		return list, ErrRobotListScope
	}
	indexOf := func(id string) int {
		return slices.IndexFunc(list.Projects, func(item RobotEntry) bool { return item.ID == id })
	}
	switch change.Action {
	case "add", "import":
		if change.Action == "import" && list.Initialized {
			return list, nil
		}
		selectedNew := false
		for _, item := range change.Projects {
			if !filepath.IsAbs(item.Path) {
				return list, errors.New("机器人目录必须是绝对路径。")
			}
			if slices.ContainsFunc(list.Projects, func(saved RobotEntry) bool { return SameRobotPath(saved.Path, item.Path) }) {
				continue
			}
			if len(list.Projects) >= 500 {
				return list, errors.New("机器人列表已达上限。")
			}
			item.Path = filepath.Clean(item.Path)
			item.ID, item.Name = item.Path, filepath.Base(item.Path)
			item.Pinned = change.Action == "import" && item.Pinned
			list.Projects = append(list.Projects, item)
			if list.ActiveProjectID == "" || (change.Action == "add" && !selectedNew) {
				list.ActiveProjectID = item.ID
				selectedNew = true
			}
		}
		if change.Action == "import" && indexOf(change.ID) >= 0 {
			list.ActiveProjectID = change.ID
		}
	case "select":
		if indexOf(change.ID) >= 0 {
			list.ActiveProjectID = change.ID
		}
	case "remove":
		if index := indexOf(change.ID); index >= 0 {
			list.Projects = slices.Delete(list.Projects, index, index+1)
		}
	case "pin":
		if index := indexOf(change.ID); index >= 0 {
			list.Projects[index].Pinned = !list.Projects[index].Pinned
			slices.SortStableFunc(list.Projects, func(left, right RobotEntry) int {
				if left.Pinned == right.Pinned {
					return 0
				}
				if left.Pinned {
					return -1
				}
				return 1
			})
		}
	case "reorder":
		from, to := indexOf(change.ID), indexOf(change.TargetID)
		if from >= 0 && to >= 0 && from != to {
			item := list.Projects[from]
			list.Projects = slices.Delete(list.Projects, from, from+1)
			to = indexOf(change.TargetID)
			list.Projects = slices.Insert(list.Projects, to, item)
		}
	default:
		return list, errors.New("未知的机器人列表操作。")
	}
	if indexOf(list.ActiveProjectID) < 0 {
		list.ActiveProjectID = ""
		if len(list.Projects) > 0 {
			list.ActiveProjectID = list.Projects[0].ID
		}
	}
	list.Initialized = true
	data, err := json.Marshal(list)
	if err != nil {
		return list, err
	}
	file, err := os.CreateTemp(dir, ".robots-*")
	if err != nil {
		return list, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return list, err
	}
	return list, os.Rename(file.Name(), path)
}
