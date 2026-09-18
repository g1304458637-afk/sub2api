// Package buildinfo 持有构建信息，供 /healthz 等运维端点读取。
// 值由 cmd/server 在启动时从 ldflags 注入的变量回填，避免 routes 包反向依赖 main。
package buildinfo

var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
)
