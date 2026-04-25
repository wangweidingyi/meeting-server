package admin

import (
	"context"
	"sync"
)

type MemoryMeetingStore struct {
	mu       sync.RWMutex
	meetings map[string]MeetingRecord
}

func NewMemoryMeetingStore() *MemoryMeetingStore {
	return &MemoryMeetingStore{
		meetings: make(map[string]MeetingRecord),
	}
}

func (s *MemoryMeetingStore) UpsertMeeting(_ context.Context, meeting MeetingRecord) (MeetingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.meetings[meeting.ID] = cloneMeetingRecord(meeting)
	return cloneMeetingRecord(meeting), nil
}

func (s *MemoryMeetingStore) ListMeetings(_ context.Context) ([]MeetingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meetings := make([]MeetingRecord, 0, len(s.meetings))
	for _, meeting := range s.meetings {
		meetings = append(meetings, cloneMeetingRecord(meeting))
	}

	return meetings, nil
}

func (s *MemoryMeetingStore) ListMeetingsByUser(_ context.Context, userID string) ([]MeetingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meetings := make([]MeetingRecord, 0)
	for _, meeting := range s.meetings {
		if meeting.UserID != userID {
			continue
		}
		meetings = append(meetings, cloneMeetingRecord(meeting))
	}

	return meetings, nil
}

func cloneMeetingRecord(meeting MeetingRecord) MeetingRecord {
	cloned := meeting
	if meeting.EndedAt != nil {
		endedAt := *meeting.EndedAt
		cloned.EndedAt = &endedAt
	}
	return cloned
}
