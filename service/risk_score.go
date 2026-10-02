package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	riskEventSampleLimit       = 20000
	riskLivenessPromptTokenMax = 64
	riskEventRetentionDays     = 7
)

var riskPruneMutex sync.Mutex
var riskLocation = time.FixedZone("UTC+8", 8*60*60)

type RiskSignals struct {
	RegularTiming          int `json:"regular_timing"`
	RepeatedInput          int `json:"repeated_input"`
	MultiModel             int `json:"multi_model"`
	Post402                int `json:"post_402"`
	NonStream              int `json:"non_stream"`
	NightActivity          int `json:"night_activity"`
	PostChannelUnavailable int `json:"post_channel_unavailable"`
	BlockedLiveness        int `json:"blocked_liveness"`
	MultiIP                int `json:"multi_ip"`
}

type RiskEvidence struct {
	Requests                       int  `json:"requests"`
	DistinctModels                 int  `json:"distinct_models"`
	DistinctIPs                    int  `json:"distinct_ips"`
	RepeatedInputs                 int  `json:"repeated_inputs"`
	RegularIntervals               int  `json:"regular_intervals"`
	Post402Requests                int  `json:"post_402_requests"`
	NonStreamRequests              int  `json:"non_stream_requests"`
	NightRequests                  int  `json:"night_requests"`
	PostChannelUnavailableRequests int  `json:"post_channel_unavailable_requests"`
	BlockedLivenessRequests        int  `json:"blocked_liveness_requests"`
	SampleLimited                  bool `json:"sample_limited"`
}

type RiskUserAssessment struct {
	UserID        int          `json:"user_id"`
	Username      string       `json:"username"`
	Score         int          `json:"score"`
	Level         string       `json:"level"`
	Status        string       `json:"status"`
	Signals       RiskSignals  `json:"signals"`
	RequestCount  int          `json:"request_count"`
	IPCount       int          `json:"ip_count"`
	LastRequestAt int64        `json:"last_request_at"`
	Evidence      RiskEvidence `json:"evidence"`
}

type RiskListQuery struct {
	WindowDays int
	Page       int
	PageSize   int
	Username   string
	Level      string
	Status     string
}

