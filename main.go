/*
   Copyright The containerd Authors.
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
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/tj/go-naturaldate"
)

func nerdctlVersion() (string, map[string]string) {
	nv, err := exec.Command("nerdctl", "--version").Output()
	if err != nil {
		// log stderr for basic troubleshooting
		if exiterr, ok := err.(*exec.ExitError); ok {
			log.Print(string(exiterr.Stderr))
		}
		log.Fatal(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	v = strings.Replace(v, "nerdctl version ", "", 1)
	return v, nil
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
		v = strings.Replace(c[2], "v", "", 1)
		if c[3] != "" {
			return v, map[string]string{"GitCommit": c[3]}
		}
	}
	return v, nil
}

func buildctlVersion() (string, map[string]string) {
	nv, err := exec.Command("buildctl", "--version").Output()
	if err != nil {
		log.Print(err)
	}
	v := strings.TrimSuffix(string(nv), "\n")
	// buildctl github.com/moby/buildkit Version GitCommit
	c := strings.SplitN(v, " ", 4)
	if len(c) == 4 && c[0] == "buildctl" {
		v = strings.Replace(c[2], "v", "", 1)
		if c[3] != "" {
			return v, map[string]string{"GitCommit": c[3]}
		}
	}
	return v, nil
}

func runcVersion() (string, map[string]string) {
	nv, err := exec.Command("runc", "--version").Output()
	if err != nil {
		log.Fatal(err)
	}
	l := strings.Split(string(nv), "\n")
	if len(l) == 0 {
		return "", nil
	}
	// runc version Version
	v := strings.Replace(l[0], "runc version ", "", 1)
	if len(l) > 1 && strings.HasPrefix(l[1], "commit: ") {
		s := strings.Replace(l[1], "commit: ", "", 1)
		if strings.Contains(s, "g") {
			s = strings.Split(s, "g")[1]
		}
		return v, map[string]string{"GitCommit": s}
	}
	return v, nil
}

func tiniVersion() (string, map[string]string) {
	nv, err := exec.Command("tini", "--version").Output()
	if err != nil {
		// tini is optional (--init-binary)
		return "", nil
	}
	v := strings.TrimSuffix(string(nv), "\n")
	// tini version Version - git.da39a3e
	v = strings.Replace(v, "tini version ", "", 1)
	c := strings.SplitN(v, " ", 3)
	if len(c) == 3 && c[1] == "-" {
		v = c[0]
		s := strings.Replace(c[2], "git.", "", 1)
		return v, map[string]string{"GitCommit": s}
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
	return Platform{Name: "\U0001f913"}
}

type ComponentVersion struct {
	Name    string
	Version string
	Details map[string]string `json:",omitempty"`
}

func nerdctlComponents() []ComponentVersion {
	var cmp []ComponentVersion
	version, details := nerdctlVersion()
	cmp = append(cmp, ComponentVersion{Name: "nerdctl", Version: version, Details: details})
	if version, details := buildctlVersion(); version != "" {
		cmp = append(cmp, ComponentVersion{Name: "buildctl", Version: version, Details: details})
	}
	version, details = containerdVersion()
	cmp = append(cmp, ComponentVersion{Name: "containerd", Version: version, Details: details})
	if version, details := runcVersion(); version != "" {
		cmp = append(cmp, ComponentVersion{Name: "runc", Version: version, Details: details})
	}
	if version, details := tiniVersion(); version != "" { // renamed to "docker-init" in docker
		cmp = append(cmp, ComponentVersion{Name: "tini", Version: version, Details: details})
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

func parseImageFilter(param []byte) string {
	if len(param) == 0 {
		return ""
	}
	// filters: {"reference":{"busybox":true}}
	var filters map[string]interface{}
	err := json.Unmarshal(param, &filters)
	if err != nil {
		log.Fatal(err)
	}
	ref := filters["reference"].(map[string]interface{})
	for key := range ref {
		return key
	}
	return ""
}

func nerdctlImages(filter string) []map[string]interface{} {
	args := []string{"images"}
	if filter != "" {
		args = append(args, filter)
	}
	args = append(args, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
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

func nerdctlImage(name string) (map[string]interface{}, error) {
	args := []string{"image", "inspect", "--mode", "dockercompat"}
	args = append(args, name, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		return nil, err
	}
	var image map[string]interface{}
	err = json.Unmarshal(nc, &image)
	if err != nil {
		log.Fatal(err)
	}
	return image, nil
}

func nerdctlHistory(name string) ([]map[string]interface{}, error) {
	args := []string{"history"}
	args = append(args, name, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		return nil, err
	}
	var history []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(nc))
	for scanner.Scan() {
		var entry map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &entry)
		if err != nil {
			log.Fatal(err)
		}
		history = append(history, entry)
	}
	return history, nil
}

func nerdctlTag(source string, target string) error {
	args := []string{"tag"}
	args = append(args, source)
	args = append(args, target)
	err := exec.Command("nerdctl", args...).Run()
	if err != nil {
		return err
	}
	return nil
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

func nerdctlContainer(name string) (map[string]interface{}, error) {
	args := []string{"container", "inspect", "--mode", "dockercompat"}
	args = append(args, name, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		return nil, err
	}
	var image map[string]interface{}
	err = json.Unmarshal(nc, &image)
	if err != nil {
		log.Fatal(err)
	}
	return image, nil
}

func parseVolumeFilter(param []byte) string {
	if len(param) == 0 {
		return ""
	}
	// filters: {"name":{"vol":true}}
	var filters map[string]map[string]interface{}
	err := json.Unmarshal(param, &filters)
	if err != nil {
		log.Fatal(err)
	}
	filter := ""
	for key, ref := range filters {
		for val := range ref {
			filter += fmt.Sprintf("%s=%s", key, val)
		}
	}
	return filter
}

func nerdctlVolumes(filter string) []map[string]interface{} {
	args := []string{"volume", "ls"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	args = append(args, "--format", "{{json .}}")
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		log.Fatal(err)
	}
	var volumes []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(nc))
	for scanner.Scan() {
		var volume map[string]interface{}
		err = json.Unmarshal(scanner.Bytes(), &volume)
		if err != nil {
			log.Fatal(err)
		}
		volumes = append(volumes, volume)
	}
	return volumes
}

func unixTime(s string) int64 {
	t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", s)
	if err != nil {
		log.Fatal(err)
	}
	return t.Unix()
}

func unixNatural(s string) int64 {
	t, err := naturaldate.Parse(s, time.Now())
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
		_, err = w.Write(l)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte{'\n'})
		if err != nil {
			return err
		}
	}
	return nil
}

func nerdctlPush(name string, w io.Writer) error {
	args := []string{"push"}
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
		_, err = w.Write(l)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte{'\n'})
		if err != nil {
			return err
		}
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
	errors := make(chan error)
	go func() {
		defer stdin.Close()
		if _, err := io.Copy(stdin, r); err != nil {
			errors <- err
		}
		errors <- nil
	}()
	nc, err := cmd.Output()
	if err != nil {
		return err
	}
	if err := <-errors; err != nil {
		return err
	}
	lines := strings.Split(string(nc), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		data := map[string]string{"stream": line + "\n"}
		l, _ := json.Marshal(data)
		_, err = w.Write(l)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte{'\n'})
		if err != nil {
			return err
		}
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
	errors := make(chan error)
	go func() {
		defer stdout.Close()
		if _, err := io.Copy(w, stdout); err != nil {
			errors <- err
		}
		errors <- nil
	}()
	err = cmd.Run()
	if err != nil {
		return err
	}
	if err := <-errors; err != nil {
		return err
	}
	return nil
}

func nerdctlRmi(name string, w io.Writer) error {
	args := []string{"rmi"}
	args = append(args, name)
	nc, err := exec.Command("nerdctl", args...).Output()
	if err != nil {
		return err
	}
	removed := []map[string]string{}
	lines := strings.Split(string(nc), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Untagged: ") {
			image := strings.Replace(line, "Untagged: ", "", 1)
			removed = append(removed, map[string]string{"Untagged": image})
		} else if strings.HasPrefix(line, "Deleted:") {
			image := strings.Replace(line, "Deleted: ", "", 1)
			removed = append(removed, map[string]string{"Deleted": image})
		}
	}
	d, _ := json.Marshal(removed)
	_, err = w.Write(d)
	if err != nil {
		return err
	}
	return nil
}

func parseObject(param []byte) map[string]interface{} {
	if len(param) == 0 {
		return nil
	}
	var args map[string]interface{}
	err := json.Unmarshal(param, &args)
	if err != nil {
		log.Fatal(err)
	}
	return args
}

func nerdctlBuild(dir string, w io.Writer, t string, f string, p string, ba map[string]interface{}, l map[string]interface{}) error {
	args := []string{"build"}
	if t != "" {
		args = append(args, "-t")
		args = append(args, t)
	}
	if f != "" {
		args = append(args, "-f")
		args = append(args, filepath.Join(dir, f))
	}
	if p != "" {
		args = append(args, "--platform")
		args = append(args, p)
	}
	if len(ba) > 0 {
		for k, v := range ba {
			arg := fmt.Sprintf("%s=%s", k, v.(string))
			args = append(args, "--build-arg="+arg)
		}
	}
	if len(l) > 0 {
		for k, v := range l {
			arg := fmt.Sprintf("%s=%s", k, v.(string))
			args = append(args, "--label="+arg)
		}
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
		_, err = w.Write(l)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte{'\n'})
		if err != nil {
			return err
		}
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

func stringArray(options []interface{}) []string {
	result := []string{}
	for _, option := range options {
		result = append(result, option.(string))
	}
	return result
}

// regular expression for slashes-in-parameter workaround
var reImagesPush = regexp.MustCompile(`^/(?P<ver>.*)/images/(?P<name>.*)/push$`)

// regular expression for starting version number in url
var reApiVersion = regexp.MustCompile(`^/(?P<ver>[0-9][.][0-9]+)/.*$`)

const CurrentAPIVersion = "1.40" // 19.03
const MinimumAPIVersion = "1.24" // 1.12

func setupRouter() *gin.Engine {

	r := gin.Default()
	err := r.SetTrustedProxies(nil)
	if err != nil {
		log.Print(err)
	}

	// new in 1.40 API:
	r.HEAD("/_ping", func(c *gin.Context) {
		c.Writer.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Add("Pragma", "no-cache")
		c.Writer.Header().Set("API-Version", CurrentAPIVersion)
		c.Writer.Header().Set("Content-Length", "0")
		c.Status(http.StatusOK)
	})

	r.GET("/_ping", func(c *gin.Context) {
		c.Writer.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Add("Pragma", "no-cache")
		c.Writer.Header().Set("API-Version", MinimumAPIVersion)
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.String(http.StatusOK, "OK")
	})

	r.GET("/:ver/version", func(c *gin.Context) {
		apiver := c.Param("ver")
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
		ver.Version, _ = nerdctlVersion()
		ver.APIVersion = CurrentAPIVersion
		ver.MinAPIVersion = MinimumAPIVersion
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
		type runtime struct {
			Path string   `json:"path"`
			Args []string `json:"runtimeArgs,omitempty"`
		}
		type swarm struct {
			LocalNodeState string
		}
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
			CgroupVersion      string `json:",omitempty"`
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
			Runtimes          map[string]runtime
			DefaultRuntime    string
			Swarm             swarm
			// LiveRestoreEnabled determines whether containers should be kept
			// running when the daemon is shutdown or upon daemon start if
			// running containers are detected
			LiveRestoreEnabled bool
			InitBinary         string
		}
		info := nerdctlInfo()
		inf.ID = info["ID"].(string)
		inf.Containers = len(nerdctlContainers(true))
		inf.Images = len(nerdctlImages(""))
		inf.Name = info["Name"].(string)
		inf.ServerVersion, _ = nerdctlVersion()
		inf.NCPU = int(info["NCPU"].(float64))
		inf.MemTotal = int64(info["MemTotal"].(float64))
		inf.Driver = info["Driver"].(string)
		inf.MemoryLimit = info["MemoryLimit"].(bool)
		inf.SwapLimit = info["SwapLimit"].(bool)
		inf.OomKillDisable = info["OomKillDisable"].(bool)
		inf.CPUCfsPeriod = info["CpuCfsPeriod"].(bool)
		inf.CPUCfsQuota = info["CpuCfsQuota"].(bool)
		inf.CPUShares = info["CPUShares"].(bool)
		inf.CPUSet = info["CPUSet"].(bool)
		inf.IPv4Forwarding = info["IPv4Forwarding"].(bool)
		inf.BridgeNfIptables = info["BridgeNfIptables"].(bool)
		inf.BridgeNfIP6tables = info["BridgeNfIp6tables"].(bool)
		inf.LoggingDriver = info["LoggingDriver"].(string)
		inf.CgroupDriver = info["CgroupDriver"].(string)
		inf.CgroupVersion = info["CgroupVersion"].(string)
		inf.KernelVersion = info["KernelVersion"].(string)
		inf.OperatingSystem = info["OperatingSystem"].(string)
		inf.OSType = info["OSType"].(string)
		inf.Architecture = info["Architecture"].(string)
		inf.ExperimentalBuild = true
		inf.DefaultRuntime = "runc"
		inf.Runtimes = map[string]runtime{"runc": {Path: "runc"}}
		inf.Swarm = swarm{LocalNodeState: "inactive"}
		inf.InitBinary = "tini"
		inf.SecurityOptions = stringArray(info["SecurityOptions"].([]interface{}))
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, inf)
	})

	r.GET("/:ver/images/json", func(c *gin.Context) {
		filters := c.Query("filters")
		filter := parseImageFilter([]byte(filters))
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
		images := nerdctlImages(filter)
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

	r.GET("/:ver/images/:name/json", func(c *gin.Context) {
		name := c.Param("name")
		image, err := nerdctlImage(name)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusNotFound)
			return
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, image)
	})

	r.GET("/:ver/images/:name/history", func(c *gin.Context) {
		name := c.Param("name")
		nchistory, err := nerdctlHistory(name)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusNotFound)
			return
		}

		type hist struct {
			Comment   string   `json:"Comment"`
			Created   int64    `json:"Created"`
			CreatedBy string   `json:"CreatedBy"`
			ID        string   `json:"Id"`
			Size      int64    `json:"Size"`
			Tags      []string `json:"Tags"`
		}
		history := []hist{}
		for _, nch := range nchistory {
			var h hist
			h.ID = nch["Snapshot"].(string)
			h.Created = unixNatural(nch["CreatedSince"].(string))
			h.CreatedBy = nch["CreatedBy"].(string)
			h.Size = byteSize(nch["Size"].(string))
			h.Comment = nch["Comment"].(string)
			history = append(history, h)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, history)
	})

	r.POST("/:ver/images/:name/tag", func(c *gin.Context) {
		name := c.Param("name")
		repo := c.Query("repo")
		tag := c.Query("tag")
		err := nerdctlTag(name, fmt.Sprintf("%s:%s", repo, tag))
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	r.POST("/:ver/images/:name/push", func(c *gin.Context) {
		name := c.Param("name")
		tag := c.Query("tag")
		name = name + ":" + tag
		log.Printf("name: %s", name)
		c.Writer.Header().Set("Content-Type", "application/json")
		err := nerdctlPush(name, c.Writer)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
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

	r.DELETE("/:ver/images/*name", func(c *gin.Context) {
		name := c.Param("name")
		// handle extra slash from using parameter wildcard
		if strings.HasPrefix(name, "/") {
			name = strings.Replace(name, "/", "", 1)
		}
		log.Printf("name: %s", name)
		err := nerdctlRmi(name, c.Writer)
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
			Mounts []interface{} // MountPoint
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
			ctr.Mounts = make([]interface{}, 0)
			ctrs = append(ctrs, ctr)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, ctrs)
	})

	r.GET("/:ver/containers/:name/json", func(c *gin.Context) {
		name := c.Param("name")
		container, err := nerdctlContainer(name)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusNotFound)
			return
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, container)
	})

	r.GET("/:ver/volumes", func(c *gin.Context) {
		filters := c.Query("filters")
		filter := parseVolumeFilter([]byte(filters))
		type vol struct {
			Driver     string
			Labels     map[string]string
			Mountpoint string
			Name       string
			Options    map[string]string
			Scope      string
		}
		vols := []vol{}
		volumes := nerdctlVolumes(filter)
		for _, volume := range volumes {
			var vol vol
			vol.Name = volume["Name"].(string)
			vol.Driver = volume["Driver"].(string)
			vol.Scope = volume["Scope"].(string)
			vol.Mountpoint = volume["Mountpoint"].(string)
			vols = append(vols, vol)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		data := map[string]interface{}{"Volumes": vols, "Warnings": []string{}}
		c.JSON(http.StatusOK, data)
	})

	r.GET("/:ver/system/df", func(c *gin.Context) {
		type DiskUsage struct {
			LayersSize  int64
			Images      []interface{} // *ImageSummary
			Containers  []interface{} // *Container
			Volumes     []interface{}
			BuildCache  []interface{}
			BuilderSize int64
		}
		var du DiskUsage
		du.Images = make([]interface{}, 0)
		for i := range nerdctlImages("") {
			// TODO: du.Images = append(du.Images, ...)
			_ = i
		}
		du.Containers = make([]interface{}, 0)
		for c := range nerdctlContainers(true) {
			// TODO: du.Containers = append(du.Containers, ...)
			_ = c
		}
		du.Volumes = make([]interface{}, 0)
		for v := range nerdctlVolumes("") {
			// TODO: du.Volumes = append(du.Volumes, ...)
			_ = v
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, du)
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
		dir, err := os.MkdirTemp("", "build")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		err = extractTar(dir, r)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		tag := c.Query("t")
		dockerfile := c.Query("dockerfile")
		platform := c.Query("platform")
		c.Writer.Header().Set("Content-Type", "application/json")
		buildargs := parseObject([]byte(c.Query("buildargs")))
		labels := parseObject([]byte(c.Query("labels")))
		err = nerdctlBuild(dir, c.Writer, tag, dockerfile, platform, buildargs, labels)
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	r.NoRoute(func(c *gin.Context) {
		// the "push" route doesn't match name containing slashes (like repo)
		if m := reImagesPush.FindStringSubmatch(c.Request.URL.Path); m != nil {
			name := m[reImagesPush.SubexpIndex("name")]
			tag := c.Query("tag")
			name = name + ":" + tag
			log.Printf("name: %s", name)
			c.Writer.Header().Set("Content-Type", "application/json")
			err := nerdctlPush(name, c.Writer)
			if err != nil {
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusOK)
		}
		// some clients don't negotiate for the API version, before commands
		if m := reApiVersion.FindStringSubmatch(c.Request.URL.Path); m == nil {
			c.Request.URL.Path = "/" + CurrentAPIVersion + c.Request.URL.Path
			r.HandleContext(c)
		}
	})

	return r
}

var rootCmd = &cobra.Command{
	Use:          "nerdctld",
	Short:        "A docker api endpoint for nerdctl and containerd",
	RunE:         run,
	Version:      version(),
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "debug mode")
	rootCmd.PersistentFlags().StringVar(&addr, "addr", "", "listening address")
	rootCmd.PersistentFlags().StringVar(&socket, "socket", "nerdctl.sock", "location of socket file")
}

var debug bool
var addr string
var socket string

func run(cmd *cobra.Command, args []string) error {
	nerdctlVersion()

	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := setupRouter()
	// deprecated parameter
	if addr == "" && socket != "" {
		addr = "unix://" + socket
	}
	addrSlice := strings.SplitN(addr, "://", 2)
	if len(addrSlice) < 2 {
		return fmt.Errorf("did you mean unix://%s", addr)
	}
	proto := addrSlice[0]
	listenAddr := addrSlice[1]
	switch proto {
	case "tcp":
		return r.Run(listenAddr)
	case "fd":
		_, err := daemon.SdNotify(false, daemon.SdNotifyReady)
		if err != nil {
			return err
		}
		files := activation.Files(true)
		return r.RunFd(int(files[0].Fd()))
	case "unix":
		socket := listenAddr
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			// http.Serve never returns, if successful
			os.Remove(socket)
			os.Exit(0)
		}()
		return r.RunUnix(socket)
	default:
		return fmt.Errorf("addr %s not supported", addr)
	}
}

func version() string {
	return "0.2.1"
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
