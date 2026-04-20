package analyzer

import "testing"

func TestDetermineUrgency_LifeUsedOver100_EscalatesToReplaceSoon(t *testing.T) {
	dp := DrivePlan{
		HealthPassed:   true,
		HealthScore:    92,
		LifeUsedPct:    127,
		RemainingYears: 0.08,
		FailureMult:    2.5,
	}
	got, _ := determineUrgency(dp)
	if got != UrgencyReplaceSoon {
		t.Errorf("127%% life used should be ReplaceSoon, got %s", got)
	}
}

func TestDetermineUrgency_RemainingUnder3Months_EscalatesToReplaceSoon(t *testing.T) {
	dp := DrivePlan{
		HealthPassed:   true,
		HealthScore:    95,
		LifeUsedPct:    90,
		RemainingYears: 0.2, // ~2.4 months
		FailureMult:    1.5,
	}
	got, _ := determineUrgency(dp)
	if got != UrgencyReplaceSoon {
		t.Errorf("<3 months remaining should be ReplaceSoon, got %s", got)
	}
}

func TestDetermineUrgency_HealthyDriveWithGoodLife_StaysHealthy(t *testing.T) {
	dp := DrivePlan{
		HealthPassed:   true,
		HealthScore:    95,
		LifeUsedPct:    45,
		RemainingYears: 4.5,
		FailureMult:    0.8,
	}
	got, _ := determineUrgency(dp)
	if got != UrgencyHealthy {
		t.Errorf("healthy drive should stay Healthy, got %s", got)
	}
}
