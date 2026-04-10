package analyzer

import (
	"fmt"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func analyzeKubernetes(k8s *internal.KubeInfo) []internal.Finding {
	var findings []internal.Finding

	// Node not ready
	for _, n := range k8s.Nodes {
		if n.Status != "Ready" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("K8s node not ready: %s", n.Name),
				Description: fmt.Sprintf("Node %s is in '%s' state. Roles: %s, Version: %s", n.Name, n.Status, n.Roles, n.Version),
				Impact:      "Pods on this node may be evicted or unable to schedule",
				Action:      "Check node health: kubectl describe node " + n.Name,
				Priority:    "immediate",
			})
		}
		if n.Unschedulable {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("K8s node cordoned: %s", n.Name),
				Description: fmt.Sprintf("Node %s is marked unschedulable (cordoned). No new pods will be placed here.", n.Name),
				Impact:      "Reduced cluster capacity",
				Action:      "Uncordon when ready: kubectl uncordon " + n.Name,
				Priority:    "short-term",
			})
		}
		// Node conditions (MemoryPressure, DiskPressure, PIDPressure)
		for _, c := range n.Conditions {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategorySystem,
				Title:       fmt.Sprintf("K8s node pressure: %s on %s", c, n.Name),
				Description: fmt.Sprintf("Node %s reports %s condition active", n.Name, c),
				Impact:      "Pods may be evicted to relieve pressure",
				Action:      "Investigate resource usage on node " + n.Name,
				Priority:    "immediate",
			})
		}
		// Pod capacity warning (>90%)
		if n.PodCapacity > 0 && n.PodCount > 0 {
			pct := float64(n.PodCount) / float64(n.PodCapacity) * 100
			if pct > 90 {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategorySystem,
					Title:       fmt.Sprintf("K8s node pod capacity high: %s (%.0f%%)", n.Name, pct),
					Description: fmt.Sprintf("Node %s has %d/%d pods (%.0f%% capacity)", n.Name, n.PodCount, n.PodCapacity, pct),
					Impact:      "New pods may fail to schedule on this node",
					Action:      "Consider adding nodes or migrating workloads",
					Priority:    "short-term",
				})
			}
		}
	}

	// Pod issues
	for _, p := range k8s.Pods {
		switch {
		case p.Status == "CrashLoopBackOff":
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s pod crash loop: %s/%s", p.Namespace, p.Name),
				Description: fmt.Sprintf("Pod %s in namespace %s is in CrashLoopBackOff with %d restarts", p.Name, p.Namespace, p.Restarts),
				Impact:      "Application is repeatedly crashing and restarting",
				Action:      "Check logs: kubectl logs " + p.Name + " -n " + p.Namespace + " --previous",
				Priority:    "immediate",
			})
		case p.Status == "Failed":
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s pod failed: %s/%s", p.Namespace, p.Name),
				Description: fmt.Sprintf("Pod %s in namespace %s has failed", p.Name, p.Namespace),
				Impact:      "Workload is not running",
				Action:      "Check events: kubectl describe pod " + p.Name + " -n " + p.Namespace,
				Priority:    "short-term",
			})
		case p.Status == "Pending" && p.Node == "":
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s pod pending (unscheduled): %s/%s", p.Namespace, p.Name),
				Description: fmt.Sprintf("Pod %s in namespace %s is pending and not assigned to any node", p.Name, p.Namespace),
				Impact:      "Workload is not running — may be waiting for resources",
				Action:      "Check events: kubectl describe pod " + p.Name + " -n " + p.Namespace,
				Priority:    "short-term",
			})
		case strings.Contains(p.Status, "OOMKilled"):
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryMemory,
				Title:       fmt.Sprintf("K8s pod OOM killed: %s/%s", p.Namespace, p.Name),
				Description: fmt.Sprintf("Pod %s was killed due to out-of-memory. Restarts: %d", p.Name, p.Restarts),
				Impact:      "Application exceeded memory limits",
				Action:      "Increase memory limits or optimize application memory usage",
				Priority:    "immediate",
			})
		}
		// Container-level issues
		for _, c := range p.Containers {
			if c.Reason == "OOMKilled" || c.LastTermMsg == "OOMKilled" {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityCritical,
					Category:    internal.CategoryMemory,
					Title:       fmt.Sprintf("K8s container OOM: %s/%s/%s", p.Namespace, p.Name, c.Name),
					Description: fmt.Sprintf("Container %s in pod %s was OOM killed (restarts: %d)", c.Name, p.Name, c.RestartCount),
					Impact:      "Container exceeded memory limit and was terminated",
					Action:      "Increase memory limit for container " + c.Name,
					Priority:    "immediate",
				})
			}
			if c.Reason == "ImagePullBackOff" || c.Reason == "ErrImagePull" {
				findings = append(findings, internal.Finding{
					Severity:    internal.SeverityWarning,
					Category:    internal.CategoryDocker,
					Title:       fmt.Sprintf("K8s image pull failed: %s/%s", p.Namespace, p.Name),
					Description: fmt.Sprintf("Container %s cannot pull image %s: %s", c.Name, c.Image, c.Reason),
					Impact:      "Pod cannot start",
					Action:      "Check image name, registry credentials, and network connectivity",
					Priority:    "immediate",
				})
			}
		}
		// High restart count warning
		if p.Restarts > 10 && p.Status == "Running" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s pod high restarts: %s/%s (%d)", p.Namespace, p.Name, p.Restarts),
				Description: fmt.Sprintf("Pod %s has restarted %d times. May indicate instability.", p.Name, p.Restarts),
				Impact:      "Application may be intermittently failing",
				Action:      "Check logs for recurring errors",
				Priority:    "short-term",
			})
		}
	}

	// Deployment issues
	for _, d := range k8s.Deployments {
		if d.Unavailable > 0 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s deployment unhealthy: %s/%s", d.Namespace, d.Name),
				Description: fmt.Sprintf("Deployment %s has %d unavailable replicas (%d/%d ready)", d.Name, d.Unavailable, d.ReadyReplicas, d.Replicas),
				Impact:      "Service may be degraded",
				Action:      "Check pod status: kubectl get pods -l app=" + d.Name + " -n " + d.Namespace,
				Priority:    "short-term",
			})
		}
		if d.Replicas > 0 && d.ReadyReplicas == 0 {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryDocker,
				Title:       fmt.Sprintf("K8s deployment down: %s/%s", d.Namespace, d.Name),
				Description: fmt.Sprintf("Deployment %s has 0/%d ready replicas", d.Name, d.Replicas),
				Impact:      "Service is completely unavailable",
				Action:      "Investigate immediately: kubectl describe deployment " + d.Name + " -n " + d.Namespace,
				Priority:    "immediate",
			})
		}
	}

	// PVC issues
	for _, pvc := range k8s.PVCs {
		if pvc.Status == "Pending" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityWarning,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("K8s PVC pending: %s/%s", pvc.Namespace, pvc.Name),
				Description: fmt.Sprintf("PersistentVolumeClaim %s (class: %s, capacity: %s) is still pending", pvc.Name, pvc.StorageClass, pvc.Capacity),
				Impact:      "Pods depending on this PVC cannot start",
				Action:      "Check storage provisioner and available PVs",
				Priority:    "short-term",
			})
		}
		if pvc.Status == "Lost" {
			findings = append(findings, internal.Finding{
				Severity:    internal.SeverityCritical,
				Category:    internal.CategoryDisk,
				Title:       fmt.Sprintf("K8s PVC lost: %s/%s", pvc.Namespace, pvc.Name),
				Description: fmt.Sprintf("PersistentVolumeClaim %s has lost its backing volume", pvc.Name),
				Impact:      "Data may be inaccessible",
				Action:      "Investigate PV status and storage backend immediately",
				Priority:    "immediate",
			})
		}
	}

	return findings
}
