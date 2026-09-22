package service

import (
	"encoding/json"
	"fmt"
)

// Durable coordinator replays are decoded JSON maps, while first executions
// return typed results. Normalize both paths without losing response fields.
func decodeSubscriptionOperationResult[T any](data any) (*T, error) {
	if data == nil {
		return nil, fmt.Errorf("subscription operation returned no stored result")
	}
	if result, ok := data.(*T); ok && result != nil {
		return result, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encode stored subscription result: %w", err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode stored subscription result: %w", err)
	}
	return &result, nil
}
