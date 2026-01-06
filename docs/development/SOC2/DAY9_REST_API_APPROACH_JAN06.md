# Day 9: REST API Approach (Revised)

**Date**: January 6, 2026
**Status**: REVISED - CLI-free implementation
**Original Plan**: CLI verification tool
**Revised Plan**: REST API verification endpoint
**Confidence**: 100%

---

## 🚨 **Problem Identified**

**Original Day 9 Plan**: Included `kubernaut audit verify-export <file>` CLI tool

**Issue**:
- ❌ Kubernaut has **NO CLI infrastructure** currently
- ❌ Building CLI from scratch adds 4-6 hours of tooling work
- ❌ Adds maintenance burden (CLI versioning, packaging, distribution)

---

## ✅ **REVISED APPROACH: REST API Verification**

### **Recommendation**: Replace CLI with REST API endpoint in Data Storage service

**Benefits**:
1. ✅ **No new infrastructure needed** - reuse existing REST API patterns
2. ✅ **Consistent with microservices** - all verification via HTTP
3. ✅ **Client-agnostic** - Works with curl, Postman, Web UI, or future CLI
4. ✅ **Easier testing** - Integration tests already use HTTP
5. ✅ **Auditor-friendly** - Can use standard HTTP tools
6. ✅ **Faster implementation** - 2 hours → 1 hour (50% time savings)

---

## 📋 **Revised Day 9 Implementation**

### **Task 9.1: Export API with Digital Signatures** (4 hours) - UNCHANGED

**File**: `pkg/datastorage/server/audit_export_handler.go`

**Endpoint**: `POST /api/v1/audit/export`

**Request**:
```json
{
  "correlation_id": "rr-2025-001",
  "date_range": {
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-12-31T23:59:59Z"
  }
}
```

**Response**:
```json
{
  "export_metadata": {
    "export_id": "exp-2026-01-06-abc123",
    "exported_by": "compliance-officer@example.com",
    "exported_at": "2026-01-06T18:00:00Z",
    "event_count": 42,
    "export_hash": "sha256:a1b2c3d4e5f6...",
    "signature": "7890abcdef...",
    "public_key_url": "https://datastorage.svc/api/v1/audit/public-key"
  },
  "events": [ /* array of audit events */ ]
}
```

---

### **Task 9.2: REST API Verification Endpoint** (1 hour) - REVISED ⚡

**File**: `pkg/datastorage/server/audit_verify_handler.go` (NEW)

**Endpoint**: `POST /api/v1/audit/verify-export`

**Request** (two options):

**Option A: Inline Export Verification**
```json
{
  "export": {
    "export_metadata": { /* full metadata */ },
    "events": [ /* array of events */ ]
  }
}
```

**Option B: Export ID Verification** (for exports stored in DB)
```json
{
  "export_id": "exp-2026-01-06-abc123"
}
```

**Response**:
```json
{
  "verification_result": "valid",
  "verified_at": "2026-01-06T18:30:00Z",
  "details": {
    "export_id": "exp-2026-01-06-abc123",
    "event_count": 42,
    "hash_verified": true,
    "signature_verified": true,
    "public_key_used": "RSA-2048-pubkey-fingerprint-abc123"
  }
}
```

**Error Response** (if tampered):
```json
{
  "verification_result": "invalid",
  "verified_at": "2026-01-06T18:30:00Z",
  "errors": [
    {
      "code": "SIGNATURE_MISMATCH",
      "message": "Export signature verification failed - data has been tampered with"
    },
    {
      "code": "HASH_MISMATCH",
      "message": "Export hash does not match recalculated hash"
    }
  ]
}
```

