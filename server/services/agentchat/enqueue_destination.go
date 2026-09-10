package agentchat

import (
	"errors"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	EnqueueFromUser  = "user"
	EnqueueFromAgent = "agent"
)

var (
	ErrPairwiseViewOnly  = errors.New("pairwise rooms are view-only")
	ErrBotHomeRejectsA2A = errors.New(
		"cannot enqueue agent-to-agent mail into a bot home",
	)
	ErrPairwiseSpeakerNotInPair = errors.New(
		"pairwise enqueue accepts only the two bots in the pair",
	)
)

// EnqueueMail is the destination-guard input used by human compose and later
// A2A (message_room) POSTs. from_kind=agent never means "post into dest home".
type EnqueueMail struct {
	FromKind    string
	FromAgentID string
}

func GuardHumanCompose(thread db.AgentThread) error {
	if thread.RoomKind == RoomKindPairwise {
		return ErrPairwiseViewOnly
	}
	return nil
}

func GuardEnqueueDestination(thread db.AgentThread, mail EnqueueMail) error {
	fromKind := strings.TrimSpace(mail.FromKind)
	fromAgentID := strings.TrimSpace(mail.FromAgentID)
	switch thread.RoomKind {
	case RoomKindPairwise:
		if fromKind != EnqueueFromAgent {
			return ErrPairwiseViewOnly
		}
		a := strings.TrimSpace(thread.PairAgentIDA.String)
		b := strings.TrimSpace(thread.PairAgentIDB.String)
		if fromAgentID == "" || (fromAgentID != a && fromAgentID != b) {
			return ErrPairwiseSpeakerNotInPair
		}
		return nil
	case RoomKindBotHome:
		if fromKind != EnqueueFromAgent {
			return nil
		}
		homeID := strings.TrimSpace(thread.AgentID.String)
		if fromAgentID != "" && fromAgentID == homeID {
			return nil
		}
		return ErrBotHomeRejectsA2A
	case RoomKindPlan:
		return nil
	default:
		if fromKind == EnqueueFromAgent {
			return ErrBotHomeRejectsA2A
		}
		return nil
	}
}
