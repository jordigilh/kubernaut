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
	"regexp"
	"strings"

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
	GroupBy []string `yaml:"groupBy,omitempty" json:"groupBy,omitempty"`

	// Match is the exact match criteria for routing attributes
	Match map[string]string `yaml:"match,omitempty" json:"match,omitempty"`

	// MatchRE is the regex match criteria for routing attributes.
	// Issue #416: Patterns are compiled at ParseConfig time; invalid patterns are rejected.
	MatchRE map[string]string `yaml:"matchRe,omitempty" json:"matchRe,omitempty"`

	// compiledRE holds compiled regexp patterns from MatchRE, keyed by attribute name.
	compiledRE map[string]*regexp.Regexp

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
	SlackConfigs []SlackConfig `yaml:"slackConfigs,omitempty" json:"slackConfigs,omitempty"`

	// PagerDutyConfigs is the list of PagerDuty configurations
	PagerDutyConfigs []PagerDutyConfig `yaml:"pagerdutyConfigs,omitempty" json:"pagerdutyConfigs,omitempty"`

	// TeamsConfigs is the list of Microsoft Teams configurations (#593)
	TeamsConfigs []TeamsConfig `yaml:"teamsConfigs,omitempty" json:"teamsConfigs,omitempty"`

	// EmailConfigs is the list of email configurations
	EmailConfigs []EmailConfig `yaml:"emailConfigs,omitempty" json:"emailConfigs,omitempty"`

	// WebhookConfigs is the list of webhook configurations
	WebhookConfigs []WebhookConfig `yaml:"webhookConfigs,omitempty" json:"webhookConfigs,omitempty"`

	// ConsoleConfigs is the list of console (stdout) configurations
	ConsoleConfigs []ConsoleConfig `yaml:"consoleConfigs,omitempty" json:"consoleConfigs,omitempty"`

	// FileConfigs is the list of file delivery configurations (#261)
	FileConfigs []FileConfig `yaml:"fileConfigs,omitempty" json:"fileConfigs,omitempty"`

	// LogConfigs is the list of structured log delivery configurations (#261)
	LogConfigs []LogConfig `yaml:"logConfigs,omitempty" json:"logConfigs,omitempty"`
}

// SlackConfig represents Slack webhook configuration.
// BR-NOT-104-004: credentialRef is the sole mechanism for specifying webhook URLs.
type SlackConfig struct {
	// Channel is the Slack channel (e.g., "#alerts")
	Channel string `yaml:"channel" json:"channel"`

	// CredentialRef is the name of the credential file in the projected volume
	// that contains the Slack webhook URL. Required for all Slack receivers.
	// DD-NOT-104: Replaces APIURL; no fallback mechanism.
	CredentialRef string `yaml:"credentialRef" json:"credentialRef"`

	// Username is the bot username
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// IconEmoji is the bot icon emoji
	IconEmoji string `yaml:"iconEmoji,omitempty" json:"iconEmoji,omitempty"`
}

// PagerDutyConfig represents PagerDuty configuration.
// BR-NOT-104: CredentialRef is the sole mechanism for specifying the routing key.
// The referenced file in the projected volume must contain the PagerDuty Events API v2 routing key.
type PagerDutyConfig struct {
	// CredentialRef is the name of the credential file in the projected volume
	// that contains the PagerDuty routing key (Events API v2). Required.
	CredentialRef string `yaml:"credentialRef" json:"credentialRef"`

	// Severity is the PagerDuty severity (critical, error, warning, info)
	Severity string `yaml:"severity,omitempty" json:"severity,omitempty"`
}

// TeamsConfig represents Microsoft Teams configuration.
// BR-NOT-104: CredentialRef is the sole mechanism for specifying the webhook URL.
// The referenced file in the projected volume must contain the Power Automate Workflows webhook URL.
type TeamsConfig struct {
	// CredentialRef is the name of the credential file in the projected volume
	// that contains the Teams incoming webhook URL. Required.
	CredentialRef string `yaml:"credentialRef" json:"credentialRef"`
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
	RequireTLS bool `yaml:"requireTls,omitempty" json:"requireTls,omitempty"`
}

