package controller

// BuildTimeoutContextForTest exposes buildTimeoutContext for external test
// packages (#2658 E11c: the notification target string is group-aware).
var BuildTimeoutContextForTest = buildTimeoutContext

// FormatRemediationTargetStringForTest exposes formatRemediationTargetString
// for external test packages (#2658 E2b): the busy/cooldown gate input and the
// distributed lock key share this identity.
var FormatRemediationTargetStringForTest = formatRemediationTargetString
