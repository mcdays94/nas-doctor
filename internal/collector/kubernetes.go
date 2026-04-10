package collector

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// KubeConfig holds connection details for a Kubernetes cluster.
type KubeConfig struct {
	Enabled   bool
	URL       string // e.g. https://192.168.1.10:6443
	Token     string // bearer token
	Alias     string // friendly display name
	InCluster bool   // auto-detect from mounted service account
}

type kubeClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newKubeClient(cfg KubeConfig) *kubeClient {
	token := cfg.Token
	base := strings.TrimRight(cfg.URL, "/")

	// In-cluster detection: read service account token
	if cfg.InCluster || (base == "" && token == "") {
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
			token = strings.TrimSpace(string(data))
			base = "https://kubernetes.default.svc"
		}
	}

	return &kubeClient{
		baseURL: base,
		token:   token,
		http: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (c *kubeClient) get(path string) (json.RawMessage, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("no Kubernetes API URL configured")
	}
	url := c.baseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
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
	return body, nil
}

// CollectKubernetes gathers data from a Kubernetes API.
func CollectKubernetes(cfg KubeConfig) *internal.KubeInfo {
	if !cfg.Enabled {
		return nil
	}
	client := newKubeClient(cfg)
	info := &internal.KubeInfo{Connected: false}

	// Version
	data, err := client.get("/version")
	if err != nil {
		info.Error = fmt.Sprintf("K8s API connection failed: %v", err)
		return info
	}
	info.Connected = true
	var ver struct {
		GitVersion string `json:"gitVersion"`
		Platform   string `json:"platform"`
	}
	json.Unmarshal(data, &ver)
	info.Version = ver.GitVersion
	info.Platform = detectKubePlatform(ver.GitVersion)

	// Nodes
	if data, err := client.get("/api/v1/nodes"); err == nil {
		var nodeList struct {
			Items []struct {
				Metadata struct {
					Name              string            `json:"name"`
					Labels            map[string]string `json:"labels"`
					CreationTimestamp string            `json:"creationTimestamp"`
				} `json:"metadata"`
				Spec struct {
					Unschedulable bool `json:"unschedulable"`
				} `json:"spec"`
				Status struct {
					Conditions []struct {
						Type   string `json:"type"`
						Status string `json:"status"`
					} `json:"conditions"`
					Addresses []struct {
						Type    string `json:"type"`
						Address string `json:"address"`
					} `json:"addresses"`
					NodeInfo struct {
						KubeletVersion          string `json:"kubeletVersion"`
						OperatingSystem         string `json:"operatingSystem"`
						Architecture            string `json:"architecture"`
						ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
					} `json:"nodeInfo"`
					Capacity struct {
						CPU              string `json:"cpu"`
						Memory           string `json:"memory"`
						Pods             string `json:"pods"`
						EphemeralStorage string `json:"ephemeral-storage"`
					} `json:"capacity"`
					Allocatable struct {
						CPU              string `json:"cpu"`
						Memory           string `json:"memory"`
						Pods             string `json:"pods"`
						EphemeralStorage string `json:"ephemeral-storage"`
					} `json:"allocatable"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &nodeList)
		for _, n := range nodeList.Items {
			node := internal.KubeNode{
				Name:             n.Metadata.Name,
				Version:          n.Status.NodeInfo.KubeletVersion,
				OS:               n.Status.NodeInfo.OperatingSystem,
				Arch:             n.Status.NodeInfo.Architecture,
				ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
				Unschedulable:    n.Spec.Unschedulable,
				Age:              humanAge(n.Metadata.CreationTimestamp),
			}
			// Roles
			var roles []string
			for k := range n.Metadata.Labels {
				if strings.HasPrefix(k, "node-role.kubernetes.io/") {
					roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
				}
			}
			node.Roles = strings.Join(roles, ",")
			// Status from conditions
			node.Status = "NotReady"
			for _, c := range n.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					node.Status = "Ready"
				}
				// Only report actual pressure conditions, not informational ones
				if c.Type != "Ready" && c.Status == "True" {
					// Filter out non-problematic conditions (k3s EtcdIsVoter, etc.)
					switch c.Type {
					case "MemoryPressure", "DiskPressure", "PIDPressure", "NetworkUnavailable":
						node.Conditions = append(node.Conditions, c.Type)
					}
				}
			}
			// IP
			for _, addr := range n.Status.Addresses {
				if addr.Type == "InternalIP" {
					node.InternalIP = addr.Address
				}
			}
			// Capacity
			node.CPUCores = parseKubeQuantityInt(n.Status.Capacity.CPU)
			node.MemTotal = parseKubeMemBytes(n.Status.Capacity.Memory)
			node.PodCapacity = parseKubeQuantityInt(n.Status.Allocatable.Pods)
			node.DiskTotal = parseKubeMemBytes(n.Status.Capacity.EphemeralStorage)
			node.DiskAllocatable = parseKubeMemBytes(n.Status.Allocatable.EphemeralStorage)
			info.Nodes = append(info.Nodes, node)
		}
	}

	// Namespaces
	if data, err := client.get("/api/v1/namespaces"); err == nil {
		var nsList struct {
			Items []struct {
				Metadata struct {
					Name              string `json:"name"`
					CreationTimestamp string `json:"creationTimestamp"`
				} `json:"metadata"`
				Status struct {
					Phase string `json:"phase"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &nsList)
		for _, ns := range nsList.Items {
			info.Namespaces = append(info.Namespaces, internal.KubeNamespace{
				Name:   ns.Metadata.Name,
				Status: ns.Status.Phase,
				Age:    humanAge(ns.Metadata.CreationTimestamp),
			})
		}
	}

	// Pods (all namespaces)
	if data, err := client.get("/api/v1/pods"); err == nil {
		var podList struct {
			Items []struct {
				Metadata struct {
					Name              string `json:"name"`
					Namespace         string `json:"namespace"`
					CreationTimestamp string `json:"creationTimestamp"`
				} `json:"metadata"`
				Spec struct {
					NodeName string `json:"nodeName"`
				} `json:"spec"`
				Status struct {
					Phase      string `json:"phase"`
					PodIP      string `json:"podIP"`
					Conditions []struct {
						Type   string `json:"type"`
						Status string `json:"status"`
					} `json:"conditions"`
					ContainerStatuses []struct {
						Name         string `json:"name"`
						Image        string `json:"image"`
						Ready        bool   `json:"ready"`
						RestartCount int    `json:"restartCount"`
						State        struct {
							Running *struct{} `json:"running"`
							Waiting *struct {
								Reason  string `json:"reason"`
								Message string `json:"message"`
							} `json:"waiting"`
							Terminated *struct {
								Reason  string `json:"reason"`
								Message string `json:"message"`
							} `json:"terminated"`
						} `json:"state"`
						LastTerminationState struct {
							Terminated *struct {
								Reason  string `json:"reason"`
								Message string `json:"message"`
							} `json:"terminated"`
						} `json:"lastState"`
					} `json:"containerStatuses"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &podList)
		// Count pods per node and namespace
		nodePodCount := map[string]int{}
		nsPodCount := map[string]int{}
		for _, p := range podList.Items {
			nodePodCount[p.Spec.NodeName]++
			nsPodCount[p.Metadata.Namespace]++
			totalReady := 0
			totalContainers := len(p.Status.ContainerStatuses)
			totalRestarts := 0
			var containers []internal.KubeContainer
			podStatus := string(p.Status.Phase)
			for _, cs := range p.Status.ContainerStatuses {
				if cs.Ready {
					totalReady++
				}
				totalRestarts += cs.RestartCount
				c := internal.KubeContainer{
					Name:         cs.Name,
					Image:        cs.Image,
					Ready:        cs.Ready,
					RestartCount: cs.RestartCount,
				}
				if cs.State.Running != nil {
					c.State = "running"
				} else if cs.State.Waiting != nil {
					c.State = "waiting"
					c.Reason = cs.State.Waiting.Reason
					if c.Reason != "" {
						podStatus = c.Reason // e.g. CrashLoopBackOff
					}
				} else if cs.State.Terminated != nil {
					c.State = "terminated"
					c.Reason = cs.State.Terminated.Reason
				}
				if cs.LastTerminationState.Terminated != nil {
					c.LastTermMsg = cs.LastTerminationState.Terminated.Reason
					if cs.LastTerminationState.Terminated.Message != "" {
						c.LastTermMsg += ": " + cs.LastTerminationState.Terminated.Message
					}
				}
				containers = append(containers, c)
			}
			info.Pods = append(info.Pods, internal.KubePod{
				Name:       p.Metadata.Name,
				Namespace:  p.Metadata.Namespace,
				Node:       p.Spec.NodeName,
				Status:     podStatus,
				Phase:      string(p.Status.Phase),
				Ready:      fmt.Sprintf("%d/%d", totalReady, totalContainers),
				Restarts:   totalRestarts,
				Age:        humanAge(p.Metadata.CreationTimestamp),
				IP:         p.Status.PodIP,
				Containers: containers,
			})
		}
		// Update node pod counts
		for i := range info.Nodes {
			info.Nodes[i].PodCount = nodePodCount[info.Nodes[i].Name]
		}
		// Update namespace pod counts
		for i := range info.Namespaces {
			info.Namespaces[i].PodCount = nsPodCount[info.Namespaces[i].Name]
		}
	}

	// Deployments (all namespaces)
	if data, err := client.get("/apis/apps/v1/deployments"); err == nil {
		var depList struct {
			Items []struct {
				Metadata struct {
					Name              string `json:"name"`
					Namespace         string `json:"namespace"`
					CreationTimestamp string `json:"creationTimestamp"`
				} `json:"metadata"`
				Spec struct {
					Replicas int `json:"replicas"`
					Strategy struct {
						Type string `json:"type"`
					} `json:"strategy"`
				} `json:"spec"`
				Status struct {
					ReadyReplicas       int `json:"readyReplicas"`
					AvailableReplicas   int `json:"availableReplicas"`
					UnavailableReplicas int `json:"unavailableReplicas"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &depList)
		for _, d := range depList.Items {
			info.Deployments = append(info.Deployments, internal.KubeDeployment{
				Name:          d.Metadata.Name,
				Namespace:     d.Metadata.Namespace,
				Replicas:      d.Spec.Replicas,
				ReadyReplicas: d.Status.ReadyReplicas,
				Available:     d.Status.AvailableReplicas,
				Unavailable:   d.Status.UnavailableReplicas,
				Age:           humanAge(d.Metadata.CreationTimestamp),
				Strategy:      d.Spec.Strategy.Type,
			})
		}
	}

	// Services (all namespaces)
	if data, err := client.get("/api/v1/services"); err == nil {
		var svcList struct {
			Items []struct {
				Metadata struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"metadata"`
				Spec struct {
					Type        string   `json:"type"`
					ClusterIP   string   `json:"clusterIP"`
					ExternalIPs []string `json:"externalIPs"`
					Ports       []struct {
						Port     int    `json:"port"`
						Protocol string `json:"protocol"`
					} `json:"ports"`
				} `json:"spec"`
				Status struct {
					LoadBalancer struct {
						Ingress []struct {
							IP       string `json:"ip"`
							Hostname string `json:"hostname"`
						} `json:"ingress"`
					} `json:"loadBalancer"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &svcList)
		for _, s := range svcList.Items {
			var ports []string
			for _, p := range s.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
			}
			extIP := ""
			if len(s.Spec.ExternalIPs) > 0 {
				extIP = strings.Join(s.Spec.ExternalIPs, ",")
			} else if len(s.Status.LoadBalancer.Ingress) > 0 {
				ing := s.Status.LoadBalancer.Ingress[0]
				if ing.IP != "" {
					extIP = ing.IP
				} else {
					extIP = ing.Hostname
				}
			}
			info.Services = append(info.Services, internal.KubeService{
				Name:       s.Metadata.Name,
				Namespace:  s.Metadata.Namespace,
				Type:       string(s.Spec.Type),
				ClusterIP:  s.Spec.ClusterIP,
				ExternalIP: extIP,
				Ports:      ports,
			})
		}
	}

	// PVCs (all namespaces)
	if data, err := client.get("/api/v1/persistentvolumeclaims"); err == nil {
		var pvcList struct {
			Items []struct {
				Metadata struct {
					Name              string `json:"name"`
					Namespace         string `json:"namespace"`
					CreationTimestamp string `json:"creationTimestamp"`
				} `json:"metadata"`
				Spec struct {
					StorageClassName string   `json:"storageClassName"`
					AccessModes      []string `json:"accessModes"`
					VolumeName       string   `json:"volumeName"`
				} `json:"spec"`
				Status struct {
					Phase    string            `json:"phase"`
					Capacity map[string]string `json:"capacity"`
				} `json:"status"`
			} `json:"items"`
		}
		json.Unmarshal(data, &pvcList)
		for _, pvc := range pvcList.Items {
			capacity := ""
			if v, ok := pvc.Status.Capacity["storage"]; ok {
				capacity = v
			}
			info.PVCs = append(info.PVCs, internal.KubePVC{
				Name:         pvc.Metadata.Name,
				Namespace:    pvc.Metadata.Namespace,
				Status:       string(pvc.Status.Phase),
				StorageClass: pvc.Spec.StorageClassName,
				Capacity:     capacity,
				AccessModes:  strings.Join(pvc.Spec.AccessModes, ","),
				VolumeName:   pvc.Spec.VolumeName,
				Age:          humanAge(pvc.Metadata.CreationTimestamp),
			})
		}
	}

	// Events (Warning only, last hour)
	if data, err := client.get("/api/v1/events?fieldSelector=type=Warning"); err == nil {
		var eventList struct {
			Items []struct {
				Type           string `json:"type"`
				Reason         string `json:"reason"`
				Message        string `json:"message"`
				InvolvedObject struct {
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"involvedObject"`
				Count          int    `json:"count"`
				FirstTimestamp string `json:"firstTimestamp"`
				LastTimestamp  string `json:"lastTimestamp"`
			} `json:"items"`
		}
		json.Unmarshal(data, &eventList)
		cutoff := time.Now().Add(-1 * time.Hour)
		for _, e := range eventList.Items {
			lastSeen, _ := time.Parse(time.RFC3339, e.LastTimestamp)
			if !lastSeen.IsZero() && lastSeen.Before(cutoff) {
				continue
			}
			info.Events = append(info.Events, internal.KubeEvent{
				Type:      e.Type,
				Reason:    e.Reason,
				Message:   truncate(e.Message, 200),
				Object:    e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
				Namespace: e.InvolvedObject.Namespace,
				Count:     e.Count,
				FirstSeen: e.FirstTimestamp,
				LastSeen:  e.LastTimestamp,
			})
		}
		// Sort events newest first
		sort.Slice(info.Events, func(i, j int) bool {
			return info.Events[i].LastSeen > info.Events[j].LastSeen
		})
		// Cap at 50 events
		if len(info.Events) > 50 {
			info.Events = info.Events[:50]
		}
	}

	// Sort pods: non-running first (problems surface), then by namespace/name
	sort.Slice(info.Pods, func(i, j int) bool {
		pi, pj := info.Pods[i], info.Pods[j]
		iOk := pi.Status == "Running" || pi.Status == "Succeeded"
		jOk := pj.Status == "Running" || pj.Status == "Succeeded"
		if iOk != jOk {
			return !iOk // problems first
		}
		if pi.Namespace != pj.Namespace {
			return pi.Namespace < pj.Namespace
		}
		return pi.Name < pj.Name
	})

	return info
}

func detectKubePlatform(version string) string {
	v := strings.ToLower(version)
	if strings.Contains(v, "k3s") {
		return "k3s"
	}
	if strings.Contains(v, "eks") {
		return "eks"
	}
	if strings.Contains(v, "gke") {
		return "gke"
	}
	if strings.Contains(v, "aks") {
		return "aks"
	}
	return "k8s"
}

func humanAge(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func parseKubeQuantityInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseKubeMemBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "Ki") {
		var v int64
		fmt.Sscanf(s, "%d", &v)
		return v * 1024
	}
	if strings.HasSuffix(s, "Mi") {
		var v int64
		fmt.Sscanf(s, "%d", &v)
		return v * 1024 * 1024
	}
	if strings.HasSuffix(s, "Gi") {
		var v int64
		fmt.Sscanf(s, "%d", &v)
		return v * 1024 * 1024 * 1024
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
