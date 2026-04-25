package admin

import (
	"context"
	"errors"
	"sort"
	"strings"
)

const (
	UserRoleAdmin  = "admin"
	UserRoleMember = "member"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

type UserRecord struct {
	ID                 string  `json:"id"`
	Username           string  `json:"username"`
	DisplayName        string  `json:"display_name"`
	Role               string  `json:"role"`
	Status             string  `json:"status"`
	PasswordHash       string  `json:"-"`
	CreatedAt          string  `json:"created_at,omitempty"`
	UpdatedAt          string  `json:"updated_at,omitempty"`
	LastLoginAt        *string `json:"last_login_at"`
	PasswordChangedAt  *string `json:"password_changed_at,omitempty"`
	MeetingCount       int     `json:"meeting_count"`
	LastMeetingStarted *string `json:"last_meeting_started_at"`
}

type BootstrapAdminConfig struct {
	Username    string
	DisplayName string
	Password    string
}

type CreateUserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type UpdateUserInput struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
	Status      *string `json:"status"`
}

type ResetPasswordInput struct {
	Password string `json:"password"`
}

type UserStore interface {
	UpsertUser(ctx context.Context, user UserRecord) (UserRecord, error)
	ListUsers(ctx context.Context) ([]UserRecord, error)
	FindUserByID(ctx context.Context, userID string) (UserRecord, bool, error)
	FindUserByUsername(ctx context.Context, username string) (UserRecord, bool, error)
	AdminExists(ctx context.Context) (bool, error)
	CreateInitialAdmin(ctx context.Context, user UserRecord) (UserRecord, error)
}

type UserService struct {
	store        UserStore
	meetingStore MeetingStore
}

func NewUserService(store UserStore, meetingStore MeetingStore) *UserService {
	if store == nil {
		panic("user store is required")
	}
	if meetingStore == nil {
		panic("meeting store is required")
	}

	return &UserService{
		store:        store,
		meetingStore: meetingStore,
	}
}

func (s *UserService) EnsureBootstrapAdmin(ctx context.Context, bootstrap BootstrapAdminConfig) error {
	bootstrap.Username = strings.TrimSpace(bootstrap.Username)
	bootstrap.DisplayName = strings.TrimSpace(bootstrap.DisplayName)
	bootstrap.Password = strings.TrimSpace(bootstrap.Password)
	if bootstrap.Username == "" || bootstrap.Password == "" {
		return nil
	}

	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.Role == UserRoleAdmin {
			return nil
		}
	}

	_, err = s.Create(ctx, CreateUserInput{
		Username:    bootstrap.Username,
		DisplayName: firstNonEmpty(bootstrap.DisplayName, bootstrap.Username),
		Password:    bootstrap.Password,
		Role:        UserRoleAdmin,
		Status:      UserStatusActive,
	})
	return err
}

func (s *UserService) HasAdmin(ctx context.Context) (bool, error) {
	return s.store.AdminExists(ctx)
}

func (s *UserService) Upsert(ctx context.Context, user UserRecord) (UserRecord, error) {
	normalized := normalizeUserRecord(user)
	if err := validateUserRecord(normalized, false); err != nil {
		return UserRecord{}, err
	}

	return s.store.UpsertUser(ctx, normalized)
}

func (s *UserService) Create(ctx context.Context, input CreateUserInput) (UserRecord, error) {
	normalizedInput := normalizeCreateUserInput(input)
	if err := validateCreateUserInput(normalizedInput); err != nil {
		return UserRecord{}, err
	}

	existing, found, err := s.store.FindUserByUsername(ctx, normalizedInput.Username)
	if err != nil {
		return UserRecord{}, err
	}
	if found {
		return UserRecord{}, errors.New("user.username already exists")
	}

	passwordHash, err := HashPassword(normalizedInput.Password)
	if err != nil {
		return UserRecord{}, err
	}

	user := normalizeUserRecord(UserRecord{
		ID:           buildUserID(normalizedInput.Username),
		Username:     normalizedInput.Username,
		DisplayName:  normalizedInput.DisplayName,
		Role:         normalizedInput.Role,
		Status:       normalizedInput.Status,
		PasswordHash: passwordHash,
	})
	if user.ID == "" {
		user.ID = existing.ID
	}

	return s.store.UpsertUser(ctx, user)
}

