package policy

import (
	"context"
	"fmt"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type StaticProvider struct {
	policies map[string]pii.Policy
}

func NewStaticProvider(policies map[string]pii.Policy) *StaticProvider {
	copied := make(map[string]pii.Policy, len(policies))
	for consumerID, pol := range policies {
		copied[consumerID] = clonePolicy(pol)
	}
	return &StaticProvider{policies: copied}
}

func (p *StaticProvider) Get(ctx context.Context, consumerID string) (pii.Policy, error) {
	if err := ctx.Err(); err != nil {
		return pii.Policy{}, err
	}

	pol, ok := p.policies[consumerID]
	if !ok {
		return pii.Policy{}, fmt.Errorf("policy rejected: %w", pii.ErrPolicyRejected)
	}

	return clonePolicy(pol), nil
}

func clonePolicy(pol pii.Policy) pii.Policy {
	clone := pol
	if pol.AllowedKinds != nil {
		clone.AllowedKinds = make(map[pii.PIIKind]bool, len(pol.AllowedKinds))
		for kind, allowed := range pol.AllowedKinds {
			clone.AllowedKinds[kind] = allowed
		}
	}
	return clone
}
