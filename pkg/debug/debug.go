package debug

import (
	"encoding/json"
	"fmt"
)

func IndentedJsonFmt(s any) string {
	jsonB, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return fmt.Sprintf("failed to marshal value: %v", err)
	}
	return string(jsonB)
}