type RiskListPage struct {
	Items    []RiskUserAssessment `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Window   string               `json:"window"`
}

var riskWeights = RiskSignals{
	RegularTiming:          18,
	RepeatedInput:          16,
	MultiModel:             10,
	Post402:                12,
	NonStream:              8,
	NightActivity:          8,
	PostChannelUnavailable: 10,
	BlockedLiveness:        12,
	MultiIP:                6,
}

func riskScoreFromCount(count int, minimum int, perExtra int) int {
	if count < minimum {
		return 0
	}
	return min(100, (count-minimum+1)*perExtra)
}

func riskLevel(score int) string {
	switch {
	case score >= 75:
		return "critical"
	case score >= 50:
		return "high"
	case score >= 25:
		return "medium"
	default:
		return "low"
	}
}

// Detect a repeated interval from the observed data (one minute to one day),
// allowing 10% jitter. At least three local calendar days and 70% agreement
// are required. Sorting once avoids pairwise comparison on large windows.
func riskRegularity(events []model.RiskEvent) (int, int) {
	type interval struct {
		seconds int64
		day     int
	}
	intervals := make([]interval, 0, len(events))
	for index := 1; index < len(events); index++ {
		seconds := events[index].OccurredAt - events[index-1].OccurredAt
		if seconds < 60 || seconds > 24*3600 {
			continue
		}
		localTime := time.Unix(events[index].OccurredAt, 0).In(riskLocation)
		intervals = append(intervals, interval{seconds, localTime.Year()*1000 + localTime.YearDay()})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].seconds < intervals[j].seconds })
	days := make(map[int]int)
	start, bestCount, bestDays := 0, 0, 0
	for end, value := range intervals {
		days[value.day]++
		for value.seconds*100 > intervals[start].seconds*110 {
			day := intervals[start].day
			days[day]--
			if days[day] == 0 {
				delete(days, day)
			}
			start++
		}
		count := end - start + 1
		if count > bestCount || (count == bestCount && len(days) > bestDays) {
			bestCount, bestDays = count, len(days)
		}
	}
	if bestCount < 6 || bestDays < 3 || bestCount*10 < len(intervals)*7 ||
		events[len(events)-1].OccurredAt-events[0].OccurredAt < 48*3600 {
		return bestCount, 0
	}
	return bestCount, min(100, (bestCount-5)*8+(bestDays-2)*10)
}

func riskNightActivity(events []model.RiskEvent) (int, int) {
	times := make([]int64, 0)
	for _, event := range events {
		if time.Unix(event.OccurredAt, 0).In(riskLocation).Hour() < 8 {
			times = append(times, event.OccurredAt)
		}
	}
	start, peak := 0, 0
	for end, timestamp := range times {
		for times[start] < timestamp-3600 {
			start++
		}
		peak = max(peak, end-start+1)
	}
	if peak < 20 {
		return len(times), 0
	}
	return len(times), min(100, (peak-10)*3)
}

// CalculateRiskSnapshot scores metadata from one 3- or 7-day window. The
// events may arrive in any order; no request content is examined here.
func CalculateRiskSnapshot(userID int, windowDays int, events []model.RiskEvent, now time.Time) model.RiskSnapshot {
	copyOfEvents := append([]model.RiskEvent(nil), events...)
	sort.Slice(copyOfEvents, func(i, j int) bool {
		if copyOfEvents[i].OccurredAt == copyOfEvents[j].OccurredAt {
			return copyOfEvents[i].ID < copyOfEvents[j].ID
		}
		return copyOfEvents[i].OccurredAt < copyOfEvents[j].OccurredAt
	})
	cutoff := now.AddDate(0, 0, -windowDays).Unix()
	filtered := copyOfEvents[:0]
	for _, event := range copyOfEvents {
		if event.UserID == userID && event.OccurredAt >= cutoff && event.OccurredAt <= now.Unix() {
			filtered = append(filtered, event)
		}
	}

	snapshot := model.RiskSnapshot{UserID: userID, WindowDays: windowDays, UpdatedAt: now.Unix()}
	if len(filtered) == 0 {
		snapshot.Level = "low"
		return snapshot
	}
	snapshot.RequestCount = len(filtered)
	snapshot.LastRequestAt = filtered[len(filtered)-1].OccurredAt

	ipSet := make(map[string]struct{})
	modelSet := make(map[string]struct{})
	signatureCounts := make(map[string]int)
	tokenCounts := make(map[int]int)
	tokenIPSets := make(map[int]map[string]struct{})
	var last402 int64
	lastUnavailable := make(map[string]int64)
	maxSignature := 0
	maxTokenCount := 0
	knownTokenCounts := 0
	maxIPsPerToken := 0
	knownStreamRequests := 0

	for _, event := range filtered {
		if event.Model != "" {
			modelSet[event.Model] = struct{}{}
		}
		if event.IPFingerprint != "" {
			ipSet[event.IPFingerprint] = struct{}{}
			if event.TokenID > 0 {
				if tokenIPSets[event.TokenID] == nil {
					tokenIPSets[event.TokenID] = make(map[string]struct{})
				}
				tokenIPSets[event.TokenID][event.IPFingerprint] = struct{}{}
				maxIPsPerToken = max(maxIPsPerToken, len(tokenIPSets[event.TokenID]))
			}
		}
		if event.InputSignature != "" {
			signatureCounts[event.InputSignature]++
			maxSignature = max(maxSignature, signatureCounts[event.InputSignature])
		} else if event.PromptTokens > 0 {
			tokenCounts[event.PromptTokens]++
			knownTokenCounts++
			maxTokenCount = max(maxTokenCount, tokenCounts[event.PromptTokens])
		}
		// Authentication/routing failures may occur before stream is parsed.
		if event.StatusCode >= 200 && event.StatusCode < 400 {
			knownStreamRequests++
			if !event.Stream {
				snapshot.NonStreamRequests++
			}
		}
		if event.LivenessKeywordHit && event.PromptTokens > 0 && event.PromptTokens <= riskLivenessPromptTokenMax && event.StatusCode >= 400 {
			snapshot.BlockedLivenessRequests++
		}
		if event.StatusCode >= 200 && event.StatusCode < 400 {
			// Successful requests indicate quota or channel availability recovered.
			last402 = 0
			delete(lastUnavailable, event.Model)
		}
		if last402 > 0 && event.OccurredAt-last402 <= 24*3600 && event.StatusCode >= 400 {
			snapshot.Post402Requests++
		}
		if event.StatusCode == 402 || event.QuotaExhausted {
			last402 = event.OccurredAt
		}
		if failedAt := lastUnavailable[event.Model]; failedAt > 0 && event.OccurredAt-failedAt <= 3600 && event.StatusCode >= 400 {
			snapshot.PostChannelUnavailableRequests++
		}
		if event.ChannelUnavailable {
			lastUnavailable[event.Model] = event.OccurredAt
		}
	}

	snapshot.IPCount = len(ipSet)
	snapshot.DistinctModels = len(modelSet)
	if maxSignature >= 5 {
		snapshot.RepeatedInputs = maxSignature
	} else if maxTokenCount >= 8 {
		snapshot.RepeatedInputs = maxTokenCount
	}
	snapshot.RegularIntervals, snapshot.RegularTiming = riskRegularity(filtered)
	if maxSignature >= 5 && maxSignature*10 >= len(filtered)*4 {
		snapshot.RepeatedInput = min(100, maxSignature*12)
	} else if knownTokenCounts >= 8 && maxTokenCount*10 >= knownTokenCounts*8 {
		snapshot.RepeatedInput = min(60, maxTokenCount*5)
	}

	// Sliding 15-minute window for bursts across distinct models.
	windowModels := make(map[string]int)
	start := 0
	maxWindowModels := 0
	for _, event := range filtered {
		if event.Model != "" {
			windowModels[event.Model]++
		}
		for filtered[start].OccurredAt < event.OccurredAt-15*60 {
			oldModel := filtered[start].Model
			if oldModel != "" {
				windowModels[oldModel]--
				if windowModels[oldModel] == 0 {
					delete(windowModels, oldModel)
				}
			}
			start++
		}
		maxWindowModels = max(maxWindowModels, len(windowModels))
	}
	snapshot.MultiModel = riskScoreFromCount(maxWindowModels, 4, 22)
	snapshot.Post402 = riskScoreFromCount(snapshot.Post402Requests, 3, 25)
	if knownStreamRequests >= 20 && snapshot.NonStreamRequests*10 >= knownStreamRequests*7 {
		snapshot.NonStream = min(100, (snapshot.NonStreamRequests*100/knownStreamRequests-60)*3)
	}
	snapshot.NightRequests, snapshot.NightActivity = riskNightActivity(filtered)
	snapshot.PostChannelUnavailable = riskScoreFromCount(snapshot.PostChannelUnavailableRequests, 3, 25)
	snapshot.BlockedLiveness = riskScoreFromCount(snapshot.BlockedLivenessRequests, 1, 35)
	if maxIPsPerToken < 3 && len(tokenIPSets) == 0 {
		maxIPsPerToken = len(ipSet)
	}
	snapshot.MultiIP = riskScoreFromCount(maxIPsPerToken, 3, 25)

	snapshot.Score = (snapshot.RegularTiming*riskWeights.RegularTiming +
		snapshot.RepeatedInput*riskWeights.RepeatedInput +
		snapshot.MultiModel*riskWeights.MultiModel +
		snapshot.Post402*riskWeights.Post402 +
		snapshot.NonStream*riskWeights.NonStream +
		snapshot.NightActivity*riskWeights.NightActivity +
		snapshot.PostChannelUnavailable*riskWeights.PostChannelUnavailable +
		snapshot.BlockedLiveness*riskWeights.BlockedLiveness +
		snapshot.MultiIP*riskWeights.MultiIP + 50) / 100
	snapshot.Level = riskLevel(snapshot.Score)
	return snapshot
}

func RefreshRiskScoresForUser(userID int) error {
	if userID <= 0 {
		return errors.New("invalid risk user")
	}
	now := time.Now().UTC()
	events, err := model.ListRiskEvents(userID, now.AddDate(0, 0, -riskEventRetentionDays).Unix(), riskEventSampleLimit)
	if err != nil {
		return err
	}
	for _, windowDays := range []int{3, 7} {
		snapshot := CalculateRiskSnapshot(userID, windowDays, events, now)
		snapshot.SampleLimited = len(events) == riskEventSampleLimit
		if err := model.UpsertRiskSnapshot(&snapshot); err != nil {
			return err
		}
	}
	return nil
}

// PruneExpiredRiskEvents is also invoked by the background timer, including
// while there are no requests. Batches keep each write transaction bounded.
func PruneExpiredRiskEvents(now time.Time) error {
	if !riskPruneMutex.TryLock() {
		return nil
	}
	defer riskPruneMutex.Unlock()
	cutoff := now.AddDate(0, 0, -riskEventRetentionDays).Unix()
	for range 50 {
		deleted, err := model.PruneRiskEvents(cutoff, 1000)
		if err != nil {
			return err
		}
		if deleted < 1000 {
			break
		}
	}
	return model.PruneRiskSnapshots(now.Unix())
}

func riskAssessment(snapshot model.RiskSnapshot, username string, status string) RiskUserAssessment {
	if status == "" {
		status = "unreviewed"
	}
	return RiskUserAssessment{
		UserID: snapshot.UserID, Username: username, Score: snapshot.Score,
		Level: snapshot.Level, Status: status,
		Signals: RiskSignals{
			RegularTiming: snapshot.RegularTiming, RepeatedInput: snapshot.RepeatedInput,
			MultiModel: snapshot.MultiModel, Post402: snapshot.Post402,
			NonStream: snapshot.NonStream, NightActivity: snapshot.NightActivity,
			PostChannelUnavailable: snapshot.PostChannelUnavailable,
			BlockedLiveness:        snapshot.BlockedLiveness, MultiIP: snapshot.MultiIP,
		},
		RequestCount: snapshot.RequestCount, IPCount: snapshot.IPCount,
		LastRequestAt: snapshot.LastRequestAt,
		Evidence: RiskEvidence{
			Requests: snapshot.RequestCount, DistinctModels: snapshot.DistinctModels,
			DistinctIPs: snapshot.IPCount, RepeatedInputs: snapshot.RepeatedInputs,
			RegularIntervals: snapshot.RegularIntervals, Post402Requests: snapshot.Post402Requests,
			NonStreamRequests: snapshot.NonStreamRequests, NightRequests: snapshot.NightRequests,
			PostChannelUnavailableRequests: snapshot.PostChannelUnavailableRequests,
			BlockedLivenessRequests:        snapshot.BlockedLivenessRequests,
			SampleLimited:                  snapshot.SampleLimited,
		},
	}
}

func ListRiskUsers(query RiskListQuery) (RiskListPage, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -query.WindowDays).Unix()
	snapshots, total, err := model.ListRiskSnapshots(query.WindowDays, cutoff, query.Page, query.PageSize, query.Username, query.Level, query.Status)
	if err != nil {
		return RiskListPage{}, err
	}
	page := RiskListPage{Items: make([]RiskUserAssessment, 0, len(snapshots)), Total: total,
		Page: query.Page, PageSize: query.PageSize, Window: strconv.Itoa(query.WindowDays) + "d"}
	ids := make([]int, 0, len(snapshots))
	for _, snapshot := range snapshots {
		ids = append(ids, snapshot.UserID)
	}
	names, err := model.RiskUserNames(ids)
	if err != nil {
		return RiskListPage{}, err
	}
	statuses, err := model.RiskReviewStatuses(ids)
	if err != nil {
		return RiskListPage{}, err
	}
	for _, snapshot := range snapshots {
		page.Items = append(page.Items, riskAssessment(snapshot, names[snapshot.UserID], statuses[snapshot.UserID]))
	}
	return page, nil
}

func GetRiskUser(userID int, windowDays int) (RiskUserAssessment, error) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return RiskUserAssessment{}, err
	}
	if err := RefreshRiskScoresForUser(userID); err != nil {
		return RiskUserAssessment{}, err
	}
	snapshot, err := model.GetRiskSnapshot(userID, windowDays)
	if err != nil {
		return RiskUserAssessment{}, err
	}
	statuses, err := model.RiskReviewStatuses([]int{userID})
	if err != nil {
		return RiskUserAssessment{}, err
	}
	return riskAssessment(*snapshot, user.Username, statuses[userID]), nil
}

func SetRiskReviewStatus(userID int, reviewedBy int, status string) error {
	status = strings.TrimSpace(status)
	if status != "unreviewed" && status != "watching" && status != "confirmed" && status != "dismissed" {
		return errors.New("invalid risk review status")
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		return err
	}
	return model.UpsertRiskReview(&model.RiskReview{UserID: userID, Status: status,
		ReviewedBy: reviewedBy, UpdatedAt: time.Now().UTC().Unix()})
}
