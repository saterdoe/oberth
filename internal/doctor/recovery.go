package doctor

import "time"

type RecoverableSession struct {
	ID        string
	Status    string
	UpdatedAt time.Time
	Artifacts []string
}

type RecoveryAction string

const (
	RecoveryResume RecoveryAction = "resume"
	RecoveryCancel RecoveryAction = "cancel"
)

type RecoveryManager struct {
	StaleAfter time.Duration
	Now        func() time.Time
	Resume     func(string) error
	Cancel     func(string) error
	Cleanup    func(string) error
}

func (m RecoveryManager) Detect(sessions []RecoverableSession) []RecoverableSession {
	now := time.Now()
	if m.Now != nil {
		now = m.Now()
	}
	var stale []RecoverableSession
	for _, session := range sessions {
		if session.Status == "active" && now.Sub(session.UpdatedAt) >= m.StaleAfter {
			stale = append(stale, session)
		}
	}
	return stale
}

func (m RecoveryManager) Recover(session RecoverableSession, action RecoveryAction) error {
	if m.Cleanup != nil {
		if err := m.Cleanup(session.ID); err != nil {
			return err
		}
	}
	switch action {
	case RecoveryResume:
		if m.Resume != nil {
			return m.Resume(session.ID)
		}
	case RecoveryCancel:
		if m.Cancel != nil {
			return m.Cancel(session.ID)
		}
	}
	return nil
}