// WebhookConfig represents generic webhook configuration.
type WebhookConfig struct {
	// URL is the webhook endpoint URL
	URL string `yaml:"url" json:"url"`

	// HTTPConfig contains HTTP client configuration
	HTTPConfig *HTTPConfig `yaml:"httpConfig,omitempty" json:"httpConfig,omitempty"`
}

// HTTPConfig represents HTTP client configuration.
type HTTPConfig struct {
	// BearerToken is the bearer token for authentication
	BearerToken string `yaml:"bearerToken,omitempty" json:"bearerToken,omitempty"`

	// BasicAuth contains basic authentication credentials
	BasicAuth *BasicAuth `yaml:"basicAuth,omitempty" json:"basicAuth,omitempty"`
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

// FileConfig represents file-based delivery configuration (#261).
type FileConfig struct {
	// Path is the output file path (optional; delivery service determines default)
	Path string `yaml:"path,omitempty" json:"path,omitempty"`

	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// LogConfig represents structured-log delivery configuration (#261).
type LogConfig struct {
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

	// Issue #416: Compile matchRe patterns at parse time
	if err := config.compileRouteRegexes(config.Route); err != nil {
		return nil, err
	}

	// Build receiver lookup map
	config.buildReceiverMap()

	return &config, nil
}

// compileRouteRegexes recursively compiles MatchRE patterns on all routes.
// Issue #416: Invalid regex patterns are rejected at parse time.
func (c *Config) compileRouteRegexes(route *Route) error {
	if route == nil {
		return nil
	}
	if len(route.MatchRE) > 0 {
		route.compiledRE = make(map[string]*regexp.Regexp, len(route.MatchRE))
		for key, pattern := range route.MatchRE {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("invalid matchRe pattern for key %q: %q: %w", key, pattern, err)
			}
			route.compiledRE[key] = re
		}
	}
	for _, child := range route.Routes {
		if err := c.compileRouteRegexes(child); err != nil {
			return err
		}
	}
	return nil
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

// ValidateCredentialRefs validates that all credential-bound configs have a non-empty credentialRef.
// BR-NOT-104-004: Called during rebuildDeliveryServices, not during general Validate().
// This separation of concerns keeps routing validation focused on structure,
// while credential validation happens where credentials are actually resolved.
// Covers: SlackConfigs (#244), PagerDutyConfigs (#60), TeamsConfigs (#593).
func (c *Config) ValidateCredentialRefs() error {
	var missing []string
	for _, r := range c.Receivers {
		for i, sc := range r.SlackConfigs {
			if sc.CredentialRef == "" {
				missing = append(missing, fmt.Sprintf("receiver %q slackConfigs[%d]", r.Name, i))
			}
		}
		for i, pc := range r.PagerDutyConfigs {
			if pc.CredentialRef == "" {
				missing = append(missing, fmt.Sprintf("receiver %q pagerdutyConfigs[%d]", r.Name, i))
			}
		}
		for i, tc := range r.TeamsConfigs {
			if tc.CredentialRef == "" {
				missing = append(missing, fmt.Sprintf("receiver %q teamsConfigs[%d]", r.Name, i))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("credentialRef is required: %s", strings.Join(missing, "; "))
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

// FindReceivers finds all matching receivers for the given routing attributes,
// respecting the Continue flag for multi-receiver fanout.
// BR-NOT-068: Multi-Channel Fanout support
//
// Routing follows Alertmanager semantics:
//   - Routes are evaluated depth-first in declaration order.
//   - When Continue is false (default), evaluation stops at the first match.
//   - When Continue is true, evaluation continues to sibling routes.
//   - Children take over from their parent: if a child route has sub-routes
//     and those sub-routes match, the sub-route receivers are collected
//     instead of the child's own Receiver field.
//   - If no children match, the parent's Receiver is used as fallback.
//
// Deduplication by receiver name is performed here (canonical location).
func (r *Route) FindReceivers(attrs map[string]string) []string {
	if attrs == nil {
		attrs = make(map[string]string)
	}
	var matched []string
	seen := make(map[string]bool)
	for _, childRoute := range r.Routes {
		if childRoute.matchesAttributes(attrs) {
			if len(childRoute.Routes) > 0 {
				sub := childRoute.FindReceivers(attrs)
				if len(sub) > 0 {
					for _, s := range sub {
						if !seen[s] {
							matched = append(matched, s)
							seen[s] = true
						}
					}
					if !childRoute.Continue {
						return matched
					}
					continue
				}
			}
			if childRoute.Receiver != "" && !seen[childRoute.Receiver] {
				matched = append(matched, childRoute.Receiver)
				seen[childRoute.Receiver] = true
			}
			if !childRoute.Continue {
				return matched
			}
		}
	}
	if len(matched) == 0 && r.Receiver != "" {
		return []string{r.Receiver}
	}
	return matched
}

// FindReceiver finds the first matching receiver for the given routing attributes.
// BR-NOT-065: First matching route wins (ordered evaluation)
//
// This is a backward-compatible wrapper that delegates to FindReceivers and
// returns only the first matched receiver. Callers that need multi-receiver
// fanout should use FindReceivers directly.
func (r *Route) FindReceiver(attrs map[string]string) string {
	receivers := r.FindReceivers(attrs)
	if len(receivers) > 0 {
		return receivers[0]
	}
	return r.Receiver
}

// matchesAttributes checks if the route's match and matchRe criteria both match.
// Issue #416: match + matchRe have AND semantics — both must satisfy.
func (r *Route) matchesAttributes(attrs map[string]string) bool {
	for key, value := range r.Match {
		if attrs[key] != value {
			return false
		}
	}

	for key, re := range r.compiledRE {
		val, exists := attrs[key]
		if !exists || !re.MatchString(val) {
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
	if len(r.TeamsConfigs) > 0 {
		channels = append(channels, "teams")
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
	if len(r.FileConfigs) > 0 {
		channels = append(channels, "file")
	}
	if len(r.LogConfigs) > 0 {
		channels = append(channels, "log")
	}

	return channels
}

// QualifiedChannels returns channel names qualified with the receiver name for
// channels that support per-receiver credentials (e.g., "slack:sre-critical", "pagerduty:oncall").
// Non-credential channels (console, email, webhook) use unqualified names.
// BR-NOT-104-004: Per-receiver delivery binding via receiver-qualified orchestrator keys.
func (r *Receiver) QualifiedChannels() []string {
	var channels []string

	for i, sc := range r.SlackConfigs {
		if sc.CredentialRef != "" {
			if len(r.SlackConfigs) > 1 {
				channels = append(channels, fmt.Sprintf("slack:%s:%d", r.Name, i))
			} else {
				channels = append(channels, fmt.Sprintf("slack:%s", r.Name))
			}
		} else {
			channels = append(channels, "slack")
		}
	}
	for i, pc := range r.PagerDutyConfigs {
		if pc.CredentialRef != "" {
			if len(r.PagerDutyConfigs) > 1 {
				channels = append(channels, fmt.Sprintf("pagerduty:%s:%d", r.Name, i))
			} else {
				channels = append(channels, fmt.Sprintf("pagerduty:%s", r.Name))
			}
		} else {
			channels = append(channels, "pagerduty")
		}
	}
	for i, tc := range r.TeamsConfigs {
		if tc.CredentialRef != "" {
			if len(r.TeamsConfigs) > 1 {
				channels = append(channels, fmt.Sprintf("teams:%s:%d", r.Name, i))
			} else {
				channels = append(channels, fmt.Sprintf("teams:%s", r.Name))
			}
		} else {
			channels = append(channels, "teams")
		}
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
	if len(r.FileConfigs) > 0 {
		channels = append(channels, "file")
	}
	if len(r.LogConfigs) > 0 {
		channels = append(channels, "log")
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
