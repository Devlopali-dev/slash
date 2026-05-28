package store

import (
	storepb "github.com/devlopali-dev/slash/proto/gen/store"
)

func ConvertRowStatusStringToStorepb(status string) storepb.RowStatus {
	if status == "NORMAL" {
		return storepb.RowStatus_NORMAL
	}
	// Otherwise, fallback to archived status.
	return storepb.RowStatus_ARCHIVED
}

func ConvertVisibilityStringToStorepb(visibility string) storepb.Visibility {
	switch visibility {
	case "PUBLIC":
		return storepb.Visibility_PUBLIC
	case "WORKSPACE":
		return storepb.Visibility_WORKSPACE
	default:
		// 'PRIVATE' or any unknown value: treat as WORKSPACE (the minimum shared scope).
		// PRIVATE was removed in migration 1.0; rows with this value should not exist.
		return storepb.Visibility_WORKSPACE
	}
}
