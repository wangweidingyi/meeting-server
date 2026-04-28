package analysis

import (
	"strings"
	"sync"
	"time"

	"meeting-server/internal/pipeline/action_items"
	"meeting-server/internal/pipeline/summary"
	"meeting-server/internal/protocol"
	"meeting-server/internal/runtime/transcripts"
)

type Publisher interface {
	Publish(messages []protocol.RoutedMessage)
}

type Job struct {
	ClientID  string
	SessionID string
	IsFinal   bool
}

type Options struct {
	TranscriptStore transcripts.Store
	ActionItems     *action_items.Service
	Summary         *summary.Service
	Publisher       Publisher
}

type Runtime struct {
	transcriptStore transcripts.Store
	actionItems     *action_items.Service
	summary         *summary.Service
	publisher       Publisher
	jobs            chan Job
	closeOnce       sync.Once
	done            chan struct{}
}

func NewRuntime(options Options) *Runtime {
	runtime := &Runtime{
		transcriptStore: options.TranscriptStore,
		actionItems:     options.ActionItems,
		summary:         options.Summary,
		publisher:       options.Publisher,
		jobs:            make(chan Job, 64),
		done:            make(chan struct{}),
	}

	go runtime.loop()
	return runtime
}

func (r *Runtime) Schedule(clientID, sessionID string, isFinal bool) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}

	select {
	case r.jobs <- Job{ClientID: clientID, SessionID: sessionID, IsFinal: isFinal}:
	default:
		go func() {
			r.jobs <- Job{ClientID: clientID, SessionID: sessionID, IsFinal: isFinal}
		}()
	}
}

func (r *Runtime) Close() {
	r.closeOnce.Do(func() {
		close(r.jobs)
		<-r.done
	})
}

func (r *Runtime) loop() {
	defer close(r.done)

	for job := range r.jobs {
		r.process(job)
	}
}

func (r *Runtime) process(job Job) {
	if r.transcriptStore == nil || r.actionItems == nil || r.summary == nil || r.publisher == nil {
		return
	}

	snapshot, ok, err := r.transcriptStore.LoadSnapshot(job.SessionID)
	if err != nil || !ok || strings.TrimSpace(snapshot.Text) == "" {
		return
	}

	actionItemsResult := r.actionItems.Generate(job.SessionID, snapshot.Text, job.IsFinal)
	summaryResult := r.summary.Generate(job.SessionID, snapshot.Text, actionItemsResult.Items, job.IsFinal)

	updatedAt := time.Now().UTC().Format(time.RFC3339)
	if actionItemsResult.UpdatedAt == "" {
		actionItemsResult.UpdatedAt = updatedAt
	}
	if summaryResult.UpdatedAt == "" {
		summaryResult.UpdatedAt = updatedAt
	}

	summaryType := protocol.TypeSummaryDelta
	actionItemsType := protocol.TypeActionItemDelta
	if job.IsFinal {
		summaryType = protocol.TypeSummaryFinal
		actionItemsType = protocol.TypeActionItemFinal
	}

	r.publisher.Publish([]protocol.RoutedMessage{
		{
			Topic: protocol.ActionItemsTopic(job.ClientID, job.SessionID),
			Type:  actionItemsType,
			Payload: protocol.ActionItemsPayload{
				Version:   actionItemsResult.Version,
				UpdatedAt: actionItemsResult.UpdatedAt,
				Items:     actionItemsResult.Items,
				IsFinal:   actionItemsResult.IsFinal,
			},
		},
		{
			Topic: protocol.SummaryTopic(job.ClientID, job.SessionID),
			Type:  summaryType,
			Payload: protocol.SummaryPayload{
				Version:      summaryResult.Version,
				UpdatedAt:    summaryResult.UpdatedAt,
				AbstractText: summaryResult.AbstractText,
				KeyPoints:    summaryResult.KeyPoints,
				Decisions:    summaryResult.Decisions,
				Risks:        summaryResult.Risks,
				ActionItems:  summaryResult.ActionItems,
				IsFinal:      summaryResult.IsFinal,
			},
		},
	})
}
