/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

Minimal Server wiring for direct HTTP-handler unit tests invoked from packages
outside server (see test/unit/datastorage). Production startup uses NewServer.
*/

package server

import (
	"database/sql"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// MinimalAuditHandlersHTTPServerDeps configures NewMinimalAuditHandlersHTTPServer.
// Unused pointer fields may be nil when a handler exits before dereferencing them.
type MinimalAuditHandlersHTTPServerDeps struct {
	Logger          logr.Logger
	DB              *sql.DB
	AuditEventsRepo *repository.AuditEventsRepository
	Signer          *cert.Signer
	MaxBodyBytes    int64
}

// NewMinimalAuditHandlersHTTPServer returns a Server suitable for invoking
// HandleVerifyChain / HandleExportAuditEvents in isolation (RFC7807-layer tests).
// BR-STORAGE-033 / SOC2 Gap #9: handler tests without PostgreSQL bootstrap.
func NewMinimalAuditHandlersHTTPServer(d MinimalAuditHandlersHTTPServerDeps) *Server {
	maxBody := d.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 1024 * 1024
	}
	return &Server{
		logger:          d.Logger,
		db:              d.DB,
		auditEventsRepo: d.AuditEventsRepo,
		signer:          d.Signer,
		maxBodySize:     maxBody,
	}
}
