package server

import (
	"runtime"
)

var (
	BuildVersion = "dev"
	BuildCommit  = ""
	BuildDate    = ""
)

func About() AboutInfo {
	return AboutInfo{
		Name:        "LocalTwitter",
		Version:     BuildVersion,
		Commit:      BuildCommit,
		BuildDate:   BuildDate,
		GoVersion:   runtime.Version(),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		BuildMode:   "single-go-service",
		Description: "局域网版本地 Twitter 作品浏览 Web 应用",
	}
}
