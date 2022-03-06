package main

import (
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

func nerdctlVersion() string {
	nv, err := exec.Command("nerdctl", "--version").Output()
	if err != nil {
		log.Fatal(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	v = strings.Replace(v, "nerdctl version ", "", 1)
	return v
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	// new in 1.40 API:
	r.HEAD("/_ping", func(c *gin.Context) {
		c.Writer.Header().Set("API-Version", "1.26")
		c.Writer.Header().Set("Content-Length", "0")
		c.Status(http.StatusOK)
	})

	r.GET("/_ping", func(c *gin.Context) {
		c.Writer.Header().Set("API-Version", "1.24")
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.String(http.StatusOK, "OK")
	})

	r.GET("/v1.26/version", func(c *gin.Context) {
		var ver struct {
			Version       string
			APIVersion    string `json:"ApiVersion"`
			MinAPIVersion string `json:"MinAPIVersion,omitempty"`
			GitCommit     string
			GoVersion     string
			Os            string
			Arch          string
			KernelVersion string `json:",omitempty"`
			Experimental  bool   `json:",omitempty"`
			BuildTime     string `json:",omitempty"`
		}
		ver.Version = nerdctlVersion()
		ver.APIVersion = "1.26"
		ver.MinAPIVersion = "1.24"
		ver.GoVersion = runtime.Version()
		ver.Os = runtime.GOOS
		ver.Arch = runtime.GOARCH
		ver.Experimental = true
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, ver)
	})

	return r
}

func main() {
	r := setupRouter()
	//r.Run(":2375")
	r.RunUnix("nerdctl.sock")
}
