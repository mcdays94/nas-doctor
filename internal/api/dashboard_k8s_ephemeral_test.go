package api

import (
	"strings"
	"testing"
)

// Issue #244: the Kubernetes node cards drew a red/amber/green "Disk X%"
// bar from ephemeral-storage capacity minus allocatable. Both come from the
// node spec, so the bar showed the kubelet's reservation and never moved as
// the disk filled. The cards now show capacity and allocatable as text.

func k8sSection(t *testing.T) string {
	t.Helper()
	js := DashboardJS
	start := strings.Index(js, "sections.kubernetes = function")
	if start < 0 {
		t.Fatal("DashboardJS: sections.kubernetes function not found")
	}
	rest := js[start:]
	if end := strings.Index(rest[1:], "\nsections."); end >= 0 {
		rest = rest[:end+1]
	}
	return rest
}

func TestK8sNodes_NoEphemeralStorageUsageBar(t *testing.T) {
	sec := k8sSection(t)
	for _, bad := range []string{"disk_total-kn.disk_allocatable", "disk_total-nodeInfo.disk_allocatable", "<span>Disk</span>", "'Disk '+"} {
		if strings.Contains(sec, bad) {
			t.Errorf("sections.kubernetes still renders a disk usage bar (%q); ephemeral storage is capacity, not usage (issue #244)", bad)
		}
	}
	if !strings.Contains(sec, "Ephemeral storage ") {
		t.Error("sections.kubernetes should label node storage as ephemeral-storage capacity (issue #244)")
	}
}

func TestSettingsHTML_K8sCardMatchesShippedScope(t *testing.T) {
	for _, bad := range []string{"EKS, GKE, AKS", "deployments, services, and PVCs"} {
		if strings.Contains(SettingsPage, bad) {
			t.Errorf("settings.html Kubernetes card still claims %q (issue #244)", bad)
		}
	}
}
