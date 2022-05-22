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
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func nerdctlVersion() string {
	nv, err := exec.Command("nerdctl", "--version").Output()
	if err != nil {
		// log stderr for basic troubleshooting
		if exiterr, ok := err.(*exec.ExitError); ok {
			log.Printf(string(exiterr.Stderr))
		}
		log.Fatal(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	v = strings.Replace(v, "nerdctl version ", "", 1)
	return v
}

func containerdVersion() (string, map[string]string) {
	nv, err := exec.Command("containerd", "--version").Output()
	if err != nil {
		log.Fatal(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	// containerd github.com/containerd/containerd Version GitCommit
	c := strings.SplitN(v, " ", 4)
	if len(c) == 4 && c[0] == "containerd" {
		return c[2], map[string]string{"GitCommit": c[3]}
	}
	return v, nil
}

func buildkitdVersion() (string, map[string]string) {
	nv, err := exec.Command("buildkitd", "--version").Output()
	if err != nil {
		log.Print(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	// buildkitd github.com/moby/buildkit Version GitCommit
	c := strings.SplitN(v, " ", 4)
	if len(c) == 4 && c[0] == "buildkitd" {
		return c[2], map[string]string{"GitCommit": c[3]}
	}
	return v, nil
}

// vercmp compares two version strings
// returns -1 if v1 < v2, 1 if v1 > v2, 0 otherwise.
func vercmp(v1, v2 string) int {
	var (
		currTab  = strings.Split(v1, ".")
		otherTab = strings.Split(v2, ".")
	)

	max := len(currTab)
	if len(otherTab) > max {
		max = len(otherTab)
	}
	for i := 0; i < max; i++ {
		var currInt, otherInt int

		if len(currTab) > i {
			currInt, _ = strconv.Atoi(currTab[i])
		}
		if len(otherTab) > i {
			otherInt, _ = strconv.Atoi(otherTab[i])
		}
		if currInt > otherInt {
			return 1
		}
		if otherInt > currInt {
			return -1
		}
	}
	return 0
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

type Platform struct {
	Name string
}

func nerdctlPlatform() Platform {
	return Platform{Name: ""}
}

type ComponentVersion struct {
	Name    string
	Version string
	Details map[string]string `json:",omitempty"`
}

func nerdctlComponents() []ComponentVersion {
	var cmp []ComponentVersion
	cmp = append(cmp, ComponentVersion{Name: "nerdctl", Version: nerdctlVersion()})
	version, details := containerdVersion()
	cmp = append(cmp, ComponentVersion{Name: "containerd", Version: version, Details: details})
	if version, details = buildkitdVersion(); version != "" {
		cmp = append(cmp, ComponentVersion{Name: "buildkitd", Version: version, Details: details})
	}
	return cmp
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

func nerdctlContainers(all bool) []map[string]interface{} {
	args := []string{"ps"}
	if all {
		args = append(args, "-a")
	}
	args = append(args, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
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

func nerdctlPull(name string, w io.Writer) error {
	args := []string{"pull"}
	args = append(args, name)
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(nc), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		data := map[string]string{"stream": line + "\n"}
		l, _ := json.Marshal(data)
		w.Write(l)
		w.Write([]byte{'\n'})
	}
	return nil
}

func nerdctlLoad(quiet bool, r io.Reader, w io.Writer) error {
	args := []string{"load"}
	cmd := exec.Command("nerdctl", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	go func() error {
		defer stdin.Close()
		if _, err := io.Copy(stdin, r); err != nil {
			return err
		}
		return nil
	}()
	nc, err := cmd.Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(nc), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		data := map[string]string{"stream": line + "\n"}
		l, _ := json.Marshal(data)
		w.Write(l)
		w.Write([]byte{'\n'})
	}
	return nil
}

func nerdctlSave(names []string, w io.Writer) error {
	args := []string{"save"}
	args = append(args, names...)
	cmd := exec.Command("nerdctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() error {
		defer stdout.Close()
		if _, err := io.Copy(w, stdout); err != nil {
			return err
		}
		return nil
	}()
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func nerdctlBuild(dir string, w io.Writer, t string, f string) error {
	args := []string{"build"}
	if t != "" {
		args = append(args, "-t")
		args = append(args, t)
	}
	if f != "" {
		args = append(args, "-f")
		args = append(args, f)
	}
	args = append(args, dir)
	log.Printf("build %v\n", args)
	// TODO: stream
	cmd := exec.Command("nerdctl", args...)
	nc, err := cmd.Output()
	if err != nil {
		return err
	}
	lines := strings.Split(string(nc), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		data := map[string]string{"stream": line + "\n"}
		l, _ := json.Marshal(data)
		w.Write(l)
		w.Write([]byte{'\n'})
	}
	return nil
}

func extractTar(dst string, r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
			log.Fatal(err)
		}

		target := filepath.Join(dst, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
			f.Close()
		}
	}
	return nil
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	// new in 1.40 API:
	r.HEAD("/_ping", func(c *gin.Context) {
		c.Writer.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Add("Pragma", "no-cache")
		c.Writer.Header().Set("API-Version", "1.40")
		c.Writer.Header().Set("Content-Length", "0")
		c.Status(http.StatusOK)
	})

	r.GET("/_ping", func(c *gin.Context) {
		c.Writer.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Add("Pragma", "no-cache")
		c.Writer.Header().Set("API-Version", "1.24")
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.String(http.StatusOK, "OK")
	})

	r.GET("/:ver/version", func(c *gin.Context) {
		apiver := c.Param("ver")
		log.Printf("api: %s", apiver)
		var ver struct {
			Platform   struct{ Name string } `json:",omitempty"`
			Components []ComponentVersion    `json:",omitempty"`

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
		ver.APIVersion = "1.40"
		ver.MinAPIVersion = "1.24"
		ver.GitCommit = client["GitCommit"].(string)
		ver.GoVersion = client["GoVersion"].(string)
		ver.Os = client["Os"].(string)
		ver.Arch = client["Arch"].(string)
		ver.Experimental = true
		if vercmp(apiver, "v1.35") > 0 {
			ver.Platform = nerdctlPlatform()
			ver.Components = nerdctlComponents()
		}
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

	r.POST("/:ver/images/create", func(c *gin.Context) {
		from := c.Query("fromImage")
		tag := c.Query("tag")
		name := from + ":" + tag
		log.Printf("name: %s", name)
		c.Writer.Header().Set("Content-Type", "application/json")
		err := nerdctlPull(name, c.Writer)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	r.POST("/:ver/images/load", func(c *gin.Context) {
		quiet := c.Query("quiet")
		log.Printf("quiet: %s", quiet)
		contentType := c.Request.Header.Get("Content-Type")
		if contentType != "application/tar" && contentType != "application/x-tar" {
			http.Error(c.Writer, fmt.Sprintf("%s not tar", contentType), http.StatusBadRequest)
			return
		}
		br := bufio.NewReader(c.Request.Body)
		c.Writer.Header().Set("Content-Type", "application/json")
		err := nerdctlLoad(quiet == "1", br, c.Writer)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	r.GET("/:ver/images/get", func(c *gin.Context) {
		names, exists := c.GetQueryArray("names")
		if !exists {
			c.Status(http.StatusInternalServerError)
			return
		}
		log.Printf("names: %s", names)
		c.Writer.Header().Set("Content-Type", "application/x-tar")
		err := nerdctlSave(names, c.Writer)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	r.GET("/:ver/containers/json", func(c *gin.Context) {
		all := c.Query("all")
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
		containers := nerdctlContainers(all == "1")
		for _, container := range containers {
			var ctr ctr
			ctr.ID = container["ID"].(string)
			ctr.Names = []string{"/" + container["Names"].(string)}
			ctr.Image = container["Image"].(string)
			ctr.Command = strings.Trim(container["Command"].(string), "\"")
			ctr.Created = unixTime(container["CreatedAt"].(string))
			ctr.Status = container["Status"].(string)
			ctrs = append(ctrs, ctr)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, ctrs)
	})

	r.POST("/:ver/build", func(c *gin.Context) {
		contentType := c.Request.Header.Get("Content-Type")
		if contentType != "application/tar" && contentType != "application/x-tar" {
			http.Error(c.Writer, fmt.Sprintf("%s not tar", contentType), http.StatusBadRequest)
			return
		}
		var r io.Reader
		br := bufio.NewReader(c.Request.Body)
		r = br
		magic, err := br.Peek(2)
		if err != nil {
			log.Fatal(err)
		}
		if magic[0] == 0x1f && magic[1] == 0x8b {
			r, err = gzip.NewReader(br)
			if err != nil {
				log.Fatal(err)
			}
		}
		dir, err := ioutil.TempDir("", "build")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		extractTar(dir, r)
		tag := c.Query("t")
		dockerfile := c.Query("dockerfile")
		c.Writer.Header().Set("Content-Type", "application/json")
		err = nerdctlBuild(dir, c.Writer, tag, dockerfile)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	return r
}

func main() {
	nerdctlVersion()
	r := setupRouter()
	//r.Run(":2375")
	r.RunUnix("nerdctl.sock")
}
