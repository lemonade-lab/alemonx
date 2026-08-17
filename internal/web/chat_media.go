package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	chatMediaMaxBytes = 20 << 20
	chatMediaTimeout  = 20 * time.Second
)

var chatMediaIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func chatMediaHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *chatHistoryStore) mediaRootPath(root string) string {
	return filepath.Join(s.mediaDir, chatMediaHash(root))
}

func (s *chatHistoryStore) mediaPath(root, id string) (string, error) {
	if !chatMediaIDPattern.MatchString(id) {
		return "", errors.New("图片缓存标识无效")
	}
	return filepath.Join(s.mediaRootPath(root), id), nil
}

func (s *chatHistoryStore) hasMedia(root, id string) bool {
	path, err := s.mediaPath(root, id)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func (s *chatHistoryStore) writeMedia(root, id string, content []byte) error {
	path, err := s.mediaPath(root, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".chat-media-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func (s *chatHistoryStore) pruneMediaExpired(cutoff time.Time) error {
	if strings.TrimSpace(s.mediaDir) == "" {
		return nil
	}
	if _, err := os.Stat(s.mediaDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(s.mediaDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			return os.Remove(path)
		}
		return nil
	})
}

func validChatMediaSource(source *url.URL) bool {
	if source == nil || !strings.EqualFold(source.Scheme, "https") {
		return false
	}
	host := strings.ToLower(source.Hostname())
	return host == "multimedia.nt.qq.com.cn" ||
		host == "qpic.cn" ||
		strings.HasSuffix(host, ".qpic.cn") ||
		host == "qq.com" ||
		strings.HasSuffix(host, ".qq.com") ||
		host == "qq.com.cn" ||
		strings.HasSuffix(host, ".qq.com.cn")
}

func cacheChatMedia(
	ctx context.Context,
	store *chatHistoryStore,
	root string,
	rawURL string,
	client *http.Client,
	allowSource func(*url.URL) bool,
) (string, error) {
	source, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !allowSource(source) {
		return "", errors.New("图片来源不受支持")
	}
	id := chatMediaHash(source.String())
	if store.hasMedia(root, id) {
		return id, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("图片下载返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > chatMediaMaxBytes {
		return "", errors.New("图片超过 20 MiB 限制")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, chatMediaMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", errors.New("图片内容为空")
	}
	if len(content) > chatMediaMaxBytes {
		return "", errors.New("图片超过 20 MiB 限制")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	detectedType := strings.ToLower(http.DetectContentType(content))
	if !strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(detectedType, "image/") {
		return "", errors.New("下载内容不是图片")
	}
	if err := store.writeMedia(root, id, content); err != nil {
		return "", err
	}
	return id, nil
}

func (s *server) robotChatMediaHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireChatHistory(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.robotChatMediaRead(w, r)
	case http.MethodPost:
		s.robotChatMediaCache(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) robotChatMediaCache(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Root string `json:"root"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "图片缓存请求无法识别。")
		return
	}
	root := strings.TrimSpace(input.Root)
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || !validChatMediaSource(source) {
		writeError(w, http.StatusBadGateway, "图片来源不受支持。")
		return
	}
	client := s.network.Client(chatMediaTimeout)
	client.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if !validChatMediaSource(request.URL) {
			return errors.New("图片重定向来源不受支持")
		}
		return nil
	}
	id, err := cacheChatMedia(r.Context(), s.chatHistory, root, source.String(), client, validChatMediaSource)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	cachedURL := "/api/v1/robot/chat/media?" + url.Values{
		"root": {root},
		"id":   {id},
	}.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"url": cachedURL})
}

func (s *server) robotChatMediaRead(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if root == "" || !chatMediaIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "图片缓存地址无效。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path, err := s.chatHistory.mediaPath(root, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "图片缓存不存在或已过期。")
		} else {
			writeError(w, http.StatusBadGateway, "读取图片缓存失败。")
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "图片缓存不存在或已过期。")
		return
	}
	header := make([]byte, 512)
	read, _ := file.Read(header)
	_, _ = file.Seek(0, io.SeekStart)
	w.Header().Set("Content-Type", http.DetectContentType(header[:read]))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_ = os.Chtimes(path, time.Now(), time.Now())
	http.ServeContent(w, r, id, info.ModTime(), file)
}
