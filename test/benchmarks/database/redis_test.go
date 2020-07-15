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

package database

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gvisor.dev/gvisor/pkg/test/dockerutil"
)

// All possible operations from redis. Note: "ping" will
// run both PING_INLINE and PING_BUILD.
var operations []string = []string{
	"PING_INLINE",
	"PING_BULK",
	"SET",
	"GET",
	"INCR",
	"LPUSH",
	"RPUSH",
	"LPOP",
	"RPOP",
	"SADD",
	"HSET",
	"SPOP",
	"LRANGE_100",
	"LRANGE_300",
	"LRANGE_500",
	"LRANGE_600",
	"MSET",
}

// BenchmarkRedis runs redis-benchmark against a redis instance and reports
// data in queries per second. Each is reported by named operation (e.g. LPUSH).
func BenchmarkRedis(b *testing.B) {
	dockerutil.RuntimePath()

	clientMachine, err := h.GetMachine()
	if err != nil {
		b.Fatalf("failed to get machine: %v", err)
	}
	defer clientMachine.CleanUp()

	serverMachine, err := h.GetMachine()
	if err != nil {
		b.Fatalf("failed to get machine: %v", err)
	}
	defer serverMachine.CleanUp()

	// Redis runs on port 6379 by default.
	port := 6379
	ctx := context.Background()

	for _, operation := range operations {
		b.Run(operation, func(b *testing.B) {
			server := serverMachine.GetContainer(ctx, b)
			defer server.CleanUp(ctx)

			// The redis docker container takes no arguments to run a redis server.
			if err := server.Spawn(ctx, dockerutil.RunOpts{
				Image: "benchmarks/redis",
				Ports: []int{port},
			}); err != nil {
				b.Fatalf("failed to start redis server with: %v", err)
			}

			ip, err := serverMachine.IPAddress()
			if err != nil {
				b.Fatal("failed to get IP from server: %v", err)
			}

			serverPort, err := server.FindPort(ctx, port)
			if err != nil {
				b.Fatal("failed to get IP from server: %v", err)
			}

			// Client should run on native.
			client := clientMachine.GetNativeContainer(ctx, b)
			if err := client.Spawn(ctx, dockerutil.RunOpts{
				Image: "benchmarks/redis",
			}, "sleep", "1000"); err != nil {
				b.Fatalf("running redis-benchmark failed with %v", err)
			}
			defer client.CleanUp(ctx)

			// Wait for server to be up. Redis server responds "PONG" to a ping.
			cliCmd := strings.Split(fmt.Sprintf("redis-cli -h %s -p %d -r 1", ip, serverPort), " ")
			for out, err := client.Exec(ctx, dockerutil.ExecOpts{}, cliCmd...); strings.Contains(out, "PONG"); {
				b.Logf("out: %s, err: %v", out, err)
			}
			// runs redis benchmark -t operation for 100K requests against server.
			cmd := strings.Split(
				fmt.Sprintf("redis-benchmark --csv -t %s -h %s -p %d", operation, ip, serverPort), " ")

			// There is no -t PING_BULK for redis-benchmark, so adjust the command in that case.
			// Note that "ping" will run both PING_INLINE and PING_BULK.
			if operation == "PING_BULK" {
				cmd = strings.Split(
					fmt.Sprintf("redis-benchmark --csv -t %s -h %s -p %d", "ping", ip, serverPort), " ")
			}
			// Reset profiles and timer to begin the measurement.
			server.RestartProfiles()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := client.Exec(ctx, dockerutil.ExecOpts{}, cmd...)
				if err != nil {
					b.Fatalf("redis-benchmark failed with: %v", err)
				}

				// Stop time while we parse results.
				b.StopTimer()
				result, err := parseOperation(operation, out)
				if err != nil {
					b.Fatalf("parsing result %s failed with err: %v", out, err)
				}
				b.ReportMetric(result, operation)
				b.StartTimer()
			}
		})
	}
}

func parseOperation(operation, data string) (float64, error) {
	re := regexp.MustCompile(fmt.Sprintf(`"%s( .*)?","(\d*\.\d*)"`, operation))
	match := re.FindStringSubmatch(data)
	// If no match, simply don't add it to the result map.
	if len(match) < 3 {
		return 0.0, fmt.Errorf("could not find %s in %s", operation, data)
	}
	return strconv.ParseFloat(match[2], 64)
}

const sampleData = `
"PING_INLINE","48661.80"
"PING_BULK","50301.81"
"SET","48923.68"
"GET","49382.71"
"INCR","49975.02"
"LPUSH","49875.31"
"RPUSH","50276.52"
"LPOP","50327.12"
"RPOP","50556.12"
"SADD","49504.95"
"HSET","49504.95"
"SPOP","50025.02"
"LPUSH (needed to benchmark LRANGE)","48875.86"
"LRANGE_100 (first 100 elements)","33955.86"
"LRANGE_300 (first 300 elements)","16550.81"
"LRANGE_500 (first 450 elements)","13653.74"
"LRANGE_600 (first 600 elements)","11219.57"
"MSET (10 keys)","44682.75"
`

func TestParser(t *testing.T) {
	wants := map[string]float64{
		"PING_INLINE": 48661.80,
		"PING_BULK":   50301.81,
		"SET":         48923.68,
		"GET":         49382.71,
		"INCR":        49975.02,
		"LPUSH":       49875.31,
		"RPUSH":       50276.52,
		"LPOP":        50327.12,
		"RPOP":        50556.12,
		"SADD":        49504.95,
		"HSET":        49504.95,
		"SPOP":        50025.02,
		"LRANGE_100":  33955.86,
		"LRANGE_300":  16550.81,
		"LRANGE_500":  13653.74,
		"LRANGE_600":  11219.57,
		"MSET":        44682.75,
	}
	for op, want := range wants {
		if got, err := parseOperation(op, sampleData); err != nil {
			t.Fatalf("failed to parse %s: %v", op, err)
		} else if want != got {
			t.Fatalf("wanted %f for op %s, got %f", want, op, got)
		}
	}
}
