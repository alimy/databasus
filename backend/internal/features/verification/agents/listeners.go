package verification_agents

import "github.com/google/uuid"

type AgentHeartbeatedListener interface {
	OnAgentHeartbeated(agent *Agent, currentVerificationIDs []uuid.UUID) ([]uuid.UUID, error)
}
