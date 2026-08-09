package setupplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

const (
	maxPluginCacheSize = int64(1 << 30)
	maxCachedVersions  = 3
)

type cacheVersion struct {
	ID            string `json:"id"`
	Tag           string `json:"tag"`
	Asset         string `json:"asset"`
	ArchiveSHA256 string `json:"archiveSha256"`
	Fingerprint   string `json:"fingerprint"`
	Size          int64  `json:"size"`
	CacheBytes    int64  `json:"cacheBytes"`
	Package       string `json:"package"`
	Extracted     string `json:"extracted"`
	CreatedAt     string `json:"createdAt"`
	LastUsedAt    string `json:"lastUsedAt"`
}

type CachedVersion struct {
	Tag           string `json:"tag"`
	Asset         string `json:"asset"`
	Size          int64  `json:"size"`
	ArchiveSHA256 string `json:"archiveSha256"`
	Fingerprint   string `json:"fingerprint"`
	Active        bool   `json:"active"`
	Cached        bool   `json:"cached"`
	LastUsedAt    string `json:"lastUsedAt,omitempty"`
}

type CacheSummary struct {
	Bytes        int64 `json:"bytes"`
	Limit        int64 `json:"limit"`
	Entries      int   `json:"entries"`
	MaxPerPlugin int   `json:"maxPerPlugin"`
}

func (r *Registry) cacheBase() string {
	if r.cacheRoot != "" {
		return r.cacheRoot
	}
	if len(r.roots) > 0 {
		return filepath.Join(r.roots[0], ".plugin-cache")
	}
	if config, err := os.UserConfigDir(); err == nil {
		return filepath.Join(config, "alx", "plugin-cache")
	}
	return filepath.Join(".", "plugin-cache")
}

func safeCacheComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var builder strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._-", char) {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func (r *Registry) cacheDirectory(id, tag, asset string) string {
	return filepath.Join(r.cacheBase(), safeCacheComponent(id), safeCacheComponent(tag), safeCacheComponent(asset))
}

func cacheMetadataPath(directory string) string { return filepath.Join(directory, "cache.json") }

func readCacheVersion(directory string) (cacheVersion, error) {
	data, err := os.ReadFile(cacheMetadataPath(directory))
	if err != nil {
		return cacheVersion{}, err
	}
	var item cacheVersion
	if err := json.Unmarshal(data, &item); err != nil || item.ID == "" || item.Tag == "" || item.Asset == "" {
		return cacheVersion{}, errors.New("插件缓存元数据无效")
	}
	return item, nil
}

func writeCacheVersion(item cacheVersion) error {
	if err := os.MkdirAll(filepath.Dir(item.Package), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	temporary := cacheMetadataPath(filepath.Dir(item.Package)) + ".new"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, cacheMetadataPath(filepath.Dir(item.Package)))
}

func (r *Registry) listCached(id string) ([]cacheVersion, error) {
	root := filepath.Join(r.cacheBase(), safeCacheComponent(id))
	tags, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []cacheVersion{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]cacheVersion, 0)
	for _, tag := range tags {
		if !tag.IsDir() || strings.HasPrefix(tag.Name(), ".") {
			continue
		}
		assets, err := os.ReadDir(filepath.Join(root, tag.Name()))
		if err != nil {
			continue
		}
		for _, asset := range assets {
			if !asset.IsDir() {
				continue
			}
			item, err := readCacheVersion(filepath.Join(root, tag.Name(), asset.Name()))
			if err != nil || item.ID != id {
				continue
			}
			if _, err := os.Stat(item.Package); err != nil {
				continue
			}
			if _, err := os.Stat(item.Extracted); err != nil {
				continue
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LastUsedAt > items[j].LastUsedAt })
	return items, nil
}

func (r *Registry) readActiveMetadata(id string) (installMetadata, error) {
	plugin, err := r.Find(id)
	if err != nil {
		return installMetadata{}, err
	}
	data, err := os.ReadFile(filepath.Join(plugin.Source, installMetadataName))
	if err != nil {
		return installMetadata{}, err
	}
	var metadata installMetadata
	if err := json.Unmarshal(data, &metadata); err != nil || metadata.ID != id {
		return installMetadata{}, errors.New("活动插件安装元数据无效")
	}
	return metadata, nil
}

func (r *Registry) cachedVersions(id string) ([]CachedVersion, error) {
	items, err := r.listCached(id)
	if err != nil {
		return nil, err
	}
	active, _ := r.readActiveMetadata(id)
	result := make([]CachedVersion, 0, len(items)+1)
	for _, item := range items {
		result = append(result, CachedVersion{Tag: item.Tag, Asset: item.Asset, Size: item.Size, ArchiveSHA256: item.ArchiveSHA256, Fingerprint: item.Fingerprint, Cached: true, Active: item.Fingerprint != "" && item.Fingerprint == active.Fingerprint, LastUsedAt: item.LastUsedAt})
	}
	if active.Tag != "" {
		found := false
		for _, item := range result {
			if item.Active {
				found = true
				break
			}
		}
		if !found {
			result = append(result, CachedVersion{Tag: active.Tag, Asset: active.Asset, ArchiveSHA256: active.ArchiveSHA256, Fingerprint: active.Fingerprint, Active: true, Cached: false, LastUsedAt: active.LastUsedAt})
		}
	}
	if active.Tag == "" {
		// Legacy installations predate .alx-install.json. Keep them visible in
		// version management without pretending that their source version is a
		// Release tag or that the package is cached.
		if plugin, err := r.Find(id); err == nil && !plugin.Online && plugin.Version != "" {
			result = append(result, CachedVersion{Tag: plugin.Version, Active: true, Cached: false})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Tag > result[j].Tag })
	return result, nil
}