func (s *UserService) CreateInitialAdmin(ctx context.Context, input CreateUserInput) (UserRecord, error) {
	normalizedInput := normalizeCreateUserInput(input)
	normalizedInput.Role = UserRoleAdmin
	normalizedInput.Status = UserStatusActive
	if err := validateCreateUserInput(normalizedInput); err != nil {
		return UserRecord{}, err
	}

	passwordHash, err := HashPassword(normalizedInput.Password)
	if err != nil {
		return UserRecord{}, err
	}

	user := normalizeUserRecord(UserRecord{
		ID:           buildUserID(normalizedInput.Username),
		Username:     normalizedInput.Username,
		DisplayName:  normalizedInput.DisplayName,
		Role:         UserRoleAdmin,
		Status:       UserStatusActive,
		PasswordHash: passwordHash,
	})

	return s.store.CreateInitialAdmin(ctx, user)
}

func (s *UserService) Update(ctx context.Context, userID string, input UpdateUserInput) (UserRecord, error) {
	user, found, err := s.store.FindUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return UserRecord{}, err
	}
	if !found {
		return UserRecord{}, errors.New("user not found")
	}

	if input.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Role != nil {
		user.Role = strings.TrimSpace(*input.Role)
	}
	if input.Status != nil {
		user.Status = strings.TrimSpace(*input.Status)
	}

	user = normalizeUserRecord(user)
	if err := validateUserRecord(user, false); err != nil {
		return UserRecord{}, err
	}

	return s.store.UpsertUser(ctx, user)
}

func (s *UserService) ResetPassword(ctx context.Context, userID string, password string) (UserRecord, error) {
	user, found, err := s.store.FindUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return UserRecord{}, err
	}
	if !found {
		return UserRecord{}, errors.New("user not found")
	}

	if err := validatePassword(password); err != nil {
		return UserRecord{}, err
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return UserRecord{}, err
	}

	user.PasswordHash = passwordHash
	updated := nowRFC3339()
	user.PasswordChangedAt = &updated
	return s.store.UpsertUser(ctx, user)
}

func (s *UserService) FindByID(ctx context.Context, userID string) (UserRecord, bool, error) {
	return s.store.FindUserByID(ctx, strings.TrimSpace(userID))
}

func (s *UserService) FindByUsername(ctx context.Context, username string) (UserRecord, bool, error) {
	return s.store.FindUserByUsername(ctx, strings.TrimSpace(username))
}

func (s *UserService) Authenticate(ctx context.Context, username, password string) (UserRecord, error) {
	user, found, err := s.store.FindUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return UserRecord{}, err
	}
	if !found || user.PasswordHash == "" {
		return UserRecord{}, errors.New("invalid credentials")
	}
	if user.Status != UserStatusActive {
		return UserRecord{}, errors.New("user is disabled")
	}
	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return UserRecord{}, errors.New("invalid credentials")
	}

	return user, nil
}

func (s *UserService) TouchLastLogin(ctx context.Context, userID string) error {
	user, found, err := s.store.FindUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if !found {
		return errors.New("user not found")
	}

	now := nowRFC3339()
	user.LastLoginAt = &now
	_, err = s.store.UpsertUser(ctx, user)
	return err
}

