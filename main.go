/*
   Copyright 2022 Anders F Björklund

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

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

func nerdctlVer() map[string]interface{} {
	nc, err := exec.Command("nerdctl", "version", "--format", "{{json .}}").Output()
	if err != nil {
		log.Fatal(err)
	}
	var version map[string]interface{}
	err = json.Unmarshal(nc, &version)
	if err != nil {
		log.Fatal(err)
	}
	return version
}

func nerdctlInfo() map[string]interface{} {
	nc, err := exec.Command("nerdctl", "info", "--format", "{{json .}}").Output()
	if err != nil {
		log.Fatal(err)
	}
	var info map[string]interface{}
	err = json.Unmarshal(nc, &info)
	if err != nil {
		log.Fatal(err)
	}
	return info
}

func nerdctlImages() []map[string]interface{} {
	nc, err := exec.Command("nerdctl", "images", "--format", "{{json .}}").Output()
	if err != nil {
		log.Fatal(err)
	}
	var images []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(nc))
	for scanner.Scan() {
		var image map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &image)
		if err != nil {
			log.Fatal(err)
		}
		images = append(images, image)
	}
	return images
}

func nerdctlContainers() []map[string]interface{} {
	nc, err := exec.Command("nerdctl", "ps", "--format", "{{json .}}").Output()
	if err != nil {
		log.Fatal(err)
	}
	var containers []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(nc))
	for scanner.Scan() {
		var container map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &container)
		if err != nil {
			log.Fatal(err)
		}
		containers = append(containers, container)
	}
	return containers
}

func unixTime(s string) int64 {
	t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", s)
	if err != nil {
		log.Fatal(err)
	}
	return t.Unix()
}

func byteSize(s string) int64 {
	w := strings.Split(s, " ")
	n, err := strconv.ParseFloat(w[0], 64)
	if err != nil {
		log.Fatal(err)
	}
	m := 1
	switch w[1] {
	case "KiB":
		m = 1024
	case "MiB":
		m = 1024 * 1024
	case "GiB":
		m = 1024 * 1024 * 1024
	}
	return int64(n * float64(m))
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

	r.GET("/:ver/version", func(c *gin.Context) {
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
		version := nerdctlVer()
		client := version["Client"].(map[string]interface{})
		ver.Version = nerdctlVersion()
		ver.APIVersion = "1.26"
		ver.MinAPIVersion = "1.24"
		ver.GitCommit = client["GitCommit"].(string)
		ver.GoVersion = client["GoVersion"].(string)
		ver.Os = client["Os"].(string)
		ver.Arch = client["Arch"].(string)
		ver.Experimental = true
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, ver)
	})

	r.GET("/:ver/info", func(c *gin.Context) {
		var inf struct {
			ID                string
			Containers        int
			ContainersRunning int
			ContainersPaused  int
			ContainersStopped int
			Images            int
			Driver            string
			DriverStatus      [][2]string
			SystemStatus      [][2]string
			//Plugins            PluginsInfo
			MemoryLimit        bool
			SwapLimit          bool
			KernelMemory       bool
			CPUCfsPeriod       bool `json:"CpuCfsPeriod"`
			CPUCfsQuota        bool `json:"CpuCfsQuota"`
			CPUShares          bool
			CPUSet             bool
			IPv4Forwarding     bool
			BridgeNfIptables   bool
			BridgeNfIP6tables  bool `json:"BridgeNfIp6tables"`
			Debug              bool
			NFd                int
			OomKillDisable     bool
			NGoroutines        int
			SystemTime         string
			ExecutionDriver    string
			LoggingDriver      string
			CgroupDriver       string
			NEventsListener    int
			KernelVersion      string
			OperatingSystem    string
			OSType             string
			Architecture       string
			IndexServerAddress string
			//RegistryConfig     *registry.ServiceConfig
			NCPU              int
			MemTotal          int64
			DockerRootDir     string
			HTTPProxy         string `json:"HttpProxy"`
			HTTPSProxy        string `json:"HttpsProxy"`
			NoProxy           string
			Name              string
			Labels            []string
			ExperimentalBuild bool
			ServerVersion     string
			ClusterStore      string
			ClusterAdvertise  string
			SecurityOptions   []string
			//Runtimes           map[string]Runtime
			DefaultRuntime string
			//Swarm              swarm.Info
			// LiveRestoreEnabled determines whether containers should be kept
			// running when the daemon is shutdown or upon daemon start if
			// running containers are detected
			LiveRestoreEnabled bool
		}
		info := nerdctlInfo()
		inf.ID = info["ID"].(string)
		inf.Name = info["Name"].(string)
		inf.ServerVersion = nerdctlVersion()
		inf.NCPU = int(info["NCPU"].(float64))
		inf.MemTotal = int64(info["MemTotal"].(float64))
		inf.Driver = info["Driver"].(string)
		inf.LoggingDriver = info["LoggingDriver"].(string)
		inf.CgroupDriver = info["CgroupDriver"].(string)
		inf.KernelVersion = info["KernelVersion"].(string)
		inf.OperatingSystem = info["OperatingSystem"].(string)
		inf.OSType = info["OSType"].(string)
		inf.Architecture = info["Architecture"].(string)
		inf.ExperimentalBuild = true
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, inf)
	})

	r.GET("/:ver/images/json", func(c *gin.Context) {
		type img struct {
			ID          string `json:"Id"`
			ParentID    string `json:"ParentId"`
			RepoTags    []string
			RepoDigests []string
			Created     int64
			Size        int64
			VirtualSize int64
			Labels      map[string]string
		}
		imgs := []img{}
		images := nerdctlImages()
		for _, image := range images {
			var img img
			img.ID = image["ID"].(string)
			img.RepoTags = []string{image["Repository"].(string) + ":" + image["Tag"].(string)}
			img.RepoDigests = []string{image["Digest"].(string)}
			img.Created = unixTime(image["CreatedAt"].(string))
			img.Size = byteSize(image["Size"].(string))
			imgs = append(imgs, img)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, imgs)
	})

	r.GET("/:ver/containers/json", func(c *gin.Context) {
		type port struct {
			IP          string `json:"IP,omitempty"`
			PrivatePort uint16 `json:"PrivatePort"`
			PublicPort  uint16 `json:"PublicPort,omitempty"`
			Type        string `json:"Type"`
		}
		type ctr struct {
			ID         string `json:"Id"`
			Names      []string
			Image      string
			ImageID    string
			Command    string
			Created    int64
			Ports      []port
			SizeRw     int64 `json:",omitempty"`
			SizeRootFs int64 `json:",omitempty"`
			Labels     map[string]string
			State      string
			Status     string
			HostConfig struct {
				NetworkMode string `json:",omitempty"`
			}
			//NetworkSettings *SummaryNetworkSettings
			//Mounts          []MountPoint
		}
		ctrs := []ctr{}
		containers := nerdctlContainers()
		for _, container := range containers {
			var ctr ctr
			ctr.ID = container["ID"].(string)
			ctr.Names = []string{"/" + container["Names"].(string)}
			ctr.Image = container["Image"].(string)
			ctr.Command = container["Command"].(string)
			ctr.Created = unixTime(container["CreatedAt"].(string))
			ctr.Status = container["Status"].(string)
			ctrs = append(ctrs, ctr)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, ctrs)
	})

	return r
}

func main() {
	r := setupRouter()
	//r.Run(":2375")
	r.RunUnix("nerdctl.sock")
}
