package db

import "embed"

// Files embeds the migration and seed SQL into the compiled binary so the
// migration runner works no matter where the binary is invoked from -
// no dependency on the process's working directory or on a deploy step
// copying this directory to the right relative path at runtime.
//
//go:embed migrations seeds
var Files embed.FS
