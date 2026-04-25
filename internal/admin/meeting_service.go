package admin

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type MeetingRecord struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	UserName   string  `json:"user_name"`
	ClientID   string  `json:"client_id"`
	Title      string  `json:"title"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"started_at"`
	EndedAt    *string `json:"ended_at"`
	DurationMS uint64  `json:"duration_ms"`
}

type MeetingStore interface {
	UpsertMeeting(ctx context.Context, meeting MeetingRecord) (MeetingRecord, error)
	ListMeetings(ctx context.Context) ([]MeetingRecord, error)
	ListMeetingsByUser(ctx context.Context, userID string) ([]MeetingRecord, error)
}

type MeetingService struct {
	store MeetingStore
}

func NewMeetingService(store MeetingStore) *MeetingService {
	if store == nil {
		panic("meeting store is required")
	}

	return &MeetingService{store: store}
}

func (s *MeetingService) Upsert(ctx context.Context, meeting MeetingRecord) (MeetingRecord, error) {
	if err := validateMeetingRecord(meeting); err != nil {
		return MeetingRecord{}, err
	}

	return s.store.UpsertMeeting(ctx, normalizeMeetingRecord(meeting))
}

func (s *MeetingService) List(ctx context.Context) ([]MeetingRecord, error) {
	meetings, err := s.store.ListMeetings(ctx)
	if err != nil {
		return nil, err
	}

	sortMeetings(meetings)
	return meetings, nil
}

func (s *MeetingService) ListByUser(ctx context.Context, userID string) ([]MeetingRecord, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	meetings, err := s.store.ListMeetingsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	sortMeetings(meetings)
	return meetings, nil
}

func sortMeetings(meetings []MeetingRecord) {
	sort.Slice(meetings, func(left, right int) bool {
		if meetings[left].StartedAt == meetings[right].StartedAt {
			return meetings[left].ID > meetings[right].ID
		}
		return meetings[left].StartedAt > meetings[right].StartedAt
	})
}

func validateMeetingRecord(meeting MeetingRecord) error {
	if strings.TrimSpace(meeting.ID) == "" {
		return errors.New("meeting.id is required")
	}
	if strings.TrimSpace(meeting.UserID) == "" {
		return errors.New("meeting.user_id is required")
	}
	if strings.TrimSpace(meeting.ClientID) == "" {
		return errors.New("meeting.client_id is required")
	}
	if strings.TrimSpace(meeting.Title) == "" {
		return errors.New("meeting.title is required")
	}
	if strings.TrimSpace(meeting.Status) == "" {
		return errors.New("meeting.status is required")
	}
	if strings.TrimSpace(meeting.StartedAt) == "" {
		return errors.New("meeting.started_at is required")
	}

	return nil
}

func normalizeMeetingRecord(meeting MeetingRecord) MeetingRecord {
	normalized := meeting
	normalized.ID = strings.TrimSpace(normalized.ID)
	normalized.UserID = strings.TrimSpace(normalized.UserID)
	normalized.UserName = strings.TrimSpace(normalized.UserName)
	normalized.ClientID = strings.TrimSpace(normalized.ClientID)
	normalized.Title = strings.TrimSpace(normalized.Title)
	normalized.Status = strings.TrimSpace(normalized.Status)
	normalized.StartedAt = strings.TrimSpace(normalized.StartedAt)
	if normalized.UserName == "" {
		normalized.UserName = normalized.UserID
	}
	if normalized.EndedAt != nil {
		endedAt := strings.TrimSpace(*normalized.EndedAt)
		if endedAt == "" {
			normalized.EndedAt = nil
		} else {
			normalized.EndedAt = &endedAt
		}
	}

	return normalized
}
