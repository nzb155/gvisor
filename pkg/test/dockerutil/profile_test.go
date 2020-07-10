// Copyright 2020 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dockerutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testCase struct {
	name  string
	pprof Pprof
	// Wait `timeout` seconds before calling cleanup() on the profiled container.
	timeout       int
	expectedFiles []string
}

func TestPprof(t *testing.T) {
	// Basepath and expected file names for each type of profile.
	basePath := "/tmp/test/profile"
	block := "block.pprof"
	cpu := "cpu.pprof"
	goprofle := "go.pprof"
	heap := "heap.pprof"
	mutex := "mutex.pprof"

	testCases := []testCase{
		{
			name: "Cpu",
			pprof: Pprof{
				BasePath:   basePath,
				CPUProfile: true,
				Duration:   "4s",
			},
			timeout:       8,
			expectedFiles: []string{cpu},
		},
		{
			name: "All",
			pprof: Pprof{
				BasePath:         basePath,
				BlockProfile:     true,
				CPUProfile:       true,
				GoRoutineProfile: true,
				HeapProfile:      true,
				MutexProfile:     true,
				Duration:         "4s",
			},
			timeout:       6,
			expectedFiles: []string{block, cpu, goprofle, heap, mutex},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := MakeContainer(ctx, t)
			// Set basepath to include the container name so there are no conflicts.
			tc.pprof.BasePath = filepath.Join(tc.pprof.BasePath, c.Name)
			c.AddProfile(&tc.pprof)

			func() {
				defer c.CleanUp(ctx)

				// Start a container.
				if err := c.Spawn(ctx, RunOpts{
					Image:   "benchmarks/absl",
					WorkDir: "/abseil-cpp",
				}, "bazel", "build", "-c", "opt", "absl/base/..."); err != nil {
					t.Fatalf("run failed with: %v", err)
				}

				// Best effort wait for container to start running.
				for now := time.Now(); time.Since(now) < 5*time.Second; {
					if status, _ := c.Status(context.Background()); status.Running {
						break
					}
					time.Sleep(500 * time.Millisecond)
				}
				time.Sleep(time.Duration(tc.timeout) * time.Second)
			}()
			// Check each file exists and has data.
			for _, file := range tc.expectedFiles {
				stat, err := os.Stat(filepath.Join(tc.pprof.BasePath, file))
				if err != nil {
					t.Fatalf("stat failed with: %v", err)
				} else if stat.Size() < 1 {
					t.Fatalf("file not written to: %+v", stat)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	EnsureSupportedDockerVersion()
	os.Exit(m.Run())
}
