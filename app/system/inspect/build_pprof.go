//go:build pprof

package inspect

func init() { buildTags = append(buildTags, "pprof") }
