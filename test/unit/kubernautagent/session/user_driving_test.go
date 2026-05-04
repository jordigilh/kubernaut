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

package session_test

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("StatusUserDriving — #774, BR-INTERACTIVE-001, BR-INTERACTIVE-004", func() {

	Describe("UT-KA-774-001: TransitionToUserDriving sets correct status and metadata", func() {
		It("should set StatusUserDriving and write identity to session metadata", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-774"})
			Expect(err).NotTo(HaveOccurred())

			err = mgr.TransitionToUserDriving(id, "alice@example.com", []string{"sre", "oncall"})
			Expect(err).NotTo(HaveOccurred())

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusUserDriving))
			Expect(sess.Metadata["acting_user"]).To(Equal("alice@example.com"))
			Expect(sess.Metadata).To(HaveKey("acting_user_groups"))
		})
	})

	Describe("UT-KA-774-002: StatusUserDriving is NOT terminal", func() {
		It("should return false from IsTerminal", func() {
			Expect(session.IsTerminal(session.StatusUserDriving)).To(BeFalse(),
				"StatusUserDriving must be non-terminal so the session remains pollable")
		})

		It("should not be garbage-collected by Cleanup", func() {
			store := session.NewStore(1 * time.Millisecond)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			err = mgr.TransitionToUserDriving(id, "bob@example.com", nil)
			Expect(err).NotTo(HaveOccurred())

			// Wait past the TTL; session should survive because StatusUserDriving is non-terminal
			Eventually(func() session.Status {
				sess, sErr := mgr.GetSession(id)
				if sErr != nil {
					return ""
				}
				return sess.Status
			}, 50*time.Millisecond, 5*time.Millisecond).Should(Equal(session.StatusUserDriving))
		})
	})

	Describe("UT-KA-774-003: Groups serialization round-trip", func() {
		It("should preserve groups as JSON in metadata and deserialize correctly", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			groups := []string{"engineering", "sre", "oncall"}
			err = mgr.TransitionToUserDriving(id, "carol@example.com", groups)
			Expect(err).NotTo(HaveOccurred())

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Metadata["acting_user_groups"]).To(ContainSubstring("engineering"))
			Expect(sess.Metadata["acting_user_groups"]).To(ContainSubstring("sre"))
			Expect(sess.Metadata["acting_user_groups"]).To(ContainSubstring("oncall"))
		})
	})

	Describe("UT-KA-774-004: Concurrent access safety", func() {
		It("should not panic or race under concurrent TransitionToUserDriving + GetSession", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				_ = mgr.TransitionToUserDriving(id, "concurrent-user", []string{"g1"})
			}()

			go func() {
				defer wg.Done()
				_, _ = mgr.GetSession(id)
			}()

			wg.Wait()
		})
	})

	Describe("UT-KA-774-005: TransitionToUserDriving cancels the investigation goroutine", func() {
		It("should cancel the autonomous investigation context", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			ctxCancelled := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(ctxCancelled)
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			err = mgr.TransitionToUserDriving(id, "dan@example.com", nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(ctxCancelled).Should(BeClosed(),
				"TransitionToUserDriving must cancel the investigation goroutine")
		})
	})
})
