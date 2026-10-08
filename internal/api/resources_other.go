//go:build !linux

package api

import "runtime"

type resources struct {
	LoadAvg     [3]float64 `json:"load_avg"`
	CPUs        int        `json:"cpus"`
	MemTotalMB  int64      `json:"mem_total_mb"`
	MemUsedMB   int64      `json:"mem_used_mb"`
	DiskTotalGB float64    `json:"disk_total_gb"`
	DiskUsedGB  float64    `json:"disk_used_gb"`
}

func readResources() resources {
	return resources{CPUs: runtime.NumCPU()}
}
