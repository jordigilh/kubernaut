package acp

import "time"

// AgentName follows RFC 1123 DNS label naming: ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
type AgentName = string

type AgentManifest struct {
	Name              AgentName  `json:"name"`
	Description       string     `json:"description"`
	InputContentTypes []string   `json:"input_content_types"`
	OutputContentTypes []string  `json:"output_content_types"`
	Metadata          *Metadata  `json:"metadata,omitempty"`
	Status            *AgentStatus `json:"status,omitempty"`
}

type Metadata struct {
	Annotations       map[string]interface{} `json:"annotations,omitempty"`
	Documentation     string                 `json:"documentation,omitempty"`
	License           string                 `json:"license,omitempty"`
	ProgrammingLang   string                 `json:"programming_language,omitempty"`
	NaturalLanguages  []string               `json:"natural_languages,omitempty"`
	Framework         string                 `json:"framework,omitempty"`
	Capabilities      []Capability           `json:"capabilities,omitempty"`
	Domains           []string               `json:"domains,omitempty"`
	Tags              []string               `json:"tags,omitempty"`
	CreatedAt         *time.Time             `json:"created_at,omitempty"`
	UpdatedAt         *time.Time             `json:"updated_at,omitempty"`
	RecommendedModels []string               `json:"recommended_models,omitempty"`
}

type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AgentStatus struct {
	AvgRunTokens      *float64 `json:"avg_run_tokens,omitempty"`
	AvgRunTimeSeconds *float64 `json:"avg_run_time_seconds,omitempty"`
	SuccessRate       *float64 `json:"success_rate,omitempty"`
}

type RunMode string

const (
	RunModeSync   RunMode = "sync"
	RunModeAsync  RunMode = "async"
	RunModeStream RunMode = "stream"
)

type RunStatus string

const (
	RunStatusCreated    RunStatus = "created"
	RunStatusInProgress RunStatus = "in-progress"
	RunStatusAwaiting   RunStatus = "awaiting"
	RunStatusCancelling RunStatus = "cancelling"
	RunStatusCancelled  RunStatus = "cancelled"
	RunStatusCompleted  RunStatus = "completed"
	RunStatusFailed     RunStatus = "failed"
)

type RunCreateRequest struct {
	AgentName AgentName `json:"agent_name"`
	SessionID string    `json:"session_id,omitempty"`
	Input     []Message `json:"input"`
	Mode      RunMode   `json:"mode,omitempty"`
}

type RunResumeRequest struct {
	RunID       string    `json:"run_id"`
	AwaitResume bool      `json:"await_resume,omitempty"`
	Input       []Message `json:"input,omitempty"`
	Mode        RunMode   `json:"mode,omitempty"`
}

type Run struct {
	AgentName    AgentName  `json:"agent_name"`
	SessionID    string     `json:"session_id,omitempty"`
	RunID        string     `json:"run_id"`
	Status       RunStatus  `json:"status"`
	AwaitRequest *AwaitRequest `json:"await_request,omitempty"`
	Output       []Message  `json:"output"`
	Error        *Error     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type AwaitRequest struct {
	Description string                 `json:"description,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

type Message struct {
	Role        string        `json:"role"`
	Parts       []MessagePart `json:"parts"`
	CreatedAt   *time.Time    `json:"created_at,omitempty"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
}

type MessagePart struct {
	Name            string      `json:"name,omitempty"`
	ContentType     string      `json:"content_type"`
	Content         string      `json:"content,omitempty"`
	ContentEncoding string      `json:"content_encoding,omitempty"`
	ContentURL      string      `json:"content_url,omitempty"`
	Metadata        interface{} `json:"metadata,omitempty"`
}

type TrajectoryMetadata struct {
	Kind       string                 `json:"kind"` // "trajectory"
	Message    string                 `json:"message,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	ToolOutput map[string]interface{} `json:"tool_output,omitempty"`
}

type Error struct {
	Code    string      `json:"code"` // server_error, invalid_input, not_found
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SSE Event types per ACP spec
type EventType string

const (
	EventMessageCreated EventType = "message.created"
	EventMessagePart    EventType = "message.part"
	EventMessageDone    EventType = "message.completed"
	EventRunCreated     EventType = "run.created"
	EventRunInProgress  EventType = "run.in-progress"
	EventRunAwaiting    EventType = "run.awaiting"
	EventRunCompleted   EventType = "run.completed"
	EventRunFailed      EventType = "run.failed"
	EventRunCancelled   EventType = "run.cancelled"
	EventError          EventType = "error"
	EventGeneric        EventType = "generic"
)

type Event struct {
	Type    EventType   `json:"type"`
	Run     *Run        `json:"run,omitempty"`
	Message *Message    `json:"message,omitempty"`
	Part    *MessagePart `json:"part,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	Generic interface{} `json:"generic,omitempty"`
}
