//go:build norecover

package inspect

func init() { buildTags = append(buildTags, "norecover") }
