package usecases_physical_postgresql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_ClassifySlotBreak_WhenWalStatusLost_RebuildsForSlotLost(t *testing.T) {
	var extendedSince time.Time

	reason, shouldRebuild := classifySlotBreak(&SlotState{WalStatus: "lost"}, 0, &extendedSince)

	require.True(t, shouldRebuild)
	require.Equal(t, breakReasonSlotLost, reason)
}

func Test_ClassifySlotBreak_WhenWalStatusUnreserved_RebuildsForWalLag(t *testing.T) {
	var extendedSince time.Time

	reason, shouldRebuild := classifySlotBreak(&SlotState{WalStatus: "unreserved"}, 0, &extendedSince)

	require.True(t, shouldRebuild)
	require.Equal(t, breakReasonWalLag, reason)
}

func Test_ClassifySlotBreak_WhenExtendedPersistsPastHold_RebuildsForWalLag(t *testing.T) {
	extendedSince := time.Now().UTC().Add(-extendedSlotStatusHoldPeriod - time.Second)

	reason, shouldRebuild := classifySlotBreak(&SlotState{WalStatus: "extended"}, 0, &extendedSince)

	require.True(t, shouldRebuild)
	require.Equal(t, breakReasonWalLag, reason)
}

func Test_ClassifySlotBreak_WhenLagExceedsThreshold_RebuildsForWalLag(t *testing.T) {
	var extendedSince time.Time

	reason, shouldRebuild := classifySlotBreak(&SlotState{WalStatus: "reserved", LagBytes: 101}, 100, &extendedSince)

	require.True(t, shouldRebuild)
	require.Equal(t, breakReasonWalLag, reason)
}

func Test_ClassifySlotBreak_WhenSlotHealthy_DoesNotRebuildAndClearsExtendedSample(t *testing.T) {
	extendedSince := time.Now().UTC().Add(-extendedSlotStatusHoldPeriod - time.Second)

	_, shouldRebuild := classifySlotBreak(&SlotState{WalStatus: "reserved", LagBytes: 10}, 100, &extendedSince)

	require.False(t, shouldRebuild)
	require.True(t, extendedSince.IsZero())
}
