module github.com/CrimsonCoder42/toolbench/compare

go 1.27

require (
	github.com/CrimsonCoder42/toolbench v0.0.0
	github.com/benbjohnson/clock v1.3.5
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/oklog/run v1.2.0
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/sourcegraph/conc v0.3.0
	github.com/valyala/bytebufferpool v1.0.0
	golang.org/x/sync v0.25.0
	golang.org/x/time v0.16.0
)

require (
	github.com/CrimsonCoder42/calcbench v0.3.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
)

replace github.com/CrimsonCoder42/toolbench => ../
