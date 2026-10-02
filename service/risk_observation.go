package service

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// RiskObservationInput contains only the metadata needed for risk scoring.
// IPFingerprint and InputSignature must be keyed, irreversible digests.
type RiskObservationInput struct {
	UserID             int
	TokenID            int
	OccurredAt         time.Time
	Model              string
	Stream             bool
	PromptTokens       int
	StatusCode         int
	QuotaExhausted     bool
	ChannelUnavailable bool
	LivenessKeywordHit bool
	IPFingerprint      string
	InputSignature     string
	RequestID          string
}

const (
	riskScoreRefreshInterval = 5 * time.Minute
	riskMaintenanceInterval  = time.Minute
	riskRefreshUserLimit     = 10000
)

var (
	riskObservationQueue      = make(chan RiskObservationInput, 2048)
	riskScoreRefreshQueue     = make(chan int, 512)
	riskObservationWorkerOnce sync.Once
	riskObservationDrops      atomic.Uint64
)

// RecordRiskObservation queues a single request event without delaying a
// gateway response. A full queue drops the event instead of retaining an
// unbounded backlog of request metadata.
func RecordRiskObservation(input RiskObservationInput) {
	input, valid := normalizeRiskObservation(input)
	if !valid {
		return
	}
	StartRiskObservationWorkers()
	select {
	case riskObservationQueue <- input:
	default:
		// Logging is performed by maintenance, so even a slow log destination
		// cannot delay a gateway response when the observation queue is full.
		riskObservationDrops.Add(1)
	}
}

// StartRiskObservationWorkers is called after database initialization so
// retention continues to run after a restart even when no requests arrive.
func StartRiskObservationWorkers() {
	riskObservationWorkerOnce.Do(func() {
		go writeRiskObservations()
		go refreshRiskScores()
		go maintainRiskObservations()
	})
}

func normalizeRiskObservation(input RiskObservationInput) (RiskObservationInput, bool) {
	if input.UserID <= 0 {
		return RiskObservationInput{}, false
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}
	if input.TokenID < 0 {
		input.TokenID = 0
	}
	if input.PromptTokens < 0 {
		input.PromptTokens = 0
	}
	input.Model = strings.TrimSpace(input.Model)
	if len(input.Model) > 191 {
		input.Model = strings.ToValidUTF8(input.Model[:191], "")
	}
	// Detach bounded metadata from any larger parser/header backing buffer.
	input.Model = strings.Clone(input.Model)
	if len(input.RequestID) > 64 {
		input.RequestID = ""
	}
	for _, char := range input.RequestID {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') || char == '_' || char == '-') {
			input.RequestID = ""
			break
		}
	}
	if !isRiskDigest(input.IPFingerprint) {
		input.IPFingerprint = ""
	}
	if !isRiskDigest(input.InputSignature) {
		input.InputSignature = ""
	}
	input.RequestID = strings.Clone(input.RequestID)
	input.IPFingerprint = strings.Clone(input.IPFingerprint)
	input.InputSignature = strings.Clone(input.InputSignature)

	return input, true
}

func isRiskDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func writeRiskObservations() {
	for input := range riskObservationQueue {
		event := &model.RiskEvent{
			UserID:             input.UserID,
			TokenID:            input.TokenID,
			OccurredAt:         input.OccurredAt.UTC().Unix(),
			Model:              input.Model,
			Stream:             input.Stream,
			PromptTokens:       input.PromptTokens,
			StatusCode:         input.StatusCode,
			QuotaExhausted:     input.QuotaExhausted,
			ChannelUnavailable: input.ChannelUnavailable,
			LivenessKeywordHit: input.LivenessKeywordHit,
			IPFingerprint:      input.IPFingerprint,
			InputSignature:     input.InputSignature,
			RequestID:          input.RequestID,
		}
		if err := model.CreateRiskEvent(event); err != nil {
			common.SysError("risk observation write failed")
			continue
		}
		select {
		case riskScoreRefreshQueue <- input.UserID:
		default:
			// A later event for this user can request a refresh.
		}
	}
}

func refreshRiskScores() {
	ticker := time.NewTicker(riskMaintenanceInterval)
	defer ticker.Stop()
	scheduler := riskRefreshScheduler{users: make(map[int]riskRefreshState), refresh: RefreshRiskScoresForUser}
	for {
		select {
		case userID := <-riskScoreRefreshQueue:
			scheduler.observe(userID, time.Now())
		case now := <-ticker.C:
			scheduler.flush(now)
		}
	}
}

type riskRefreshState struct {
	nextRefresh time.Time
	pending     bool
}

// A pending flag retains the final event of a burst even when no further
// requests arrive after the cooldown. Both user state and transport queues
// have hard bounds; quiet user entries are removed on the maintenance tick.
type riskRefreshScheduler struct {
	users   map[int]riskRefreshState
	refresh func(int) error
}

func (s *riskRefreshScheduler) observe(userID int, now time.Time) {
	state, exists := s.users[userID]
	if !exists && len(s.users) >= riskRefreshUserLimit {
		return
	}
	state.pending = true
	if !state.nextRefresh.After(now) {
		state = s.refreshUser(userID, now)
	}
	s.users[userID] = state
}

func (s *riskRefreshScheduler) refreshUser(userID int, now time.Time) riskRefreshState {
	if err := s.refresh(userID); err != nil {
		common.SysError("risk score refresh failed")
		return riskRefreshState{nextRefresh: now.Add(riskMaintenanceInterval), pending: true}
	}
	return riskRefreshState{nextRefresh: now.Add(riskScoreRefreshInterval)}
}

func (s *riskRefreshScheduler) flush(now time.Time) {
	for userID, state := range s.users {
		if state.nextRefresh.After(now) {
			continue
		}
		if !state.pending {
			delete(s.users, userID)
			continue
		}
		s.users[userID] = s.refreshUser(userID, now)
	}
}

func maintainRiskObservations() {
	ticker := time.NewTicker(riskMaintenanceInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		if riskObservationDrops.Swap(0) > 0 {
			common.SysError("risk observation queue full; events dropped")
		}
		if err := PruneExpiredRiskEvents(now.UTC()); err != nil {
			common.SysError("risk observation retention cleanup failed")
		}
	}
}
