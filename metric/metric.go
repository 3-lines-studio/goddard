package metric

import "errors"

// Turn is one turn of the agent with what it cost. It keeps no content: the
// request and the answer live in chat.events and axe.entries, and what this has
// is the numbers to add them up.
type Turn struct {
	OwnerKind      string
	OwnerID        string
	ProjectID      string
	ConversationID string
	UserID         string
	Source         string
	Model          string
	Input          int
	Output         int
	CachedInput    int
	Ms             int64
	Outcome        string
}

// The three ways a turn ends. A turn that hit its turn limit is `ok`: it
// answered, it just stopped.
const (
	OutcomeOK        = "ok"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

// Totals is what a set of turns adds up to.
type Totals struct {
	Turns       int   `json:"turns"`
	Input       int   `json:"input"`
	Output      int   `json:"output"`
	CachedInput int   `json:"cached_input"`
	Ms          int64 `json:"ms"`
	Failed      int   `json:"failed"`
	Cancelled   int   `json:"cancelled"`
}

// Day is one day of one model: what every turn it answered added up to. There
// is no owner and no project in it, because this is what somebody who is not
// the owner reads.
type Day struct {
	Date  string `json:"date"`
	Model string `json:"model"`
	Totals
}

// Summary is a window of turns: day by day, and what all of them add up to.
type Summary struct {
	From  int64  `json:"from"`
	Until int64  `json:"until"`
	Days  []Day  `json:"days"`
	Total Totals `json:"total"`
}

var (
	ErrOwnerKind      = errors.New("el dueño del turno tiene que ser de una persona o de una organización")
	ErrOwner          = errors.New("el turno no dice de quién es")
	ErrProject        = errors.New("el turno no dice de qué proyecto es")
	ErrConversation   = errors.New("el turno no dice de qué conversación es")
	ErrOutcome        = errors.New("el resultado del turno tiene que ser ok, failed o cancelled")
	ErrSource         = errors.New("el origen del turno no es uno conocido")
	ErrNegativeNumber = errors.New("los números de un turno no pueden ser negativos")
)

// Valid says whether the turn can be written as it is.
func (t Turn) Valid() error {
	switch t.OwnerKind {
	case "org", "user":
	default:
		return ErrOwnerKind
	}
	if t.OwnerID == "" {
		return ErrOwner
	}
	if t.ProjectID == "" {
		return ErrProject
	}
	if t.ConversationID == "" {
		return ErrConversation
	}
	switch t.Outcome {
	case OutcomeOK, OutcomeFailed, OutcomeCancelled:
	default:
		return ErrOutcome
	}
	switch t.Source {
	case "web", "telegram", "slack", "schedule":
	default:
		return ErrSource
	}
	if t.Input < 0 || t.Output < 0 || t.CachedInput < 0 || t.Ms < 0 {
		return ErrNegativeNumber
	}
	return nil
}
