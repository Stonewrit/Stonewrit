package ingest

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func parsePgUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, errors.New("empty uuid")
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
