/*
Copyright 2026 Jordi Gil.

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

package notification

import "sync"

// notificationReconcileLockManager serializes reconciliation by resource key.
//
// A resource key is used instead of a UID because the request key is available
// before the NotificationRequest is fetched. Reference counting allows idle
// locks to be removed without allowing a new caller to create a second lock
// while an older waiter still holds the original one.
type notificationReconcileLockManager struct {
	mu    sync.Mutex
	locks map[string]*notificationReconcileLock
}

type notificationReconcileLock struct {
	mu   sync.Mutex
	refs int
}

// lock acquires the lock for key and returns its release function.
func (m *notificationReconcileLockManager) lock(key string) func() {
	m.mu.Lock()
	if m.locks == nil {
		m.locks = make(map[string]*notificationReconcileLock)
	}

	resourceLock, exists := m.locks[key]
	if !exists {
		resourceLock = &notificationReconcileLock{}
		m.locks[key] = resourceLock
	}
	resourceLock.refs++
	m.mu.Unlock()

	resourceLock.mu.Lock()
	return func() {
		resourceLock.mu.Unlock()

		m.mu.Lock()
		resourceLock.refs--
		if resourceLock.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}