**Implementation**:
```go
// pkg/datastorage/server/audit_verify_handler.go

type VerifyExportRequest struct {
    Export   *AuditExport `json:"export,omitempty"`   // Option A: Inline export
    ExportID string       `json:"export_id,omitempty"` // Option B: Stored export ID
}

type VerifyExportResponse struct {
    VerificationResult string             `json:"verification_result"` // "valid" | "invalid"
    VerifiedAt         time.Time          `json:"verified_at"`
    Details            *VerificationDetails `json:"details,omitempty"`
    Errors             []VerificationError  `json:"errors,omitempty"`
}

func (s *Server) VerifyExport(ctx context.Context, req *VerifyExportRequest) (*VerifyExportResponse, error) {
    var export *AuditExport

    // Get export (either inline or from DB)
    if req.Export != nil {
        export = req.Export
    } else if req.ExportID != "" {
        var err error
        export, err = s.db.GetExportByID(ctx, req.ExportID)
        if err != nil {
            return nil, fmt.Errorf("export not found: %w", err)
        }
    } else {
        return nil, errors.New("either 'export' or 'export_id' must be provided")
    }

    // Fetch public key
    publicKey, err := s.getPublicKey()
    if err != nil {
        return nil, fmt.Errorf("failed to load public key: %w", err)
    }

    // Recalculate export hash
    calculatedHash := calculateExportHash(export.Events)
    exportHash, _ := hex.DecodeString(export.Metadata.ExportHash)

    // Verify hash matches
    hashValid := bytes.Equal(calculatedHash, exportHash)

    // Verify signature
    signature, _ := hex.DecodeString(export.Metadata.Signature)
    signatureErr := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, calculatedHash, signature)
    signatureValid := (signatureErr == nil)

    // Build response
    response := &VerifyExportResponse{
        VerifiedAt: time.Now(),
        Details: &VerificationDetails{
            ExportID:         export.Metadata.ExportID,
            EventCount:       export.Metadata.EventCount,
            HashVerified:     hashValid,
            SignatureVerified: signatureValid,
        },
    }

    if hashValid && signatureValid {
        response.VerificationResult = "valid"
    } else {
        response.VerificationResult = "invalid"
        if !hashValid {
            response.Errors = append(response.Errors, VerificationError{
                Code:    "HASH_MISMATCH",
                Message: "Export hash does not match recalculated hash",
            })
        }
        if !signatureValid {
            response.Errors = append(response.Errors, VerificationError{
                Code:    "SIGNATURE_MISMATCH",
                Message: "Export signature verification failed",
            })
        }
    }

    return response, nil
}
```

---

### **Task 9.3: Public Key Endpoint** (30 min) - NEW

**File**: `pkg/datastorage/server/audit_keys_handler.go` (NEW)

**Endpoint**: `GET /api/v1/audit/public-key`

**Response**:
```
-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEA3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z
0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3Z5Z0c3
... (2048-bit RSA public key in PEM format)
-----END RSA PUBLIC KEY-----
```

**Implementation**:
```go
func (s *Server) GetPublicKey(ctx context.Context) ([]byte, error) {
    return pem.EncodeToMemory(&pem.Block{
        Type:  "RSA PUBLIC KEY",
        Bytes: x509.MarshalPKCS1PublicKey(s.publicKey),
    }), nil
}
```

---

### **Task 9.4: Chain of Custody Metadata** (2 hours) - UNCHANGED

Same as original plan - log exports to `audit_exports` table.

---

## 🧪 **Testing Strategy**

### **Integration Tests**

**File**: `test/integration/datastorage/audit_export_test.go`

```go
var _ = Describe("REST API Audit Export & Verification", func() {
    var dsClient *dsgen.ClientWithResponses

    Context("Export + Verification Flow", func() {
        It("should export audit logs with valid signature", func() {
            // 1. Create some audit events
            createAuditEvents(ctx, "rr-2025-001", 10)

            // 2. Export audit logs via REST API
            exportResp, err := dsClient.ExportAuditLogsWithResponse(ctx, &dsgen.ExportAuditLogsParams{
                CorrelationID: ptr.To("rr-2025-001"),
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(exportResp.StatusCode()).To(Equal(200))

            export := exportResp.JSON200
            Expect(export.ExportMetadata.EventCount).To(Equal(10))
            Expect(export.ExportMetadata.ExportHash).ToNot(BeEmpty())
            Expect(export.ExportMetadata.Signature).ToNot(BeEmpty())

            // 3. Verify export signature via REST API
            verifyResp, err := dsClient.VerifyExportWithResponse(ctx, dsgen.VerifyExportJSONRequestBody{
                Export: export,
            })
            Expect(err).ToNot(HaveOccurred())
            Expect(verifyResp.StatusCode()).To(Equal(200))

            result := verifyResp.JSON200
            Expect(result.VerificationResult).To(Equal("valid"))
            Expect(result.Details.HashVerified).To(BeTrue())
            Expect(result.Details.SignatureVerified).To(BeTrue())
        })

        It("should detect tampered exports", func() {
            // 1. Export audit logs
            exportResp, err := dsClient.ExportAuditLogsWithResponse(ctx, &dsgen.ExportAuditLogsParams{
                CorrelationID: ptr.To("rr-2025-001"),
            })
            Expect(err).ToNot(HaveOccurred())
            export := exportResp.JSON200

            // 2. Tamper with export (modify event data)
            export.Events[0].EventData["tampered"] = true

            // 3. Verify export - should fail
            verifyResp, err := dsClient.VerifyExportWithResponse(ctx, dsgen.VerifyExportJSONRequestBody{
                Export: export,
            })
            Expect(err).ToNot(HaveOccurred())

            result := verifyResp.JSON200
            Expect(result.VerificationResult).To(Equal("invalid"))
            Expect(result.Errors).To(HaveLen(1))
            Expect(result.Errors[0].Code).To(Equal("HASH_MISMATCH"))
        })
    })

    Context("Public Key Retrieval", func() {
        It("should serve public key in PEM format", func() {
            resp, err := dsClient.GetPublicKeyWithResponse(ctx)
            Expect(err).ToNot(HaveOccurred())
            Expect(resp.StatusCode()).To(Equal(200))

            publicKeyPEM := string(resp.Body)
            Expect(publicKeyPEM).To(ContainSubstring("-----BEGIN RSA PUBLIC KEY-----"))
            Expect(publicKeyPEM).To(ContainSubstring("-----END RSA PUBLIC KEY-----"))
        })
    })
})
```

