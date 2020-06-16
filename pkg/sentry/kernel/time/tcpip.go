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

package time

import (
	"sync"
	"time"
)

// TcpipAfterFunc waits for duration to elapse according to clock then runs fn.
// The timer is started immediately and will fire exactly once.
func TcpipAfterFunc(clock Clock, duration time.Duration, fn func()) *TcpipTimer {
	timer := &TcpipTimer{
		clock: clock,
	}
	timer.notifier = functionNotifier{
		fn: func() {
			go func() {
				timer.Stop()
				fn()
			}()
		},
	}
	timer.Reset(duration)
	return timer
}

// TcpipTimer is a resettable timer with variable duration expirations.
// Implements tcpip.Timer, which does not define a Destroy method; instead, all
// resources are released after timer expiration and calls to Timer.Stop.
//
// Must be created by AfterFunc.
type TcpipTimer struct {
	// clock is the time source. clock is immutable.
	clock Clock

	// notifier is called when the Timer expires. notifier is immutable.
	notifier functionNotifier

	// mu protects t.
	mu sync.Mutex

	// t stores the latest running Timer. This is replaced whenever Reset is
	// called since Timer cannot be restarted once it has been Destroyed by Stop.
	//
	// This field is nil iff Stop has been called.
	t *Timer
}

// Stop implements tcpip.Timer.Stop.
func (r *TcpipTimer) Stop() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.t == nil {
		return false
	}
	_, lastSetting := r.t.Swap(Setting{})
	r.t.Destroy()
	r.t = nil
	return lastSetting.Enabled
}

// Reset implements tcpip.Timer.Reset.
func (r *TcpipTimer) Reset(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.t == nil {
		r.t = NewTimer(r.clock, &r.notifier)
	}

	r.t.Swap(Setting{
		Enabled: true,
		Period:  0,
		Next:    r.clock.Now().Add(d),
	})
}

// functionNotifier is a TimerListener that runs a function.
//
// functionNotifier cannot be saved or loaded.
type functionNotifier struct {
	fn func()
}

// Notify implements ktime.TimerListener.Notify.
func (f *functionNotifier) Notify(uint64, Setting) (Setting, bool) {
	f.fn()
	return Setting{}, false
}

// Destroy implements ktime.TimerListener.Destroy.
func (f *functionNotifier) Destroy() {}
