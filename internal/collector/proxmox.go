package collector

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// ProxmoxConfig holds the connection details for a Proxmox VE API.
type ProxmoxConfig struct {
	URL      string // e.g. https://192.168.1.10:8006
	TokenID  string // e.g. root@pam!nas-doctor
	Secret   string // the UUID token secret
	NodeName string // optional: limit to a specific node (empty = all)
	Alias    string // optional: friendly display name
	Enabled  bool
}

type proxmoxClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newProxmoxClient(cfg ProxmoxConfig) *proxmoxClient {
	base := strings.TrimRight(cfg.URL, "/")
	return &proxmoxClient{
		baseURL: base + "/api2/json",
		token:   fmt.Sprintf("PVEAPIToken=%s=%s", cfg.TokenID, cfg.Secret),
		http: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // PVE uses self-signed certs
			},
		},
	}
}

func (c *proxmoxClient) get(path string) (json.RawMessage, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return envelope.Data, nil
}

// CollectProxmox gathers data from a Proxmox VE API.
func CollectProxmox(cfg ProxmoxConfig) *internal.ProxmoxInfo {
	if !cfg.Enabled || cfg.URL == "" || cfg.TokenID == "" || cfg.Secret == "" {
		return nil
	}
	client := newProxmoxClient(cfg)
	info := &internal.ProxmoxInfo{Connected: false}

	// Version — use as connectivity test
	data, err := client.get("/version")
	if err != nil {
		info.Error = fmt.Sprintf("PVE API connection failed: %v", err)
		return info
	}
	info.Connected = true
	var ver struct {
		Version string `json:"version"`
	}
	json.Unmarshal(data, &ver)
	info.Version = ver.Version

	// Cluster status
	if data, err := client.get("/cluster/status"); err == nil {
		var entries []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Online  int    `json:"online"`
			Quorate int    `json:"quorate"`
		}
		json.Unmarshal(data, &entries)
		for _, e := range entries {
			if e.Type == "cluster" {
				info.ClusterName = e.Name
			}
		}
	}

	// Nodes
	if data, err := client.get("/nodes"); err == nil {
		var nodes []struct {
			Node    string  `json:"node"`
			Status  string  `json:"status"`
			Uptime  int64   `json:"uptime"`
			CPU     float64 `json:"cpu"`
			MaxCPU  int     `json:"maxcpu"`
			Mem     int64   `json:"mem"`
			MaxMem  int64   `json:"maxmem"`
			Disk    int64   `json:"disk"`
			MaxDisk int64   `json:"maxdisk"`
		}
		json.Unmarshal(data, &nodes)
		for _, n := range nodes {
			if cfg.NodeName != "" && !strings.EqualFold(n.Node, cfg.NodeName) {
				continue
			}
			node := internal.ProxmoxNode{
				Name:      n.Node,
				Status:    n.Status,
				Uptime:    n.Uptime,
				CPUUsage:  n.CPU,
				CPUCores:  n.MaxCPU,
				MemUsed:   n.Mem,
				MemTotal:  n.MaxMem,
				DiskUsed:  n.Disk,
				DiskTotal: n.MaxDisk,
			}
			// Get detailed node status for kernel + PVE version
			if detail, err := client.get("/nodes/" + n.Node + "/status"); err == nil {
				var st struct {
					PVEVersion string `json:"pveversion"`
					KVer       string `json:"kversion"`
					CPUInfo    struct {
						Model string `json:"model"`
					} `json:"cpuinfo"`
				}
				json.Unmarshal(detail, &st)
				node.PVEVersion = st.PVEVersion
				node.KernelVer = st.KVer
				node.CPUModel = st.CPUInfo.Model
			}
			info.Nodes = append(info.Nodes, node)
		}
	}

	// Cluster resources (VMs + LXC + storage in one call)
	if data, err := client.get("/cluster/resources"); err == nil {
		var resources []struct {
			Type    string  `json:"type"`
			ID      string  `json:"id"`
			Node    string  `json:"node"`
			Name    string  `json:"name"`
			Status  string  `json:"status"`
			VMID    int     `json:"vmid"`
			Uptime  int64   `json:"uptime"`
			CPU     float64 `json:"cpu"`
			MaxCPU  int     `json:"maxcpu"`
			Mem     int64   `json:"mem"`
			MaxMem  int64   `json:"maxmem"`
			Disk    int64   `json:"disk"`
			MaxDisk int64   `json:"maxdisk"`
			NetIn   int64   `json:"netin"`
			NetOut  int64   `json:"netout"`
			Tags    string  `json:"tags"`
			Tmpl    int     `json:"template"`
			HAState string  `json:"hastate"`
			Storage string  `json:"storage"`
			SType   string  `json:"plugintype"`
			Content string  `json:"content"`
			Shared  int     `json:"shared"`
		}
		json.Unmarshal(data, &resources)

		for _, r := range resources {
			if cfg.NodeName != "" && r.Node != "" && !strings.EqualFold(r.Node, cfg.NodeName) {
				continue
			}
			switch r.Type {
			case "qemu", "lxc":
				guest := internal.ProxmoxGuest{
					VMID:     r.VMID,
					Name:     r.Name,
					Node:     r.Node,
					Type:     r.Type,
					Status:   r.Status,
					Uptime:   r.Uptime,
					CPUUsage: r.CPU,
					CPUs:     r.MaxCPU,
					MemUsed:  r.Mem,
					MemMax:   r.MaxMem,
					DiskUsed: r.Disk,
					DiskMax:  r.MaxDisk,
					NetIn:    r.NetIn,
					NetOut:   r.NetOut,
					Tags:     r.Tags,
					Template: r.Tmpl == 1,
					HAState:  r.HAState,
				}
				if !guest.Template {
					info.Guests = append(info.Guests, guest)
				}
			case "storage":
				usedPct := 0.0
				if r.MaxDisk > 0 {
					usedPct = float64(r.Disk) / float64(r.MaxDisk) * 100
				}
				info.Storage = append(info.Storage, internal.ProxmoxStorage{
					Storage: r.Storage,
					Node:    r.Node,
					Type:    r.SType,
					Status:  r.Status,
					Used:    r.Disk,
					Total:   r.MaxDisk,
					UsedPct: usedPct,
					Shared:  r.Shared == 1,
					Content: r.Content,
				})
			}
		}
	}

	// Sort guests: running first, then by VMID
	sort.Slice(info.Guests, func(i, j int) bool {
		if info.Guests[i].Status != info.Guests[j].Status {
			if info.Guests[i].Status == "running" {
				return true
			}
			if info.Guests[j].Status == "running" {
				return false
			}
		}
		return info.Guests[i].VMID < info.Guests[j].VMID
	})

	// Recent tasks (last 20)
	for _, node := range info.Nodes {
		if taskData, err := client.get("/nodes/" + node.Name + "/tasks?limit=20&source=active,all"); err == nil {
			var tasks []struct {
				UPID      string `json:"upid"`
				Type      string `json:"type"`
				Status    string `json:"status"`
				User      string `json:"user"`
				StartTime int64  `json:"starttime"`
				EndTime   int64  `json:"endtime"`
				ID        string `json:"id"`
			}
			json.Unmarshal(taskData, &tasks)
			for _, t := range tasks {
				vmid := 0
				if t.ID != "" {
					fmt.Sscanf(t.ID, "%d", &vmid)
				}
				info.Tasks = append(info.Tasks, internal.ProxmoxTask{
					UPID:      t.UPID,
					Node:      node.Name,
					Type:      t.Type,
					Status:    t.Status,
					User:      t.User,
					StartTime: t.StartTime,
					EndTime:   t.EndTime,
					VMID:      vmid,
				})
			}
		}
	}

	// HA status
	if data, err := client.get("/cluster/ha/status/current"); err == nil {
		var haEntries []struct {
			SID    string `json:"sid"`
			State  string `json:"state"`
			Node   string `json:"node"`
			Status string `json:"status"`
			Type   string `json:"type"`
		}
		json.Unmarshal(data, &haEntries)
		for _, h := range haEntries {
			if h.Type == "service" {
				info.HAServices = append(info.HAServices, internal.ProxmoxHA{
					SID:    h.SID,
					State:  h.State,
					Node:   h.Node,
					Status: h.Status,
				})
			}
		}
	}

	return info
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
