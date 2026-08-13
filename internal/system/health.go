package system

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LocalHealth validates the local workbench endpoint without following any
// remote URL or using user-controlled hosts.
func LocalHealth(port string, timeout time.Duration) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		port = "17390"
	}
	client := &http.Client{Timeout: timeout}
	response, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return "", fmt.Errorf("无法连接 http://127.0.0.1:%s/healthz：%w", port, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("健康检查返回 %s", response.Status)
	}
	var result struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return "", fmt.Errorf("健康检查响应无效：%w", err)
	}
	if result.Status == "" {
		result.Status = "ok"
	}
	if result.Version == "" {
		return "状态 " + result.Status, nil
	}
	return "状态 " + result.Status + "，版本 " + result.Version, nil
}