func (s *UserService) List(ctx context.Context) ([]UserRecord, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	meetings, err := s.meetingStore.ListMeetings(ctx)
	if err != nil {
		return nil, err
	}

	metrics := make(map[string]struct {
		count     int
		startedAt string
	}, len(meetings))
	for _, meeting := range meetings {
		if strings.TrimSpace(meeting.UserID) == "" {
			continue
		}

		entry := metrics[meeting.UserID]
		entry.count++
		if meeting.StartedAt > entry.startedAt {
			entry.startedAt = meeting.StartedAt
		}
		metrics[meeting.UserID] = entry
	}

	for index := range users {
		if entry, ok := metrics[users[index].ID]; ok {
			users[index].MeetingCount = entry.count
			if entry.startedAt != "" {
				startedAt := entry.startedAt
				users[index].LastMeetingStarted = &startedAt
			}
		}
	}

	sort.Slice(users, func(left, right int) bool {
		leftStarted := ""
		rightStarted := ""
		if users[left].LastMeetingStarted != nil {
			leftStarted = *users[left].LastMeetingStarted
		}
		if users[right].LastMeetingStarted != nil {
			rightStarted = *users[right].LastMeetingStarted
		}
		if leftStarted == rightStarted {
			return users[left].DisplayName < users[right].DisplayName
		}
		return leftStarted > rightStarted
	})

	return users, nil
}

func buildUserID(username string) string {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return ""
	}
	return "user-" + username
}

func normalizeCreateUserInput(input CreateUserInput) CreateUserInput {
	normalized := input
	normalized.Username = strings.TrimSpace(strings.ToLower(normalized.Username))
	normalized.DisplayName = strings.TrimSpace(normalized.DisplayName)
	normalized.Password = strings.TrimSpace(normalized.Password)
	normalized.Role = strings.TrimSpace(normalized.Role)
	normalized.Status = strings.TrimSpace(normalized.Status)
	if normalized.DisplayName == "" {
		normalized.DisplayName = normalized.Username
	}
	if normalized.Role == "" {
		normalized.Role = UserRoleMember
	}
	if normalized.Status == "" {
		normalized.Status = UserStatusActive
	}
	return normalized
}

func validateCreateUserInput(input CreateUserInput) error {
	if strings.TrimSpace(input.Username) == "" {
		return errors.New("user.username is required")
	}
	if strings.Contains(input.Username, " ") {
		return errors.New("user.username cannot contain spaces")
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		return errors.New("user.display_name is required")
	}
	if err := validatePassword(input.Password); err != nil {
		return err
	}
	if !isAllowedRole(input.Role) {
		return errors.New("user.role is invalid")
	}
	if !isAllowedStatus(input.Status) {
		return errors.New("user.status is invalid")
	}
	return nil
}

func validatePassword(password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

func validateUserRecord(user UserRecord, requirePassword bool) error {
	if strings.TrimSpace(user.ID) == "" {
		return errors.New("user.id is required")
	}
	if strings.TrimSpace(user.Username) == "" {
		return errors.New("user.username is required")
	}
	if strings.TrimSpace(user.DisplayName) == "" {
		return errors.New("user.display_name is required")
	}
	if !isAllowedRole(user.Role) {
		return errors.New("user.role is invalid")
	}
	if !isAllowedStatus(user.Status) {
		return errors.New("user.status is invalid")
	}
	if requirePassword && strings.TrimSpace(user.PasswordHash) == "" {
		return errors.New("user.password_hash is required")
	}
	return nil
}

func normalizeUserRecord(user UserRecord) UserRecord {
	normalized := user
	normalized.ID = strings.TrimSpace(normalized.ID)
	normalized.Username = strings.TrimSpace(strings.ToLower(normalized.Username))
	normalized.DisplayName = strings.TrimSpace(normalized.DisplayName)
	normalized.Role = strings.TrimSpace(normalized.Role)
	normalized.Status = strings.TrimSpace(normalized.Status)
	if normalized.Username == "" {
		normalized.Username = normalized.ID
	}
	if normalized.DisplayName == "" {
		normalized.DisplayName = normalized.Username
	}
	if normalized.Role == "" {
		normalized.Role = UserRoleMember
	}
	if normalized.Status == "" {
		normalized.Status = UserStatusActive
	}
	return normalized
}

func isAllowedRole(role string) bool {
	return role == UserRoleAdmin || role == UserRoleMember
}

func isAllowedStatus(status string) bool {
	return status == UserStatusActive || status == UserStatusDisabled
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
