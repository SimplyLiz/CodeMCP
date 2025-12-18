package federation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtoDetector(t *testing.T) {
	// Create a temp directory with some proto files
	tmpDir, err := os.MkdirTemp("", "proto-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create proto directory
	protoDir := filepath.Join(tmpDir, "proto", "api", "v1")
	if err := os.MkdirAll(protoDir, 0755); err != nil {
		t.Fatalf("failed to create proto dir: %v", err)
	}

	// Write a test proto file
	protoContent := `syntax = "proto3";

package foo.bar.v1;

import "google/protobuf/timestamp.proto";

service UserService {
    rpc GetUser(GetUserRequest) returns (GetUserResponse);
}

message GetUserRequest {
    string user_id = 1;
}

message GetUserResponse {
    string name = 1;
}
`
	protoPath := filepath.Join(protoDir, "user.proto")
	if err := os.WriteFile(protoPath, []byte(protoContent), 0644); err != nil {
		t.Fatalf("failed to write proto file: %v", err)
	}

	// Run the detector
	detector := NewProtoDetector()
	result, err := detector.Detect(tmpDir, "test-repo-uid", "test-repo")
	if err != nil {
		t.Fatalf("detection failed: %v", err)
	}

	// Verify results
	if len(result.Contracts) != 1 {
		t.Errorf("expected 1 contract, got %d", len(result.Contracts))
	}

	if len(result.Contracts) > 0 {
		contract := result.Contracts[0]

		if contract.ContractType != ContractTypeProto {
			t.Errorf("expected contract type proto, got %s", contract.ContractType)
		}

		if contract.Visibility != VisibilityPublic {
			t.Errorf("expected visibility public (proto/ root), got %s", contract.Visibility)
		}

		// Verify import keys generated
		if len(contract.ImportKeys) == 0 {
			t.Error("expected import keys to be generated")
		}
	}

	// Verify references (the import)
	if len(result.References) != 1 {
		t.Errorf("expected 1 reference (import), got %d", len(result.References))
	}

	if len(result.References) > 0 {
		ref := result.References[0]
		if ref.ImportKey != "google/protobuf/timestamp.proto" {
			t.Errorf("expected import key 'google/protobuf/timestamp.proto', got '%s'", ref.ImportKey)
		}
		if ref.Tier != TierDeclared {
			t.Errorf("expected tier declared, got %s", ref.Tier)
		}
	}
}

func TestProtoVisibilityClassification(t *testing.T) {
	detector := &ProtoDetector{}

	testCases := []struct {
		path               string
		metadata           ProtoMetadata
		expectedVisibility Visibility
	}{
		{
			path:               "proto/api/v1/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.bar.v1"},
			expectedVisibility: VisibilityPublic,
		},
		{
			path:               "internal/proto/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.bar"},
			expectedVisibility: VisibilityInternal,
		},
		{
			path:               "api/v1/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.bar.v1"},
			expectedVisibility: VisibilityPublic,
		},
		{
			path:               "testdata/test.proto",
			metadata:           ProtoMetadata{PackageName: "test"},
			expectedVisibility: VisibilityInternal,
		},
		{
			path:               "some/path/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.internal.v1"},
			expectedVisibility: VisibilityInternal,
		},
		{
			path:               "some/path/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.bar.v1", Services: []string{"UserService"}},
			expectedVisibility: VisibilityPublic,
		},
		{
			path:               "some/path/user.proto",
			metadata:           ProtoMetadata{PackageName: "foo.bar"},
			expectedVisibility: VisibilityUnknown,
		},
	}

	for _, tc := range testCases {
		visibility, _, _ := detector.classifyVisibility(tc.path, tc.metadata)
		if visibility != tc.expectedVisibility {
			t.Errorf("path=%s, pkg=%s: expected %s, got %s",
				tc.path, tc.metadata.PackageName, tc.expectedVisibility, visibility)
		}
	}
}

func TestOpenAPIDetector(t *testing.T) {
	// Create a temp directory with an OpenAPI file
	tmpDir, err := os.MkdirTemp("", "openapi-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create api directory
	apiDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatalf("failed to create api dir: %v", err)
	}

	// Write a test OpenAPI file
	openapiContent := `openapi: 3.0.3
info:
  title: User API
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /users:
    get:
      summary: List users
      responses:
        '200':
          description: OK
`
	openapiPath := filepath.Join(apiDir, "openapi.yaml")
	if err := os.WriteFile(openapiPath, []byte(openapiContent), 0644); err != nil {
		t.Fatalf("failed to write openapi file: %v", err)
	}

	// Run the detector
	detector := NewOpenAPIDetector()
	result, err := detector.Detect(tmpDir, "test-repo-uid", "test-repo")
	if err != nil {
		t.Fatalf("detection failed: %v", err)
	}

	// Verify results
	if len(result.Contracts) != 1 {
		t.Errorf("expected 1 contract, got %d", len(result.Contracts))
	}

	if len(result.Contracts) > 0 {
		contract := result.Contracts[0]

		if contract.ContractType != ContractTypeOpenAPI {
			t.Errorf("expected contract type openapi, got %s", contract.ContractType)
		}

		if contract.Visibility != VisibilityPublic {
			t.Errorf("expected visibility public (api/ path and public server), got %s", contract.Visibility)
		}
	}
}

func TestComputeEdgeKey(t *testing.T) {
	// Same inputs should produce same key
	key1 := ComputeEdgeKey("contract1", "repo-uid", "proto_import", []string{"file1.proto", "file2.proto"})
	key2 := ComputeEdgeKey("contract1", "repo-uid", "proto_import", []string{"file2.proto", "file1.proto"})

	if key1 != key2 {
		t.Errorf("expected same key regardless of path order, got %s and %s", key1, key2)
	}

	// Different inputs should produce different keys
	key3 := ComputeEdgeKey("contract2", "repo-uid", "proto_import", []string{"file1.proto"})
	if key1 == key3 {
		t.Error("expected different keys for different contracts")
	}
}
