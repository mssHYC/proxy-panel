package handler

import (
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"proxy-panel/internal/kernel"
)

// The catalog is a public directory selected for the requesting server by Fedora
// MirrorManager. Only metadata is read; mirror checks are a separate user action.
const realityCatalogURL = "https://mirrors.fedoraproject.org/metalink?repo=epel-9&arch=x86_64&protocol=https"

var realityCatalogClient = &http.Client{
	Timeout:       8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type realityMirror struct {
	Host    string `json:"host"`
	Country string `json:"country"`
}

func (h *NodeHandler) RealityCandidateCatalog(c *gin.Context) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, realityCatalogURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建镜像目录请求失败"})
		return
	}
	resp, err := realityCatalogClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "VPS 无法访问 Fedora 镜像目录，可手动输入候选域名"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Fedora 镜像目录暂时不可用，可手动输入候选域名"})
		return
	}
	mirrors, err := readRealityMirrors(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Fedora 目录没有可用的 HTTPS 镜像，可手动输入候选域名"})
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.JSON(http.StatusOK, gin.H{"mirrors": mirrors})
}

func readRealityMirrors(r io.Reader) ([]realityMirror, error) {
	const maxBytes = 256 * 1024
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, errors.New("invalid mirror catalog size")
	}
	var catalog struct {
		XMLName xml.Name `xml:"metalink"`
		URLs    []struct {
			URL        string `xml:",chardata"`
			Preference int    `xml:"preference,attr"`
			Country    string `xml:"location,attr"`
		} `xml:"files>file>resources>url"`
	}
	if err := xml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	sort.SliceStable(catalog.URLs, func(i, j int) bool {
		return catalog.URLs[i].Preference > catalog.URLs[j].Preference
	})
	seen := make(map[string]bool)
	var mirrors []realityMirror
	for _, entry := range catalog.URLs {
		u, err := url.Parse(strings.TrimSpace(entry.URL))
		if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		if seen[host] || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
			continue
		}
		if _, _, err := kernel.ParseRealityTarget(host); err != nil {
			continue
		}
		country := strings.ToUpper(entry.Country)
		if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
			country = ""
		}
		mirrors = append(mirrors, realityMirror{Host: host, Country: country})
		seen[host] = true
		if len(mirrors) == 8 {
			break
		}
	}
	if len(mirrors) == 0 {
		return nil, errors.New("no HTTPS mirrors in catalog")
	}
	return mirrors, nil
}
