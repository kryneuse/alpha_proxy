package store

import (
	"context"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type Store interface {
	Get(ctx context.Context, payloadID string) (*pii.Session, error)
	PutIfAbsent(ctx context.Context, session *pii.Session) (bool, error)
	Update(ctx context.Context, session *pii.Session) error
	Delete(ctx context.Context, payloadID string) error
}