---

## 📊 **Effort Comparison**

### **Original Plan (with CLI)**:
| Task | Effort |
|------|--------|
| Export API + Signatures | 4 hours |
| **CLI Tool** | **2 hours** |
| Chain of Custody | 2 hours |
| **Total** | **8 hours** |

### **Revised Plan (REST API Only)**:
| Task | Effort |
|------|--------|
| Export API + Signatures | 4 hours |
| **REST Verification Endpoint** | **1 hour** ⚡ |
| Public Key Endpoint | 0.5 hour |
| Chain of Custody | 2 hours |
| **Total** | **7.5 hours** ✅ |

**Time Saved**: 0.5 hours (6% faster)
**Complexity Reduced**: ✅ No CLI infrastructure needed

---

## 🎯 **Auditor Usage (Without CLI)**

### **Option 1: curl (Command Line)**
```bash
# 1. Export audit logs
curl -X POST https://datastorage.svc/api/v1/audit/export \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"correlation_id": "rr-2025-001"}' \
  -o export.json

# 2. Verify export
curl -X POST https://datastorage.svc/api/v1/audit/verify-export \
  -H "Content-Type: application/json" \
  -d @export.json

# Output:
# {
#   "verification_result": "valid",
#   "details": {
#     "hash_verified": true,
#     "signature_verified": true
#   }
# }
```

### **Option 2: Postman / Insomnia**
1. Create POST request to `/api/v1/audit/export`
2. Save response to file
3. Create POST request to `/api/v1/audit/verify-export`
4. Upload file as request body
5. View verification result

### **Option 3: Web UI** (Future)
- Add "Export Audit Logs" button in UI
- Add "Verify Export" file upload
- Display verification result visually

---

## ✅ **RECOMMENDATION**

**Approved**: REST API-only approach for Day 9

**Benefits**:
1. ✅ **No CLI infrastructure needed** - saves 0.5 hours
2. ✅ **Consistent with existing patterns** - all services use REST
3. ✅ **Auditor-friendly** - standard HTTP tools (curl, Postman)
4. ✅ **Future-proof** - can add CLI wrapper later if needed
5. ✅ **Easier testing** - integration tests use HTTP already

**Next Steps**:
1. Update OpenAPI spec (`pkg/datastorage/openapi.yaml`) with new endpoints
2. Generate client code (`make generate-datastorage-client`)
3. Implement handlers in `pkg/datastorage/server/`
4. Write integration tests

---

## 🔗 **Updated Day 9 Endpoints**

| Endpoint | Method | Purpose | Status |
|----------|--------|---------|--------|
| `/api/v1/audit/export` | POST | Export audit logs with signature | ✅ Required |
| `/api/v1/audit/verify-export` | POST | Verify export signature | ✅ Required |
| `/api/v1/audit/public-key` | GET | Get RSA public key (PEM) | ✅ Required |
| `/api/v1/audit/exports` | GET | List all exports (metadata) | ⚠️ Nice-to-have |

---

## ✅ **Confidence Assessment**

**Confidence**: 100%
**Complexity**: Low (REST API patterns already established)
**Risk**: Minimal (no new infrastructure dependencies)
**Recommendation**: ✅ **PROCEED with REST API approach**

**Is this approach acceptable?** If yes, I'll update the revised Day 9 plan and proceed with implementation.

