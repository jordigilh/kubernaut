/*
Copyright 2025 Jordi Gil.

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

// Package routing implements BR-NOT-065 (Channel Routing Based on Spec Fields)
// and BR-NOT-066 (Alertmanager-Compatible Configuration Format).
//
// Issue #91: Routing now uses immutable spec fields and metadata instead of
// mutable Kubernetes labels. The Alertmanager-compatible configuration format
// is preserved, enabling SREs to use familiar syntax for channel selection rules.
package routing

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config represents the complete routing configuration.
// BR-NOT-066: Alertmanager-compatible format
type Config struct {
	// Route is the root routing node
	Route *Route `yaml:"route" json:"route"`

	// Receivers is the list of notification receivers
	Receivers []*Receiver `yaml:"receivers" json:"receivers"`

	// receiverMap is an internal lookup map
	receiverMap map[string]*Receiver
}

// Route represents a routing node in the routing tree.
// BR-NOT-065: Attribute-based routing with ordered evaluation
type Route struct {
	// Receiver is the name of the receiver for this route
	Receiver string `yaml:"receiver" json:"receiver"`

	// GroupBy specifies attributes to group notifications by
	GroupBy []string `yaml:"group_by,omitempty" json:"group_by,omitempty"`

	// Match is the exact match criteria for routing attributes
	Match map[string]string `yaml:"match,omitempty" json:"match,omitempty"`

	// MatchRE is the regex match criteria for routing attributes (not implemented in V1.0)
	MatchRE map[string]string `yaml:"match_re,omitempty" json:"match_re,omitempty"`

	// Continue indicates whether to continue to sibling routes after matching
	// BR-NOT-068: Multi-Channel Fanout support
	Continue bool `yaml:"continue,omitempty" json:"continue,omitempty"`

	// Routes is the list of child routes
	Routes []*Route `yaml:"routes,omitempty" json:"routes,omitempty"`
}

// Receiver represents a notification receiver with channel configurations.
type Receiver struct {
	// Name is the unique identifier for this receiver
	Name string `yaml:"name" json:"name"`

	// SlackConfigs is the list of Slack channel configurations
	SlackConfigs []SlackConfig `yaml:"slack_configs,omitempty" json:"slack_configs,omitempty"`

	// PagerDutyConfigs is the list of PagerDuty configurations
	PagerDutyConfigs []PagerDutyConfig `yaml:"pagerduty_configs,omitempty" json:"pagerduty_configs,omitempty"`

	// EmailConfigs is the list of email configurations
	EmailConfigs []EmailConfig `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`

	// WebhookConfigs is the list of webhook configurations
	WebhookConfigs []WebhookConfig `yaml:"webhook_configs,omitempty" json:"webhook_configs,omitempty"`

	// ConsoleConfigs is the list of console (stdout) configurations
	ConsoleConfigs []ConsoleConfig `yaml:"console_configs,omitempty" json:"console_configs,omitempty"`
}

// SlackConfig represents Slack webhook configuration.
type SlackConfig struct {
	// Channel is the Slack channel (e.g., "#alerts")
	Channel string `yaml:"channel" json:"channel"`

	// APIURL is the Slack webhook URL (can be templated)
	APIURL string `yaml:"api_url,omitempty" json:"api_url,omitempty"`

	// Username is the bot username
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// IconEmoji is the bot icon emoji
	IconEmoji string `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
}

// PagerDutyConfig represents PagerDuty configuration.
type PagerDutyConfig struct {
	// ServiceKey is the PagerDuty service integration key
	ServiceKey string `yaml:"service_key" json:"service_key"`

	// RoutingKey is an alternative routing key (v2 API)
	RoutingKey string `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`

	// Severity is the PagerDuty severity (critical, error, warning, info)
	Severity string `yaml:"severity,omitempty" json:"severity,omitempty"`
}

// EmailConfig represents email configuration.
type EmailConfig struct {
	// To is the recipient email address
	To string `yaml:"to" json:"to"`

	// From is the sender email address
	From string `yaml:"from,omitempty" json:"from,omitempty"`

	// SmartHost is the SMTP server address
	SmartHost string `yaml:"smarthost,omitempty" json:"smarthost,omitempty"`

	// RequireTLS specifies whether TLS is required
	RequireTLS bool `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
}

// WebhookConfig represents generic webhook configuration.
type WebhookConfig struct {
	// URL is the webhook endpoint URL
	URL string `yaml:"url" json:"url"`

	// HTTPConfig contains HTTP client configuration
	HTTPConfig *HTTPConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`
}

// HTTPConfig represents HTTP client configuration.
type HTTPConfig struct {
	// BearerToken is the bearer token for authentication
	BearerToken string `yaml:"bearer_token,omitempty" json:"bearer_token,omitempty"`

	// BasicAuth contains basic authentication credentials
	BasicAuth *BasicAuth `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
}

// BasicAuth represents basic authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// ConsoleConfig represents console (stdout) configuration.
// Used as fallback when no other receivers are configured.
type ConsoleConfig struct {
	// Enabled specifies whether console output is enabled
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// ParseConfig parses YAML configuration into a Config struct.
// BR-NOT-066: Alertmanager-compatible format parsing
func ParseConfig(data []byte) (*Config, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty configuration")
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Build receiver lookup map
	config.buildReceiverMap()

	return &config, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Route == nil {
		return fmt.Errorf("route is required")
	}

	if len(c.Receivers) == 0 {
		return fmt.Errorf("at least one receiver is required")
	}

	// Build a set of valid receiver names
	receiverNames := make(map[string]bool)
	for _, r := range c.Receivers {
		if r.Name == "" {
			return fmt.Errorf("receiver name cannot be empty")
		}
		receiverNames[r.Name] = true
	}

	// Validate root route receiver exists
	if c.Route.Receiver != "" && !receiverNames[c.Route.Receiver] {
		return fmt.Errorf("receiver '%s' not found in receivers list", c.Route.Receiver)
	}

	// Validate child route receivers exist
	if err := c.validateRouteReceivers(c.Route, receiverNames); err != nil {
		return err
	}

	return nil
}

// validateRouteReceivers recursively validates that all route receivers exist.
func (c *Config) validateRouteReceivers(route *Route, receiverNames map[string]bool) error {
	for _, childRoute := range route.Routes {
		if childRoute.Receiver != "" && !receiverNames[childRoute.Receiver] {
			return fmt.Errorf("receiver '%s' not found in receivers list", childRoute.Receiver)
		}
		if err := c.validateRouteReceivers(childRoute, receiverNames); err != nil {
			return err
		}
	}
	return nil
}

// buildReceiverMap builds the internal receiver lookup map.
func (c *Config) buildReceiverMap() {
	c.receiverMap = make(map[string]*Receiver)
	for _, r := range c.Receivers {
		c.receiverMap[r.Name] = r
	}
}

// GetReceiver returns a receiver by name, or nil if not found.
func (c *Config) GetReceiver(name string) *Receiver {
	if c.receiverMap == nil {
		c.buildReceiverMap()
	}
	return c.receiverMap[name]
}

// FindReceiver finds the matching receiver for the given routing attributes.
// BR-NOT-065: First matching route wins (ordered evaluation)
func (r *Route) FindReceiver(attrs map[string]string) string {
	if attrs == nil {
		attrs = make(map[string]string)
	}

	// Check child routes first (depth-first)
	for _, childRoute := range r.Routes {
		if childRoute.matchesAttributes(attrs) {
			if len(childRoute.Routes) > 0 {
				result := childRoute.FindReceiver(attrs)
				if result != "" {
					return result
				}
			}
			if childRoute.Receiver != "" {
				return childRoute.Receiver
			}
		}
	}

	return r.Receiver
}

// matchesAttributes checks if the route's match criteria match the given routing attributes.
func (r *Route) matchesAttributes(attrs map[string]string) bool {
	if len(r.Match) == 0 {
		return true
	}

	for key, value := range r.Match {
		if attrs[key] != value {
			return false
		}
	}

	return true
}

// GetChannels returns the list of channel types configured for this receiver.
func (r *Receiver) GetChannels() []string {
	var channels []string

	if len(r.SlackConfigs) > 0 {
		channels = append(channels, "slack")
	}
	if len(r.PagerDutyConfigs) > 0 {
		channels = append(channels, "pagerduty")
	}
	if len(r.EmailConfigs) > 0 {
		channels = append(channels, "email")
	}
	if len(r.WebhookConfigs) > 0 {
		channels = append(channels, "webhook")
	}
	if len(r.ConsoleConfigs) > 0 {
		channels = append(channels, "console")
	}

	return channels
}

// DefaultConfig returns a default configuration with console fallback.
func DefaultConfig() *Config {
	config := &Config{
		Route: &Route{
			Receiver: "console-fallback",
		},
		Receivers: []*Receiver{
			{
				Name: "console-fallback",
				ConsoleConfigs: []ConsoleConfig{
					{Enabled: true},
				},
			},
		},
	}
	config.buildReceiverMap()
	return config
}
