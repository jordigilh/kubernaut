// Package client provides OpenAPI-generated client for DataStorage Service.
//
// The client is automatically generated from the OpenAPI specification using oapi-codegen.
//
// Generation is triggered by:
//   - make generate-datastorage-client (manual generation)
//   - make test-e2e-datastorage (pre-test validation)
//   - go generate ./pkg/datastorage/client/... (direct go generate)
//
// Authority:
//   - DD-API-001: OpenAPI Client Mandate
//   - Spec Location: api/openapi/data-storage-v1.yaml
//
//go:generate oapi-codegen -package client -generate types,client -o generated.go ../../../api/openapi/data-storage-v1.yaml
package client

