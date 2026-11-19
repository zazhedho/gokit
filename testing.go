package gokit

import "github.com/zazhedho/gokit/internal/utils"

// ResetEnvErrors clears what EnvErrors reports. It exists for tests that
// exercise GetEnv with invalid values; production code has no use for it.
func ResetEnvErrors() { utils.ResetEnvErrors() }
