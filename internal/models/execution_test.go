package models

import "testing"

func TestSubmissionReservationPreventsTargetOvershoot(t *testing.T) {
	state := NewExecutionState()

	if !state.TryStartSubmission(1) {
		t.Fatal("first reservation should be allowed")
	}
	if state.TryStartSubmission(1) {
		t.Fatal("second reservation should be rejected while target slot is in flight")
	}

	state.CompleteSubmission(true)
	if state.GetCurNum() != 1 {
		t.Fatalf("CurNum = %d, want 1", state.GetCurNum())
	}
	if state.TryStartSubmission(1) {
		t.Fatal("reservation should be rejected after target is complete")
	}
}

func TestAbortSubmissionReservationReleasesSlot(t *testing.T) {
	state := NewExecutionState()

	if !state.TryStartSubmission(1) {
		t.Fatal("reservation should be allowed")
	}
	state.AbortSubmissionReservation()
	if !state.TryStartSubmission(1) {
		t.Fatal("slot should be available after abort")
	}
}
