module github.com/danmuck/edgectl

go 1.25.6

replace github.com/danmuck/smplog => ./third_party/smplog

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/danmuck/smplog v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
)
