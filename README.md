<h1><a href="https://ergo.services"><img src=".github/images/logo.svg" alt="Ergo Framework" width="159" height="49"></a></h1>

[![Gitbook Documentation](https://img.shields.io/badge/GitBook-Documentation-f37f40?style=plastic&logo=gitbook&logoColor=white&style=flat)](https://docs.ergo.services)
[![GoDoc](https://pkg.go.dev/badge/ergo-services/ergo)](https://pkg.go.dev/ergo.services/ergo)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Telegram Community](https://img.shields.io/badge/Telegram-ergo__services-229ed9?style=flat&logo=telegram&logoColor=white)](https://t.me/ergo_services)
[![Twitter](https://img.shields.io/badge/Twitter-ergo__services-00acee?style=flat&logo=twitter&logoColor=white)](https://twitter.com/ergo_services)
[![Reddit](https://img.shields.io/badge/Reddit-r/ergo__services-ff4500?style=plastic&logo=reddit&logoColor=white&style=flat)](https://reddit.com/r/ergo_services)

[https://ergo.services](https://ergo.services)

### Purpose ###

### Quick start ###

### Features ###

### Requirements ###

* Go 1.20.x and above

### Versioning ###

Golang introduced [v2 rule](https://go.dev/blog/v2-go-modules) a while ago to solve complicated dependency issues. We found this solution very controversial and there is still a lot of discussion around it. So, we decided to keep the old way for the versioning, but have to use the git tag with v1 as a major version (due to "v2 rule" restrictions). Since now we use git tag pattern 1.999.XYZ where X - major number, Y - minor, Z - patch version.

### Changelog ###

Fully detailed changelog see in the [ChangeLog](ChangeLog.md) file.

### Benchmarks ###

You can find available benchmarks in the following repository https://github.com/ergo-services/benchmarks.

* Messaging performance (local, network)

* Memory consumption (demonstrates framework memory footprint) per process.

### Development and debugging ###

To enable Golang profiler just add `--tags debug` in your `go run` or `go build` (profiler runs at
`http://localhost:9009/debug/pprof`)

To disable panic recovery use `--tags norecover`.

To enable trace logging level for the internals (node, network,...) use `--tags trace` and set gen.LogLevelTrace for your node.

To run tests with cleaned test cache:

```
go vet
go clean -testcache
go test -v ./...
```

### Commercial support

please, contact support@ergo.services for more information
