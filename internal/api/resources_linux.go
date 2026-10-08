//go:build linux

package api

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type resources struct {
	LoadAvg     [3]float64 `json:"load_avg"`
	CPUs        int        `json:"cpus"`
	MemTotalMB  int64      `json:"mem_total_mb"`
	MemUsedMB   int64      `json:"mem_used_mb"`
	DiskTotalGB float64    `json:"disk_total_gb"`
	DiskUsedGB  float64    `json:"disk_used_gb"`
}

func readResources() resources {
	var r resources
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		for i := 0; i < 3 && i < len(f); i++ {
			r.LoadAvg[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		defer f.Close()
		var total, avail int64
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 2 {
				continue
			}
			v, _ := strconv.ParseInt(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = v
			case "MemAvailable:":
				avail = v
			}
		}
		r.MemTotalMB = total / 1024
		r.MemUsedMB = (total - avail) / 1024
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs("/home", &st); err == nil {
		total := float64(st.Blocks) * float64(st.Bsize)
		free := float64(st.Bavail) * float64(st.Bsize)
		r.DiskTotalGB = total / (1 << 30)
		r.DiskUsedGB = (total - free) / (1 << 30)
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		r.CPUs = strings.Count(string(b), "\nprocessor")
		if strings.HasPrefix(string(b), "processor") {
			r.CPUs++
		}
	}
	return r
}
