package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ActionRegistry", func() {
	var registry *ActionRegistry

	BeforeEach(func() {
		registry = NewActionRegistry()
	})

	Describe("NewActionRegistry", func() {
		It("should create a new registry with zero count", func() {
			Expect(registry).ToNot(BeNil())
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("Register", func() {
		var handler ActionHandler

		BeforeEach(func() {
			handler = func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}
		})

		It("should register action successfully", func() {
			err := registry.Register("test_action", handler)
			Expect(err).ToNot(HaveOccurred())
			Expect(registry.Count()).To(Equal(1))
			Expect(registry.IsRegistered("test_action")).To(BeTrue())
		})

		It("should return error for duplicate registration", func() {
			err := registry.Register("test_action", handler)
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register("test_action", handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already registered"))
		})
	})

	Describe("Unregister", func() {
		var handler ActionHandler

		BeforeEach(func() {
			handler = func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}
		})

		It("should unregister action successfully", func() {
			_ = registry.Register("test_action", handler)
			Expect(registry.Count()).To(Equal(1))

			registry.Unregister("test_action")
			Expect(registry.Count()).To(Equal(0))
			Expect(registry.IsRegistered("test_action")).To(BeFalse())
		})

		It("should not panic when unregistering non-existent action", func() {
			Expect(func() {
				registry.Unregister("non_existent")
			}).ToNot(Panic())
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("Execute", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("should execute registered action successfully", func() {
			executed := false
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				executed = true
				return nil
			}

			_ = registry.Register("test_action", handler)

			action := &types.ActionRecommendation{
				Action: "test_action",
			}
			alert := types.Alert{
				Name: "test_alert",
			}

			err := registry.Execute(ctx, action, alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(executed).To(BeTrue())
		})

		It("should return error for unknown action", func() {
			action := &types.ActionRecommendation{
				Action: "unknown_action",
			}
			alert := types.Alert{
				Name: "test_alert",
			}

			err := registry.Execute(ctx, action, alert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown action"))
		})

		It("should propagate handler errors", func() {
			expectedError := errors.New("handler error")
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return expectedError
			}

			err := registry.Register("error_action", handler)
			Expect(err).ToNot(HaveOccurred())

			action := &types.ActionRecommendation{
				Action: "error_action",
			}
			alert := types.Alert{
				Name: "test_alert",
			}

			err = registry.Execute(ctx, action, alert)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(expectedError))
		})
	})

	Describe("GetRegisteredActions", func() {
		It("should return empty slice for empty registry", func() {
			actions := registry.GetRegisteredActions()
			Expect(actions).To(BeEmpty())
		})

		It("should return all registered actions", func() {
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}

			_ = registry.Register("action1", handler)
			_ = registry.Register("action2", handler)
			_ = registry.Register("action3", handler)

			actions := registry.GetRegisteredActions()
			Expect(actions).To(HaveLen(3))
			Expect(actions).To(ContainElement("action1"))
			Expect(actions).To(ContainElement("action2"))
			Expect(actions).To(ContainElement("action3"))
		})
	})

	Describe("IsRegistered", func() {
		It("should correctly report registration status", func() {
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}

			Expect(registry.IsRegistered("test_action")).To(BeFalse())

			_ = registry.Register("test_action", handler)
			Expect(registry.IsRegistered("test_action")).To(BeTrue())

			registry.Unregister("test_action")
			Expect(registry.IsRegistered("test_action")).To(BeFalse())
		})
	})

	Describe("Count", func() {
		It("should correctly count registered actions", func() {
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}

			Expect(registry.Count()).To(Equal(0))

			_ = registry.Register("action1", handler)
			Expect(registry.Count()).To(Equal(1))

			_ = registry.Register("action2", handler)
			Expect(registry.Count()).To(Equal(2))

			registry.Unregister("action1")
			Expect(registry.Count()).To(Equal(1))

			registry.Unregister("action2")
			Expect(registry.Count()).To(Equal(0))
		})
	})

	Describe("Concurrent access", func() {
		It("should handle concurrent operations safely", func() {
			handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil
			}

			done := make(chan bool)

			// Goroutine 1: Register actions
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					_ = registry.Register(fmt.Sprintf("action%d", i), handler)
				}
				done <- true
			}()

			// Goroutine 2: Check registered actions
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					registry.GetRegisteredActions()
					registry.Count()
				}
				done <- true
			}()

			// Wait for both goroutines to complete
			<-done
			<-done

			Expect(registry.Count()).To(Equal(10))
		})
	})
})
