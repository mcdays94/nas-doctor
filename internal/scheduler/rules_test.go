package scheduler

import (
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
)

func TestEvalProcess(t *testing.T) {
	tests := []struct {
		name      string
		cond      string
		target    string
		val       float64
		procs     []internal.ProcessInfo
		wantCount int
		wantTitle string // if wantCount==1, check title substring
	}{
		{
			name: "cpu_above_triggered",
			cond: "cpu_above", target: "", val: 80,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 95.0, Mem: 10.0, ContainerName: "home-assistant"},
			},
			wantCount: 1,
			wantTitle: "High CPU: python",
		},
		{
			name: "cpu_below_threshold_no_finding",
			cond: "cpu_above", target: "", val: 80,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 50.0, Mem: 10.0},
			},
			wantCount: 0,
		},
		{
			name: "mem_above_triggered",
			cond: "mem_above", target: "", val: 70,
			procs: []internal.ProcessInfo{
				{PID: 2, Command: "java", CPU: 10.0, Mem: 85.0, ContainerName: "minecraft"},
			},
			wantCount: 1,
			wantTitle: "High memory: java",
		},
		{
			name: "target_filter_matches",
			cond: "cpu_above", target: "python", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 90.0, Mem: 10.0},
				{PID: 2, Command: "java", CPU: 95.0, Mem: 10.0}, // higher CPU but wrong name
			},
			wantCount: 1,
			wantTitle: "High CPU: python",
		},
		{
			name: "target_empty_checks_all",
			cond: "cpu_above", target: "", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 90.0, Mem: 10.0},
				{PID: 2, Command: "java", CPU: 60.0, Mem: 10.0},
				{PID: 3, Command: "nginx", CPU: 30.0, Mem: 10.0}, // below threshold
			},
			wantCount: 2,
		},
		{
			name: "no_processes_no_findings",
			cond: "cpu_above", target: "", val: 50,
			procs:     nil,
			wantCount: 0,
		},
		{
			name: "host_process_label",
			cond: "cpu_above", target: "", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 90.0, Mem: 10.0, ContainerName: ""},
			},
			wantCount: 1,
			wantTitle: "High CPU: python (host)",
		},
		{
			name: "container_label_in_title",
			cond: "mem_above", target: "", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "node", CPU: 10.0, Mem: 75.0, ContainerName: "grafana"},
			},
			wantCount: 1,
			wantTitle: "High memory: node (grafana)",
		},
		{
			name: "target_filter_case_insensitive",
			cond: "cpu_above", target: "Python", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 90.0, Mem: 10.0},
			},
			wantCount: 1,
		},
		{
			name: "unknown_condition_no_findings",
			cond: "disk_above", target: "", val: 50,
			procs: []internal.ProcessInfo{
				{PID: 1, Command: "python", CPU: 90.0, Mem: 90.0},
			},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := &internal.Snapshot{
				System: internal.SystemInfo{
					TopProcesses: tc.procs,
				},
			}
			findings := evalProcess(tc.cond, tc.target, tc.val, snap)
			if len(findings) != tc.wantCount {
				t.Fatalf("expected %d findings, got %d: %+v", tc.wantCount, len(findings), findings)
			}
			if tc.wantCount == 1 && tc.wantTitle != "" {
				if findings[0].Title != tc.wantTitle {
					t.Errorf("expected title %q, got %q", tc.wantTitle, findings[0].Title)
				}
			}
		})
	}
}

// TestEvaluateRule_ProcessCategory verifies the top-level evaluateRule dispatch
// routes "process" category to evalProcess.
func TestEvaluateRule_ProcessCategory(t *testing.T) {
	rule := internal.NotificationRule{
		ID:        "test-1",
		Name:      "High CPU process",
		Enabled:   true,
		Category:  "process",
		Condition: "cpu_above",
		Value:     "80",
	}
	snap := &internal.Snapshot{
		System: internal.SystemInfo{
			TopProcesses: []internal.ProcessInfo{
				{PID: 1, Command: "ffmpeg", CPU: 95.0, Mem: 5.0, ContainerName: "plex"},
			},
		},
	}
	findings := evaluateRule(rule, snap)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding from evaluateRule with process category, got %d", len(findings))
	}
	if findings[0].Category != internal.CategorySystem {
		t.Errorf("expected category %q, got %q", internal.CategorySystem, findings[0].Category)
	}
}
