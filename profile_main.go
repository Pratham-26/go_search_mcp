package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/user/glsi/internal/cache"
	"github.com/user/glsi/internal/engine"
)

func main() {
	// Test query and result count
	query := "golang programming tutorial"
	count := 10

	// Create CPU profile file
	cpuFile, err := os.Create("cpu.prof")
	if err != nil {
		panic(err)
	}
	defer cpuFile.Close()

	// Create memory profile file
	memFile, err := os.Create("mem.prof")
	if err != nil {
		panic(err)
	}
	defer memFile.Close()

	// Start CPU profiling
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		panic(err)
	}
	defer pprof.StopCPUProfile()

	// Setup engine
	dbPath := os.Getenv("GLSI_DB_PATH")
	c, err := cache.New(dbPath)
	if err != nil {
		panic(fmt.Errorf("initializing cache: %w", err))
	}
	defer c.Close()

	eng := engine.New(c, engine.Config{
		SearchEngine: "duckduckgo",
		RateLimit:    1 * time.Second,
	})

	// Print initial memory stats
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	fmt.Printf("Before search - Alloc: %d MB, TotalAlloc: %d MB, Sys: %d MB, NumGC: %d\n",
		m1.Alloc/1024/1024, m1.TotalAlloc/1024/1024, m1.Sys/1024/1024, m1.NumGC)

	// Run search with timing
	start := time.Now()
	result, err := eng.Search(context.Background(), query, count, false) // Use false to allow cache hit on rerun
	elapsed := time.Since(start)

	if err != nil {
		panic(fmt.Errorf("search failed: %w", err))
	}

	// Print final memory stats
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	fmt.Printf("\nAfter search - Alloc: %d MB, TotalAlloc: %d MB, Sys: %d MB, NumGC: %d\n",
		m2.Alloc/1024/1024, m2.TotalAlloc/1024/1024, m2.Sys/1024/1024, m2.NumGC)

	// Print results summary
	fmt.Printf("\n=== Profile Results ===\n")
	fmt.Printf("Query: %s\n", query)
	fmt.Printf("Requested results: %d\n", count)
	fmt.Printf("Results scraped: %d\n", result.ResultCount)
	fmt.Printf("From cache: %v\n", result.FromCache)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Content size: %d bytes (%.2f MB)\n", len(result.Content), float64(len(result.Content))/(1024*1024))
	fmt.Printf("\nMemory delta - Alloc: %+d MB, TotalAlloc: %+d MB, Sys: %+d MB\n",
		(m2.Alloc-m1.Alloc)/1024/1024, (m2.TotalAlloc-m1.TotalAlloc)/1024/1024, (m2.Sys-m1.Sys)/1024/1024)

	// Write memory profile
	runtime.GC() // Force GC before taking memory snapshot
	if err := pprof.WriteHeapProfile(memFile); err != nil {
		panic(err)
	}

	fmt.Println("\nProfiles written to:")
	fmt.Println("  cpu.prof  - CPU profile (analyze with: go tool pprof cpu.prof)")
	fmt.Println("  mem.prof  - Memory profile (analyze with: go tool pprof mem.prof)")
}
