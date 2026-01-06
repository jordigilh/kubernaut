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

package testutil

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/codenotary/immudb/embedded/logger"
	immuschema "github.com/codenotary/immudb/pkg/api/schema"
	"github.com/codenotary/immudb/pkg/auth"
	"github.com/codenotary/immudb/pkg/client"
	"github.com/codenotary/immudb/pkg/client/state"
	"github.com/codenotary/immudb/pkg/client/tokenservice"
	"github.com/codenotary/immudb/pkg/stream"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

// ========================================
// NO-OP STUBS FOR client.ImmuClient INTERFACE
// ========================================
//
// These methods are required to satisfy client.ImmuClient but are never
// called in audit repository unit tests.
//
// All methods panic with descriptive message if accidentally called.
//
// ========================================

// User Management Stubs
func (m *MockImmudbClient) ChangePermission(ctx context.Context, action immuschema.PermissionAction, username string, database string, permissions uint32) error {
	panic("ChangePermission should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) ListUsers(ctx context.Context) (*immuschema.UserList, error) {
	panic("ListUsers should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) SetActiveUser(ctx context.Context, u *immuschema.SetActiveUserRequest) error {
	panic("SetActiveUser should not be called in audit storage unit tests")
}

// Auth Config Stubs
func (m *MockImmudbClient) UpdateAuthConfig(ctx context.Context, kind auth.Kind) error {
	panic("UpdateAuthConfig should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) UpdateMTLSConfig(ctx context.Context, enabled bool) error {
	panic("UpdateMTLSConfig should not be called in audit storage unit tests")
}

// Connection/Session Stubs
func (m *MockImmudbClient) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	panic("Connect should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) Logout(ctx context.Context) error {
	panic("Logout should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) OpenSession(ctx context.Context, user []byte, pass []byte, database string) error {
	panic("OpenSession should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WaitForHealthCheck(ctx context.Context) error {
	panic("WaitForHealthCheck should not be called in audit storage unit tests")
}

// Client Configuration Stubs
func (m *MockImmudbClient) WithOptions(options *client.Options) *client.ImmuClient {
	panic("WithOptions should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithLogger(logger logger.Logger) *client.ImmuClient {
	panic("WithLogger should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithStateService(rs state.StateService) *client.ImmuClient {
	panic("WithStateService should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithClientConn(clientConn *grpc.ClientConn) *client.ImmuClient {
	panic("WithClientConn should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithServiceClient(serviceClient immuschema.ImmuServiceClient) *client.ImmuClient {
	panic("WithServiceClient should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithTokenService(tokenService tokenservice.TokenService) *client.ImmuClient {
	panic("WithTokenService should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithServerSigningPubKey(serverSigningPubKey *ecdsa.PublicKey) *client.ImmuClient {
	panic("WithServerSigningPubKey should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) WithStreamServiceFactory(ssf stream.ServiceFactory) *client.ImmuClient {
	panic("WithStreamServiceFactory should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) GetServiceClient() immuschema.ImmuServiceClient {
	panic("GetServiceClient should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) GetOptions() *client.Options {
	panic("GetOptions should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) SetupDialOptions(options *client.Options) []grpc.DialOption {
	panic("SetupDialOptions should not be called in audit storage unit tests")
}

// Database Management Stubs
func (m *MockImmudbClient) DatabaseList(ctx context.Context) (*immuschema.DatabaseListResponse, error) {
	panic("DatabaseList should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) DatabaseListV2(ctx context.Context) (*immuschema.DatabaseListResponseV2, error) {
	panic("DatabaseListV2 should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) CreateDatabase(ctx context.Context, d *immuschema.DatabaseSettings) error {
	panic("CreateDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) CreateDatabaseV2(ctx context.Context, database string, settings *immuschema.DatabaseNullableSettings) (*immuschema.CreateDatabaseResponse, error) {
	panic("CreateDatabaseV2 should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) LoadDatabase(ctx context.Context, r *immuschema.LoadDatabaseRequest) (*immuschema.LoadDatabaseResponse, error) {
	panic("LoadDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) UnloadDatabase(ctx context.Context, r *immuschema.UnloadDatabaseRequest) (*immuschema.UnloadDatabaseResponse, error) {
	panic("UnloadDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) DeleteDatabase(ctx context.Context, r *immuschema.DeleteDatabaseRequest) (*immuschema.DeleteDatabaseResponse, error) {
	panic("DeleteDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) UseDatabase(ctx context.Context, d *immuschema.Database) (*immuschema.UseDatabaseReply, error) {
	panic("UseDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) UpdateDatabase(ctx context.Context, settings *immuschema.DatabaseSettings) error {
	panic("UpdateDatabase should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) UpdateDatabaseV2(ctx context.Context, database string, settings *immuschema.DatabaseNullableSettings) (*immuschema.UpdateDatabaseResponse, error) {
	panic("UpdateDatabaseV2 should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) GetDatabaseSettings(ctx context.Context) (*immuschema.DatabaseSettings, error) {
	panic("GetDatabaseSettings should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) GetDatabaseSettingsV2(ctx context.Context) (*immuschema.DatabaseSettingsResponse, error) {
	panic("GetDatabaseSettingsV2 should not be called in audit storage unit tests")
}

// Database Operations Stubs
func (m *MockImmudbClient) FlushIndex(ctx context.Context, cleanupPercentage float32, synced bool) (*immuschema.FlushIndexResponse, error) {
	panic("FlushIndex should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) CompactIndex(ctx context.Context, req *empty.Empty) error {
	panic("CompactIndex should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) ServerInfo(ctx context.Context, req *immuschema.ServerInfoRequest) (*immuschema.ServerInfoResponse, error) {
	panic("ServerInfo should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) Health(ctx context.Context) (*immuschema.DatabaseHealthResponse, error) {
	panic("Health should not be called in audit storage unit tests")
}

// Key-Value Operations Stubs (partial - we implement some)
func (m *MockImmudbClient) Set(ctx context.Context, key []byte, value []byte) (*immuschema.TxHeader, error) {
	panic("Set should not be called in audit storage unit tests (use VerifiedSet)")
}

func (m *MockImmudbClient) ExpirableSet(ctx context.Context, key []byte, value []byte, expiresAt time.Time) (*immuschema.TxHeader, error) {
	panic("ExpirableSet should not be called in audit storage unit tests")
}

func (m *MockImmudbClient) Get(ctx context.Context, key []byte, opts ...client.GetOption) (*immuschema.Entry, error) {
	panic("Get should not be called in audit storage unit tests (use VerifiedGet)")
}

func (m *MockImmudbClient) GetSince(ctx context.Context, key []byte, tx uint64) (*immuschema.Entry, error) {
	panic("GetSince should not be called in audit storage unit tests")
}

// More stub methods will be added as needed when compilation fails
// This file can be extended incrementally

