package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"alemonx/internal/httpcache"
	"alemonx/internal/systemnetwork"
)

const PlatformURL = "https://open.alemonjs.com/api/v2"

var platformURL = PlatformURL

type Resource struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Subtype       string `json:"subtype"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	RepositoryURL string `json:"repositoryUrl"`
	InstallMode   string `json:"installMode"`
	PackageName   string `json:"packageName,omitempty"`
	OwnerLogin    string `json:"ownerLogin,omitempty"`
	Markdown      string `json:"markdown,omitempty"`
}
type ResourceSubtype struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type ResourcePage struct {
	Data     []Resource `json:"data"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

// PlatformGet only reads the official public catalog; never falls back to a repository.
func PlatformGet(path string, query url.Values, target any) error {
	endpoint := platformURL + "/" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	response, err := httpcache.Get(systemnetwork.DefaultClient(10*time.Second), endpoint, time.Minute)
	if (err != nil && response.Status == 0) || response.Stale {
		return fmt.Errorf("生态资源平台暂时无法连接，请重试")
	}
	if response.Status != http.StatusOK {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(response.Body, &failure)
		return &PlatformError{Status: response.Status, Message: failure.Error.Message}
	}
	if err := json.Unmarshal(response.Body, target); err != nil {
		return fmt.Errorf("生态资源平台返回格式无效")
	}
	return nil
}

type PlatformError struct {
	Status  int
	Message string
}

func (e *PlatformError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "生态资源平台暂时不可用"
}
