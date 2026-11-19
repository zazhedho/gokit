package gokit

import "github.com/zazhedho/gokit/internal/utils"

// ResetEnvErrors clears what EnvErrors reports. Tests only: it resets global
// state shared by every GetEnv caller in the process.
func ResetEnvErrors() { utils.ResetEnvErrors() }