func (r *Registry) cacheSummary() (CacheSummary, error) {
	if err := os.MkdirAll(r.cacheBase(), 0o700); err != nil {
		return CacheSummary{}, err
	}
	var summary CacheSummary
	summary.Limit = maxPluginCacheSize
	summary.MaxPerPlugin = maxCachedVersions
	err := filepath.WalkDir(r.cacheBase(), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		summary.Bytes += info.Size()
		if filepath.Base(path) == "cache.json" {
			summary.Entries++
		}
		return nil
	})
	return summary, err
}

func (r *Registry) touchCache(item cacheVersion) error {
	item.LastUsedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeCacheVersion(item)
}

func (r *Registry) removeCache(item cacheVersion) error {
	return os.RemoveAll(filepath.Dir(item.Package))
}

func (r *Registry) cleanupCache() (CacheSummary, error) {
	if err := os.MkdirAll(r.cacheBase(), 0o700); err != nil {
		return CacheSummary{}, err
	}
	entries := make([]cacheVersion, 0)
	pluginDirs, err := os.ReadDir(r.cacheBase())
	if err != nil {
		return CacheSummary{}, err
	}
	for _, pluginDir := range pluginDirs {
		if !pluginDir.IsDir() {
			continue
		}
		id := pluginDir.Name()
		items, _ := r.listCached(id)
		entries = append(entries, items...)
	}
	active := map[string]bool{}
	for _, item := range entries {
		if metadata, err := r.readActiveMetadata(item.ID); err == nil && metadata.Fingerprint == item.Fingerprint {
			active[item.Fingerprint] = true
		}
	}
	byPlugin := map[string][]cacheVersion{}
	for _, item := range entries {
		byPlugin[item.ID] = append(byPlugin[item.ID], item)
	}
	remove := map[string]bool{}
	for id, items := range byPlugin {
		sort.Slice(items, func(i, j int) bool { return items[i].LastUsedAt > items[j].LastUsedAt })
		hasActive := false
		for _, item := range items {
			if active[item.Fingerprint] {
				hasActive = true
				break
			}
		}
		limit := maxCachedVersions
		if hasActive {
			limit--
		}
		kept := 0
		for _, item := range items {
			if active[item.Fingerprint] {
				continue
			}
			if kept < limit {
				kept++
				continue
			}
			remove[item.Fingerprint] = true
		}
		_ = id
	}
	summary, err := r.cacheSummary()
	if err != nil {
		return CacheSummary{}, err
	}
	if summary.Bytes > maxPluginCacheSize {
		sort.Slice(entries, func(i, j int) bool { return entries[i].LastUsedAt < entries[j].LastUsedAt })
		for _, item := range entries {
			if active[item.Fingerprint] || remove[item.Fingerprint] {
				continue
			}
			remove[item.Fingerprint] = true
			_ = r.removeCache(item)
			current, _ := r.cacheSummary()
			if current.Bytes <= maxPluginCacheSize {
				break
			}
		}
	}
	for _, item := range entries {
		if remove[item.Fingerprint] {
			// Entries removed during the global pass are already gone.
			if _, err := os.Stat(filepath.Dir(item.Package)); err == nil {
				_ = r.removeCache(item)
			}
		}
	}
	return r.cacheSummary()
}

func copyTree(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("插件缓存目录无效")
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		fileInfo, err := entry.Info()
		if err != nil || !fileInfo.Mode().IsRegular() {
			return errors.New("插件缓存包含不支持的文件类型")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = input.Close()
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileInfo.Mode().Perm()|0600)
		if err == nil {
			_, err = io.Copy(output, input)
			_ = output.Close()
		}
		_ = input.Close()
		return err
	})
}

func (r *Registry) Versions(id string) ([]CachedVersion, error) {
	return r.cachedVersions(id)
}

func (r *Registry) CacheSummary() (CacheSummary, error) {
	return r.cacheSummary()
}

func (r *Registry) CleanupCache() (CacheSummary, error) {
	return r.cleanupCache()
}

func (r *Registry) DeleteVersion(id, tag string) error {
	items, err := r.listCached(id)
	if err != nil {
		return err
	}
	active, _ := r.readActiveMetadata(id)
	for _, item := range items {
		if item.Tag != tag {
			continue
		}
		if item.Fingerprint != "" && item.Fingerprint == active.Fingerprint {
			return errors.New("不能删除当前活动版本")
		}
		return r.removeCache(item)
	}
	return errors.New("未找到已缓存的插件版本")
}
